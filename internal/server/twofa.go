package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"gost-webui/internal/auth"
	"gost-webui/internal/store"
)

// 两步验证相关设置键（存放于 meta 桶）。
const (
	settingTOTPSecret    = "totp_secret"
	settingTOTPEnabled   = "totp_enabled"
	settingTOTPEnabledAt = "totp_enabled_at"
	settingTOTPLastStep  = "totp_last_step"
	settingTOTPRecovery  = "totp_recovery_codes"
)

const (
	deviceCookieName  = "gost_webui_device"
	challengeTTL      = 5 * time.Minute
	pendingSetupTTL   = 10 * time.Minute
	deviceTTL         = 30 * 24 * time.Hour
	recoveryCodeCount = 10
	challengeMaxTries = 5
)

// 两步验证相关错误。
var (
	errTOTPNotSetup = errors.New("尚未绑定两步验证")
	errTOTPCode     = errors.New("动态验证码不正确")
	errTOTPReused   = errors.New("该验证码已使用，请等待验证器刷新后重试")
)

// pendingSetup 是尚未确认的绑定流程（只在内存中保存，面板重启即失效）。
type pendingSetup struct {
	secret  string
	expires time.Time
}

// loginChallenge 是密码校验通过后、等待动态码校验的临时凭据。
// 与发起登录的 IP、UA 绑定，单次有效，避免挑战被搬运到其它客户端。
type loginChallenge struct {
	user     string
	ip       string
	ua       string
	expires  time.Time
	attempts int
}

// ---------- 状态读取 ----------

func (s *Server) totpSecret() string {
	v, _ := s.ctl.Store.GetSetting(settingTOTPSecret)
	return auth.NormalizeSecret(v)
}

// TwoFactorEnabled 返回是否已启用两步验证。
func (s *Server) TwoFactorEnabled() bool {
	v, ok := s.ctl.Store.GetSetting(settingTOTPEnabled)
	return ok && v == "1" && s.totpSecret() != ""
}

func (s *Server) recoveryHashes() []string {
	v, ok := s.ctl.Store.GetSetting(settingTOTPRecovery)
	if !ok || v == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return nil
	}
	return out
}

func (s *Server) setRecoveryHashes(hashes []string) error {
	if hashes == nil {
		hashes = []string{}
	}
	b, err := json.Marshal(hashes)
	if err != nil {
		return err
	}
	return s.ctl.Store.SetSetting(settingTOTPRecovery, string(b))
}

// clearTOTP 清除全部两步验证状态（密钥、恢复码、可信设备）。
func (s *Server) clearTOTP() error {
	for _, k := range []string{settingTOTPSecret, settingTOTPEnabled, settingTOTPEnabledAt,
		settingTOTPLastStep, settingTOTPRecovery} {
		if err := s.ctl.Store.DelSetting(k); err != nil {
			return err
		}
	}
	s.otpMu.Lock()
	delete(s.pending, s.adminUser())
	s.otpMu.Unlock()
	return s.ctl.Store.DeleteAllDevices()
}

// ---------- 动态码校验 ----------

// verifyCode 校验动态码。record 为 true 时记录时间步防止重放（登录场景）。
func (s *Server) verifyCode(secret, code string, record bool) error {
	if secret == "" {
		return errTOTPNotSetup
	}
	now := time.Now()
	step, ok := auth.Verify(secret, code, now)
	if !ok {
		return errTOTPCode
	}
	if !record {
		return nil
	}
	last := int64(0)
	if v, ok := s.ctl.Store.GetSetting(settingTOTPLastStep); ok {
		last, _ = strconv.ParseInt(v, 10, 64)
	}
	// 时钟回拨时历史记录不可信，直接忽略
	if cur := auth.Step(now); last > cur+2 {
		last = 0
	}
	if step <= last {
		return errTOTPReused
	}
	return s.ctl.Store.SetSetting(settingTOTPLastStep, strconv.FormatInt(step, 10))
}

// consumeRecoveryCode 校验并作废一个恢复码。
func (s *Server) consumeRecoveryCode(code string) bool {
	hashes := s.recoveryHashes()
	idx := auth.MatchRecoveryCode(code, hashes)
	if idx < 0 {
		return false
	}
	rest := make([]string, 0, len(hashes)-1)
	rest = append(rest, hashes[:idx]...)
	rest = append(rest, hashes[idx+1:]...)
	if err := s.setRecoveryHashes(rest); err != nil {
		return false
	}
	return true
}

// checkSecondFactor 校验登录时的动态码或恢复码。
func (s *Server) checkSecondFactor(code string) error {
	if strings.TrimSpace(code) == "" {
		return errTOTPCode
	}
	if digitsOnly(code) {
		return s.verifyCode(s.totpSecret(), code, true)
	}
	// 恢复码：一次有效，用后即焚
	if len(auth.NormalizeRecoveryCode(code)) == auth.RecoveryCodeLen && s.consumeRecoveryCode(code) {
		return nil
	}
	return errTOTPCode
}

func digitsOnly(s string) bool {
	if len(s) != auth.Digits {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ---------- 登录挑战 ----------

func (s *Server) createChallenge(user, ip, ua string) (string, error) {
	token, err := auth.NewToken(32)
	if err != nil {
		return "", err
	}
	now := time.Now()
	s.otpMu.Lock()
	defer s.otpMu.Unlock()
	for k, v := range s.chals {
		if now.After(v.expires) {
			delete(s.chals, k)
		}
	}
	s.chals[token] = &loginChallenge{user: user, ip: ip, ua: ua, expires: now.Add(challengeTTL)}
	return token, nil
}

// lookupChallenge 返回挑战的副本；令牌无效、绑定信息不符或已过期时返回 false。
func (s *Server) lookupChallenge(token, ip, ua string) (loginChallenge, bool) {
	if token == "" {
		return loginChallenge{}, false
	}
	s.otpMu.Lock()
	defer s.otpMu.Unlock()
	c, ok := s.chals[token]
	if !ok {
		return loginChallenge{}, false
	}
	if time.Now().After(c.expires) || c.ip != ip || c.ua != ua || c.attempts >= challengeMaxTries {
		delete(s.chals, token)
		return loginChallenge{}, false
	}
	return *c, true
}

// failChallenge 累计失败次数；次数用尽后删除挑战并返回 false。
func (s *Server) failChallenge(token string) bool {
	s.otpMu.Lock()
	defer s.otpMu.Unlock()
	c, ok := s.chals[token]
	if !ok {
		return false
	}
	c.attempts++
	if c.attempts >= challengeMaxTries {
		delete(s.chals, token)
		return false
	}
	return true
}

func (s *Server) dropChallenge(token string) {
	s.otpMu.Lock()
	delete(s.chals, token)
	s.otpMu.Unlock()
}

// ---------- 可信设备令牌 ----------

func deviceKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// issueDeviceToken 签发“记住此设备”令牌，30 天内该浏览器可免二次验证。
func (s *Server) issueDeviceToken(w http.ResponseWriter, r *http.Request, user string) {
	token, err := auth.NewToken(32)
	if err != nil {
		return
	}
	now := time.Now()
	dev := store.Device{
		User:      user,
		UA:        uaHash(r),
		CreatedAt: now.Unix(),
		ExpiresAt: now.Add(deviceTTL).Unix(),
	}
	if err := s.ctl.Store.SaveDevice(deviceKey(token), dev); err != nil {
		s.log.Warn("保存可信设备失败", "err", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     deviceCookieName,
		Value:    token,
		Path:     s.cookiePath(),
		Expires:  time.Unix(dev.ExpiresAt, 0),
		MaxAge:   int(deviceTTL / time.Second),
		HttpOnly: true,
		Secure:   requestIsTLS(r),
		SameSite: http.SameSiteStrictMode,
	})
}

// knownDevice 校验可信设备 Cookie，通过时返回对应账号。
func (s *Server) knownDevice(r *http.Request) (string, bool) {
	c, err := r.Cookie(deviceCookieName)
	if err != nil || c.Value == "" {
		return "", false
	}
	key := deviceKey(c.Value)
	dev, ok := s.ctl.Store.GetDevice(key)
	if !ok {
		return "", false
	}
	ua := uaHash(r)
	if time.Now().Unix() >= dev.ExpiresAt || dev.User != s.adminUser() ||
		(dev.UA != "" && ua != "" && dev.UA != ua) {
		_ = s.ctl.Store.DeleteDevice(key)
		return "", false
	}
	return dev.User, true
}

func (s *Server) clearDeviceCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: deviceCookieName, Value: "", Path: s.cookiePath(), MaxAge: -1})
}

// ---------- 账号安全接口 ----------

func (s *Server) totpIssuer() string {
	if t, ok := s.ctl.Store.GetSetting("site_title"); ok && strings.TrimSpace(t) != "" {
		return strings.TrimSpace(t)
	}
	return "Gost-WebUI"
}

// handle2FAStatus 返回两步验证状态（需登录）。
func (s *Server) handle2FAStatus(w http.ResponseWriter, r *http.Request) {
	enabledAt := int64(0)
	if v, ok := s.ctl.Store.GetSetting(settingTOTPEnabledAt); ok {
		enabledAt, _ = strconv.ParseInt(v, 10, 64)
	}
	s.otpMu.Lock()
	p, pending := s.pending[s.adminUser()]
	pending = pending && time.Now().Before(p.expires)
	s.otpMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       s.TwoFactorEnabled(),
		"enabledAt":     enabledAt,
		"recoveryLeft":  len(s.recoveryHashes()),
		"pending":       pending,
		"deviceDays":    int(deviceTTL / (24 * time.Hour)),
		"issuer":        s.totpIssuer(),
		"account":       s.adminUser(),
		"supportedApps": "Google Authenticator / Microsoft Authenticator / 1Password / Bitwarden 等",
	})
}

// handle2FASetup 生成新的绑定密钥与二维码（需密码确认）。
func (s *Server) handle2FASetup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	user := s.adminUser()
	ip := clientIP(r)
	if d := s.guard.blocked(ip, user); d > 0 {
		writeErr(w, http.StatusTooManyRequests, "尝试过于频繁，请稍后再试")
		return
	}
	if !s.checkPassword(req.Password) {
		s.guard.fail(ip, user)
		s.log.Warn("两步验证绑定失败：密码不正确", "ip", ip)
		writeErr(w, http.StatusUnauthorized, "密码不正确")
		return
	}
	s.guard.success(ip, user)

	if s.TwoFactorEnabled() {
		writeErr(w, http.StatusBadRequest, "两步验证已启用，如需更换请先停用")
		return
	}

	secret, err := auth.GenerateSecret()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "生成密钥失败: "+err.Error())
		return
	}
	s.otpMu.Lock()
	s.pending[user] = pendingSetup{secret: secret, expires: time.Now().Add(pendingSetupTTL)}
	s.otpMu.Unlock()

	uri := auth.URI(s.totpIssuer(), user, secret)
	qrPNG, err := qrcode.Encode(uri, qrcode.Medium, 256)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "生成二维码失败: "+err.Error())
		return
	}
	s.log.Info("已生成两步验证绑定密钥", "user", user, "ip", ip)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"secret":    auth.FormatSecret(secret),
		"otpauth":   uri,
		"qr":        "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrPNG),
		"issuer":    s.totpIssuer(),
		"account":   user,
		"expiresIn": int(pendingSetupTTL / time.Second),
	})
}

// handle2FAEnable 校验绑定密钥对应的动态码，确认启用并返回恢复码。
func (s *Server) handle2FAEnable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	user := s.adminUser()
	ip := clientIP(r)
	s.otpMu.Lock()
	p, ok := s.pending[user]
	s.otpMu.Unlock()
	if !ok || time.Now().After(p.expires) {
		writeErr(w, http.StatusBadRequest, "二维码已过期，请重新获取")
		return
	}
	if d := s.guard.blocked(ip, user); d > 0 {
		writeErr(w, http.StatusTooManyRequests, "尝试过于频繁，请稍后再试")
		return
	}
	if err := s.verifyCode(p.secret, req.Code, false); err != nil {
		s.guard.fail(ip, user)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.ctl.Store.SetSetting(settingTOTPSecret, p.secret); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.ctl.Store.SetSetting(settingTOTPEnabled, "1")
	_ = s.ctl.Store.SetSetting(settingTOTPEnabledAt, strconv.FormatInt(time.Now().Unix(), 10))
	_ = s.ctl.Store.DelSetting(settingTOTPLastStep)

	codes, err := auth.NewRecoveryCodes(recoveryCodeCount)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "生成恢复码失败: "+err.Error())
		return
	}
	hashes := make([]string, 0, len(codes))
	formatted := make([]string, 0, len(codes))
	for _, c := range codes {
		hashes = append(hashes, auth.HashRecoveryCode(c))
		formatted = append(formatted, auth.FormatRecoveryCode(c))
	}
	if err := s.setRecoveryHashes(hashes); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.otpMu.Lock()
	delete(s.pending, user)
	s.otpMu.Unlock()
	s.guard.success(ip, user)
	s.log.Info("已启用两步验证", "user", user, "ip", ip)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"recoveryCodes": formatted,
	})
}

// handle2FADisable 停用两步验证（需密码 + 动态码/恢复码）。
func (s *Server) handle2FADisable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	user := s.adminUser()
	ip := clientIP(r)
	if d := s.guard.blocked(ip, user); d > 0 {
		writeErr(w, http.StatusTooManyRequests, "尝试过于频繁，请稍后再试")
		return
	}
	if !s.checkPassword(req.Password) {
		s.guard.fail(ip, user)
		writeErr(w, http.StatusUnauthorized, "密码不正确")
		return
	}
	if s.TwoFactorEnabled() {
		if err := s.checkManageCode(req.Code); err != nil {
			s.guard.fail(ip, user)
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := s.clearTOTP(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.revokeOtherSessions(r)
	s.guard.success(ip, user)
	s.clearDeviceCookie(w)
	s.log.Info("已停用两步验证", "user", user, "ip", ip)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handle2FARecovery 重新生成恢复码（旧恢复码立即失效）。
func (s *Server) handle2FARecovery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	user := s.adminUser()
	ip := clientIP(r)
	if d := s.guard.blocked(ip, user); d > 0 {
		writeErr(w, http.StatusTooManyRequests, "尝试过于频繁，请稍后再试")
		return
	}
	if !s.TwoFactorEnabled() {
		writeErr(w, http.StatusBadRequest, "请先启用两步验证")
		return
	}
	if !s.checkPassword(req.Password) {
		s.guard.fail(ip, user)
		writeErr(w, http.StatusUnauthorized, "密码不正确")
		return
	}
	if err := s.checkManageCode(req.Code); err != nil {
		s.guard.fail(ip, user)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	codes, err := auth.NewRecoveryCodes(recoveryCodeCount)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "生成恢复码失败: "+err.Error())
		return
	}
	hashes := make([]string, 0, len(codes))
	formatted := make([]string, 0, len(codes))
	for _, c := range codes {
		hashes = append(hashes, auth.HashRecoveryCode(c))
		formatted = append(formatted, auth.FormatRecoveryCode(c))
	}
	if err := s.setRecoveryHashes(hashes); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.guard.success(ip, user)
	s.log.Info("已重新生成两步验证恢复码", "user", user, "ip", ip)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recoveryCodes": formatted})
}

// checkManageCode 校验管理操作（停用/重置恢复码）所需的动态码或恢复码。
func (s *Server) checkManageCode(code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return errTOTPCode
	}
	if digitsOnly(code) {
		return s.verifyCode(s.totpSecret(), code, false)
	}
	if len(auth.NormalizeRecoveryCode(code)) == auth.RecoveryCodeLen && s.consumeRecoveryCode(code) {
		return nil
	}
	return errTOTPCode
}

// handleRevokeOtherSessions 注销除当前会话外的全部登录态。
func (s *Server) handleRevokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	token := sessionToken(r)
	n := s.sess.removeOthers(token)
	if s.ctl.Store != nil {
		if err := s.ctl.Store.DeleteOtherSessions(token); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	s.log.Info("已注销其它登录会话", "count", n, "ip", clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revoked": n})
}

// handleLogin2FA 完成登录第二步：校验动态码/恢复码后建立会话。
func (s *Server) handleLogin2FA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Challenge string `json:"challenge"`
		Code      string `json:"code"`
		Remember  bool   `json:"remember"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	ip, ua := clientIP(r), uaHash(r)
	c, ok := s.lookupChallenge(req.Challenge, ip, ua)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "验证已过期，请重新登录")
		return
	}
	if d := s.guard.blocked(ip, c.user); d > 0 {
		writeErr(w, http.StatusTooManyRequests, "尝试过于频繁，请稍后再试")
		return
	}
	code := strings.TrimSpace(req.Code)
	if len(code) > 32 {
		code = code[:32]
	}
	if err := s.checkSecondFactor(code); err != nil {
		alive := s.failChallenge(req.Challenge)
		s.guard.fail(ip, c.user)
		s.log.Warn("两步验证失败", "ip", ip, "err", err)
		if alive {
			writeErr(w, http.StatusUnauthorized, err.Error())
		} else {
			writeErr(w, http.StatusUnauthorized, "验证失败次数过多，请重新登录")
		}
		return
	}
	s.dropChallenge(req.Challenge)
	s.guard.success(ip, c.user)
	s.startSession(w, r, c.user)
	if req.Remember {
		s.issueDeviceToken(w, r, c.user)
	}
	s.log.Info("登录成功（两步验证）", "user", c.user, "ip", ip)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": c.user})
}
