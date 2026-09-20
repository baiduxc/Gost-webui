package server

import (
	"bufio"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gost-webui/internal/store"
	"gost-webui/internal/system"
)

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	nodes := s.ctl.Nodes()

	var todayIn, todayOut, monthIn, monthOut, totalIn, totalOut uint64
	var curConns, totalConns, blocked uint64
	enabled := 0
	type nodeStat struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		In      uint64 `json:"in"`
		Out     uint64 `json:"out"`
		Blocked bool   `json:"blocked"`
	}
	stats := make([]nodeStat, 0, len(nodes))

	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	for _, n := range nodes {
		lv := s.ctl.Live(n.ID)
		if n.Enabled {
			enabled++
		}
		totalIn += n.TotalIn
		totalOut += n.TotalOut
		curConns += lv.CurrentConns
		totalConns += lv.TotalConns
		if lv.QuotaBlocked {
			blocked++
		}
		var nIn, nOut uint64
		if pts, err := s.ctl.Store.RangeTraffic(n.ID, startOfMonth.Unix(), now.Unix()+1); err == nil {
			for _, p := range pts {
				nIn += p.In
				nOut += p.Out
				monthIn += p.In
				monthOut += p.Out
				if p.TS >= startOfDay.Unix() {
					todayIn += p.In
					todayOut += p.Out
				}
			}
		}
		stats = append(stats, nodeStat{ID: n.ID, Name: n.Name, Enabled: n.Enabled, In: nIn, Out: nOut, Blocked: lv.QuotaBlocked})
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].In+stats[i].Out > stats[j].In+stats[j].Out })
	if len(stats) > 5 {
		stats = stats[:5]
	}

	// 近 30 天曲线
	from := now.AddDate(0, 0, -29)
	dayStart := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, now.Location())
	type bucket struct{ In, Out uint64 }
	daily := map[int64]*bucket{}
	for _, n := range nodes {
		pts, err := s.ctl.Store.RangeTraffic(n.ID, dayStart.Unix(), now.Unix()+1)
		if err != nil {
			continue
		}
		for _, p := range pts {
			key := store.LocalDayStart(p.TS)
			b, ok := daily[key]
			if !ok {
				b = &bucket{}
				daily[key] = b
			}
			b.In += p.In
			b.Out += p.Out
		}
	}
	series := make([]map[string]any, 0, len(daily))
	for ts, b := range daily {
		series = append(series, map[string]any{"ts": ts, "in": b.In, "out": b.Out})
	}
	sort.Slice(series, func(i, j int) bool {
		return series[i]["ts"].(int64) < series[j]["ts"].(int64)
	})

	gst := s.ctl.Gost.Status()
	host := s.publicHost()

	// 主机负载（供概览页展示）
	var met system.Metrics
	var metAt time.Time
	if s.ctl.Alerts != nil {
		met, metAt = s.ctl.Alerts.Metrics()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"publicHost":        host,
		"publicHostPrivate": isPrivateHost(host),
		"host": map[string]any{
			"metrics":   met,
			"sampledAt": metAt.Unix(),
		},
		"gost": map[string]any{
			"running":   gst.Running,
			"reachable": s.ctl.GostReachable(),
			"pid":       gst.PID,
			"restarts":  gst.Restarts,
			"lastExit":  gst.LastExit,
		},
		"nodes": map[string]any{
			"total":        len(nodes),
			"enabled":      enabled,
			"blocked":      blocked,
			"currentConns": curConns,
			"totalConns":   totalConns,
		},
		"today": map[string]uint64{"in": todayIn, "out": todayOut},
		"month": map[string]uint64{"in": monthIn, "out": monthOut},
		"total": map[string]uint64{"in": totalIn, "out": totalOut},
		"series": series,
		"top":    stats,
	})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	sample := s.ctl.SampleSeconds
	retention := s.ctl.RetentionDays
	if v, ok := s.ctl.Store.GetSetting("sample_seconds"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			sample = n
		}
	}
	if v, ok := s.ctl.Store.GetSetting("retention_days"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			retention = n
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"publicHost":    s.publicHost(),
		"configured":    s.configuredHost(),
		"sampleSeconds": sample,
		"retentionDays": retention,
		"username":      s.adminUser(),
		"gost": map[string]any{
			"bin":        s.cfg.Gost.Bin,
			"configFile": s.cfg.Gost.ConfigFile,
			"apiAddr":    s.cfg.Gost.APIAddr,
			"logFile":    s.cfg.Gost.LogFile,
		},
		"listen": s.cfg.Listen,
	})
}

func (s *Server) configuredHost() string {
	if v, ok := s.ctl.Store.GetSetting("public_host"); ok {
		return v
	}
	return s.cfg.PublicHost
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicHost    *string `json:"publicHost"`
		SampleSeconds *int    `json:"sampleSeconds"`
		RetentionDays *int    `json:"retentionDays"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if req.PublicHost != nil {
		host := strings.TrimSpace(*req.PublicHost)
		if host != "" && !validHostname(host) {
			writeErr(w, http.StatusBadRequest, "中转机地址格式不正确")
			return
		}
		if err := s.ctl.Store.SetSetting("public_host", host); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.SampleSeconds != nil && *req.SampleSeconds >= 5 && *req.SampleSeconds <= 3600 {
		_ = s.ctl.Store.SetSetting("sample_seconds", strconv.Itoa(*req.SampleSeconds))
		s.ctl.SampleSeconds = *req.SampleSeconds
	}
	if req.RetentionDays != nil && *req.RetentionDays >= 1 && *req.RetentionDays <= 3650 {
		_ = s.ctl.Store.SetSetting("retention_days", strconv.Itoa(*req.RetentionDays))
		s.ctl.RetentionDays = *req.RetentionDays
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if !s.checkPassword(req.OldPassword) {
		writeErr(w, http.StatusBadRequest, "原密码不正确")
		return
	}
	if len(req.NewPassword) < 6 {
		writeErr(w, http.StatusBadRequest, "新密码至少 6 位")
		return
	}
	h, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.ctl.Store.SetSetting("admin_hash", string(h)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleGostStatus(w http.ResponseWriter, r *http.Request) {
	st := s.ctl.Gost.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"process":   st,
		"reachable": s.ctl.GostReachable(),
	})
}

func (s *Server) handleGostRestart(w http.ResponseWriter, r *http.Request) {
	if err := s.ctl.SyncConfigFile(); err != nil {
		writeErr(w, http.StatusInternalServerError, "写入配置失败: "+err.Error())
		return
	}
	s.ctl.Gost.Restart(r.Context())
	if err := s.ctl.Gost.WaitReady(r.Context(), 20*time.Second); err != nil {
		writeErr(w, http.StatusInternalServerError, "gost 启动失败: "+err.Error())
		return
	}
	_ = s.ctl.Reconcile(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleGostLogs(w http.ResponseWriter, r *http.Request) {
	n := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		if x, err := strconv.Atoi(v); err == nil && x > 0 && x <= 5000 {
			n = x
		}
	}
	lines, err := tailFile(s.cfg.Gost.LogFile, n)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取日志失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	ring := make([]string, 0, n)
	for sc.Scan() {
		if len(ring) == n {
			copy(ring, ring[1:])
			ring[n-1] = sc.Text()
			continue
		}
		ring = append(ring, sc.Text())
	}
	return ring, sc.Err()
}
