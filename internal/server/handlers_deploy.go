package server

import (
	"fmt"
	"net/http"
	"strconv"

	"gost-webui/internal/gostmgr"
)

// handleNodeDeploy 返回 GOST 体系节点在落地机上的一键部署内容：
// ss 服务配置、启动命令与操作步骤。中转机侧仍是纯 TCP 透传，无需改动。
func (s *Server) handleNodeDeploy(w http.ResponseWriter, r *http.Request) {
	node := s.ctl.Node(r.PathValue("id"))
	if node == nil {
		writeErr(w, http.StatusNotFound, "节点不存在")
		return
	}
	if node.Mode != "gost" {
		writeErr(w, http.StatusBadRequest, "仅 GOST 体系节点提供部署命令")
		return
	}

	yaml, runCmd := gostmgr.BuildLandingSS(node.ID, node.GostCipher, node.GostPassword, node.TargetPort, node.UDP)
	port := strconv.Itoa(node.TargetPort)

	proto := port + "/tcp"
	if node.UDP {
		proto += " 与 " + port + "/udp"
	}

	confPath := fmt.Sprintf("/etc/gost/ss-%s.yml", node.ID)
	saveCmd := fmt.Sprintf("sudo mkdir -p /etc/gost && sudo tee %s >/dev/null <<'EOF'\n%sEOF", confPath, yaml)
	serviceName, serviceCmd := gostmgr.BuildLandingService(node.ID, confPath)

	steps := []string{
		"1. 在落地机安装 GOST（版本需 ≥ 3.3）：任选「下载二进制」或「官方脚本」一种，已安装可跳过。",
		"2. 运行「保存配置」命令，把 ss 服务配置写入 " + confPath + "。",
		"3. 运行「后台服务」命令，注册 systemd 服务 " + serviceName + "，实现自动后台运行与开机自启。",
		fmt.Sprintf("4. 在落地机放行防火墙 / 云安全组的 %s 端口。", proto),
		"5. 回到面板确认该节点已启用，客户端导入面板生成的 ss:// 链接（地址已指向中转机）即可使用。",
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"config":           yaml,
		"installBinaryCmd": gostmgr.InstallBinaryCmd(),
		"installScriptCmd": gostmgr.InstallScriptCmd(),
		"saveCommand":      saveCmd,
		"serviceCommand":   serviceCmd,
		"serviceName":      serviceName,
		"runCommand":       runCmd,
		"confPath":         confPath,
		"steps":            steps,
		"cipher":           node.GostCipher,
		"password":         node.GostPassword,
		"host":             node.TargetHost,
		"port":             node.TargetPort,
		"udp":              node.UDP,
	})
}
