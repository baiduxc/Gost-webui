// Package gostmgr 负责生成 gost 配置、守护 gost 进程，并通过 REST API 控制它。
package gostmgr

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"gost-webui/internal/model"
)

// ---------- gost 配置结构（与 go-gost/x/config 对齐） ----------

type Config struct {
	Services  []*Service `yaml:"services" json:"services"`
	Quotas    []*Quota   `yaml:"quotas,omitempty" json:"quotas,omitempty"`
	Limiters  []*Limiter `yaml:"limiters,omitempty" json:"limiters,omitempty"`
	CLimiters []*Limiter `yaml:"climiters,omitempty" json:"climiters,omitempty"`
	API       *API       `yaml:"api,omitempty" json:"api,omitempty"`
	Log       *Log       `yaml:"log,omitempty" json:"log,omitempty"`
}

type Service struct {
	Name      string         `yaml:"name" json:"name"`
	Addr      string         `yaml:"addr,omitempty" json:"addr,omitempty"`
	Quotas    []string       `yaml:"quotas,omitempty" json:"quotas,omitempty"`
	Limiter   string         `yaml:"limiter,omitempty" json:"limiter,omitempty"`
	CLimiter  string         `yaml:"climiter,omitempty" json:"climiter,omitempty"`
	Handler   *Handler       `yaml:"handler,omitempty" json:"handler,omitempty"`
	Listener  *Listener      `yaml:"listener,omitempty" json:"listener,omitempty"`
	Forwarder *Forwarder     `yaml:"forwarder,omitempty" json:"forwarder,omitempty"`
	Metadata  map[string]any `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	Status    *Status        `yaml:"-" json:"status,omitempty"`
}

type Handler struct {
	Type     string         `yaml:"type" json:"type"`
	Auth     *Auth          `yaml:"auth,omitempty" json:"auth,omitempty"`
	Metadata map[string]any `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

type Listener struct {
	Type     string         `yaml:"type" json:"type"`
	Auth     *Auth          `yaml:"auth,omitempty" json:"auth,omitempty"`
	Metadata map[string]any `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

type Forwarder struct {
	Nodes []*ForwardNode `yaml:"nodes,omitempty" json:"nodes,omitempty"`
}

type ForwardNode struct {
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	Addr string `yaml:"addr" json:"addr"`
}

type Quota struct {
	Name      string       `yaml:"name" json:"name"`
	Limit     string       `yaml:"limit,omitempty" json:"limit,omitempty"`
	StartsAt  string       `yaml:"startsAt,omitempty" json:"startsAt,omitempty"`
	ExpiresAt string       `yaml:"expiresAt,omitempty" json:"expiresAt,omitempty"`
	Direction string       `yaml:"direction,omitempty" json:"direction,omitempty"`
	Flush     string       `yaml:"flush,omitempty" json:"flush,omitempty"`
	Store     *QuotaStore  `yaml:"store,omitempty" json:"store,omitempty"`
	Status    *QuotaStatus `yaml:"-" json:"status,omitempty"`
}

type QuotaStore struct {
	Type string `yaml:"type,omitempty" json:"type,omitempty"`
	File string `yaml:"file,omitempty" json:"file,omitempty"`
}

type QuotaStatus struct {
	Used      uint64 `yaml:"used" json:"used"`
	Limit     uint64 `yaml:"limit" json:"limit"`
	StartsAt  int64  `yaml:"startsAt,omitempty" json:"startsAt,omitempty"`
	ExpiresAt int64  `yaml:"expiresAt,omitempty" json:"expiresAt,omitempty"`
	Active    bool   `yaml:"active" json:"active"`
	Expired   bool   `yaml:"expired,omitempty" json:"expired"`
	Blocked   bool   `yaml:"blocked" json:"blocked"`
	Direction string `yaml:"direction,omitempty" json:"direction,omitempty"`
}

type Limiter struct {
	Name   string   `yaml:"name" json:"name"`
	Limits []string `yaml:"limits,omitempty" json:"limits,omitempty"`
}

type API struct {
	Addr       string `yaml:"addr" json:"addr"`
	PathPrefix string `yaml:"pathPrefix,omitempty" json:"pathPrefix,omitempty"`
	Auth       *Auth  `yaml:"auth,omitempty" json:"auth,omitempty"`
}

type Auth struct {
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
}

type Log struct {
	Output string `yaml:"output,omitempty" json:"output,omitempty"`
	Level  string `yaml:"level,omitempty" json:"level,omitempty"`
	Format string `yaml:"format,omitempty" json:"format,omitempty"`
}

// Status 是 gost 返回的服务运行状态。
type Status struct {
	State string `json:"state"`
	Stats *Stats `json:"stats,omitempty"`
}

type Stats struct {
	TotalConns   uint64 `json:"totalConns"`
	CurrentConns uint64 `json:"currentConns"`
	TotalErrs    uint64 `json:"totalErrs"`
	InputBytes   uint64 `json:"inputBytes"`
	OutputBytes  uint64 `json:"outputBytes"`
}

// BuildOptions 生成配置所需的运行参数。
type BuildOptions struct {
	APIAddr   string
	APIUser   string
	APIPass   string
	QuotaFile string
	LogLevel  string
}

// BuildConfig 根据节点列表生成完整的 gost 配置。
func BuildConfig(nodes []*model.Node, opts BuildOptions) *Config {
	cfg := &Config{
		Services: []*Service{},
		API: &API{
			Addr: opts.APIAddr,
			Auth: &Auth{Username: opts.APIUser, Password: opts.APIPass},
		},
		Log: &Log{Output: "stderr", Level: opts.LogLevel, Format: "text"},
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}

	for _, n := range nodes {
		if n == nil || !n.Enabled {
			continue
		}
		var quotas []string
		if n.Quota.Enabled && n.Quota.Bytes > 0 {
			qname := model.QuotaName(n.ID)
			q := &Quota{
				Name:      qname,
				Limit:     fmt.Sprintf("%dB", n.Quota.Bytes),
				Direction: n.Quota.Direction,
				// 较短的刷盘间隔，尽量降低重启导致计数丢失的窗口
				Flush: "5s",
				Store: &QuotaStore{Type: "file", File: opts.QuotaFile},
			}
			if s, e := QuotaWindow(n.Quota.Period); s != "" || e != "" {
				q.StartsAt, q.ExpiresAt = s, e
			}
			cfg.Quotas = append(cfg.Quotas, q)
			quotas = []string{qname}
		}

		var limiter, climiter string
		if n.Rate.Enabled && (n.Rate.InBps > 0 || n.Rate.OutBps > 0) {
			limiter = model.LimiterName(n.ID)
			cfg.Limiters = append(cfg.Limiters, &Limiter{
				Name:   limiter,
				Limits: []string{fmt.Sprintf("$ %d %d", n.Rate.InBps, n.Rate.OutBps)},
			})
		}
		if n.ConnLimit > 0 {
			climiter = model.CLimiterName(n.ID)
			cfg.CLimiters = append(cfg.CLimiters, &Limiter{
				Name:   climiter,
				Limits: []string{fmt.Sprintf("$ %d", n.ConnLimit)},
			})
		}

		var services []*Service
		if n.Mode == "gost" && n.GostLocal {
			services = BuildGostProxyServices(n, n.ListenPort)
		} else {
			target := fmt.Sprintf("%s:%d", n.TargetHost, n.TargetPort)
			network := "tcp"
			if n.Mode == "gost" {
				if t, ok := TransportSpec(NodeGostTransport(n)); ok && t.Network == "udp" {
					network = "udp"
				}
			}
			services = append(services, relayService(model.TCPName(n.ID), n.ListenPort, network, target))
			if n.UDP && network == "tcp" {
				services = append(services, relayService(model.UDPName(n.ID), n.ListenPort, "udp", target))
			}
		}

		for _, service := range services {
			service.Quotas = quotas
			service.Limiter = limiter
			service.CLimiter = climiter
			if service.Metadata == nil {
				service.Metadata = map[string]any{}
			}
			service.Metadata["enableStats"] = true
			cfg.Services = append(cfg.Services, service)
		}
	}
	return cfg
}

func relayService(name string, port int, network, target string) *Service {
	return &Service{
		Name:      name,
		Addr:      fmt.Sprintf(":%d", port),
		Handler:   &Handler{Type: network},
		Listener:  &Listener{Type: network},
		Forwarder: &Forwarder{Nodes: []*ForwardNode{{Name: "target", Addr: target}}},
	}
}

func nodeAuth(n *model.Node, protocol string) *Auth {
	p, _ := ProtocolSpec(protocol)
	switch p.Auth {
	case "ss":
		return &Auth{Username: n.GostCipher, Password: n.GostPassword}
	case "user":
		return &Auth{Username: n.GostUsername}
	case "userpass":
		return &Auth{Username: n.GostUsername, Password: n.GostPassword}
	default:
		return nil
	}
}

// BuildGostProxyServices 生成由 GOST 直接承载的本机落地服务。
// 返回的服务尚未附加面板配额、限速和连接数限制，便于落地机部署配置复用。
func BuildGostProxyServices(n *model.Node, port int) []*Service {
	protocol, transport := NodeGostProtocol(n), NodeGostTransport(n)
	auth := nodeAuth(n, protocol)
	handler := &Handler{Type: protocol, Auth: auth}
	listener := &Listener{Type: transport}
	if transport == "dtls" {
		// GOST 3.3.0 将未设置的 DTLS flight interval 作为零值传给 pion/dtls，
		// 后者会拒绝启动；为服务端和客户端提供一个明确的兼容默认值。
		listener.Metadata = map[string]any{"flightInterval": "500ms", "mtu": 1200}
	}

	// SSH 通道在 GOST 中由 listener 认证；URL 中的一组凭据也会被客户端交给 dialer。
	if transport == "ssh" || transport == "sshd" {
		handler.Auth = nil
		listener.Auth = &Auth{Username: n.GostUsername, Password: n.GostPassword}
	}
	if t, ok := TransportSpec(transport); ok && t.Path && strings.TrimSpace(n.GostPath) != "" {
		if listener.Metadata == nil {
			listener.Metadata = map[string]any{}
		}
		listener.Metadata["path"] = strings.TrimSpace(n.GostPath)
	}

	services := []*Service{{
		Name:     model.TCPName(n.ID),
		Addr:     fmt.Sprintf(":%d", port),
		Handler:  handler,
		Listener: listener,
	}}
	if n.UDP && protocol == "ss" {
		services = append(services, &Service{
			Name:     model.UDPName(n.ID),
			Addr:     fmt.Sprintf(":%d", port),
			Handler:  &Handler{Type: "ssu", Auth: nodeAuth(n, protocol)},
			Listener: &Listener{Type: "udp"},
		})
	}
	return services
}

// Marshal 生成 YAML 文本。
func (c *Config) Marshal() ([]byte, error) {
	var sb strings.Builder
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return []byte(sb.String()), nil
}
