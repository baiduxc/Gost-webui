// Package notify 提供基于 HTTP 的通知推送（Telegram Bot API）。
// 只做「发一条消息」，不建立任何长连接。
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 事件类型
const (
	EventNodeUp       = "nodeUp"       // 中转节点上线
	EventNodeDown     = "nodeDown"     // 中转节点下线/异常
	EventQuotaBlocked = "quotaBlocked" // 流量超限暂停
	EventTrafficWarn  = "trafficWarn"  // 流量预警
	EventLoadWarn     = "loadWarn"     // 服务器负载预警
	EventPanelStart   = "panelStart"   // 面板启动
	EventPanelStop    = "panelStop"    // 面板停止
	EventNodeAdded    = "nodeAdded"    // 面板新增节点
	EventNodeDeleted  = "nodeDeleted"  // 面板删除节点
	EventNodeClient   = "nodeClient"   // 客户端连接节点
)

// AllEvents 返回全部事件类型及中文名。
func AllEvents() map[string]string {
	return map[string]string{
		EventNodeUp:       "节点上线",
		EventNodeDown:     "节点下线",
		EventQuotaBlocked: "流量超限暂停",
		EventTrafficWarn:  "流量预警",
		EventLoadWarn:     "服务器负载预警",
		EventPanelStart:   "面板启动",
		EventPanelStop:    "面板停止",
		EventNodeAdded:    "新增节点",
		EventNodeDeleted:  "删除节点",
		EventNodeClient:   "客户端上下线",
	}
}

// Config 是通知配置。
type Config struct {
	Enabled  bool            `json:"enabled"`
	Token    string          `json:"token"`
	ChatID   string          `json:"chatId"`
	APIBase  string          `json:"apiBase"`
	Cooldown int             `json:"cooldown"` // 秒，同一事件的发送间隔
	Events   map[string]bool `json:"events"`
}

// DefaultEvents 默认开启的事件。
func DefaultEvents() map[string]bool {
	m := map[string]bool{}
	for k := range AllEvents() {
		m[k] = true
	}
	return m
}

// Notifier 负责发送通知。
type Notifier struct {
	mu     sync.RWMutex
	cfg    Config
	client *http.Client
	last   map[string]time.Time
	log    *slog.Logger
}

// New 创建 Notifier。
func New(log *slog.Logger) *Notifier {
	return &Notifier{
		client: &http.Client{Timeout: 12 * time.Second},
		last:   map[string]time.Time{},
		log:    log,
	}
}

// Update 更新配置。
func (n *Notifier) Update(cfg Config) {
	if cfg.APIBase == "" {
		cfg.APIBase = "https://api.telegram.org"
	}
	cfg.APIBase = strings.TrimRight(cfg.APIBase, "/")
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 600
	}
	if cfg.Events == nil {
		cfg.Events = DefaultEvents()
	}
	n.mu.Lock()
	n.cfg = cfg
	n.mu.Unlock()
}

// Snapshot 返回当前配置（Token 脱敏由调用方处理）。
func (n *Notifier) Snapshot() Config {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.cfg
}

// Enabled 判断是否具备发送条件。
func (n *Notifier) Enabled() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.cfg.Enabled && n.cfg.Token != "" && n.cfg.ChatID != ""
}

// EventEnabled 判断某事件是否开启。
func (n *Notifier) EventEnabled(event string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if !n.cfg.Enabled || n.cfg.Token == "" || n.cfg.ChatID == "" {
		return false
	}
	if n.cfg.Events == nil {
		return true
	}
	v, ok := n.cfg.Events[event]
	if !ok {
		return true
	}
	return v
}

// Send 发送一条通知。key 用于冷却去重（同 key 在冷却期内只发一次）。
// 返回 error 仅用于调试，调用方通常忽略。
func (n *Notifier) Send(ctx context.Context, event, key, text string) error {
	if !n.EventEnabled(event) {
		return nil
	}
	n.mu.Lock()
	cfg := n.cfg
	if key != "" {
		ck := event + "|" + key
		if t, ok := n.last[ck]; ok && time.Since(t) < time.Duration(cfg.Cooldown)*time.Second {
			n.mu.Unlock()
			return nil
		}
		n.last[ck] = time.Now()
	}
	n.mu.Unlock()

	var firstErr error
	for _, chat := range strings.Split(cfg.ChatID, ",") {
		chat = strings.TrimSpace(chat)
		if chat == "" {
			continue
		}
		if err := n.post(ctx, cfg, chat, text); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		n.log.Warn("通知发送失败", "event", event, "err", firstErr)
	}
	return firstErr
}

// Test 发送一条测试通知。
func (n *Notifier) Test(ctx context.Context) error {
	n.mu.RLock()
	cfg := n.cfg
	n.mu.RUnlock()
	if cfg.Token == "" || cfg.ChatID == "" {
		return fmt.Errorf("请先填写 Bot Token 与 Chat ID")
	}
	text := fmt.Sprintf("<b>[GOST 面板] 通知测试</b>\n时间：%s\n如果你收到这条消息，说明通知配置成功。",
		time.Now().Format("2006-01-02 15:04:05"))
	for _, chat := range strings.Split(cfg.ChatID, ",") {
		chat = strings.TrimSpace(chat)
		if chat == "" {
			continue
		}
		if err := n.post(ctx, cfg, chat, text); err != nil {
			return err
		}
	}
	return nil
}

func (n *Notifier) post(ctx context.Context, cfg Config, chat, text string) error {
	url := fmt.Sprintf("%s/bot%s/sendMessage", cfg.APIBase, cfg.Token)
	body, _ := json.Marshal(map[string]any{
		"chat_id":                  chat,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(data)))
	}
	var r struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if json.Unmarshal(data, &r) == nil && !r.OK {
		return fmt.Errorf("telegram: %s", r.Description)
	}
	return nil
}
