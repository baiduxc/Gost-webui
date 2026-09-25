// Package alerts 负责事件检测与通知分发（节点上下线、流量预警、负载预警）。
package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"gost-webui/internal/model"
	"gost-webui/internal/notify"
	"gost-webui/internal/store"
	"gost-webui/internal/system"
)

// Manager 监控运行状态并按阈值发送通知。
type Manager struct {
	Notify *notify.Notifier
	Store  *store.Store
	Log    *slog.Logger

	mu          sync.Mutex
	metrics     system.Metrics
	metricsAt   time.Time
	nodeRunning map[string]bool
	nodeUDP     map[string]bool
	nodeTotal   map[string]uint64
	quotaLevel  map[string]int
	windowKey   map[string]int64

	TrafficThresholds []int
	CPUThreshold      float64
	MemThreshold      float64
	DiskThreshold     float64
}

// New 创建 Manager。
func New(st *store.Store, n *notify.Notifier, log *slog.Logger) *Manager {
	m := &Manager{
		Notify:            n,
		Store:             st,
		Log:               log,
		nodeRunning:       map[string]bool{},
		nodeUDP:           map[string]bool{},
		nodeTotal:         map[string]uint64{},
		quotaLevel:        map[string]int{},
		windowKey:         map[string]int64{},
		TrafficThresholds: []int{80, 95},
		CPUThreshold:      90,
		MemThreshold:      90,
		DiskThreshold:     90,
	}
	m.Reload()
	return m
}

// Reload 从数据库读取通知与阈值配置。
func (m *Manager) Reload() {
	cfg := notify.Config{
		Enabled:  m.getBool("notify_enabled", false),
		Token:    m.get("notify_token", ""),
		ChatID:   m.get("notify_chat_id", ""),
		APIBase:  m.get("notify_api_base", "https://api.telegram.org"),
		Events:   notify.DefaultEvents(),
		Cooldown: m.getInt("notify_cooldown", 600),
	}
	if raw := m.get("notify_events", ""); raw != "" {
		var ev map[string]bool
		if json.Unmarshal([]byte(raw), &ev) == nil {
			cfg.Events = ev
		}
	}
	m.Notify.Update(cfg)

	if raw := m.get("alert_traffic_thresholds", ""); raw != "" {
		var v []int
		if json.Unmarshal([]byte(raw), &v) == nil && len(v) > 0 {
			m.TrafficThresholds = v
		}
	}
	m.CPUThreshold = m.getFloat("alert_cpu", 90)
	m.MemThreshold = m.getFloat("alert_mem", 90)
	m.DiskThreshold = m.getFloat("alert_disk", 90)
}

// Run 周期采集主机指标并检查负载阈值。
func (m *Manager) Run(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	m.checkHost(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.checkHost(ctx)
		}
	}
}

func (m *Manager) checkHost(ctx context.Context) {
	met := system.Read()
	m.mu.Lock()
	m.metrics = met
	m.metricsAt = time.Now()
	m.mu.Unlock()

	if !m.Notify.Enabled() {
		return
	}
	const miB = 1024 * 1024
	cpu := met.CPUPercent
	if m.CPUThreshold > 0 && cpu >= m.CPUThreshold {
		text := fmt.Sprintf("<b>[GOST 面板] 服务器 CPU 负载偏高</b>\n当前负载(1分钟)：%.2f（%d 核，约 %.1f%%）\n阈值：%.0f%%",
			met.Load1, met.CPUCount, cpu, m.CPUThreshold)
		_ = m.Notify.Send(ctx, notify.EventLoadWarn, "cpu", text)
	}
	if m.MemThreshold > 0 && met.MemPercent >= m.MemThreshold {
		text := fmt.Sprintf("<b>[GOST 面板] 服务器内存占用偏高</b>\n已用：%s / %s（%.1f%%）\n阈值：%.0f%%",
			fmtBytes(met.MemUsed*miB), fmtBytes(met.MemTotal*miB), met.MemPercent, m.MemThreshold)
		_ = m.Notify.Send(ctx, notify.EventLoadWarn, "mem", text)
	}
	if m.DiskThreshold > 0 && met.DiskPercent >= m.DiskThreshold {
		text := fmt.Sprintf("<b>[GOST 面板] 服务器磁盘占用偏高</b>\n已用：%s / %s（%.1f%%）\n阈值：%.0f%%",
			fmtBytes(met.DiskUsed), fmtBytes(met.DiskTotal), met.DiskPercent, m.DiskThreshold)
		_ = m.Notify.Send(ctx, notify.EventLoadWarn, "disk", text)
	}
}

// Metrics 返回最近一次采集的主机指标。
func (m *Manager) Metrics() (system.Metrics, time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.metrics, m.metricsAt
}

// ObserveNodes 检查节点状态变化与流量阈值，由采样器周期调用。
func (m *Manager) ObserveNodes(ctx context.Context, nodes []*model.Node, live map[string]*model.Live) {
	for _, n := range nodes {
		lv := live[n.ID]
		if lv == nil {
			continue
		}
		// 1) 上下线
		prev, seen := m.nodeRunning[n.ID]
		now := lv.Running && n.Enabled
		if seen && prev != now {
			if now {
				text := fmt.Sprintf("<b>[GOST 面板] 中转节点已上线</b>\n节点：%s\n端口：%d\n落地：%s:%d",
					n.Name, n.ListenPort, n.TargetHost, n.TargetPort)
				_ = m.Notify.Send(ctx, notify.EventNodeUp, n.ID, text)
			} else {
				reason := "服务已停止"
				if !n.Enabled {
					reason = "节点已被手动停用"
				} else if lv.QuotaBlocked {
					reason = "流量超限已暂停"
				}
				text := fmt.Sprintf("<b>[GOST 面板] 中转节点已下线</b>\n节点：%s\n端口：%d\n原因：%s",
					n.Name, n.ListenPort, reason)
				_ = m.Notify.Send(ctx, notify.EventNodeDown, n.ID, text)
			}
		}
		m.mu.Lock()
		m.nodeRunning[n.ID] = now
		m.mu.Unlock()

		// 2) 新客户端连接（用累计连接数增量判断，可捕获采样间隙的短连接）
		if seen2, ok2 := m.nodeTotal[n.ID]; ok2 && lv.TotalConns > seen2 {
			delta := lv.TotalConns - seen2
			text := fmt.Sprintf("<b>[GOST 面板] 节点迎来新连接</b>\n节点：%s\n新增客户端：%d\n当前在线：%d",
				n.Name, delta, lv.CurrentConns)
			_ = m.Notify.Send(ctx, notify.EventNodeClient, "", text)
		}
		m.mu.Lock()
		m.nodeTotal[n.ID] = lv.TotalConns
		m.mu.Unlock()

		// 3) 流量超限暂停
		if lv.QuotaLimit > 0 && lv.QuotaBlocked {
			text := fmt.Sprintf("<b>[GOST 面板] 节点流量已达上限，已自动暂停</b>\n节点：%s\n已用：%s / %s（100%%）\n周期结束：%s",
				n.Name, fmtBytes(lv.QuotaUsed), fmtBytes(lv.QuotaLimit), tsDesc(lv.QuotaUntil))
			_ = m.Notify.Send(ctx, notify.EventQuotaBlocked, n.ID, text)
		}

		// 4) 流量预警（按阈值档位，窗口切换后重置）
		if lv.QuotaLimit > 0 && !lv.QuotaBlocked {
			m.mu.Lock()
			if m.windowKey[n.ID] != lv.QuotaUntil {
				m.windowKey[n.ID] = lv.QuotaUntil
				m.quotaLevel[n.ID] = 0
			}
			level := m.quotaLevel[n.ID]
			m.mu.Unlock()

			pct := float64(lv.QuotaUsed) / float64(lv.QuotaLimit) * 100
			best := 0
			for _, th := range m.TrafficThresholds {
				if pct >= float64(th) && th > best {
					best = th
				}
			}
			if best > level {
				m.mu.Lock()
				m.quotaLevel[n.ID] = best
				m.mu.Unlock()
				text := fmt.Sprintf("<b>[GOST 面板] 节点流量预警</b>\n节点：%s\n已用：%s / %s（%.1f%%，已达 %d%% 阈值）\n周期结束：%s",
					n.Name, fmtBytes(lv.QuotaUsed), fmtBytes(lv.QuotaLimit), pct, best, tsDesc(lv.QuotaUntil))
				_ = m.Notify.Send(ctx, notify.EventTrafficWarn, fmt.Sprintf("%s-%d", n.ID, best), text)
			}
		}
	}
}

// NotifyPanelStart / NotifyPanelStop 面板生命周期通知。
func (m *Manager) NotifyPanelStart(ctx context.Context, url string) {
	text := fmt.Sprintf("<b>[GOST 面板] 面板已启动</b>\n时间：%s\n访问地址：%s",
		time.Now().Format("2006-01-02 15:04:05"), url)
	_ = m.Notify.Send(ctx, notify.EventPanelStart, "", text)
}

// NotifyPanelStop 面板停止通知。
func (m *Manager) NotifyPanelStop(ctx context.Context) {
	text := fmt.Sprintf("<b>[GOST 面板] 面板已停止</b>\n时间：%s", time.Now().Format("2006-01-02 15:04:05"))
	_ = m.Notify.Send(ctx, notify.EventPanelStop, "", text)
}

// nodeKind 返回节点类型的中文描述。
func nodeKind(n *model.Node) string {
	if n.Mode == "gost" {
		return "GOST 体系"
	}
	return "中转链接"
}

// NotifyNodeAdded 面板新增节点通知。
func (m *Manager) NotifyNodeAdded(ctx context.Context, n *model.Node) {
	text := fmt.Sprintf("<b>[GOST 面板] 已新增节点</b>\n节点：%s\n类型：%s（%s）\n中转端口：%d\n落地：%s:%d\n时间：%s",
		n.Name, nodeKind(n), n.Protocol, n.ListenPort, n.TargetHost, n.TargetPort,
		time.Now().Format("2006-01-02 15:04:05"))
	_ = m.Notify.Send(ctx, notify.EventNodeAdded, "", text)
}

// NotifyNodeDeleted 面板删除节点通知。
func (m *Manager) NotifyNodeDeleted(ctx context.Context, n *model.Node) {
	text := fmt.Sprintf("<b>[GOST 面板] 已删除节点</b>\n节点：%s\n类型：%s（%s）\n中转端口：%d\n落地：%s:%d\n时间：%s",
		n.Name, nodeKind(n), n.Protocol, n.ListenPort, n.TargetHost, n.TargetPort,
		time.Now().Format("2006-01-02 15:04:05"))
	_ = m.Notify.Send(ctx, notify.EventNodeDeleted, "", text)
}

// ---------- 设置读取辅助 ----------

func (m *Manager) get(key, def string) string {
	if v, ok := m.Store.GetSetting(key); ok {
		return v
	}
	return def
}

func (m *Manager) getBool(key string, def bool) bool {
	if v, ok := m.Store.GetSetting(key); ok {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return def
}

func (m *Manager) getInt(key string, def int) int {
	if v, ok := m.Store.GetSetting(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func (m *Manager) getFloat(key string, def float64) float64 {
	if v, ok := m.Store.GetSetting(key); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func fmtBytes(n uint64) string {
	f := float64(n)
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.2f %s", f, units[i])
}

func tsDesc(ts int64) string {
	if ts <= 0 {
		return "不重置"
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04")
}

// MaskToken 脱敏 Token。
func MaskToken(t string) string {
	t = strings.TrimSpace(t)
	if len(t) <= 8 {
		if t == "" {
			return ""
		}
		return "****"
	}
	return t[:4] + "****" + t[len(t)-4:]
}
