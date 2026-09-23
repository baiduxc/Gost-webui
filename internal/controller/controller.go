// Package controller 把面板的数据模型同步到 gost（服务、配额、限速），
// 并负责流量采样与统计。
package controller

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"gost-webui/internal/alerts"
	"gost-webui/internal/gostmgr"
	"gost-webui/internal/model"
	"gost-webui/internal/store"
)

// Options 控制器参数。
type Options struct {
	APIAddr   string
	APIUser   string
	APIPass   string
	QuotaFile string
	LogLevel  string
}

// Controller 是面板的核心业务逻辑。
type Controller struct {
	Store  *store.Store
	Gost   *gostmgr.Manager
	Log    *slog.Logger
	Opts   Options
	Alerts *alerts.Manager

	SampleSeconds int
	RetentionDays int

	mu   sync.RWMutex
	live map[string]*model.Live
	// nodes 缓存，避免采样时频繁读库
	nodes  []*model.Node
	gostOK bool
}

// New 创建控制器。
func New(st *store.Store, mgr *gostmgr.Manager, log *slog.Logger, opts Options) *Controller {
	return &Controller{
		Store: st,
		Gost:  mgr,
		Log:   log,
		Opts:  opts,
		live:  make(map[string]*model.Live),
	}
}

func (c *Controller) client() *gostmgr.Client { return c.Gost.Client() }

// Bootstrap 根据数据库中的节点生成配置文件并启动 gost。
func (c *Controller) Bootstrap(ctx context.Context) error {
	if err := c.SyncConfigFile(); err != nil {
		return err
	}
	if _, err := c.LoadNodes(); err != nil {
		return err
	}
	// gost 每次（重新）启动后，等待 API 就绪并校正配额计数
	c.Gost.SetOnStarted(func() {
		rctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := c.Gost.WaitReady(rctx, 20*time.Second); err != nil {
			c.Log.Warn("gost 重启后 API 未就绪", "err", err)
			c.setGostReachable(false)
			return
		}
		c.setGostReachable(true)
		if err := c.Reconcile(rctx); err != nil {
			c.Log.Warn("gost 重启后同步失败", "err", err)
		}
	})
	c.Gost.Start(ctx)
	if err := c.Gost.WaitReady(ctx, 20*time.Second); err != nil {
		c.Log.Warn("gost 尚未就绪", "err", err)
		c.setGostReachable(false)
		return nil // 不阻塞面板启动
	}
	c.setGostReachable(true)
	c.Log.Info("gost 已就绪")
	return c.Reconcile(ctx)
}

// LoadNodes 重新读取节点列表（含启用状态过滤）。
func (c *Controller) LoadNodes() ([]*model.Node, error) {
	nodes, err := c.Store.ListNodes()
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.nodes = nodes
	c.mu.Unlock()
	return nodes, nil
}

// Nodes 返回缓存中的节点。
func (c *Controller) Nodes() []*model.Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*model.Node, len(c.nodes))
	copy(out, c.nodes)
	return out
}

// Node 按 ID 返回缓存节点。
func (c *Controller) Node(id string) *model.Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, n := range c.nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

// SyncConfigFile 根据全部节点重写 gost 配置文件。
func (c *Controller) SyncConfigFile() error {
	nodes, err := c.Store.ListNodes()
	if err != nil {
		return err
	}
	cfg := gostmgr.BuildConfig(nodes, gostmgr.BuildOptions{
		APIAddr:   c.Opts.APIAddr,
		APIUser:   c.Opts.APIUser,
		APIPass:   c.Opts.APIPass,
		QuotaFile: c.Opts.QuotaFile,
		LogLevel:  c.Opts.LogLevel,
	})
	return c.Gost.WriteConfig(cfg)
}

// ApplyNode 将节点同步到 gost（服务 + 配额 + 限速）。
func (c *Controller) ApplyNode(ctx context.Context, n *model.Node) error {
	cfg := gostmgr.BuildConfig([]*model.Node{n}, gostmgr.BuildOptions{
		APIAddr:   c.Opts.APIAddr,
		APIUser:   c.Opts.APIUser,
		APIPass:   c.Opts.APIPass,
		QuotaFile: c.Opts.QuotaFile,
		LogLevel:  c.Opts.LogLevel,
	})
	cl := c.client()

	// 1. 配额
	for _, q := range cfg.Quotas {
		if err := cl.CreateQuota(ctx, q); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				return err
			}
			if err := cl.UpdateQuota(ctx, q); err != nil {
				return err
			}
		}
	}
	if len(cfg.Quotas) == 0 {
		_ = cl.DeleteQuota(ctx, model.QuotaName(n.ID))
	}

	// 2. 限速 / 连接数限制
	for _, l := range cfg.Limiters {
		if err := cl.UpsertLimiter(ctx, "limiters", l); err != nil {
			return err
		}
	}
	if len(cfg.Limiters) == 0 {
		_ = cl.DeleteLimiter(ctx, "limiters", model.LimiterName(n.ID))
	}
	for _, l := range cfg.CLimiters {
		if err := cl.UpsertLimiter(ctx, "climiters", l); err != nil {
			return err
		}
	}
	if len(cfg.CLimiters) == 0 {
		_ = cl.DeleteLimiter(ctx, "climiters", model.CLimiterName(n.ID))
	}

	// 3. 服务
	if !n.Enabled {
		for _, name := range []string{model.TCPName(n.ID), model.UDPName(n.ID)} {
			if err := cl.DeleteService(ctx, name); err != nil {
				return err
			}
		}
		return c.SyncConfigFile()
	}
	for _, s := range cfg.Services {
		if err := cl.CreateService(ctx, s); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				return err
			}
			if err := cl.UpdateService(ctx, s); err != nil {
				return err
			}
		}
	}
	expected := make(map[string]bool, len(cfg.Services))
	for _, service := range cfg.Services {
		expected[service.Name] = true
	}
	// 协议/通道切换后清除不再生成的辅助服务，避免旧 UDP 服务残留。
	for _, name := range []string{model.TCPName(n.ID), model.UDPName(n.ID)} {
		if expected[name] {
			continue
		}
		if err := cl.DeleteService(ctx, name); err != nil {
			return err
		}
	}
	return c.SyncConfigFile()
}

// RemoveNode 从 gost 中删除节点相关资源。
func (c *Controller) RemoveNode(ctx context.Context, n *model.Node) error {
	cl := c.client()
	var firstErr error
	keep := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	keep(cl.DeleteService(ctx, model.TCPName(n.ID)))
	keep(cl.DeleteService(ctx, model.UDPName(n.ID)))
	keep(cl.DeleteQuota(ctx, model.QuotaName(n.ID)))
	keep(cl.DeleteLimiter(ctx, "limiters", model.LimiterName(n.ID)))
	keep(cl.DeleteLimiter(ctx, "climiters", model.CLimiterName(n.ID)))
	keep(c.SyncConfigFile())
	if firstErr != nil {
		c.Log.Warn("删除节点资源时出错", "node", n.ID, "err", firstErr)
	}
	return firstErr
}

// Reconcile 校正所有节点的配额窗口（跨天/跨月自动重置），
// 并用面板自身的流量明细核对/恢复 gost 配额计数（防止重启丢计数）。
func (c *Controller) Reconcile(ctx context.Context) error {
	quotas, err := c.client().Quotas(ctx)
	if err != nil {
		return err
	}
	index := make(map[string]*gostmgr.Quota, len(quotas))
	for _, q := range quotas {
		index[q.Name] = q
	}
	for _, n := range c.Nodes() {
		if !n.Enabled || !n.Quota.Enabled || n.Quota.Bytes <= 0 {
			continue
		}
		name := model.QuotaName(n.ID)
		want := &gostmgr.Quota{
			Name:      name,
			Limit:     formatLimit(n.Quota.Bytes),
			Direction: n.Quota.Direction,
			Flush:     "5s",
		}
		if s, e := gostmgr.QuotaWindow(n.Quota.Period); s != "" || e != "" {
			want.StartsAt, want.ExpiresAt = s, e
		}
		cur := index[name]
		switch {
		case cur == nil:
			c.Log.Info("创建缺失的配额", "quota", name)
			if err := c.client().CreateQuota(ctx, want); err != nil {
				c.Log.Warn("创建配额失败", "quota", name, "err", err)
				continue
			}
			if q, err := c.findQuota(ctx, name); err == nil {
				cur = q
			}
		case cur.StartsAt != want.StartsAt || cur.ExpiresAt != want.ExpiresAt ||
			cur.Direction != want.Direction || cur.Limit != want.Limit:
			c.Log.Info("更新配额窗口/额度", "quota", name,
				"startsAt", want.StartsAt, "expiresAt", want.ExpiresAt)
			if err := c.client().UpdateQuota(ctx, want); err != nil {
				c.Log.Warn("更新配额失败", "quota", name, "err", err)
				continue
			}
			if q, err := c.findQuota(ctx, name); err == nil {
				cur = q
			}
		}
		c.restoreQuotaUsage(ctx, n, cur)
	}
	return nil
}

func (c *Controller) findQuota(ctx context.Context, name string) (*gostmgr.Quota, error) {
	quotas, err := c.client().Quotas(ctx)
	if err != nil {
		return nil, err
	}
	for _, q := range quotas {
		if q.Name == name {
			return q, nil
		}
	}
	return nil, ErrNotFound
}

// ErrNotFound 表示资源不存在。
var ErrNotFound = errors.New("not found")

// WindowUsed 计算节点在当前配额窗口内的流量（依据面板自身明细）。
func (c *Controller) WindowUsed(n *model.Node, q *gostmgr.Quota) uint64 {
	from := int64(0)
	if q != nil && q.Status != nil {
		from = q.Status.StartsAt
	}
	if n.QuotaSince > from {
		from = n.QuotaSince
	}
	pts, err := c.Store.RangeTraffic(n.ID, from, time.Now().Unix()+1)
	if err != nil {
		return 0
	}
	var used uint64
	for _, p := range pts {
		switch n.Quota.Direction {
		case "in":
			used += p.In
		case "out":
			used += p.Out
		default:
			used += p.In + p.Out
		}
	}
	return used
}

// restoreQuotaUsage 当 gost 计数低于面板统计时，用面板统计恢复配额计数。
func (c *Controller) restoreQuotaUsage(ctx context.Context, n *model.Node, q *gostmgr.Quota) {
	if q == nil || q.Status == nil {
		return
	}
	panelUsed := c.WindowUsed(n, q)
	if panelUsed <= q.Status.Used {
		return
	}
	c.Log.Info("恢复配额计数", "node", n.ID, "gost", q.Status.Used, "面板", panelUsed)
	if err := c.client().ResetQuota(ctx, q.Name, panelUsed); err != nil {
		c.Log.Warn("恢复配额计数失败", "node", n.ID, "err", err)
	}
}

// ResetNodeTraffic 重置节点流量：清零 gost 配额计数与面板累计。
func (c *Controller) ResetNodeTraffic(ctx context.Context, n *model.Node) error {
	if n.Quota.Enabled && n.Quota.Bytes > 0 {
		if err := c.client().ResetQuota(ctx, model.QuotaName(n.ID), 0); err != nil {
			return err
		}
	}
	n.TotalIn, n.TotalOut = 0, 0
	return c.Store.SaveNode(n)
}

func formatLimit(b int64) string {
	return strconv.FormatInt(b, 10) + "B"
}
