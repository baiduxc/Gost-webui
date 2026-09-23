package gostmgr

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"gost-webui/internal/model"
)

// GostProtocol 描述可作为公网落地代理的 GOST 数据处理协议。
type GostProtocol struct {
	Value            string `json:"value"`
	Label            string `json:"label"`
	DefaultTransport string `json:"defaultTransport"`
	Auth             string `json:"auth"` // userpass / user / ss / none
}

// GostTransport 描述可承载代理协议的 GOST 网络通道。
type GostTransport struct {
	Value      string `json:"value"`
	Label      string `json:"label"`
	Network    string `json:"network"` // tcp / udp / raw
	Path       bool   `json:"path,omitempty"`
	LocalOnly  bool   `json:"localOnly,omitempty"`
	Privileged bool   `json:"privileged,omitempty"`
}

// GostProtocols 包含 GOST v3 官方“代理协议”分类中的全部服务协议。
// socks 是 socks5 的别名，因此界面只保留含义明确的 socks5。
var GostProtocols = []GostProtocol{
	{Value: "http", Label: "HTTP", DefaultTransport: "tcp", Auth: "userpass"},
	{Value: "http2", Label: "HTTP/2", DefaultTransport: "http2", Auth: "userpass"},
	{Value: "socks4", Label: "SOCKS4", DefaultTransport: "tcp", Auth: "user"},
	{Value: "socks4a", Label: "SOCKS4A", DefaultTransport: "tcp", Auth: "user"},
	{Value: "socks5", Label: "SOCKS5", DefaultTransport: "tcp", Auth: "userpass"},
	{Value: "ss", Label: "Shadowsocks TCP", DefaultTransport: "tcp", Auth: "ss"},
	{Value: "ssu", Label: "Shadowsocks UDP", DefaultTransport: "udp", Auth: "ss"},
	{Value: "sni", Label: "SNI 透明代理", DefaultTransport: "tcp", Auth: "none"},
	{Value: "relay", Label: "GOST Relay", DefaultTransport: "tcp", Auth: "userpass"},
}

// GostTransports 包含 GOST v3 面向网络代理的全部官方数据通道。
// unix/serial/tun/tap 等本地设备监听器不是公网落地通道，不在此列表中。
var GostTransports = []GostTransport{
	{Value: "tcp", Label: "TCP", Network: "tcp"},
	{Value: "mtcp", Label: "Multiplex TCP", Network: "tcp"},
	{Value: "udp", Label: "UDP", Network: "udp"},
	{Value: "tls", Label: "TLS", Network: "tcp"},
	{Value: "dtls", Label: "DTLS（客户端需证书）", Network: "udp"},
	{Value: "mtls", Label: "Multiplex TLS", Network: "tcp"},
	{Value: "ws", Label: "WebSocket", Network: "tcp", Path: true},
	{Value: "wss", Label: "WebSocket TLS", Network: "tcp", Path: true},
	{Value: "mws", Label: "Multiplex WebSocket", Network: "tcp", Path: true},
	{Value: "mwss", Label: "Multiplex WebSocket TLS", Network: "tcp", Path: true},
	{Value: "h2", Label: "HTTP/2 TLS", Network: "tcp", Path: true},
	{Value: "h2c", Label: "HTTP/2 Cleartext", Network: "tcp", Path: true},
	{Value: "http2", Label: "HTTP/2 Proxy Channel", Network: "tcp"},
	{Value: "grpc", Label: "gRPC", Network: "tcp", Path: true},
	{Value: "pht", Label: "HTTP Tunnel", Network: "tcp"},
	{Value: "phts", Label: "HTTPS Tunnel", Network: "tcp"},
	{Value: "ssh", Label: "SSH", Network: "tcp"},
	{Value: "sshd", Label: "SSHD", Network: "tcp"},
	{Value: "kcp", Label: "KCP", Network: "udp"},
	{Value: "quic", Label: "QUIC", Network: "udp"},
	{Value: "h3", Label: "HTTP/3", Network: "udp"},
	{Value: "http3", Label: "HTTP/3 Proxy Channel", Network: "udp"},
	{Value: "wt", Label: "WebTransport", Network: "udp"},
	{Value: "ohttp", Label: "HTTP Obfuscation", Network: "tcp"},
	{Value: "otls", Label: "TLS Obfuscation", Network: "tcp"},
	{Value: "icmp", Label: "ICMPv4", Network: "raw", LocalOnly: true, Privileged: true},
	{Value: "icmp6", Label: "ICMPv6", Network: "raw", LocalOnly: true, Privileged: true},
	{Value: "ftcp", Label: "Fake TCP", Network: "raw", LocalOnly: true, Privileged: true},
}

func protocolSpec(value string) (GostProtocol, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, p := range GostProtocols {
		if p.Value == value {
			return p, true
		}
	}
	return GostProtocol{}, false
}

func transportSpec(value string) (GostTransport, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, t := range GostTransports {
		if t.Value == value {
			return t, true
		}
	}
	return GostTransport{}, false
}

// ProtocolSpec 返回协议定义。
func ProtocolSpec(value string) (GostProtocol, bool) { return protocolSpec(value) }

// TransportSpec 返回传输通道定义。
func TransportSpec(value string) (GostTransport, bool) { return transportSpec(value) }

// NodeGostProtocol/NodeGostTransport 为旧版 SS 节点补上兼容默认值。
func NodeGostProtocol(n *model.Node) string {
	if n == nil || strings.TrimSpace(n.GostProtocol) == "" {
		return "ss"
	}
	return strings.ToLower(strings.TrimSpace(n.GostProtocol))
}

func NodeGostTransport(n *model.Node) string {
	if n != nil && strings.TrimSpace(n.GostTransport) != "" {
		return strings.ToLower(strings.TrimSpace(n.GostTransport))
	}
	if p, ok := protocolSpec(NodeGostProtocol(n)); ok {
		return p.DefaultTransport
	}
	return "tcp"
}

// ValidateGostNode 验证代理协议与传输通道组合是否能作为本项目的落地节点。
func ValidateGostNode(n *model.Node) error {
	if n == nil {
		return fmt.Errorf("节点为空")
	}
	p, ok := protocolSpec(NodeGostProtocol(n))
	if !ok {
		return fmt.Errorf("不支持的 GOST 代理协议: %s", n.GostProtocol)
	}
	t, ok := transportSpec(NodeGostTransport(n))
	if !ok {
		return fmt.Errorf("不支持的 GOST 传输通道: %s", n.GostTransport)
	}
	if t.LocalOnly && !n.GostLocal {
		return fmt.Errorf("%s 通道使用原始网络报文，只能作为本机落地", t.Label)
	}
	if t.Value == "udp" && p.Value != "ssu" && p.Value != "relay" {
		return fmt.Errorf("UDP 通道仅支持 Shadowsocks UDP 或 GOST Relay")
	}
	if p.Value == "ssu" && t.Value != "udp" {
		return fmt.Errorf("Shadowsocks UDP 必须使用 UDP 通道")
	}
	if p.Value == "http2" && t.Value != "http2" {
		return fmt.Errorf("HTTP/2 代理协议必须使用 HTTP/2 Proxy Channel")
	}
	if (t.Value == "ssh" || t.Value == "sshd") && (p.Value == "ss" || p.Value == "ssu") {
		return fmt.Errorf("Shadowsocks 不能与 SSH/SSHD 通道共用一组认证信息")
	}
	if n.UDP && p.Value != "ss" {
		return fmt.Errorf("附加 UDP 服务仅适用于 Shadowsocks TCP")
	}
	if n.UDP && t.Network != "tcp" {
		return fmt.Errorf("当前通道已占用 UDP 端口，不能再启用 Shadowsocks UDP 扩展")
	}
	return nil
}

func gostScheme(protocol, transport string) string {
	p, _ := protocolSpec(protocol)
	if transport == "" || transport == p.DefaultTransport {
		return protocol
	}
	return protocol + "+" + transport
}

func gostUser(n *model.Node, protocol, transport string) *url.Userinfo {
	p, _ := protocolSpec(protocol)
	if p.Auth == "ss" {
		cred := base64.RawURLEncoding.EncodeToString([]byte(n.GostCipher + ":" + n.GostPassword))
		return url.User(cred)
	}
	// SSH/SSHD 在 GOST 中由通道层认证，即使处理器本身（例如 SNI）不认证也需要凭据。
	if transport == "ssh" || transport == "sshd" {
		return url.UserPassword(n.GostUsername, n.GostPassword)
	}
	if p.Auth == "none" {
		return nil
	}
	if p.Auth == "user" {
		return url.User(n.GostUsername)
	}
	return url.UserPassword(n.GostUsername, n.GostPassword)
}

// GostClientURL 生成可直接传给 GOST -F 或支持对应标准协议客户端的节点地址。
func GostClientURL(n *model.Node, host string, port int) (string, error) {
	if err := ValidateGostNode(n); err != nil {
		return "", err
	}
	protocol, transport := NodeGostProtocol(n), NodeGostTransport(n)
	u := &url.URL{
		Scheme: gostScheme(protocol, transport),
		Host:   net.JoinHostPort(strings.TrimSpace(host), strconv.Itoa(port)),
		User:   gostUser(n, protocol, transport),
	}
	q := url.Values{}
	if t, _ := transportSpec(transport); t.Path && strings.TrimSpace(n.GostPath) != "" {
		q.Set("path", strings.TrimSpace(n.GostPath))
	}
	if transport == "dtls" {
		q.Set("flightInterval", "500ms")
		q.Set("mtu", "1200")
	}
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}
	if strings.TrimSpace(n.Name) != "" {
		u.Fragment = n.Name
	}
	return u.String(), nil
}
