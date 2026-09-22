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
	saveCmd := fmt.Sprintf("mkdir -p /etc/gost && cat > %s <<'EOF'\n%sEOF", confPath, yaml)
	startFile := fmt.Sprintf("gost -C %s", confPath)

	steps := []string{
		"1. 在落地机安装 GOST（版本需 ≥ 3.3），已安装可跳过。",
		"2. 方式一：直接运行下方「启动命令」前台跑起来；方式二：运行「保存配置」命令写入 " + confPath + "，再用 gost -C 常驻（推荐配合 systemd）。",
		fmt.Sprintf("3. 在落地机放行防火墙 / 云安全组的 %s 端口。", proto),
		"4. 回到面板确认该节点已启用，客户端导入面板生成的 ss:// 链接（地址已指向中转机）即可使用。",
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"config":      yaml,
		"runCommand":  runCmd,
		"saveCommand": saveCmd,
		"startFile":   startFile,
		"confPath":    confPath,
		"steps":       steps,
		"cipher":      node.GostCipher,
		"password":    node.GostPassword,
		"host":        node.TargetHost,
		"port":        node.TargetPort,
		"udp":         node.UDP,
	})
}
