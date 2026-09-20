package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"gost-webui/internal/alerts"
	"gost-webui/internal/config"
	"gost-webui/internal/notify"
	"gost-webui/internal/system"
)

// handleSystemInfo 返回面板/主机/系统配置信息。
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	gst := s.ctl.Gost.Status()
	var met system.Metrics
	var metAt time.Time
	if s.ctl.Alerts != nil {
		met, metAt = s.ctl.Alerts.Metrics()
	}

	listen := s.effectiveListen()
	basePath := s.effectiveBasePath()

	writeJSON(w, http.StatusOK, map[string]any{
		"panel": map[string]any{
			"version":    Version,
			"listen":     listen,
			"basePath":   basePath,
			"startedAt":  s.started.Unix(),
			"uptime":     int64(time.Since(s.started).Seconds()),
			"runtimeOS":   met.RuntimeOS,
			"runtimeArch": met.RuntimeArch,
			"configFile":  cfgPath,
		},
		"host": map[string]any{
			"metrics":   met,
			"sampledAt": metAt.Unix(),
		},
		"gost": map[string]any{
			"process":   gst,
			"reachable": s.ctl.GostReachable(),
			"configFile": s.cfg.Gost.ConfigFile,
			"logFile":    s.cfg.Gost.LogFile,
			"bin":        s.cfg.Gost.Bin,
		},
		"publicHost":        s.publicHost(),
		"publicHostPrivate": isPrivateHost(s.publicHost()),
	})
}

// Version 面板版本号（由 main 注入）。
var Version = "1.0.0"

// cfgPath 配置文件路径（由 main 注入）。
var cfgPath = "/etc/gost-webui/panel.yml"

// SetConfigPath 设置配置文件路径。
func SetConfigPath(p string) { cfgPath = p }

func (s *Server) effectiveListen() string {
	if v, ok := s.ctl.Store.GetSetting("panel_listen"); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return s.cfg.Listen
}

func (s *Server) effectiveBasePath() string {
	if v, ok := s.ctl.Store.GetSetting("base_path"); ok {
		return config.NormalizeBasePath(v)
	}
	return s.basePath
}

// handleSystemUpdate 修改面板端口 / 访问路径（需重启生效）。
func (s *Server) handleSystemUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Listen   *string `json:"listen"`
		BasePath *string `json:"basePath"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if req.Listen != nil {
		v := strings.TrimSpace(*req.Listen)
		if !validListen(v) {
			writeErr(w, http.StatusBadRequest, "监听地址格式不正确，例如 :8787 或 0.0.0.0:8787")
			return
		}
		if err := s.ctl.Store.SetSetting("panel_listen", v); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.BasePath != nil {
		v := config.NormalizeBasePath(*req.BasePath)
		if v != "" && !strings.HasPrefix(v, "/") {
			v = "/" + v
		}
		if strings.ContainsAny(v, " \t?#") {
			writeErr(w, http.StatusBadRequest, "访问路径不能包含空格或 ? # 等字符")
			return
		}
		if err := s.ctl.Store.SetSetting("base_path", v); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"listen":     s.effectiveListen(),
		"basePath":   s.effectiveBasePath(),
		"needRestart": true,
	})
}

func validListen(v string) bool {
	if v == "" || strings.ContainsAny(v, " \t/?") {
		return false
	}
	i := strings.LastIndex(v, ":")
	if i < 0 {
		return false
	}
	port, err := strconv.Atoi(v[i+1:])
	if err != nil || port < 1 || port > 65535 {
		return false
	}
	host := v[:i]
	if host == "" || host == "0.0.0.0" {
		return true
	}
	return strings.Count(host, ".") == 3 || host == "localhost" || strings.Contains(host, ":")
}

// handleSystemRestart 重启面板进程（先返回响应，再执行重启）。
func (s *Server) handleSystemRestart(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "msg": "面板正在重启"})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	s.restartMu.Lock()
	fn := s.restartFn
	s.restartMu.Unlock()
	if fn == nil {
		s.log.Warn("未配置重启回调，跳过重启")
		return
	}
	go func() {
		time.Sleep(600 * time.Millisecond)
		fn()
	}()
}

// ---------- 通知（Telegram） ----------

func (s *Server) notifyManager() *alerts.Manager { return s.ctl.Alerts }

func (s *Server) handleNotifyGet(w http.ResponseWriter, r *http.Request) {
	cfg := notify.Config{}
	if m := s.notifyManager(); m != nil {
		cfg = m.Notify.Snapshot()
	}
	events := map[string]bool{}
	if cfg.Events != nil {
		events = cfg.Events
	}
	th := []int{80, 95}
	cpu, mem, disk := 90.0, 90.0, 90.0
	if m := s.notifyManager(); m != nil {
		th = m.TrafficThresholds
		cpu, mem, disk = m.CPUThreshold, m.MemThreshold, m.DiskThreshold
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":   cfg.Enabled,
		"token":     alerts.MaskToken(cfg.Token),
		"hasToken":  cfg.Token != "",
		"chatId":    cfg.ChatID,
		"apiBase":   cfg.APIBase,
		"cooldown":  cfg.Cooldown,
		"events":    events,
		"allEvents": notify.AllEvents(),
		"trafficThresholds": th,
		"cpu":  cpu,
		"mem":  mem,
		"disk": disk,
	})
}

func (s *Server) handleNotifyPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled           *bool           `json:"enabled"`
		Token             *string         `json:"token"`
		ChatID            *string         `json:"chatId"`
		APIBase           *string         `json:"apiBase"`
		Cooldown          *int            `json:"cooldown"`
		Events            map[string]bool `json:"events"`
		TrafficThresholds []int           `json:"trafficThresholds"`
		CPU               *float64        `json:"cpu"`
		Mem               *float64        `json:"mem"`
		Disk              *float64        `json:"disk"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	set := func(k, v string) {
		_ = s.ctl.Store.SetSetting(k, v)
	}
	if req.Enabled != nil {
		set("notify_enabled", strconv.FormatBool(*req.Enabled))
	}
	if req.Token != nil {
		t := strings.TrimSpace(*req.Token)
		// 掩码值表示不修改
		if !strings.Contains(t, "****") {
			set("notify_token", t)
		}
	}
	if req.ChatID != nil {
		set("notify_chat_id", strings.TrimSpace(*req.ChatID))
	}
	if req.APIBase != nil {
		v := strings.TrimSpace(*req.APIBase)
		if v == "" {
			v = "https://api.telegram.org"
		}
		set("notify_api_base", v)
	}
	if req.Cooldown != nil && *req.Cooldown >= 0 {
		set("notify_cooldown", strconv.Itoa(*req.Cooldown))
	}
	if req.Events != nil {
		if b, err := json.Marshal(req.Events); err == nil {
			set("notify_events", string(b))
		}
	}
	if len(req.TrafficThresholds) > 0 {
		v := sanitizeThresholds(req.TrafficThresholds)
		if b, err := json.Marshal(v); err == nil {
			set("alert_traffic_thresholds", string(b))
		}
	}
	if req.CPU != nil {
		set("alert_cpu", strconv.FormatFloat(clampPct(*req.CPU), 'f', 1, 64))
	}
	if req.Mem != nil {
		set("alert_mem", strconv.FormatFloat(clampPct(*req.Mem), 'f', 1, 64))
	}
	if req.Disk != nil {
		set("alert_disk", strconv.FormatFloat(clampPct(*req.Disk), 'f', 1, 64))
	}
	if m := s.notifyManager(); m != nil {
		m.Reload()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	m := s.notifyManager()
	if m == nil {
		writeErr(w, http.StatusInternalServerError, "通知模块未初始化")
		return
	}
	m.Reload()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := m.Notify.Test(ctx); err != nil {
		writeErr(w, http.StatusBadRequest, "发送失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func sanitizeThresholds(in []int) []int {
	out := []int{}
	seen := map[int]bool{}
	for _, v := range in {
		if v <= 0 || v >= 100 || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	if len(out) == 0 {
		out = []int{80, 95}
	}
	// 简单排序
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// SystemdAvailable 判断是否由 systemd 托管。
func SystemdAvailable(unit string) bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	return exec.Command("systemctl", "cat", unit).Run() == nil
}
