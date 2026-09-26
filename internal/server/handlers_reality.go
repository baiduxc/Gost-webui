package server

import (
	"fmt"
	"net/http"
	"strings"

	"gost-webui/internal/model"
	"gost-webui/internal/singbox"
)

// handleRealityCredential 为 reality 节点生成服务端凭据（密钥对/UUID/ShortID）。
func (s *Server) handleRealityCredential(w http.ResponseWriter, r *http.Request) {
	priv, pub, err := singbox.GenerateRealityKeypair()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "生成密钥对失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"uuid":    singbox.GenerateUUID(),
		"privKey": priv,
		"pubKey":  pub,
		"shortId": singbox.GenerateShortID(),
	})
}

// handleRealityAvailable 报告 sing-box 引擎状态（前端据此决定是否展示 reality 选项）。
func (s *Server) handleRealityAvailable(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"available": s.ctl.RealityAvailable()})
}

// buildRealityNode 处理 VLESS+REALITY 节点（sing-box 引擎，本机监听）。
func (s *Server) buildRealityNode(in *nodeInput, old *model.Node) (*model.Node, error) {
	if !s.ctl.RealityAvailable() {
		return nil, fmt.Errorf("本机未安装 sing-box，无法创建 REALITY 节点（安装后重试）")
	}
	n := newNodeBase(old)
	n.Mode = "reality"
	n.Protocol = "VLESS+REALITY"
	n.GostLocal = true // 复用"本机落地"语义：监听端口即服务端口
	n.UDP = false

	n.RealitySNI = strings.ToLower(strings.TrimSpace(in.RealitySni))
	if !singbox.ValidServerName(n.RealitySNI) {
		return nil, fmt.Errorf("伪装域名无效（应为 www.example.com 形式）")
	}
	n.RealityUUID = strings.TrimSpace(in.RealityUuid)
	n.RealityPriv = strings.TrimSpace(in.RealityPriv)
	n.RealityPub = strings.TrimSpace(in.RealityPub)
	n.RealityShortID = strings.ToLower(strings.TrimSpace(in.RealityShortId))

	// 编辑保留：私钥列表接口不下发，提交为空时沿用旧值
	if old != nil && old.IsReality() {
		if n.RealityPriv == "" {
			n.RealityPriv = old.RealityPriv
		}
		if n.RealityUUID == "" {
			n.RealityUUID = old.RealityUUID
		}
		if n.RealityShortID == "" {
			n.RealityShortID = old.RealityShortID
		}
	}
	if n.RealityPriv != "" && n.RealityPub == "" {
		n.RealityPub = old.RealityPub
	}
	switch {
	case n.RealityUUID == "":
		return nil, fmt.Errorf("缺少 UUID，请点击生成")
	case n.RealityPriv == "" || n.RealityPub == "":
		return nil, fmt.Errorf("缺少 REALITY 密钥对，请点击生成")
	case n.RealityShortID == "":
		return nil, fmt.Errorf("缺少 ShortID，请点击生成")
	}
	if n.RealitySNI == "" {
		return nil, fmt.Errorf("请填写伪装域名")
	}

	n.Name = strings.TrimSpace(in.Name)
	if n.Name == "" {
		n.Name = "REALITY-" + n.RealitySNI
	}
	n.TargetHost = ""
	n.TargetPort = 0
	if err := s.applyCommon(n, in, old, false); err != nil {
		return nil, err
	}
	return n, nil
}
