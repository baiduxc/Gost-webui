// Package sub 生成客户端订阅：通用 base64 链接聚合与 Clash Meta（mihomo）YAML。
//
// 订阅聚合面板内所有「已启用」节点：每个节点先用 link.Rewrite 把落地机链接的
// 地址端口替换为中转机（其余鉴权/传输参数原样保留），再据此生成订阅内容。
package sub

import (
	"encoding/base64"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"gost-webui/internal/link"
	"gost-webui/internal/model"
)

// Result 是一次订阅生成的结果。
type Result struct {
	// Links 是各节点指向中转机的分享链接（已按节点顺序）。
	Links []string
	// Names 是各节点在订阅中使用的名称（与 Links 一一对应）。
	Names []string
	// Skipped 记录被跳过的节点及原因（协议不支持、链接无法解析等）。
	Skipped []string
}

// clientLink 解析并改写单个节点的落地链接，返回指向中转机的链接与解析结果。
func clientLink(n *model.Node, host string) (string, *link.Info, error) {
	if strings.TrimSpace(n.LandingLink) == "" {
		return "", nil, fmt.Errorf("缺少落地链接")
	}
	info, err := link.Parse(n.LandingLink)
	if err != nil {
		return "", nil, err
	}
	out, err := info.Rewrite(host, n.ListenPort, n.Name, n.SNI, n.Host)
	if err != nil {
		return "", nil, err
	}
	return out, info, nil
}

// Collect 遍历节点，生成每个已启用节点的中转链接与名称。
// host 为中转机对外地址（公网 IP / 域名），空则返回错误。
func Collect(nodes []*model.Node, host string) (*Result, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, fmt.Errorf("未配置中转机地址，请到「系统设置 → 服务器地址」填写公网 IP 或域名")
	}
	res := &Result{}
	for _, n := range nodes {
		if n == nil || !n.Enabled {
			continue
		}
		out, _, err := clientLink(n, host)
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s（%v）", n.Name, err))
			continue
		}
		res.Links = append(res.Links, out)
		name := n.Name
		if name == "" {
			name = n.ID
		}
		res.Names = append(res.Names, name)
	}
	return res, nil
}

// Universal 返回通用订阅内容：所有节点链接换行拼接后 base64 编码，
// 兼容 v2rayN / Shadowrocket / Stash 等按「链接列表」解析的客户端。
func Universal(nodes []*model.Node, host string) (string, *Result, error) {
	res, err := Collect(nodes, host)
	if err != nil {
		return "", nil, err
	}
	raw := strings.Join(res.Links, "\n")
	return base64.StdEncoding.EncodeToString([]byte(raw)), res, nil
}

// clashConfig 是 Clash Meta 订阅的顶层结构（字段顺序即输出顺序）。
type clashConfig struct {
	Port         int              `yaml:"port"`
	SocksPort    int              `yaml:"socks-port"`
	MixedPort    int              `yaml:"mixed-port"`
	AllowLan     bool             `yaml:"allow-lan"`
	Mode         string           `yaml:"mode"`
	LogLevel     string           `yaml:"log-level"`
	UnifiedDelay bool             `yaml:"unified-delay"`
	Proxies      []map[string]any `yaml:"proxies"`
	ProxyGroups  []map[string]any `yaml:"proxy-groups"`
	Rules        []string         `yaml:"rules"`
}

// ClashYAML 返回 Clash Meta（mihomo）可导入的 YAML 订阅内容。
// name 为订阅/策略组展示名（如面板名）。
func ClashYAML(nodes []*model.Node, host, name string) (string, *Result, error) {
	res, err := Collect(nodes, host)
	if err != nil {
		return "", nil, err
	}
	if name == "" {
		name = "Gost-WebUI"
	}

	proxies := make([]map[string]any, 0, len(res.Links))
	names := make([]string, 0, len(res.Links))
	for idx, l := range res.Links {
		info, perr := link.Parse(l)
		if perr != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s（%v）", res.Names[idx], perr))
			continue
		}
		proxy, cerr := info.ClashProxy(res.Names[idx])
		if cerr != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s（%v）", res.Names[idx], cerr))
			continue
		}
		proxies = append(proxies, proxy)
		names = append(names, res.Names[idx])
	}

	if len(proxies) == 0 {
		return "", res, fmt.Errorf("没有可用于 Clash 的节点（%s）", strings.Join(res.Skipped, "；"))
	}

	// 去重名称，避免策略组引用冲突
	uniq := uniqueNames(names)

	selectGroup := map[string]any{
		"name":    "节点选择",
		"type":    "select",
		"proxies": append([]string{"自动选择", "DIRECT"}, uniq...),
	}
	urlTest := map[string]any{
		"name":     "自动选择",
		"type":     "url-test",
		"proxies":  uniq,
		"url":      "http://www.gstatic.com/generate_204",
		"interval": 300,
		"tolerance": 50,
	}

	cfg := clashConfig{
		Port:         7890,
		SocksPort:    7891,
		MixedPort:    7893,
		AllowLan:     false,
		Mode:         "rule",
		LogLevel:     "info",
		UnifiedDelay: true,
		Proxies:      proxies,
		ProxyGroups:  []map[string]any{selectGroup, urlTest},
		Rules: []string{
			"GEOIP,LAN,DIRECT",
			"GEOIP,CN,DIRECT",
			"MATCH,节点选择",
		},
	}

	var sb strings.Builder
	sb.WriteString("# " + name + " · Clash Meta 订阅（由 Gost-WebUI 自动生成）\n")
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		return "", res, err
	}
	_ = enc.Close()
	return sb.String(), res, nil
}

// uniqueNames 保持顺序去重。
func uniqueNames(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
