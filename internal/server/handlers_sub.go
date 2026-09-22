package server

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/skip2/go-qrcode"

	"gost-webui/internal/sub"
)

// subToken 返回订阅令牌；首次访问时自动生成并持久化。仅供已登录接口调用。
func (s *Server) subToken() string {
	if v, ok := s.ctl.Store.GetSetting("sub_token"); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	t := randomPassword(32)
	_ = s.ctl.Store.SetSetting("sub_token", t)
	return t
}

// storedSubToken 只读取已存在的订阅令牌（不存在返回空），供公开接口使用，避免未登录写入。
func (s *Server) storedSubToken() string {
	if v, ok := s.ctl.Store.GetSetting("sub_token"); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// subTokenOK 常数时间比较订阅令牌，防时序侧信道。
func (s *Server) subTokenOK(token string) bool {
	want := s.storedSubToken()
	if want == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(want)) == 1
}

// handleSubscriptionInfo 返回订阅页所需信息：令牌、节点统计与 Clash 生成预览。
func (s *Server) handleSubscriptionInfo(w http.ResponseWriter, r *http.Request) {
	token := s.subToken()
	host := s.publicHost()
	nodes := s.ctl.Nodes()
	enabled := 0
	for _, n := range nodes {
		if n != nil && n.Enabled {
			enabled++
		}
	}

	var skipped []string
	clashOK := false
	if _, res, err := sub.ClashYAML(nodes, host, "Gost-WebUI"); err == nil {
		clashOK = true
		if res != nil {
			skipped = res.Skipped
		}
	} else if res != nil {
		skipped = res.Skipped
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":             token,
		"basePath":          s.effectiveBasePath(),
		"publicHost":        host,
		"publicHostPrivate": isPrivateHost(host),
		"nodeTotal":         len(nodes),
		"nodeEnabled":       enabled,
		"clashOK":           clashOK,
		"skipped":           skipped,
	})
}

// handleSubscriptionReset 重新生成订阅令牌（旧链接与二维码立即失效）。
func (s *Server) handleSubscriptionReset(w http.ResponseWriter, r *http.Request) {
	token := randomPassword(32)
	if err := s.ctl.Store.SetSetting("sub_token", token); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token})
}

// handleQRText 生成任意文本（通常是订阅链接）的二维码 PNG，供已登录前端展示。
func (s *Server) handleQRText(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if strings.TrimSpace(text) == "" {
		writeErr(w, http.StatusBadRequest, "缺少 text 参数")
		return
	}
	png, err := qrcode.Encode(text, qrcode.Medium, 320)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

// isClashUA 判断客户端 User-Agent 是否属于 Clash 系（需要 YAML 配置）。
func isClashUA(ua string) bool {
	ua = strings.ToLower(ua)
	return strings.Contains(ua, "clash") || strings.Contains(ua, "mihomo") || strings.Contains(ua, "stash")
}

// handleSubFetch 是公开订阅入口：按 User-Agent 自动返回 Clash YAML 或通用 base64 链接。
func (s *Server) handleSubFetch(w http.ResponseWriter, r *http.Request) {
	if !s.subTokenOK(r.PathValue("token")) {
		writeErr(w, http.StatusForbidden, "订阅令牌无效")
		return
	}
	if isClashUA(r.UserAgent()) {
		s.serveSubClash(w)
		return
	}
	s.serveSubUniversal(w)
}

// handleSubClash 公开返回 Clash Meta YAML（强制）。
func (s *Server) handleSubClash(w http.ResponseWriter, r *http.Request) {
	if !s.subTokenOK(r.PathValue("token")) {
		writeErr(w, http.StatusForbidden, "订阅令牌无效")
		return
	}
	s.serveSubClash(w)
}

// handleSubUniversal 公开返回通用 base64 链接（强制）。
func (s *Server) handleSubUniversal(w http.ResponseWriter, r *http.Request) {
	if !s.subTokenOK(r.PathValue("token")) {
		writeErr(w, http.StatusForbidden, "订阅令牌无效")
		return
	}
	s.serveSubUniversal(w)
}

func (s *Server) serveSubClash(w http.ResponseWriter) {
	yml, _, err := sub.ClashYAML(s.ctl.Nodes(), s.publicHost(), "Gost-WebUI")
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

func (s *Server) serveSubUniversal(w http.ResponseWriter) {
	b64, _, err := sub.Universal(s.ctl.Nodes(), s.publicHost())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(b64))
}
