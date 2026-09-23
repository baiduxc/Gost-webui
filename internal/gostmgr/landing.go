package gostmgr

import (
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"

	"gost-webui/internal/model"
)

// BuildLandingSS 生成落地机上运行的 GOST Shadowsocks 服务配置与启动命令。
//
// GOST v3 的 ss 服务约定：加密方式（cipher）放在 handler.auth.username，
// 密码放在 handler.auth.password；TCP 与 UDP 是两个相互独立的服务
// （TCP 用 ss + listener tcp，UDP 用 ssu + listener udp）。
//
// 中转机对本服务始终是纯 TCP/UDP 透传，客户端的 ss 握手端到端直达落地机。
func BuildLandingSS(id, cipher, password string, port int, udp bool) (yaml string, runCmd string) {
	yaml = "services:\n"
	yaml += fmt.Sprintf("- name: ss-%s\n", id)
	yaml += fmt.Sprintf("  addr: \":%d\"\n", port)
	yaml += "  handler:\n"
	yaml += "    type: ss\n"
	yaml += "    auth:\n"
	yaml += fmt.Sprintf("      username: %s\n", cipher)
	yaml += fmt.Sprintf("      password: %q\n", password)
	yaml += "  listener:\n"
	yaml += "    type: tcp\n"

	runCmd = fmt.Sprintf("gost -L \"ss://%s:%s@:%d\"", cipher, password, port)

	if udp {
		yaml += fmt.Sprintf("- name: ssu-%s\n", id)
		yaml += fmt.Sprintf("  addr: \":%d\"\n", port)
		yaml += "  handler:\n"
		yaml += "    type: ssu\n"
		yaml += "    auth:\n"
		yaml += fmt.Sprintf("      username: %s\n", cipher)
		yaml += fmt.Sprintf("      password: %q\n", password)
		yaml += "  listener:\n"
		yaml += "    type: udp\n"

		runCmd += fmt.Sprintf(" -L \"ssu://%s:%s@:%d\"", cipher, password, port)
	}
	return yaml, runCmd
}

// BuildLanding 生成远程落地机的完整 GOST 配置与临时启动命令。
// 它与本机落地共用同一套服务生成器，避免两种部署方式出现协议差异。
func BuildLanding(n *model.Node) (yamlText string, runCmd string, err error) {
	if n == nil {
		return "", "", fmt.Errorf("节点为空")
	}
	clone := *n
	clone.GostLocal = true // 远程部署本身就是在目标机本地监听。
	if err := ValidateGostNode(&clone); err != nil {
		return "", "", err
	}
	port := n.TargetPort
	if port <= 0 {
		port = n.ListenPort
	}
	conf := struct {
		Services []*Service `yaml:"services"`
	}{Services: BuildGostProxyServices(&clone, port)}
	var sb strings.Builder
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(conf); err != nil {
		return "", "", err
	}
	_ = enc.Close()

	raw, err := GostClientURL(&clone, "", port)
	if err != nil {
		return "", "", err
	}
	if u, parseErr := url.Parse(raw); parseErr == nil {
		u.Fragment = ""
		raw = u.String()
	}
	return sb.String(), fmt.Sprintf("gost -L %q", raw), nil
}

// GostReleaseVersion 是落地机部署命令中固定使用的 GOST 发行版本。
const GostReleaseVersion = "3.3.0"

// InstallBinaryCmd 返回在落地机下载并安装 GOST 预编译二进制的命令（自动识别 CPU 架构）。
func InstallBinaryCmd() string {
	return fmt.Sprintf(`# 自动识别架构，下载并安装 GOST v%s 预编译二进制
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) A=amd64 ;;
  aarch64|arm64) A=arm64 ;;
  armv7l) A=armv7 ;;
  *) echo "未识别的架构: $ARCH"; exit 1 ;;
esac
curl -L -o gost.tar.gz "https://github.com/go-gost/gost/releases/download/v%s/gost_%s_linux_${A}.tar.gz"
tar -xzf gost.tar.gz gost
sudo install -m 755 gost /usr/local/bin/gost
rm -f gost gost.tar.gz
gost -V`, GostReleaseVersion, GostReleaseVersion, GostReleaseVersion)
}

// InstallScriptCmd 返回使用 GOST 官方安装脚本一键安装的命令。
func InstallScriptCmd() string {
	return "bash <(curl -fsSL https://github.com/go-gost/gost/raw/master/install.sh) --install"
}

// BuildLandingService 生成把落地机 GOST 服务注册为 systemd 后台服务的命令，
// 实现自动后台运行与开机自启。返回服务名与完整命令。
func BuildLandingService(id, confPath string) (serviceName, cmd string) {
	serviceName = "gost-" + id
	unitPath := "/etc/systemd/system/" + serviceName + ".service"
	cmd = fmt.Sprintf(`sudo tee %[1]s >/dev/null <<'EOF'
[Unit]
Description=GOST proxy service (node %[2]s)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/gost -C %[3]s
Restart=always
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload
sudo systemctl enable --now %[4]s`, unitPath, id, confPath, serviceName)
	return serviceName, cmd
}
