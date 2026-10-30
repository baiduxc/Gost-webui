// Package server 提供面板的 HTTP API 与静态资源。
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gost-webui/internal/cert"
	"gost-webui/internal/config"
	"gost-webui/internal/controller"
	"gost-webui/internal/store"
)

// Server 面板 HTTP 服务。
type Server struct {
	cfg      *config.Config
	ctl      *controller.Controller
	log      *slog.Logger
	web      fs.FS
	mux      *http.ServeMux
	sess     *sessionStore
	basePath string
	started  time.Time

	restartMu sync.Mutex
	restartFn func()

	guard     *loginGuard
	dummyHash []byte
	hostMu    sync.Mutex
	hostIP    string
	hostTime  time.Time
	// host6IP/host6Time 缓存探测到的公网 IPv6，供「仅 IPv6」的 REALITY 节点使用。
	host6IP   string
	host6Time time.Time

	// 两步验证的临时状态（仅内存）：待确认绑定密钥与登录挑战。
	otpMu   sync.Mutex
	pending map[string]pendingSetup
	chals   map[string]*loginChallenge

	// 独立订阅监听器与证书管理器。
	subMu    sync.Mutex
	subSrv   *http.Server
	subCtx   context.Context
	certProv *cert.Provider
}

// New 创建服务。
func New(cfg *config.Config, ctl *controller.Controller, web fs.FS, log *slog.Logger) *Server {
	s := &Server{
		cfg:      cfg,
		ctl:      ctl,
		log:      log,
		web:      web,
		mux:      http.NewServeMux(),
		sess:     newSessionStore(ctl.Store),
		basePath: config.NormalizeBasePath(cfg.BasePath),
		started:  time.Now(),
		guard:    newLoginGuard(),
		pending:  map[string]pendingSetup{},
		chals:    map[string]*loginChallenge{},
	}
	// 用户名不匹配时也跑一次 bcrypt，让耗时与真实校验一致，避免用户名枚举
	if h, err := bcrypt.GenerateFromPassword([]byte("gost-webui-dummy"), bcrypt.DefaultCost); err == nil {
		s.dummyHash = h
	}
	s.routes()
	return s
}

// SetRestartFunc 注册重启面板的回调。
func (s *Server) SetRestartFunc(fn func()) {
	s.restartMu.Lock()
	s.restartFn = fn
	s.restartMu.Unlock()
}

// BasePath 返回当前访问路径前缀。
func (s *Server) BasePath() string { return s.basePath }

func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/login/2fa", s.handleLogin2FA)
	s.mux.HandleFunc("POST /api/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/session", s.auth(s.handleSession))
	s.mux.HandleFunc("POST /api/sessions/revoke-others", s.auth(s.handleRevokeOtherSessions))

	s.mux.HandleFunc("GET /api/2fa", s.auth(s.handle2FAStatus))
	s.mux.HandleFunc("POST /api/2fa/setup", s.auth(s.handle2FASetup))
	s.mux.HandleFunc("POST /api/2fa/enable", s.auth(s.handle2FAEnable))
	s.mux.HandleFunc("POST /api/2fa/disable", s.auth(s.handle2FADisable))
	s.mux.HandleFunc("POST /api/2fa/recovery", s.auth(s.handle2FARecovery))

	s.mux.HandleFunc("GET /api/overview", s.auth(s.handleOverview))
	s.mux.HandleFunc("GET /api/nodes", s.auth(s.handleListNodes))
	s.mux.HandleFunc("POST /api/nodes", s.auth(s.handleCreateNode))
	s.mux.HandleFunc("GET /api/nodes/{id}", s.auth(s.handleGetNode))
	s.mux.HandleFunc("PUT /api/nodes/{id}", s.auth(s.handleUpdateNode))
	s.mux.HandleFunc("DELETE /api/nodes/{id}", s.auth(s.handleDeleteNode))
	s.mux.HandleFunc("POST /api/nodes/{id}/toggle", s.auth(s.handleToggleNode))
	s.mux.HandleFunc("POST /api/nodes/{id}/reset", s.auth(s.handleResetNode))
	s.mux.HandleFunc("GET /api/nodes/{id}/stats", s.auth(s.handleNodeStats))
	s.mux.HandleFunc("GET /api/nodes/{id}/link", s.auth(s.handleNodeLink))
	s.mux.HandleFunc("GET /api/nodes/{id}/qrcode", s.auth(s.handleQRCode))
	s.mux.HandleFunc("GET /api/nodes/{id}/deploy", s.auth(s.handleNodeDeploy))

	s.mux.HandleFunc("POST /api/parse", s.auth(s.handleParseLink))
	s.mux.HandleFunc("POST /api/test-connect", s.auth(s.handleTestConnect))
	s.mux.HandleFunc("GET /api/settings", s.auth(s.handleGetSettings))
	s.mux.HandleFunc("PUT /api/settings", s.auth(s.handleUpdateSettings))
	s.mux.HandleFunc("POST /api/password", s.auth(s.handleChangePassword))
	s.mux.HandleFunc("GET /api/brand", s.handleBrand)
	s.mux.HandleFunc("GET /api/backup", s.auth(s.handleBackup))
	s.mux.HandleFunc("POST /api/backup/restore", s.auth(s.handleBackupRestore))

	s.mux.HandleFunc("GET /api/system", s.auth(s.handleSystemInfo))
	s.mux.HandleFunc("GET /api/reality/available", s.auth(s.handleRealityAvailable))
	s.mux.HandleFunc("POST /api/reality/credential", s.auth(s.handleRealityCredential))
	s.mux.HandleFunc("PUT /api/system", s.auth(s.handleSystemUpdate))
	s.mux.HandleFunc("POST /api/system/restart", s.auth(s.handleSystemRestart))

	s.mux.HandleFunc("GET /api/notify", s.auth(s.handleNotifyGet))
	s.mux.HandleFunc("PUT /api/notify", s.auth(s.handleNotifyPut))
	s.mux.HandleFunc("POST /api/notify/test", s.auth(s.handleNotifyTest))

	s.mux.HandleFunc("GET /api/gost", s.auth(s.handleGostStatus))
	s.mux.HandleFunc("POST /api/gost/restart", s.auth(s.handleGostRestart))
	s.mux.HandleFunc("GET /api/gost/logs", s.auth(s.handleGostLogs))
	s.mux.HandleFunc("GET /api/ports/free", s.auth(s.handleFreePort))

	// 按节点订阅管理（需登录）
	s.mux.HandleFunc("GET /api/nodes/{id}/subscription", s.auth(s.handleNodeSubscription))
	s.mux.HandleFunc("POST /api/nodes/{id}/subscription/reset", s.auth(s.handleNodeSubscriptionReset))
	s.mux.HandleFunc("GET /api/qrcode", s.auth(s.handleQRText))

	// 订阅与证书配置（需登录）
	s.mux.HandleFunc("GET /api/sub-config", s.auth(s.handleSubConfigGet))
	s.mux.HandleFunc("PUT /api/sub-config", s.auth(s.handleSubConfigPut))
	s.mux.HandleFunc("POST /api/cert/issue", s.auth(s.handleCertIssue))

	s.mux.HandleFunc("/", s.handleStatic)
}

// Handler 返回 http.Handler（支持访问路径前缀）。
func (s *Server) Handler() http.Handler {
	if s.basePath == "" {
		return s.securityMiddleware(s.mux)
	}
	outer := http.NewServeMux()
	outer.Handle(s.basePath+"/", http.StripPrefix(s.basePath, s.mux))
	// 访问前缀本身（不带斜杠）时补一个斜杠
	outer.HandleFunc(s.basePath, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, s.basePath+"/", http.StatusFound)
	})
	// 根路径给出提示
	outer.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, s.basePath+"/", http.StatusFound)
	})
	return s.securityMiddleware(outer)
}

// ---------- 通用工具 ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

// ---------- 会话认证 ----------

// session 是一个已登录会话，ua 为客户端标识摘要（会话绑定）。
type session struct {
	user    string
	ua      string
	expires time.Time
}

// sessionStore 会话存储：内存缓存 + bbolt 持久化（面板重启后登录态不丢）。
type sessionStore struct {
	mu sync.Mutex
	m  map[string]session
	db *store.Store
}

func newSessionStore(db *store.Store) *sessionStore {
	st := &sessionStore{m: map[string]session{}, db: db}
	if db != nil {
		_ = db.PruneSessions()
	}
	go func() {
		for range time.Tick(10 * time.Minute) {
			now := time.Now()
			st.mu.Lock()
			for k, v := range st.m {
				if now.After(v.expires) {
					delete(st.m, k)
					if st.db != nil {
						_ = st.db.DeleteSession(k)
					}
				}
			}
			st.mu.Unlock()
			if st.db != nil {
				_, _ = st.db.PruneDevices()
			}
		}
	}()
	return st
}

const (
	cookieName = "gost_webui_session"
	// sessionTTL 是登录态的绝对有效期。
	sessionTTL = 7 * 24 * time.Hour
)

// bound 判断会话与当前客户端是否匹配。
// 会话或客户端缺少标识时不做限制，避免代理剥离 UA 后把正常用户踢下线。
func (st session) bound(ua string) bool {
	return st.ua == "" || ua == "" || st.ua == ua
}

func (st *sessionStore) create(user, ua string) (string, time.Time) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	exp := time.Now().Add(sessionTTL)
	st.mu.Lock()
	st.m[token] = session{user: user, ua: ua, expires: exp}
	st.mu.Unlock()
	if st.db != nil {
		_ = st.db.SaveSession(token, store.Session{User: user, ExpiresAt: exp.Unix(), UA: ua})
	}
	return token, exp
}

func (st *sessionStore) get(token, ua string) (session, bool) {
	st.mu.Lock()
	if v, ok := st.m[token]; ok {
		if time.Now().After(v.expires) || !v.bound(ua) {
			delete(st.m, token)
		} else {
			st.mu.Unlock()
			return v, true
		}
	}
	st.mu.Unlock()

	if st.db != nil {
		if ses, ok := st.db.GetSession(token); ok && time.Now().Unix() < ses.ExpiresAt {
			v := session{user: ses.User, ua: ses.UA, expires: time.Unix(ses.ExpiresAt, 0)}
			if !v.bound(ua) {
				_ = st.db.DeleteSession(token)
				return session{}, false
			}
			st.mu.Lock()
			st.m[token] = v
			st.mu.Unlock()
			return v, true
		}
	}
	return session{}, false
}

func (st *sessionStore) remove(token string) {
	st.mu.Lock()
	delete(st.m, token)
	st.mu.Unlock()
	if st.db != nil {
		_ = st.db.DeleteSession(token)
	}
}

// removeOthers 清理内存中除 keep 之外的会话，返回清理数量。
func (st *sessionStore) removeOthers(keep string) int {
	st.mu.Lock()
	defer st.mu.Unlock()
	n := 0
	for k := range st.m {
		if k != keep {
			delete(st.m, k)
			n++
		}
	}
	return n
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil || c.Value == "" {
			writeErr(w, http.StatusUnauthorized, "未登录")
			return
		}
		ses, ok := s.sess.get(c.Value, uaHash(r))
		if !ok {
			writeErr(w, http.StatusUnauthorized, "登录已过期")
			return
		}
		if ses.user != s.adminUser() {
			writeErr(w, http.StatusUnauthorized, "账号已变更，请重新登录")
			return
		}
		next(w, r)
	}
}

// ---------- 管理员账号 ----------

func (s *Server) adminUser() string {
	if v, ok := s.ctl.Store.GetSetting("admin_user"); ok && v != "" {
		return v
	}
	return s.cfg.Admin.Username
}

// AdminUser 返回当前管理员用户名。
func (s *Server) AdminUser() string { return s.adminUser() }

func (s *Server) adminHash() (string, bool) {
	return s.ctl.Store.GetSetting("admin_hash")
}

func (s *Server) checkPassword(pass string) bool {
	if h, ok := s.adminHash(); ok {
		return bcrypt.CompareHashAndPassword([]byte(h), []byte(pass)) == nil
	}
	// 首次运行：使用配置文件中的初始密码
	want := s.cfg.Admin.Password
	if want == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(pass), []byte(want)) == 1 {
		// 落库为哈希
		if h, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost); err == nil {
			_ = s.ctl.Store.SetSetting("admin_hash", string(h))
			_ = s.ctl.Store.SetSetting("admin_user", s.cfg.Admin.Username)
		}
		return true
	}
	return false
}

// ---------- 登录 ----------

// startSession 建立登录态并下发会话 Cookie。
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, user string) {
	token, exp := s.sess.create(user, uaHash(r))
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     s.cookiePath(),
		Expires:  exp,
		MaxAge:   int(sessionTTL / time.Second),
		HttpOnly: true,
		Secure:   requestIsTLS(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(cookieName); err == nil {
		return c.Value
	}
	return ""
}

// revokeOtherSessions 保留当前会话，其余登录态全部下线。
func (s *Server) revokeOtherSessions(r *http.Request) {
	token := sessionToken(r)
	s.sess.removeOthers(token)
	if s.ctl.Store != nil {
		_ = s.ctl.Store.DeleteOtherSessions(token)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) > 64 || len(req.Password) > 256 {
		writeErr(w, http.StatusBadRequest, "用户名或密码长度不合法")
		return
	}
	ip := clientIP(r)
	user := s.adminUser()
	// 只有真实账号才参与账号维度限流，避免用随机用户名把限流表撑爆
	isAdmin := subtle.ConstantTimeCompare([]byte(req.Username), []byte(user)) == 1
	acct := ""
	if isAdmin {
		acct = user
	}
	if d := s.guard.blocked(ip, acct); d > 0 {
		s.log.Warn("登录被限流", "user", req.Username, "ip", ip, "retry_after", d.Round(time.Second).String())
		writeErr(w, http.StatusTooManyRequests, "尝试过于频繁，请 "+humanDuration(d)+" 后再试")
		return
	}

	passOK := false
	if isAdmin {
		passOK = s.checkPassword(req.Password)
	} else if len(s.dummyHash) > 0 {
		// 恒定耗时：用户名错误时也跑一次 bcrypt，避免账号枚举
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(req.Password))
	}
	if !passOK {
		s.guard.fail(ip, acct)
		s.log.Warn("登录失败", "user", req.Username, "ip", ip)
		writeErr(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	s.guard.success(ip, acct)

	// 已开启两步验证：可信设备直接登录，否则返回一次性挑战
	if s.TwoFactorEnabled() {
		if devUser, ok := s.knownDevice(r); ok && devUser == user {
			s.startSession(w, r, user)
			s.log.Info("登录成功（可信设备）", "user", user, "ip", ip)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": user, "device": true})
			return
		}
		challenge, err := s.createChallenge(user, ip, uaHash(r))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "生成验证挑战失败")
			return
		}
		s.log.Info("密码校验通过，等待两步验证", "ip", ip)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"need2fa":   true,
			"challenge": challenge,
			"expiresIn": int(challengeTTL / time.Second),
		})
		return
	}

	s.startSession(w, r, user)
	s.log.Info("登录成功", "user", user, "ip", ip)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": user})
}

// humanDuration 把等待时间格式化为中文提示。
func humanDuration(d time.Duration) string {
	if d >= time.Minute {
		return strconv.Itoa(int(d.Round(time.Minute)/time.Minute)) + " 分钟"
	}
	s := int(d.Round(time.Second) / time.Second)
	if s < 1 {
		s = 1
	}
	return strconv.Itoa(s) + " 秒"
}

func (s *Server) cookiePath() string {
	if s.basePath == "" {
		return "/"
	}
	return s.basePath + "/"
}

// requestIsTLS 判断当前请求是否走 HTTPS（直连 TLS 或反代声明 https）。
func requestIsTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if v := r.Header.Get("X-Forwarded-Proto"); v != "" {
		return strings.EqualFold(strings.TrimSpace(strings.Split(v, ",")[0]), "https")
	}
	return false
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.sess.remove(c.Value)
	}
	// 主动退出同时忘掉本机的可信设备，共享电脑上不会残留免验证令牌
	if c, err := r.Cookie(deviceCookieName); err == nil && c.Value != "" {
		_ = s.ctl.Store.DeleteDevice(deviceKey(c.Value))
	}
	s.clearDeviceCookie(w)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: s.cookiePath(), MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"loggedIn":  true,
		"username":  s.adminUser(),
		"twoFactor": s.TwoFactorEnabled(),
	})
}

// clientIP 返回真实客户端 IP。
// 仅在直连对端是回环/内网地址（即前置反代或 Docker 网络）时才信任 X-Forwarded-For，
// 避免公网客户端伪造该头绕过登录限流。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil || !(ip.IsLoopback() || ip.IsPrivate()) {
		return host
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		first := strings.TrimSpace(strings.Split(v, ",")[0])
		if net.ParseIP(first) != nil {
			return first
		}
	}
	if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(v) != nil {
		return v
	}
	return host
}

// ---------- 静态资源 ----------

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "接口不存在")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	f, err := s.web.Open(path)
	if err != nil {
		// 前端使用 hash 路由，未知路径一律回退到 index.html
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		s.serveIndex(w, r2)
		return
	}
	_ = f.Close()
	if path == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.FileServer(http.FS(s.web)).ServeHTTP(w, r)
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, s.web, "index.html")
}

// randomPort 在 [min,max] 中随机取一个端口。
func randomPort(min, max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min
	}
	return min + int(n.Int64())
}

// randomPassword 生成 n 位 URL 安全的随机密码（不含 +/= 等需转义字符）。
func randomPassword(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if n <= 0 {
		n = 20
	}
	b := make([]byte, n)
	max := big.NewInt(int64(len(alphabet)))
	for i := range b {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			idx = big.NewInt(int64(i % len(alphabet)))
		}
		b[i] = alphabet[idx.Int64()]
	}
	return string(b)
}
