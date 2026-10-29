package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gost-webui/internal/auth"
	"gost-webui/internal/config"
	"gost-webui/internal/controller"
	"gost-webui/internal/store"
)

// newTestServer 构造一个只依赖临时数据库的 Server，用于校验安全相关逻辑。
func newTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cfg := &config.Config{}
	cfg.Admin.Username = "admin"

	return &Server{
		cfg:     cfg,
		ctl:     &controller.Controller{Store: st},
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		guard:   newLoginGuard(),
		pending: map[string]pendingSetup{},
		chals:   map[string]*loginChallenge{},
	}
}

// ---------- 登录限流 ----------

func TestLoginGuardCooldownLadder(t *testing.T) {
	g := newLoginGuard()
	for i := 0; i < guardFreeAttempts-1; i++ {
		g.fail("1.1.1.1", "admin")
	}
	if d := g.blocked("1.1.1.1", "admin"); d > 0 {
		t.Fatalf("未达阈值不应锁定，实际 %v", d)
	}
	g.fail("1.1.1.1", "admin")
	if d := g.blocked("1.1.1.1", "admin"); d <= 0 || d > time.Minute {
		t.Fatalf("第 %d 次失败应锁定 1 分钟，实际 %v", guardFreeAttempts, d)
	}
	for i := 0; i < 15; i++ {
		g.fail("1.1.1.1", "admin")
	}
	if d := g.blocked("1.1.1.1", "admin"); d <= 5*time.Minute {
		t.Fatalf("累计 20 次失败应锁定 30 分钟，实际 %v", d)
	}
	// 其它 IP 不受影响
	if d := g.blocked("2.2.2.2", ""); d != 0 {
		t.Fatalf("其它 IP 不应被牵连，实际 %v", d)
	}
	// 登录成功应清零
	g.success("1.1.1.1", "admin")
	if d := g.blocked("1.1.1.1", "admin"); d != 0 {
		t.Fatalf("成功后应解除锁定，实际 %v", d)
	}
}

func TestLoginGuardEviction(t *testing.T) {
	g := newLoginGuard()
	now := time.Now()
	for i := 0; i < guardMaxEntries+200; i++ {
		key := "ip-" + strings.Repeat("x", i%3) + string(rune('a'+i%26)) + time.Duration(i).String()
		g.byIP[key] = &guardEntry{fails: 1, seen: now.Add(time.Duration(i) * time.Millisecond)}
	}
	g.cleanup()
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.byIP) > guardMaxEntries {
		t.Fatalf("超限条目应被淘汰，实际 %d", len(g.byIP))
	}
}

// ---------- 同源校验与安全响应头 ----------

func TestSameOriginRequest(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		host    string
		want    bool
	}{
		{"无来源头-放行", nil, "panel.example.com", true},
		{"跨站请求-拒绝", map[string]string{"Sec-Fetch-Site": "cross-site"}, "panel.example.com", false},
		{"同源-允许", map[string]string{"Origin": "https://panel.example.com"}, "panel.example.com", true},
		{"不同源-拒绝", map[string]string{"Origin": "https://evil.example.com"}, "panel.example.com", false},
		{"Origin为null-拒绝", map[string]string{"Origin": "null"}, "panel.example.com", false},
		{"Referer同源-允许", map[string]string{"Referer": "https://panel.example.com/system"}, "panel.example.com", true},
		{"Referer不同源-拒绝", map[string]string{"Referer": "https://evil.example.com/x"}, "panel.example.com", false},
		{"反代改写Host-按XFH判定", map[string]string{
			"Origin":           "https://panel.example.com",
			"X-Forwarded-Host": "panel.example.com",
		}, "127.0.0.1:8787", true},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodPost, "http://"+c.host+"/api/login", nil)
		for k, v := range c.headers {
			r.Header.Set(k, v)
		}
		if got := sameOriginRequest(r); got != c.want {
			t.Errorf("%s: 期望 %v，实际 %v", c.name, c.want, got)
		}
	}
}

func TestSecurityMiddleware(t *testing.T) {
	s := newTestServer(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := s.securityMiddleware(next)

	// 跨站写请求：拒绝并带上安全头
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "http://panel.example.com/api/login", nil)
	r.Header.Set("Origin", "https://evil.example.com")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("跨站写请求应返回 403，实际 %d", w.Code)
	}
	if got := w.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("缺少 CSP 响应头")
	}
	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options 期望 DENY，实际 %q", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options 期望 nosniff，实际 %q", got)
	}

	// 同源写请求：放行
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "http://panel.example.com/api/login", nil)
	r.Header.Set("Origin", "http://panel.example.com")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("同源写请求应放行，实际 %d", w.Code)
	}
}

func TestUAHash(t *testing.T) {
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.Header.Set("User-Agent", "Mozilla/5.0 test")
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.Header.Set("User-Agent", "Mozilla/5.0 test")
	r3 := httptest.NewRequest(http.MethodGet, "/", nil)
	r3.Header.Set("User-Agent", "Mozilla/5.0 other")

	if uaHash(r1) == "" || uaHash(r1) != uaHash(r2) {
		t.Fatal("相同 UA 应得到相同摘要")
	}
	if uaHash(r1) == uaHash(r3) {
		t.Fatal("不同 UA 的摘要不应相同")
	}
	if uaHash(httptest.NewRequest(http.MethodGet, "/", nil)) != "" {
		t.Fatal("空 UA 应返回空摘要以跳过敏绑定")
	}
}

// ---------- 两步验证 ----------

func TestVerifyCodeRejectsReplay(t *testing.T) {
	s := newTestServer(t)
	secret, err := auth.GenerateSecret()
	if err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}
	code, _, err := auth.Code(secret, time.Now())
	if err != nil {
		t.Fatalf("生成动态码失败: %v", err)
	}
	if err := s.verifyCode(secret, code, true); err != nil {
		t.Fatalf("首次校验应通过: %v", err)
	}
	if err := s.verifyCode(secret, code, true); err != errTOTPReused {
		t.Fatalf("重复使用同一动态码应被拒绝，实际 %v", err)
	}
	if err := s.verifyCode(secret, "000000", true); err != errTOTPCode {
		t.Fatalf("错误动态码应被拒绝，实际 %v", err)
	}
	// 未记录模式下允许同一码重复校验（例如绑定确认）
	if err := s.verifyCode(secret, code, false); err != nil {
		t.Fatalf("非记录模式应通过: %v", err)
	}
}

func TestRecoveryCodeSingleUse(t *testing.T) {
	s := newTestServer(t)
	codes, err := auth.NewRecoveryCodes(3)
	if err != nil {
		t.Fatalf("生成恢复码失败: %v", err)
	}
	hashes := make([]string, 0, len(codes))
	for _, c := range codes {
		hashes = append(hashes, auth.HashRecoveryCode(c))
	}
	if err := s.setRecoveryHashes(hashes); err != nil {
		t.Fatalf("保存恢复码失败: %v", err)
	}

	secret, _ := auth.GenerateSecret()
	_ = s.ctl.Store.SetSetting(settingTOTPSecret, secret)
	_ = s.ctl.Store.SetSetting(settingTOTPEnabled, "1")
	if !s.TwoFactorEnabled() {
		t.Fatal("两步验证应处于启用状态")
	}

	// 恢复码带分组符也能识别，且只能使用一次
	formatted := auth.FormatRecoveryCode(codes[0])
	if err := s.checkSecondFactor(formatted); err != nil {
		t.Fatalf("恢复码校验应通过: %v", err)
	}
	if err := s.checkSecondFactor(formatted); err == nil {
		t.Fatal("恢复码不应可以重复使用")
	}
	if left := len(s.recoveryHashes()); left != len(codes)-1 {
		t.Fatalf("使用一枚后剩余应为 %d，实际 %d", len(codes)-1, left)
	}
	// 剩余恢复码仍然可用
	if err := s.checkSecondFactor(codes[1]); err != nil {
		t.Fatalf("其余恢复码应可用: %v", err)
	}
}

func TestChallengeBinding(t *testing.T) {
	s := newTestServer(t)
	token, err := s.createChallenge("admin", "1.1.1.1", "ua-a")
	if err != nil {
		t.Fatalf("创建挑战失败: %v", err)
	}
	if _, ok := s.lookupChallenge(token, "1.1.1.1", "ua-a"); !ok {
		t.Fatal("绑定信息一致时应通过")
	}
	if _, ok := s.lookupChallenge(token, "2.2.2.2", "ua-a"); ok {
		t.Fatal("IP 不一致时应拒绝")
	}
	// IP 不一致会被直接作废，需要重新申请
	if _, ok := s.lookupChallenge(token, "1.1.1.1", "ua-a"); ok {
		t.Fatal("被作废的挑战不应再可用")
	}

	token, _ = s.createChallenge("admin", "1.1.1.1", "ua-a")
	for i := 0; i < challengeMaxTries; i++ {
		if alive := s.failChallenge(token); !alive && i < challengeMaxTries-1 {
			t.Fatalf("第 %d 次失败后挑战应仍然有效", i+1)
		}
	}
	if _, ok := s.lookupChallenge(token, "1.1.1.1", "ua-a"); ok {
		t.Fatal("失败次数用尽后挑战应失效")
	}
}

func TestDeviceToken(t *testing.T) {
	s := newTestServer(t)

	r := httptest.NewRequest(http.MethodPost, "http://panel.example.com/api/login/2fa", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 device")
	w := httptest.NewRecorder()
	s.issueDeviceToken(w, r, "admin")

	var issued *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == deviceCookieName {
			issued = c
		}
	}
	if issued == nil {
		t.Fatal("应下发可信设备 Cookie")
	}
	if !issued.HttpOnly || issued.SameSite != http.SameSiteStrictMode {
		t.Fatal("设备 Cookie 应带 HttpOnly 与 SameSite=Strict")
	}

	same := httptest.NewRequest(http.MethodPost, "http://panel.example.com/api/login", nil)
	same.Header.Set("User-Agent", "Mozilla/5.0 device")
	same.AddCookie(issued)
	if user, ok := s.knownDevice(same); !ok || user != "admin" {
		t.Fatalf("同 UA 应识别为可信设备，实际 %q %v", user, ok)
	}

	other := httptest.NewRequest(http.MethodPost, "http://panel.example.com/api/login", nil)
	other.Header.Set("User-Agent", "Mozilla/5.0 another")
	other.AddCookie(issued)
	if _, ok := s.knownDevice(other); ok {
		t.Fatal("UA 不一致时应拒绝可信设备令牌")
	}
	if _, ok := s.knownDevice(same); ok {
		t.Fatal("UA 不一致的设备记录应被删除")
	}

	// 清除两步验证会同时注销全部可信设备
	if err := s.clearTOTP(); err != nil {
		t.Fatalf("清除两步验证失败: %v", err)
	}
	if s.TwoFactorEnabled() {
		t.Fatal("清除后两步验证应为关闭状态")
	}
}
