package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"gost-webui/internal/gostmgr"
	"gost-webui/internal/link"
	"gost-webui/internal/model"
	"gost-webui/internal/singbox"
)

// nodeInput 是新增/编辑节点的请求体。
type nodeInput struct {
	Name       string `json:"name"`
	Link       string `json:"link"`
	ListenPort int    `json:"listenPort"`
	UDP        *bool  `json:"udp"`
	TargetHost string `json:"targetHost"`
	TargetPort int    `json:"targetPort"`
	SNI        string `json:"sni"`
	Host       string `json:"host"`
	Enabled    *bool  `json:"enabled"`
	// Mode 为 link（默认，粘贴落地机链接）/ gost（GOST 原生代理服务）。
	Mode          string `json:"mode"`
	GostLocal     bool   `json:"gostLocal"`
	GostProtocol  string `json:"gostProtocol"`
	GostTransport string `json:"gostTransport"`
	GostUsername  string `json:"gostUsername"`
	GostCipher    string `json:"gostCipher"`
	GostPassword  string `json:"gostPassword"`
	GostPath      string `json:"gostPath"`
	// Reality 专用
	RealityUuid    string `json:"realityUuid"`
	RealityPriv    string `json:"realityPriv"`
	RealityPub     string `json:"realityPub"`
	RealityShortId string `json:"realityShortId"`
	RealitySni     string `json:"realitySni"`
	Quota          *struct {
		Enabled   bool   `json:"enabled"`
		Period    string `json:"period"`
		Bytes     int64  `json:"bytes"`
		Direction string `json:"direction"`
	} `json:"quota"`
	Rate *struct {
		Enabled bool  `json:"enabled"`
		InBps   int64 `json:"inBps"`
		OutBps  int64 `json:"outBps"`
	} `json:"rate"`
	ConnLimit int `json:"connLimit"`
}

// nodeView 是返回给前端的节点信息。
type nodeView struct {
	*model.Node
	Landing  *link.Info  `json:"landing"`
	URL      string      `json:"url"`
	Live     *model.Live `json:"live"`
	TodayIn  uint64      `json:"todayIn"`
	TodayOut uint64      `json:"todayOut"`
	MonthIn  uint64      `json:"monthIn"`
	MonthOut uint64      `json:"monthOut"`
}

func (s *Server) publicHost() string {
	if v, ok := s.ctl.Store.GetSetting("public_host"); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if s.cfg.PublicHost != "" {
		return s.cfg.PublicHost
	}
	return s.detectPublicIP()
}

// detectPublicIP 通过公共接口探测本机公网 IP（带缓存）。
func (s *Server) detectPublicIP() string {
	s.mu.Lock()
	if s.hostIP != "" && time.Since(s.hostTime) < 10*time.Minute {
		ip := s.hostIP
		s.mu.Unlock()
		return ip
	}
	s.mu.Unlock()

	endpoints := []string{
		"https://api.ipify.org",
		"https://ipv4.icanhazip.com",
		"https://ifconfig.me/ip",
		"https://ip.sb",
		"https://myip.ipip.net/s",
		"https://www.cloudflare.com/cdn-cgi/trace",
		"http://ip.3322.net",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for _, ep := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep, nil)
		if err != nil {
			continue
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		res.Body.Close()
		ip := parseIPFromBody(string(body))
		if ip != "" {
			s.mu.Lock()
			s.hostIP = ip
			s.hostTime = time.Now()
			s.mu.Unlock()
			return ip
		}
	}
	return ""
}

// parseIPFromBody 从各种 IP 回显服务的响应里提取 IP。
func parseIPFromBody(body string) string {
	// cloudflare cdn-cgi/trace: 形如 "ip=1.2.3.4"
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ip=") {
			if ip := strings.TrimSpace(strings.TrimPrefix(line, "ip=")); net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	// 其它服务：取第一个形如 IP 的字段
	for _, f := range strings.FieldsFunc(body, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ' ' || r == '\t' || r == ','
	}) {
		f = strings.TrimSpace(strings.Trim(f, "\"'"))
		if net.ParseIP(f) != nil {
			return f
		}
	}
	return ""
}

func (s *Server) buildView(n *model.Node) *nodeView {
	// API 输出副本：剥离 REALITY 私钥，避免下发到前端；缓存节点保持不变。
	public := *n
	public.RealityPriv = ""
	v := &nodeView{Node: &public, Live: s.ctl.Live(n.ID)}
	if n.Mode != "gost" {
		if info, err := link.Parse(n.LandingLink); err == nil {
			v.Landing = info
		}
	} else if info, err := link.Parse(n.LandingLink); err == nil {
		// 兼容旧版 Shadowsocks 节点的落地信息展示。
		v.Landing = info
	}
	if host := s.publicHost(); host != "" {
		if out, _, err := s.nodeClientURL(n, host); err == nil {
			v.URL = out
		}
	}
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	if pts, err := s.ctl.Store.RangeTraffic(n.ID, startOfDay.Unix(), now.Unix()+1); err == nil {
		for _, p := range pts {
			v.TodayIn += p.In
			v.TodayOut += p.Out
		}
	}
	if pts, err := s.ctl.Store.RangeTraffic(n.ID, startOfMonth.Unix(), now.Unix()+1); err == nil {
		for _, p := range pts {
			v.MonthIn += p.In
			v.MonthOut += p.Out
		}
	}
	return v
}

// nodeClientURL 生成客户端实际连接本机监听端口的地址。
func (s *Server) nodeClientURL(n *model.Node, host string) (string, *link.Info, error) {
	if n.IsReality() {
		cred := &singbox.RealityCred{
			UUID: n.RealityUUID, PublicKey: n.RealityPub,
			ShortID: n.RealityShortID, ServerName: n.RealitySNI,
		}
		return singbox.ClientURL(cred, n.ListenPort, host, n.Name), nil, nil
	}
	if n.Mode == "gost" {
		out, err := gostmgr.GostClientURL(n, host, n.ListenPort)
		return out, nil, err
	}
	info, err := link.Parse(n.LandingLink)
	if err != nil {
		return "", nil, fmt.Errorf("落地链接解析失败: %w", err)
	}
	out, err := info.Rewrite(host, n.ListenPort, n.Name, n.SNI, n.Host)
	return out, info, err
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes := s.ctl.Nodes()
	out := make([]*nodeView, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, s.buildView(n))
	}
	host := s.publicHost()
	writeJSON(w, http.StatusOK, map[string]any{
		"nodes":             out,
		"publicHost":        host,
		"publicHostPrivate": isPrivateHost(host),
	})
}

// isPrivateHost 判断地址是否为内网/回环地址（域名视为公网）。
func isPrivateHost(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

func (s *Server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	n := s.ctl.Node(r.PathValue("id"))
	if n == nil {
		writeErr(w, http.StatusNotFound, "节点不存在")
		return
	}
	writeJSON(w, http.StatusOK, s.buildView(n))
}

func (s *Server) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	var in nodeInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}
	node, err := s.buildNode(&in, nil)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	node.CreatedAt = time.Now().Unix()
	node.UpdatedAt = node.CreatedAt

	if err := s.ctl.Store.SaveNode(node); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}
	if err := s.ctl.ApplyNode(r.Context(), node); err != nil {
		_ = s.ctl.RemoveNode(context.Background(), node)
		_ = s.ctl.Store.DeleteNode(node.ID)
		writeErr(w, http.StatusInternalServerError, "应用配置失败: "+err.Error())
		return
	}
	if _, err := s.ctl.LoadNodes(); err != nil {
		s.log.Warn("刷新节点缓存失败", "err", err)
	}
	if s.ctl.Alerts != nil {
		go s.ctl.Alerts.NotifyNodeAdded(context.Background(), node)
	}
	writeJSON(w, http.StatusOK, s.buildView(node))
}

func (s *Server) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	old := s.ctl.Node(id)
	if old == nil {
		writeErr(w, http.StatusNotFound, "节点不存在")
		return
	}
	var in nodeInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}
	node, err := s.buildNode(&in, old)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	node.ID = old.ID
	node.CreatedAt = old.CreatedAt
	node.UpdatedAt = time.Now().Unix()
	node.TotalIn, node.TotalOut = old.TotalIn, old.TotalOut

	if err := s.ctl.Store.SaveNode(node); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}
	if _, err := s.ctl.LoadNodes(); err != nil {
		s.log.Warn("刷新节点缓存失败", "err", err)
	}
	if err := s.ctl.ApplyNode(r.Context(), node); err != nil {
		// 回滚
		s.log.Error("应用节点失败，已回滚", "node", id, "err", err)
		if err := s.ctl.Store.SaveNode(old); err == nil {
			_, _ = s.ctl.LoadNodes()
			_ = s.ctl.ApplyNode(context.Background(), old)
		}
		writeErr(w, http.StatusInternalServerError, "应用配置失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.buildView(node))
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	node := s.ctl.Node(r.PathValue("id"))
	if node == nil {
		writeErr(w, http.StatusNotFound, "节点不存在")
		return
	}
	_ = s.ctl.RemoveNode(r.Context(), node)
	if err := s.ctl.Store.DeleteNode(node.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "删除失败: "+err.Error())
		return
	}
	_, _ = s.ctl.LoadNodes()
	if s.ctl.Alerts != nil {
		go s.ctl.Alerts.NotifyNodeDeleted(context.Background(), node)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleToggleNode(w http.ResponseWriter, r *http.Request) {
	node := s.ctl.Node(r.PathValue("id"))
	if node == nil {
		writeErr(w, http.StatusNotFound, "节点不存在")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	node.Enabled = req.Enabled
	node.UpdatedAt = time.Now().Unix()
	if err := s.ctl.Store.SaveNode(node); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}
	_, _ = s.ctl.LoadNodes()
	if err := s.ctl.ApplyNode(r.Context(), node); err != nil {
		writeErr(w, http.StatusInternalServerError, "应用配置失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.buildView(node))
}

func (s *Server) handleResetNode(w http.ResponseWriter, r *http.Request) {
	node := s.ctl.Node(r.PathValue("id"))
	if node == nil {
		writeErr(w, http.StatusNotFound, "节点不存在")
		return
	}
	if err := s.ctl.ResetNodeTraffic(r.Context(), node); err != nil {
		writeErr(w, http.StatusInternalServerError, "重置失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.buildView(node))
}

func (s *Server) handleNodeStats(w http.ResponseWriter, r *http.Request) {
	node := s.ctl.Node(r.PathValue("id"))
	if node == nil {
		writeErr(w, http.StatusNotFound, "节点不存在")
		return
	}
	now := time.Now()
	var from time.Time
	step := int64(3600)
	switch r.URL.Query().Get("range") {
	case "24h":
		from = now.Add(-24 * time.Hour)
	case "30d":
		from = now.AddDate(0, 0, -30)
		step = 86400
	case "90d":
		from = now.AddDate(0, 0, -90)
		step = 86400
	default: // 7d
		from = now.AddDate(0, 0, -7)
		step = 86400
	}
	pts, err := s.ctl.Store.RangeTraffic(node.ID, from.Unix(), now.Unix()+1)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 按 step 聚合
	buckets := map[int64]*model.Point{}
	var order []int64
	for _, p := range pts {
		key := p.TS - p.TS%step
		b, ok := buckets[key]
		if !ok {
			b = &model.Point{TS: key}
			buckets[key] = b
			order = append(order, key)
		}
		b.In += p.In
		b.Out += p.Out
	}
	// 排序
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && order[j] < order[j-1]; j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	out := make([]model.Point, 0, len(order))
	for _, k := range order {
		out = append(out, *buckets[k])
	}
	writeJSON(w, http.StatusOK, map[string]any{"points": out, "step": step, "live": s.ctl.Live(node.ID)})
}

func (s *Server) handleNodeLink(w http.ResponseWriter, r *http.Request) {
	node := s.ctl.Node(r.PathValue("id"))
	if node == nil {
		writeErr(w, http.StatusNotFound, "节点不存在")
		return
	}
	host := s.publicHost()
	if host == "" {
		writeErr(w, http.StatusBadRequest, "无法自动探测公网地址，请到「设置」手动填写中转机地址")
		return
	}
	out, info, err := s.nodeClientURL(node, host)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":     out,
		"host":    host,
		"port":    node.ListenPort,
		"landing": info,
	})
}

func (s *Server) handleParseLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Link string `json:"link"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	info, err := link.Parse(req.Link)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "解析失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleFreePort(w http.ResponseWriter, r *http.Request) {
	port, err := pickFreePort()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"port": port})
}

// handleQRCode 返回节点中转链接的二维码 PNG。
func (s *Server) handleQRCode(w http.ResponseWriter, r *http.Request) {
	node := s.ctl.Node(r.PathValue("id"))
	if node == nil {
		writeErr(w, http.StatusNotFound, "节点不存在")
		return
	}
	host := s.publicHost()
	if host == "" {
		writeErr(w, http.StatusBadRequest, "未配置中转机地址")
		return
	}
	u, _, err := s.nodeClientURL(node, host)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	png, err := qrcode.Encode(u, qrcode.Medium, 320)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

// handleTestConnect 测试落地机是否可达（TCP 连接测试）。
func (s *Server) handleTestConnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if req.Host == "" || req.Port <= 0 || req.Port > 65535 {
		writeErr(w, http.StatusBadRequest, "地址或端口无效")
		return
	}
	addr := net.JoinHostPort(req.Host, strconv.Itoa(req.Port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 6*time.Second)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = conn.Close()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"latencyMs": time.Since(start).Milliseconds(),
		"addr":      addr,
	})
}

// buildNode 校验请求并生成节点对象；old 非空表示编辑。
func (s *Server) buildNode(in *nodeInput, old *model.Node) (*model.Node, error) {
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode == "" {
		mode = "link"
	}
	switch mode {
	case "gost":
		return s.buildGostNode(in, old)
	case "reality":
		return s.buildRealityNode(in, old)
	case "link":
		return s.buildLinkNode(in, old)
	default:
		return nil, fmt.Errorf("未知的节点类型: %s", mode)
	}
}

// newNodeBase 创建带 ID 的空节点。
func newNodeBase(old *model.Node) *model.Node {
	n := &model.Node{}
	if old != nil {
		n.ID = old.ID
		n.Mode = old.Mode
		// 编辑时保留原有订阅令牌，避免每次保存都使订阅链接失效。
		n.SubToken = old.SubToken
		if n.SubToken == "" {
			n.SubToken = randomPassword(24)
		}
	} else {
		n.ID = newID()
		n.SubToken = randomPassword(24)
	}
	return n
}

// buildLinkNode 处理“粘贴落地机链接”模式。
func (s *Server) buildLinkNode(in *nodeInput, old *model.Node) (*model.Node, error) {
	if strings.TrimSpace(in.Link) == "" {
		return nil, fmt.Errorf("请填写落地机的 v2rayN 链接")
	}
	info, err := link.Parse(in.Link)
	if err != nil {
		return nil, fmt.Errorf("落地链接解析失败: %w", err)
	}

	n := newNodeBase(old)
	n.Mode = "link"
	n.LandingLink = strings.TrimSpace(in.Link)
	n.Protocol = info.DisplayScheme()
	n.Name = strings.TrimSpace(in.Name)
	if n.Name == "" {
		if info.Name != "" {
			n.Name = info.Name + "-中转"
		} else {
			n.Name = fmt.Sprintf("中转-%s:%d", info.Host, info.Port)
		}
	}

	n.TargetHost = strings.TrimSpace(in.TargetHost)
	if n.TargetHost == "" {
		n.TargetHost = info.Host
	}
	n.TargetPort = in.TargetPort
	if n.TargetPort == 0 {
		n.TargetPort = info.Port
	}
	if n.TargetPort <= 0 || n.TargetPort > 65535 {
		return nil, fmt.Errorf("落地端口无效")
	}
	if net.ParseIP(n.TargetHost) == nil && !validHostname(n.TargetHost) {
		return nil, fmt.Errorf("落地地址无效: %s", n.TargetHost)
	}

	n.SNI = strings.TrimSpace(in.SNI)
	n.Host = strings.TrimSpace(in.Host)

	if err := s.applyCommon(n, in, old, info.UDP); err != nil {
		return nil, err
	}
	return n, nil
}

// gostCiphers 是落地机 ss 服务允许的加密方式。
var gostCiphers = map[string]bool{
	"aes-128-gcm":            true,
	"aes-256-gcm":            true,
	"chacha20-ietf-poly1305": true,
}

// buildGostNode 处理“GOST 体系”模式：既可把当前面板主机直接设为落地机，
// 也可生成远程落地配置并由当前主机做原始 TCP/UDP 透传。
func (s *Server) buildGostNode(in *nodeInput, old *model.Node) (*model.Node, error) {
	n := newNodeBase(old)
	n.Mode = "gost"
	n.GostLocal = in.GostLocal
	n.GostProtocol = strings.ToLower(strings.TrimSpace(in.GostProtocol))
	if n.GostProtocol == "" {
		n.GostProtocol = "ss"
	}
	ps, ok := gostmgr.ProtocolSpec(n.GostProtocol)
	if !ok {
		return nil, fmt.Errorf("不支持的 GOST 代理协议: %s", n.GostProtocol)
	}
	n.GostTransport = strings.ToLower(strings.TrimSpace(in.GostTransport))
	if n.GostTransport == "" {
		n.GostTransport = ps.DefaultTransport
	}
	if _, ok := gostmgr.TransportSpec(n.GostTransport); !ok {
		return nil, fmt.Errorf("不支持的 GOST 传输通道: %s", n.GostTransport)
	}

	n.GostUsername = strings.TrimSpace(in.GostUsername)
	n.GostCipher = strings.TrimSpace(in.GostCipher)
	n.GostPassword = strings.TrimSpace(in.GostPassword)
	n.GostPath = strings.TrimSpace(in.GostPath)
	if n.GostPath != "" && !strings.HasPrefix(n.GostPath, "/") {
		n.GostPath = "/" + n.GostPath
	}

	if ps.Auth == "ss" {
		if n.GostCipher == "" {
			n.GostCipher = "aes-256-gcm"
		}
		if !gostCiphers[n.GostCipher] {
			return nil, fmt.Errorf("不支持的加密方式: %s", n.GostCipher)
		}
	} else {
		n.GostCipher = ""
	}
	needsChannelAuth := n.GostTransport == "ssh" || n.GostTransport == "sshd"
	if (ps.Auth == "userpass" || ps.Auth == "user" || needsChannelAuth) && n.GostUsername == "" {
		if old != nil && old.Mode == "gost" && old.GostUsername != "" {
			n.GostUsername = old.GostUsername
		} else {
			n.GostUsername = "gost"
		}
	}
	needsPassword := ps.Auth == "ss" || ps.Auth == "userpass" || needsChannelAuth
	if needsPassword {
		if n.GostPassword == "" {
			if old != nil && old.Mode == "gost" && old.GostPassword != "" {
				n.GostPassword = old.GostPassword
			} else {
				n.GostPassword = randomPassword(20)
			}
		}
	} else if ps.Auth == "none" {
		n.GostUsername = ""
		n.GostPassword = ""
	} else {
		n.GostPassword = ""
	}

	n.Protocol = strings.ToUpper(n.GostProtocol) + "+" + strings.ToUpper(n.GostTransport)

	n.Name = strings.TrimSpace(in.Name)
	if n.Name == "" {
		if n.GostLocal {
			n.Name = "GOST-本机落地"
		} else {
			n.Name = "GOST-" + strings.TrimSpace(in.TargetHost)
		}
	}

	if err := s.applyCommon(n, in, old, false); err != nil {
		return nil, err
	}
	if n.GostLocal {
		n.TargetHost = "127.0.0.1"
		n.TargetPort = n.ListenPort
	} else {
		n.TargetHost = strings.TrimSpace(in.TargetHost)
		if n.TargetHost == "" {
			return nil, fmt.Errorf("请填写落地机的公网 IP 或域名")
		}
		if net.ParseIP(n.TargetHost) == nil && !validHostname(n.TargetHost) {
			return nil, fmt.Errorf("落地地址无效: %s", n.TargetHost)
		}
		n.TargetPort = in.TargetPort
		if n.TargetPort <= 0 || n.TargetPort > 65535 {
			return nil, fmt.Errorf("落地端口无效")
		}
	}
	if err := gostmgr.ValidateGostNode(n); err != nil {
		return nil, err
	}
	landing, err := gostmgr.GostClientURL(n, n.TargetHost, n.TargetPort)
	if err != nil {
		return nil, err
	}
	n.LandingLink = landing
	return n, nil
}

// applyCommon 填充 link 与 gost 两种模式共用的字段：
// UDP、监听端口、并发限制、配额、限速、启用状态、配额计数起点。
func (s *Server) applyCommon(n *model.Node, in *nodeInput, old *model.Node, defaultUDP bool) error {
	// UDP：默认按协议/模式判断，用户可覆盖
	if in.UDP != nil {
		n.UDP = *in.UDP
	} else if old != nil {
		n.UDP = old.UDP
	} else {
		n.UDP = defaultUDP
	}

	// 监听端口
	if in.ListenPort > 0 {
		if in.ListenPort < 1 || in.ListenPort > 65535 {
			return fmt.Errorf("中转端口无效")
		}
		n.ListenPort = in.ListenPort
	} else if old != nil && old.ListenPort > 0 {
		n.ListenPort = old.ListenPort
	} else {
		p, err := pickFreePort()
		if err != nil {
			return err
		}
		n.ListenPort = p
	}
	// 端口占用检查（编辑自身时跳过）
	if old == nil || old.ListenPort != n.ListenPort {
		if !portFree(n.ListenPort) {
			return fmt.Errorf("端口 %d 已被占用", n.ListenPort)
		}
	}

	n.ConnLimit = in.ConnLimit
	if n.ConnLimit < 0 {
		n.ConnLimit = 0
	}

	if in.Quota != nil {
		n.Quota.Enabled = in.Quota.Enabled
		n.Quota.Period = in.Quota.Period
		n.Quota.Bytes = in.Quota.Bytes
		n.Quota.Direction = in.Quota.Direction
		if n.Quota.Enabled {
			switch n.Quota.Period {
			case "daily", "monthly", "total":
			default:
				return fmt.Errorf("配额周期无效")
			}
			switch n.Quota.Direction {
			case "in", "out":
			default:
				n.Quota.Direction = "total"
			}
			if n.Quota.Bytes <= 0 {
				return fmt.Errorf("请填写有效的流量额度")
			}
		}
	}

	if in.Rate != nil {
		n.Rate.Enabled = in.Rate.Enabled
		n.Rate.InBps = in.Rate.InBps
		n.Rate.OutBps = in.Rate.OutBps
		if n.Rate.Enabled {
			if n.Rate.InBps < 0 || n.Rate.OutBps < 0 {
				return fmt.Errorf("限速值无效")
			}
			if n.Rate.InBps == 0 && n.Rate.OutBps == 0 {
				return fmt.Errorf("请填写限速值")
			}
		}
	}

	n.Enabled = true
	if in.Enabled != nil {
		n.Enabled = *in.Enabled
	} else if old != nil {
		n.Enabled = old.Enabled
	}

	// 配额计数起点：配额参数未变则沿用，否则从当前时刻重新计数
	if n.Quota.Enabled && n.Quota.Bytes > 0 {
		if old != nil && old.Quota == n.Quota && old.QuotaSince > 0 {
			n.QuotaSince = old.QuotaSince
		} else {
			n.QuotaSince = time.Now().Unix()
		}
	}
	return nil
}

func newID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 10)
	for i := range b {
		b[i] = chars[randomPort(0, len(chars)-1)]
	}
	return string(b)
}

func validHostname(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, part := range strings.Split(s, ".") {
		if part == "" || len(part) > 63 {
			return false
		}
		for _, c := range part {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	return true
}

func portFree(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = l.Close()
	u, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = u.Close()
	return true
}

func pickFreePort() (int, error) {
	for i := 0; i < 50; i++ {
		p := randomPort(10000, 60000)
		if portFree(p) {
			return p, nil
		}
	}
	return 0, fmt.Errorf("无法找到空闲端口")
}
