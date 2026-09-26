package controller

import (
	"context"
	"time"

	"gost-webui/internal/model"
	"gost-webui/internal/store"
)

type counters struct {
	in  uint64
	out uint64
}

// RunSampler 启动后台任务：流量采样、配额窗口校正、历史数据清理。
func (c *Controller) RunSampler(ctx context.Context) {
	interval := time.Duration(c.SampleSeconds) * time.Second
	if interval <= 0 {
		interval = 15 * time.Second
	}
	go func() {
		// 启动后立即采样一次，让界面尽快显示状态
		c.sampleOnce(ctx)

		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.sampleOnce(ctx)
			}
		}
	}()

	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := c.Reconcile(ctx); err != nil {
					c.Log.Debug("配额校正失败", "err", err)
				}
			}
		}
	}()

	go func() {
		// 每天清理一次超期明细
		c.prune()
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.prune()
			}
		}
	}()
}

func (c *Controller) prune() {
	days := c.RetentionDays
	if days <= 0 {
		days = 90
	}
	before := time.Now().AddDate(0, 0, -days).Unix()
	if err := c.Store.PruneTraffic(before - before%3600); err != nil {
		c.Log.Warn("清理历史流量失败", "err", err)
	}
}

// lastCounters 记录上一次采样到的 gost 累计值（用于计算增量）。
var lastCounters = struct {
	m map[string]counters
}{m: map[string]counters{}}

func (c *Controller) sampleOnce(ctx context.Context) {
	sctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	services, err := c.client().Services(sctx)
	if err != nil {
		c.Log.Debug("采样失败：无法读取服务列表", "err", err)
		c.setGostReachable(false)
		return
	}
	c.setGostReachable(true)

	byName := make(map[string]*struct {
		in, out, curConns, totalConns, errs uint64
		state                               string
	}, len(services))
	for _, s := range services {
		if s == nil {
			continue
		}
		item := &struct {
			in, out, curConns, totalConns, errs uint64
			state                               string
		}{}
		if s.Status != nil {
			item.state = s.Status.State
			if st := s.Status.Stats; st != nil {
				item.in = st.InputBytes
				item.out = st.OutputBytes
				item.curConns = st.CurrentConns
				item.totalConns = st.TotalConns
				item.errs = st.TotalErrs
			}
		}
		byName[s.Name] = item
	}

	quotas, _ := c.client().Quotas(sctx)
	quotaStatus := make(map[string]model.Live, len(quotas))
	for _, q := range quotas {
		if q == nil || q.Status == nil {
			continue
		}
		quotaStatus[q.Name] = model.Live{
			QuotaUsed:    q.Status.Used,
			QuotaLimit:   q.Status.Limit,
			QuotaBlocked: q.Status.Blocked,
			QuotaExpired: q.Status.Expired,
			QuotaActive:  q.Status.Active,
			QuotaUntil:   q.Status.ExpiresAt,
		}
	}

	hour := store.LocalHourStart(time.Now().Unix())
	nodes := c.Nodes()
	for _, n := range nodes {
		var in, out, curConns, totalConns, errs uint64
		running := false
		state := ""
		for _, name := range n.ServiceNames() {
			item := byName[name]
			if item == nil {
				continue
			}
			running = true
			state = item.state
			in += item.in
			out += item.out
			curConns += item.curConns
			totalConns += item.totalConns
			errs += item.errs
		}

		prev := lastCounters.m[n.ID]
		dIn, dOut := delta(prev.in, in), delta(prev.out, out)
		lastCounters.m[n.ID] = counters{in: in, out: out}
		if dIn > 0 || dOut > 0 {
			if err := c.Store.AddTraffic(n.ID, hour, dIn, dOut); err != nil {
				c.Log.Warn("写入流量失败", "node", n.ID, "err", err)
			}
			n.TotalIn += dIn
			n.TotalOut += dOut
			if err := c.Store.SaveNode(n); err != nil {
				c.Log.Warn("更新节点累计流量失败", "node", n.ID, "err", err)
			}
		}

		lv := &model.Live{
			CurrentConns: curConns,
			TotalConns:   totalConns,
			TotalErrs:    errs,
			Running:      running && state != "closed" && state != "failed",
			ServiceState: state,
		}
		if qs, ok := quotaStatus[model.QuotaName(n.ID)]; ok {
			lv.QuotaUsed = qs.QuotaUsed
			lv.QuotaLimit = qs.QuotaLimit
			lv.QuotaBlocked = qs.QuotaBlocked
			lv.QuotaExpired = qs.QuotaExpired
			lv.QuotaActive = qs.QuotaActive
			lv.QuotaUntil = qs.QuotaUntil
		}
		c.mu.Lock()
		c.live[n.ID] = lv
		c.mu.Unlock()
	}

	c.sampleReality(sctx, hour)

	// 触发上下线/流量预警通知
	if c.Alerts != nil {
		c.mu.RLock()
		snapshot := make(map[string]*model.Live, len(c.live))
		for k, v := range c.live {
			cp := *v
			snapshot[k] = &cp
		}
		c.mu.RUnlock()
		c.Alerts.ObserveNodes(ctx, nodes, snapshot)
	}
}

// SampleOnce 立即采样一次（用于面板退出前落盘最新流量）。
func (c *Controller) SampleOnce(ctx context.Context) {
	c.sampleOnce(ctx)
}

func delta(prev, cur uint64) uint64 {
	if cur >= prev {
		return cur - prev
	}
	return cur // 计数器被重置（服务重建/gost 重启）
}

// sampleReality 采样 sing-box 实例：进程级累计字节差分入小时桶，
// 配额超限直接停进程（blocked），恢复后重新拉起。
func (c *Controller) sampleReality(ctx context.Context, hour int64) {
	if c.SB == nil {
		return
	}
	realityNodes := false
	for _, n := range c.Nodes() {
		if n.IsReality() {
			realityNodes = true
			break
		}
	}
	if !realityNodes {
		return
	}
	nodes := c.Nodes()
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.IsReality() {
			ids = append(ids, n.ID)
		}
	}
	stats := c.SB.AnyStats(ctx, ids)
	for _, n := range nodes {
		if !n.IsReality() {
			continue
		}
		st := stats[n.ID]
		prev := lastCounters.m[n.ID]
		dIn, dOut := delta(prev.in, st.Down), delta(prev.out, st.Up)
		lastCounters.m[n.ID] = counters{in: st.Down, out: st.Up}
		if dIn > 0 || dOut > 0 {
			if err := c.Store.AddTraffic(n.ID, hour, dIn, dOut); err != nil {
				c.Log.Warn("写入流量失败", "node", n.ID, "err", err)
			}
			n.TotalIn += dIn
			n.TotalOut += dOut
			if err := c.Store.SaveNode(n); err != nil {
				c.Log.Warn("更新节点累计流量失败", "node", n.ID, "err", err)
			}
		}

		// 面板侧配额判定：按周期窗口统计（daily 次日重置 / monthly 次月重置 / total 累计）
		blocked := false
		var limit uint64
		var used uint64
		if n.Quota.Enabled && n.Quota.Bytes > 0 {
			limit = uint64(n.Quota.Bytes)
			periodStart := int64(0)
			now := time.Now()
			switch n.Quota.Period {
			case "daily":
				periodStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
			case "monthly":
				periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
			}
			if periodStart > 0 {
				if pts, err := c.Store.RangeTraffic(n.ID, periodStart, now.Unix()+1); err == nil {
					for _, pt := range pts {
						used += pt.In
						if n.Quota.Direction != "in" {
							used += pt.Out
						}
					}
				}
			} else if n.Quota.Direction == "in" {
				used = n.TotalIn
			} else {
				used = n.TotalIn + n.TotalOut
			}
			blocked = used >= limit
		}
		if blocked && st.Running {
			c.Log.Info("reality 节点超配额，暂停实例", "node", n.Name, "used", used, "limit", limit)
			if err := c.applyReality(ctx, &model.Node{ID: n.ID, ListenPort: n.ListenPort, Enabled: false}); err != nil {
				c.Log.Warn("暂停实例失败", "node", n.ID, "err", err)
			}
			st.Running = false
			st.Ready = false
		} else if !blocked && n.Enabled && !st.Running {
			c.Log.Info("reality 节点恢复，拉起实例", "node", n.Name)
			_ = c.applyReality(ctx, n)
		}

		lv := &model.Live{
			CurrentConns: uint64(st.Conns),
			TotalConns:   uint64(st.Conns),
			Running:      st.Running && !blocked,
			QuotaUsed:    used,
			QuotaLimit:   limit,
			QuotaBlocked: blocked,
			QuotaActive:  n.Quota.Enabled && n.Quota.Bytes > 0,
		}
		c.mu.Lock()
		c.live[n.ID] = lv
		c.mu.Unlock()
	}
}

// Live 返回节点运行态。
func (c *Controller) Live(id string) *model.Live {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if lv, ok := c.live[id]; ok {
		cp := *lv
		return &cp
	}
	return &model.Live{}
}

func (c *Controller) setGostReachable(ok bool) {
	c.mu.Lock()
	c.gostOK = ok
	c.mu.Unlock()
}

// GostReachable 返回最近一次采样时 gost API 是否可用。
func (c *Controller) GostReachable() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.gostOK
}
