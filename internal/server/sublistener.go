package server

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gost-webui/internal/cert"
	"gost-webui/internal/model"
	"gost-webui/internal/sub"
)

// 订阅监听器默认参数。
const (
	defaultSubPort   = 8788
	defaultSubSuffix = "/sub"
)

// subSettings 汇总订阅监听器的运行参数（均从数据库设置读取）。
type subSettings struct {
	Port    int
	Suffix  string
	Domain  string
	Email   string
	CertPEM string
	KeyPEM  string
}

// readSubSettings 读取并规范化订阅设置。
func (s *Server) readSubSettings() subSettings {
	st := s.ctl.Store
	set := subSettings{Port: defaultSubPort, Suffix: defaultSubSuffix}
	if v, ok := st.GetSetting("sub_port"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 1 && n <= 65535 {
			set.Port = n
		}
	}
	if v, ok := st.GetSetting("sub_suffix"); ok {
		set.Suffix = normalizeSubSuffix(v)
	}
	if v, ok := st.GetSetting("sub_domain"); ok {
		set.Domain = strings.ToLower(strings.TrimSpace(v))
	}
	if v, ok := st.GetSetting("sub_acme_email"); ok {
		set.Email = strings.TrimSpace(v)
	}
	if v, ok := st.GetSetting("sub_tls_cert"); ok {
		set.CertPEM = v
	}
	if v, ok := st.GetSetting("sub_tls_key"); ok {
		set.KeyPEM = v
	}
	return set
}

// normalizeSubSuffix 规范化路径后缀：补前导斜杠、去尾部斜杠；空则返回默认。
func normalizeSubSuffix(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return defaultSubSuffix
	}
	if !strings.HasPrefix(v, "/") {
		v = "/" + v
	}
	v = strings.TrimRight(v, "/")
	if v == "" {
		return defaultSubSuffix
	}
	return v
}

// SetCertProvider 注入证书管理器（由 main 调用）。
func (s *Server) SetCertProvider(p *cert.Provider) {
	s.subMu.Lock()
	s.certProv = p
	s.subMu.Unlock()
}

// EnsureSubTokens 为尚无订阅令牌的旧节点回填令牌（面板启动时调用一次）。
func (s *Server) EnsureSubTokens() {
	var changed bool
	for _, n := range s.ctl.Nodes() {
		if n == nil || strings.TrimSpace(n.SubToken) != "" {
			continue
		}
		n.SubToken = randomPassword(24)
		if err := s.ctl.Store.SaveNode(n); err != nil {
			s.log.Warn("回填订阅令牌失败", "node", n.ID, "err", err)
			continue
		}
		changed = true
	}
	if changed {
		if _, err := s.ctl.LoadNodes(); err != nil {
			s.log.Warn("刷新节点缓存失败", "err", err)
		}
	}
}

func (s *Server) certProvider() *cert.Provider {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	return s.certProv
}

// subUseTLS 表示订阅监听器是否以 HTTPS 提供（已配域名且有手动证书或 ACME 邮箱）。
func (s *Server) subUseTLS() bool {
	prov := s.certProvider()
	return prov != nil && prov.TLSEnabled()
}

// certReady 表示是否已有可用证书（用于前端展示与 URL 计算）。
func (s *Server) certReady() bool {
	prov := s.certProvider()
	if prov == nil || !prov.TLSEnabled() {
		return false
	}
	return prov.Status(context.Background()).NotAfter > 0
}

// StartSubscription 启动独立订阅监听器（HTTP 或 HTTPS，取决于证书配置）。
func (s *Server) StartSubscription(ctx context.Context) {
	s.subMu.Lock()
	s.subCtx = ctx
	s.subMu.Unlock()
	if err := s.restartSubscription(); err != nil {
		s.log.Warn("订阅监听器启动失败", "err", err)
	}
}

// RestartSubscription 热重启订阅监听器（改端口/后缀/域名/证书后调用，无需重启面板）。
func (s *Server) RestartSubscription() {
	if err := s.restartSubscription(); err != nil {
		s.log.Warn("订阅监听器重启失败", "err", err)
	}
}

// StopSubscription 停止订阅监听器。
func (s *Server) StopSubscription(ctx context.Context) {
	s.subMu.Lock()
	srv := s.subSrv
	s.subSrv = nil
	s.subMu.Unlock()
	if srv != nil {
		_ = srv.Shutdown(ctx)
	}
}

// restartSubscription 优雅停旧起新：读取设置、刷新证书、按后缀构建路由并监听。
func (s *Server) restartSubscription() error {
	s.subMu.Lock()
	ctx := s.subCtx
	old := s.subSrv
	s.subSrv = nil
	prov := s.certProv
	s.subMu.Unlock()

	if ctx == nil {
		ctx = context.Background()
	}
	if old != nil {
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = old.Shutdown(shutCtx)
		cancel()
	}

	set := s.readSubSettings()

	// 刷新证书管理器（域名/邮箱/手动证书），据此决定是否启用 HTTPS。
	if prov != nil {
		if err := prov.Reload(set.Domain, set.Email, set.CertPEM, set.KeyPEM); err != nil {
			s.log.Warn("证书配置无效，订阅将以 HTTP 提供", "err", err)
		}
	}

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(set.Port),
		Handler:           s.subHandler(set.Suffix),
		ReadHeaderTimeout: 10 * time.Second,
	}
	useTLS := prov != nil && prov.TLSEnabled()
	if useTLS {
		srv.TLSConfig = &tls.Config{
			GetCertificate: prov.GetCertificate,
			NextProtos:     []string{"h2", "http/1.1"},
		}
	}

	s.subMu.Lock()
	s.subSrv = srv
	s.subMu.Unlock()

	go func() {
		var err error
		if useTLS {
			s.log.Info("订阅监听器已启动 (HTTPS)", "addr", srv.Addr, "suffix", set.Suffix, "domain", set.Domain)
			err = srv.ListenAndServeTLS("", "")
		} else {
			s.log.Info("订阅监听器已启动 (HTTP)", "addr", srv.Addr, "suffix", set.Suffix)
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			s.log.Warn("订阅监听器退出", "err", err)
		}
	}()
	return nil
}

// subHandler 按后缀动态构建订阅路由。
func (s *Server) subHandler(suffix string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+suffix+"/{token}", s.handleSubFetch)
	mux.HandleFunc("GET "+suffix+"/{token}/clash", s.handleSubClash)
	mux.HandleFunc("GET "+suffix+"/{token}/universal", s.handleSubUniversal)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "订阅地址无效")
	})
	return mux
}

// subBaseURL 返回订阅入口基础 URL（不含令牌），如 https://example.com:8788/sub。
func (s *Server) subBaseURL() string {
	set := s.readSubSettings()
	scheme := "http"
	host := s.publicHost()
	if s.subUseTLS() {
		scheme = "https"
		host = set.Domain
	}
	if strings.TrimSpace(host) == "" {
		host = "127.0.0.1"
	}
	return scheme + "://" + net.JoinHostPort(host, strconv.Itoa(set.Port)) + set.Suffix
}

// nodeBySubToken 按订阅令牌常数时间匹配节点（防时序侧信道）。
func (s *Server) nodeBySubToken(token string) *model.Node {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	tok := []byte(token)
	for _, n := range s.ctl.Nodes() {
		if n == nil || n.SubToken == "" {
			continue
		}
		if subtle.ConstantTimeCompare(tok, []byte(n.SubToken)) == 1 {
			return n
		}
	}
	return nil
}

// handleSubFetch 公开订阅入口：按 User-Agent 自动返回 Clash YAML 或通用链接（仅含该节点）。
func (s *Server) handleSubFetch(w http.ResponseWriter, r *http.Request) {
	node := s.nodeBySubToken(r.PathValue("token"))
	if node == nil {
		writeErr(w, http.StatusForbidden, "订阅令牌无效")
		return
	}
	if isClashUA(r.UserAgent()) {
		s.serveNodeClash(w, node)
		return
	}
	s.serveNodeUniversal(w, node)
}

// handleSubClash 公开返回该节点的 Clash Meta YAML（强制）。
func (s *Server) handleSubClash(w http.ResponseWriter, r *http.Request) {
	node := s.nodeBySubToken(r.PathValue("token"))
	if node == nil {
		writeErr(w, http.StatusForbidden, "订阅令牌无效")
		return
	}
	s.serveNodeClash(w, node)
}

// handleSubUniversal 公开返回该节点的通用 base64 链接（强制）。
func (s *Server) handleSubUniversal(w http.ResponseWriter, r *http.Request) {
	node := s.nodeBySubToken(r.PathValue("token"))
	if node == nil {
		writeErr(w, http.StatusForbidden, "订阅令牌无效")
		return
	}
	s.serveNodeUniversal(w, node)
}

func (s *Server) serveNodeClash(w http.ResponseWriter, node *model.Node) {
	name := node.Name
	if name == "" {
		name = "Gost-WebUI"
	}
	yml, _, err := sub.ClashYAML([]*model.Node{node}, s.publicHost(), name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="clash.yaml"`)
	w.Header().Set("Profile-Update-Interval", "12")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(yml))
}

func (s *Server) serveNodeUniversal(w http.ResponseWriter, node *model.Node) {
	b64, _, err := sub.Universal([]*model.Node{node}, s.publicHost())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(b64))
}
