package server

import (
	"fmt"
	"net/http"
	"strconv"

	"gost-webui/internal/gostmgr"
)

// handleNodeDeploy 返回 GOST 体系节点的运行状态或远程落地一键部署内容。
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

	yaml, runCmd, err := gostmgr.BuildLanding(node)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	protocol := gostmgr.NodeGostProtocol(node)
	transport := gostmgr.NodeGostTransport(node)
	portNum := node.TargetPort
	if node.GostLocal {
		portNum = node.ListenPort
	}
	port := strconv.Itoa(portNum)
	transportSpec, _ := gostmgr.TransportSpec(transport)

	network := transportSpec.Network
	if network == "raw" {
		network = "原始网络报文（需要 root/CAP_NET_RAW）"
	}
	proto := port + "/" + network
	if node.UDP {
		proto += " 与 " + port + "/udp"
	}

	confPath := fmt.Sprintf("/etc/gost/node-%s.yml", node.ID)
	saveCmd := fmt.Sprintf("sudo mkdir -p /etc/gost && sudo tee %s >/dev/null <<'EOF'\n%sEOF", confPath, yaml)
	serviceName, serviceCmd := gostmgr.BuildLandingService(node.ID, confPath)

	steps := []string{}
	if node.GostLocal {
		steps = []string{
			"该节点已由本面板管理的 GOST 进程直接运行，无需执行额外部署命令。",
			fmt.Sprintf("在当前服务器防火墙 / 云安全组放行 %s。", proto),
			"客户端复制面板生成的地址；使用非标准传输通道时请导入 GOST 客户端。",
		}
	} else {
		steps = []string{
			"1. 在远程落地机安装 GOST（版本需 ≥ 3.3）：任选「下载二进制」或「官方脚本」一种，已安装可跳过。",
			"2. 运行「保存配置」命令，把代理服务配置写入 " + confPath + "。",
			"3. 运行「后台服务」命令，注册 systemd 服务 " + serviceName + "，实现自动后台运行与开机自启。",
			fmt.Sprintf("4. 在远程落地机放行防火墙 / 云安全组的 %s 端口。", proto),
			"5. 回到面板确认节点已启用，客户端导入面板生成的地址即可使用。",
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"managed":          node.GostLocal,
		"config":           yaml,
		"installBinaryCmd": gostmgr.InstallBinaryCmd(),
		"installScriptCmd": gostmgr.InstallScriptCmd(),
		"saveCommand":      saveCmd,
		"serviceCommand":   serviceCmd,
		"serviceName":      serviceName,
		"runCommand":       runCmd,
		"confPath":         confPath,
		"steps":            steps,
		"protocol":         protocol,
		"transport":        transport,
		"username":         node.GostUsername,
		"cipher":           node.GostCipher,
		"password":         node.GostPassword,
		"host":             node.TargetHost,
		"port":             portNum,
		"network":          transportSpec.Network,
		"udp":              node.UDP,
	})
}
