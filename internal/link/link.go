// Package link 解析并改写 v2rayN 分享链接（vmess/vless/trojan/ss/hysteria2/tuic 等）。
//
// 中转的核心思路：落地机的分享链接除了「地址 + 端口」以外的参数（UUID、密码、
// TLS、SNI、WS 路径……）全部保持不变；把地址端口替换为中转机的即可。
package link

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Info 是从分享链接中解析出的信息。
type Info struct {
	Raw       string `json:"raw"`
	Scheme    string `json:"scheme"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Name      string `json:"name"`
	Transport string `json:"transport"`
	TLS       bool   `json:"tls"`
	SNI       string `json:"sni"`
	HostHdr   string `json:"hostHeader"`
	// UDP 表示该协议依赖 UDP（QUIC/KCP 等），中转必须同时转发 UDP。
	UDP bool `json:"udp"`
	// URI 型链接的原始片段（仅内部使用）。
	rawScheme string
	userinfo  string
	rawQuery  string
	vmess     map[string]any
	isVmess   bool
	legacySS  *ssLegacy
}

type ssLegacy struct {
	method string
	pass   string
}

// Parse 解析分享链接。
func Parse(raw string) (*Info, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("链接为空")
	}
	idx := strings.Index(raw, "://")
	if idx <= 0 {
		return nil, fmt.Errorf("无法识别的链接格式（缺少 scheme://）")
	}
	scheme := strings.ToLower(raw[:idx])
	for i, c := range scheme {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.' || (c == ' ' && i > 0)) {
			return nil, fmt.Errorf("非法的链接协议 %q", scheme)
		}
	}
	rest := raw[idx+3:]

	info := &Info{Raw: raw, Scheme: scheme, rawScheme: scheme}
	switch scheme {
	case "vmess":
		if err := info.parseVmess(rest); err != nil {
			return nil, err
		}
	case "ss":
		if err := info.parseSS(rest); err != nil {
			return nil, err
		}
	default:
		if err := info.parseURI(rest); err != nil {
			return nil, err
		}
	}
	if info.Port <= 0 || info.Port > 65535 {
		return nil, fmt.Errorf("无效端口 %d", info.Port)
	}
	if info.Host == "" {
		return nil, fmt.Errorf("未解析到服务器地址")
	}
	info.UDP = info.needsUDP()
	return info, nil
}

// needsUDP 判断协议是否必须依赖 UDP 转发。
func (i *Info) needsUDP() bool {
	switch i.Scheme {
	case "hysteria2", "hy2", "hysteria", "tuic", "wireguard", "wg", "juicity":
		return true
	}
	switch strings.ToLower(i.Transport) {
	case "kcp", "quic":
		return true
	}
	return false
}

// NeedTLS 判断链接是否使用 TLS。
func (i *Info) NeedTLS() bool { return i.TLS }

func (i *Info) parseVmess(rest string) error {
	// vmess 链接可能是 base64(JSON)，也可能带 query（部分客户端变体）。
	rest = strings.TrimSpace(rest)
	if q := strings.IndexAny(rest, "?#"); q >= 0 {
		rest = rest[:q]
	}
	data, err := decodeBase64Loose(rest)
	if err != nil {
		return fmt.Errorf("vmess 链接解码失败: %w", err)
	}
	m := map[string]any{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("vmess JSON 解析失败: %w", err)
	}
	host, _ := m["add"].(string)
	if host == "" {
		return fmt.Errorf("vmess 链接缺少 add 字段")
	}
	port := toInt(m["port"])
	name, _ := m["ps"].(string)
	netw, _ := m["net"].(string)
	tls, _ := m["tls"].(string)
	sni, _ := m["sni"].(string)
	hostHdr, _ := m["host"].(string)

	i.isVmess = true
	i.vmess = m
	i.Host = host
	i.Port = port
	i.Name = name
	i.Transport = strings.ToLower(netw)
	if i.Transport == "" {
		i.Transport = "tcp"
	}
	i.TLS = strings.EqualFold(tls, "tls") || strings.EqualFold(tls, "reality")
	i.SNI = sni
	i.HostHdr = hostHdr
	return nil
}

func (i *Info) parseSS(rest string) error {
	// 形式一（SIP002）: ss://base64(method:password)@host:port?plugin=...#name
	// 形式二（旧）     : ss://base64(method:password@host:port)#name
	if at := strings.LastIndex(rest, "@"); at > 0 {
		head := rest[:at]
		tail := rest[at+1:]
		if u, err := decodeBase64Loose(head); err == nil && strings.Contains(string(u), ":") {
			hostport, query, frag := splitHostPortQueryFrag(tail)
			host, port, err := splitHostPort(hostport)
			if err != nil {
				return err
			}
			i.userinfo = strings.TrimSpace(head)
			i.rawQuery = query
			i.Host, i.Port = host, port
			i.Name = decodeFragment(frag)
			i.Transport = "tcp"
			return nil
		}
	}
	// 旧格式
	body, frag := splitFragment(rest)
	query := ""
	if k := strings.IndexByte(body, '?'); k >= 0 {
		body, query = body[:k], body[k+1:]
	}
	decoded, err := decodeBase64Loose(body)
	if err != nil {
		return fmt.Errorf("ss 链接解码失败: %w", err)
	}
	s := string(decoded)
	at := strings.LastIndex(s, "@")
	if at < 0 {
		return fmt.Errorf("ss 链接缺少服务器地址")
	}
	cred := s[:at]
	hostport := s[at+1:]
	host, port, err := splitHostPort(hostport)
	if err != nil {
		return err
	}
	method, pass := "", ""
	if k := strings.IndexByte(cred, ':'); k >= 0 {
		method, pass = cred[:k], cred[k+1:]
	}
	i.userinfo = base64.StdEncoding.EncodeToString([]byte(method + ":" + pass))
	i.legacySS = &ssLegacy{method: method, pass: pass}
	i.rawQuery = query
	i.Host, i.Port = host, port
	i.Name = decodeFragment(frag)
	i.Transport = "tcp"
	return nil
}

func (i *Info) parseURI(rest string) error {
	frag := ""
	if k := strings.IndexByte(rest, '#'); k >= 0 {
		rest, frag = rest[:k], rest[k+1:]
	}
	query := ""
	if k := strings.IndexByte(rest, '?'); k >= 0 {
		rest, query = rest[:k], rest[k+1:]
	}
	authority := rest
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		i.userinfo = rest[:at]
		authority = rest[at+1:]
	}
	host, port, err := splitHostPort(authority)
	if err != nil {
		return err
	}
	i.Host, i.Port = host, port
	i.Name = decodeFragment(frag)
	i.rawQuery = query

	vals, _ := url.ParseQuery(query)
	get := func(keys ...string) string {
		for _, k := range keys {
			if v := vals.Get(k); v != "" {
				return v
			}
		}
		return ""
	}
	i.Transport = strings.ToLower(get("type", "net", "headerType"))
	if i.Transport == "" {
		i.Transport = "tcp"
	}
	sec := strings.ToLower(get("security", "tls"))
	i.TLS = sec == "tls" || sec == "reality" || sec == "xtls"
	i.SNI = get("sni", "peer", "servername")
	i.HostHdr = get("host")

	switch i.Scheme {
	case "trojan", "trojan-go":
		i.TLS = true // trojan 默认 TLS
	case "hysteria2", "hy2", "hysteria", "tuic":
		i.TLS = true
	}
	return nil
}

// Rewrite 返回把地址端口替换为中转机后的新链接。
// sni / hostHeader 非空时覆盖原值（用于手动纠正）。
func (i *Info) Rewrite(host string, port int, name, sni, hostHeader string) (string, error) {
	if host == "" || port <= 0 || port > 65535 {
		return "", fmt.Errorf("无效的中转地址 %s:%d", host, port)
	}
	if i.isVmess {
		return i.rewriteVmess(host, port, name, sni, hostHeader)
	}
	return i.rewriteURI(host, port, name, sni, hostHeader)
}

func (i *Info) rewriteVmess(host string, port int, name, sni, hostHeader string) (string, error) {
	m := map[string]any{}
	for k, v := range i.vmess {
		m[k] = v
	}
	origHost := i.Host
	m["add"] = host
	m["port"] = strconv.Itoa(port)
	if name != "" {
		m["ps"] = name
	}
	if i.TLS {
		if sni == "" {
			sni = i.SNI
		}
		if sni == "" {
			sni = origHost
		}
		m["sni"] = sni
		if isWS(i.Transport) {
			if hostHeader == "" {
				hostHeader = i.HostHdr
			}
			if hostHeader == "" {
				hostHeader = origHost
			}
			m["host"] = hostHeader
		}
	}
	// 稳定输出
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteByte('{')
	for idx, k := range keys {
		if idx > 0 {
			sb.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		vb, err := json.Marshal(m[k])
		if err != nil {
			return "", err
		}
		sb.Write(kb)
		sb.WriteByte(':')
		sb.Write(vb)
	}
	sb.WriteByte('}')
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(sb.String())), nil
}

func (i *Info) rewriteURI(host string, port int, name, sni, hostHeader string) (string, error) {
	newAuthority := joinHostPort(host, port)
	query := i.rawQuery
	add := func(k, v string) {
		if v == "" {
			return
		}
		if query != "" {
			query += "&"
		}
		query += url.QueryEscape(k) + "=" + url.QueryEscape(v)
	}

	vals, _ := url.ParseQuery(i.rawQuery)
	has := func(keys ...string) bool {
		for _, k := range keys {
			if vals.Get(k) != "" {
				return true
			}
		}
		return false
	}

	if i.TLS || i.Scheme == "trojan" || i.Scheme == "hysteria2" || i.Scheme == "hy2" || i.Scheme == "tuic" {
		if sni != "" {
			add("sni", sni)
		} else if !has("sni", "peer", "servername") {
			add("sni", i.Host)
		}
	}
	if isWS(i.Transport) || i.Transport == "http" {
		if hostHeader != "" {
			add("host", hostHeader)
		} else if !has("host") {
			add("host", i.Host)
		}
	}

	var sb strings.Builder
	sb.WriteString(i.rawScheme)
	sb.WriteString("://")
	if i.userinfo != "" {
		sb.WriteString(i.userinfo)
		sb.WriteByte('@')
	}
	sb.WriteString(newAuthority)
	if query != "" {
		sb.WriteByte('?')
		sb.WriteString(query)
	}
	if name != "" {
		sb.WriteByte('#')
		sb.WriteString(encodeFragment(name))
	} else if i.Name != "" {
		sb.WriteByte('#')
		sb.WriteString(encodeFragment(i.Name))
	}
	return sb.String(), nil
}

// ---------- 工具函数 ----------

func isWS(transport string) bool {
	return transport == "ws" || transport == "websocket" || transport == "httpupgrade" || transport == "mws"
}

func toInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	}
	return 0
}

func splitHostPort(s string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		// 可能缺少端口
		return s, 0, fmt.Errorf("解析 %q 失败: %w", s, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return strings.Trim(host, "[]"), port, nil
}

func joinHostPort(host string, port int) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return fmt.Sprintf("[%s]:%d", host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func splitFragment(s string) (body, frag string) {
	if k := strings.IndexByte(s, '#'); k >= 0 {
		return s[:k], s[k+1:]
	}
	return s, ""
}

func splitHostPortQueryFrag(s string) (hostport, query, frag string) {
	var body string
	body, frag = splitFragment(s)
	body, query = func(b string) (string, string) {
		if k := strings.IndexByte(b, '?'); k >= 0 {
			return b[:k], b[k+1:]
		}
		return b, ""
	}(body)
	return body, query, frag
}

func decodeFragment(f string) string {
	if f == "" {
		return ""
	}
	if v, err := url.QueryUnescape(f); err == nil {
		return v
	}
	return f
}

func encodeFragment(name string) string {
	e := url.QueryEscape(name)
	return strings.ReplaceAll(e, "+", "%20")
}

func decodeBase64Loose(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("\n", "", "\r", "", "\t", "").Replace(s)
	candidates := []string{s}
	if strings.Contains(s, " ") {
		candidates = append(candidates, strings.ReplaceAll(s, " ", "+"))
	}
	var lastErr error
	for _, cand := range candidates {
		trimmed := strings.TrimRight(cand, "=")
		for _, enc := range []*base64.Encoding{
			base64.StdEncoding, base64.RawStdEncoding,
			base64.URLEncoding, base64.RawURLEncoding,
		} {
			if b, err := enc.DecodeString(trimmed); err == nil {
				return b, nil
			} else {
				lastErr = err
			}
		}
	}
	return nil, lastErr
}

// DisplayScheme 返回用于界面展示的协议名。
func (i *Info) DisplayScheme() string {
	switch i.Scheme {
	case "hy2":
		return "hysteria2"
	case "ss":
		return "shadowsocks"
	}
	return i.Scheme
}
