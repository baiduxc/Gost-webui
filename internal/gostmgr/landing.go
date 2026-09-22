package gostmgr

import "fmt"

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
