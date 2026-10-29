package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------- 安全响应头 / CSRF ----------

// securityHeaders 给所有响应加上基础安全头。
// CSP 允许内联样式（页面里有少量 style 属性）与 data: 图片（二维码），
// 但不允许内联脚本，脚本只能来自同源 app.js。
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"font-src 'self'; " +
	"object-src 'none'; " +
	"base-uri 'none'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'"

func applySecurityHeaders(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=()")
	if requestIsTLS(r) {
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
}

// sameOriginRequest 校验写操作来自本站页面（CSRF 纵深防御）。
// 浏览器跨站请求一定带 Origin 或 Sec-Fetch-Site；命令行/脚本不带，放行。
// 登录态 Cookie 本身已是 SameSite=Strict，这里只是再加一道闸。
// 反代改写 Host 时以 X-Forwarded-Host 为准，避免误伤正常请求。
func sameOriginRequest(r *http.Request) bool {
	if v := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); v == "cross-site" {
		return false
	}
	host := r.Host
	if v := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); v != "" {
		host = strings.TrimSpace(strings.Split(v, ",")[0])
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		// 同源请求时部分浏览器会发送 Origin: null（如 file://）
		if origin == "null" {
			return false
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return strings.EqualFold(u.Host, host)
	}
	if ref := strings.TrimSpace(r.Header.Get("Referer")); ref != "" {
		u, err := url.Parse(ref)
		if err != nil {
			return false
		}
		return strings.EqualFold(u.Host, host)
	}
	return true
}

// securityMiddleware 统一注入安全头，并对写操作做同源校验。
func (s *Server) securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		applySecurityHeaders(w, r)
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !sameOriginRequest(r) {
				writeErr(w, http.StatusForbidden, "请求来源校验失败，请刷新页面后重试")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// uaHash 返回客户端标识摘要，用于会话/设备令牌与浏览器绑定。
// 空 User-Agent 返回空串，此时不做绑定（避免部分反向代理剥离头导致误杀）。
func uaHash(r *http.Request) string {
	ua := strings.TrimSpace(r.UserAgent())
	if ua == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(ua))
	return hex.EncodeToString(sum[:8])
}

// ---------- 登录限流 ----------

// guardEntry 记录某个 IP 或账号的失败情况。
type guardEntry struct {
	fails int
	until time.Time
	seen  time.Time
}

// loginGuard 按 IP 与账号双维度限流，失败次数越多冷却越久。
// 纯内存实现：面板重启后自动清零，对暴力破解仍然有效。
type loginGuard struct {
	mu     sync.Mutex
	byIP   map[string]*guardEntry
	byUser map[string]*guardEntry
}

const (
	guardFreeAttempts = 5
	guardMaxEntries   = 8192
	guardRetention    = time.Hour
)

func newLoginGuard() *loginGuard {
	g := &loginGuard{byIP: map[string]*guardEntry{}, byUser: map[string]*guardEntry{}}
	go func() {
		for range time.Tick(10 * time.Minute) {
			g.cleanup()
		}
	}()
	return g
}

// blocked 返回还需等待的时间（0 表示放行）。
func (g *loginGuard) blocked(ip, user string) time.Duration {
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	var wait time.Duration
	for _, e := range []*guardEntry{g.byIP[ip], g.byUser[user]} {
		if e == nil || e.until.IsZero() {
			continue
		}
		if d := e.until.Sub(now); d > wait {
			wait = d
		}
	}
	if wait < 0 {
		return 0
	}
	return wait
}

// fail 记录一次失败，达到阈值后设置递增冷却时间。
func (g *loginGuard) fail(ip, user string) {
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	bump(g.byIP, ip, now)
	if user != "" {
		bump(g.byUser, user, now)
	}
	if len(g.byIP) > guardMaxEntries || len(g.byUser) > guardMaxEntries {
		g.cleanupLocked(now)
	}
}

// success 清空该 IP 与账号的失败记录。
func (g *loginGuard) success(ip, user string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.byIP, ip)
	delete(g.byUser, user)
}

func bump(m map[string]*guardEntry, key string, now time.Time) {
	if key == "" {
		return
	}
	e := m[key]
	if e == nil {
		e = &guardEntry{}
		m[key] = e
	}
	// 冷却期已过则从头计数
	if !e.until.IsZero() && now.After(e.until) {
		e.fails = 0
		e.until = time.Time{}
	}
	e.fails++
	e.seen = now
	if d := cooldown(e.fails); d > 0 {
		e.until = now.Add(d)
	}
}

// cooldown 根据累计失败次数返回冷却时长。
func cooldown(fails int) time.Duration {
	switch {
	case fails >= 20:
		return 30 * time.Minute
	case fails >= 10:
		return 5 * time.Minute
	case fails >= guardFreeAttempts:
		return time.Minute
	default:
		return 0
	}
}

func (g *loginGuard) cleanup() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cleanupLocked(time.Now())
}

func (g *loginGuard) cleanupLocked(now time.Time) {
	for _, m := range []map[string]*guardEntry{g.byIP, g.byUser} {
		for k, e := range m {
			if now.After(e.until) && now.Sub(e.seen) > guardRetention {
				delete(m, k)
			}
		}
		// 仍超限时按最久未活跃顺序淘汰，防止伪造请求把内存撑大
		if len(m) <= guardMaxEntries {
			continue
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			return m[keys[i]].seen.Before(m[keys[j]].seen)
		})
		for i := 0; i < len(keys)-guardMaxEntries; i++ {
			delete(m, keys[i])
		}
	}
}
