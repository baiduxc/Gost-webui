package gostmgr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client 是 gost REST API 的极简客户端。
type Client struct {
	base string
	user string
	pass string
	hc   *http.Client
}

// NewClient 创建客户端。addr 形如 127.0.0.1:18080。
func NewClient(addr, user, pass string) *Client {
	base := addr
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	return &Client{
		base: strings.TrimRight(base, "/"),
		user: user,
		pass: pass,
		hc:   &http.Client{Timeout: 15 * time.Second},
	}
}

type apiResp struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.user != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var e struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if json.Unmarshal(data, &e) == nil && e.Msg != "" {
			return fmt.Errorf("gost api %s %s: %s", method, path, e.Msg)
		}
		return fmt.Errorf("gost api %s %s: HTTP %d", method, path, res.StatusCode)
	}
	if out == nil {
		return nil
	}
	// 大多数接口返回 {"code":..,"msg":..,"data":..}
	var r apiResp
	if err := json.Unmarshal(data, &r); err == nil && (r.Data != nil || r.Msg != "" || r.Code != 0) {
		if r.Data != nil {
			return json.Unmarshal(r.Data, out)
		}
		return nil
	}
	return json.Unmarshal(data, out)
}

// Services 拉取服务列表（含运行状态与流量统计）。
func (c *Client) Services(ctx context.Context) ([]*Service, error) {
	var list struct {
		Count int        `json:"count"`
		List  []*Service `json:"list"`
	}
	if err := c.do(ctx, http.MethodGet, "/config/services", nil, &list); err != nil {
		return nil, err
	}
	return list.List, nil
}

// CreateService 新建服务。
func (c *Client) CreateService(ctx context.Context, s *Service) error {
	return c.do(ctx, http.MethodPost, "/config/services", s, nil)
}

// UpdateService 更新服务。
func (c *Client) UpdateService(ctx context.Context, s *Service) error {
	return c.do(ctx, http.MethodPut, "/config/services/"+url.PathEscape(s.Name), s, nil)
}

// DeleteService 删除服务。
func (c *Client) DeleteService(ctx context.Context, name string) error {
	err := c.do(ctx, http.MethodDelete, "/config/services/"+url.PathEscape(name), nil, nil)
	if err != nil && strings.Contains(err.Error(), "not found") {
		return nil
	}
	return err
}

// Quotas 拉取配额列表（含用量状态）。
func (c *Client) Quotas(ctx context.Context) ([]*Quota, error) {
	var list struct {
		Count int      `json:"count"`
		List  []*Quota `json:"list"`
	}
	if err := c.do(ctx, http.MethodGet, "/config/quotas", nil, &list); err != nil {
		return nil, err
	}
	return list.List, nil
}

// CreateQuota 新建配额。
func (c *Client) CreateQuota(ctx context.Context, q *Quota) error {
	return c.do(ctx, http.MethodPost, "/config/quotas", q, nil)
}

// UpdateQuota 更新配额（窗口变化会重置计数）。
func (c *Client) UpdateQuota(ctx context.Context, q *Quota) error {
	return c.do(ctx, http.MethodPut, "/config/quotas/"+url.PathEscape(q.Name), q, nil)
}

// DeleteQuota 删除配额。
func (c *Client) DeleteQuota(ctx context.Context, name string) error {
	err := c.do(ctx, http.MethodDelete, "/config/quotas/"+url.PathEscape(name), nil, nil)
	if err != nil && strings.Contains(err.Error(), "not found") {
		return nil
	}
	return err
}

// ResetQuota 重置配额计数。
func (c *Client) ResetQuota(ctx context.Context, name string, used uint64) error {
	return c.do(ctx, http.MethodPost, "/config/quotas/"+url.PathEscape(name)+"/reset",
		map[string]uint64{"used": used}, nil)
}

// UpsertLimiter 新建或更新限速/连接数限制器。kind 取值 limiters 或 climiters。
func (c *Client) UpsertLimiter(ctx context.Context, kind string, l *Limiter) error {
	err := c.do(ctx, http.MethodPost, "/config/"+kind, l, nil)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "already exists") {
		return c.do(ctx, http.MethodPut, "/config/"+kind+"/"+url.PathEscape(l.Name), l, nil)
	}
	return err
}

// DeleteLimiter 删除限制器。
func (c *Client) DeleteLimiter(ctx context.Context, kind, name string) error {
	err := c.do(ctx, http.MethodDelete, "/config/"+kind+"/"+url.PathEscape(name), nil, nil)
	if err != nil && strings.Contains(err.Error(), "not found") {
		return nil
	}
	return err
}

// Reload 让 gost 重新读取配置文件。
func (c *Client) Reload(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/config/reload", nil, nil)
}

// Ping 探测 API 是否可用。
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Services(ctx)
	return err
}
