package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"gost-webui/internal/model"
	"gost-webui/internal/notify"
)

// 客户端连接通知：采样间 TotalConns 增加时应向 TG 发送 nodeClient 事件。
func TestNodeClientNotify(t *testing.T) {
	var mu sync.Mutex
	var texts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var form map[string]any
		_ = json.Unmarshal(b, &form)
		mu.Lock()
		texts = append(texts, fmt.Sprint(form["text"]))
		mu.Unlock()
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	m := &Manager{
		Notify:            notify.New(slog.New(slog.NewTextHandler(io.Discard, nil))),
		TrafficThresholds: []int{80, 95},
		nodeRunning:       map[string]bool{},
		nodeUDP:           map[string]bool{},
		nodeConns:         map[string]uint64{},
		quotaLevel:        map[string]int{},
		windowKey:         map[string]int64{},
	}
	m.Notify.Update(notify.Config{
		Enabled: true, Token: "t", ChatID: "c",
		APIBase: srv.URL, Cooldown: 0,
		Events: map[string]bool{notify.EventNodeClient: true},
	})

	n := &model.Node{ID: "n1", Name: "新加坡", ListenPort: 45678, TargetHost: "1.2.3.4", TargetPort: 8443, Enabled: true}
	m.nodeRunning["n1"] = true // 屏蔽节点上下线事件，只测客户端连接事件
	send := func(cur, total uint64) {
		m.ObserveNodes(context.Background(), []*model.Node{n},
			map[string]*model.Live{"n1": {CurrentConns: cur, TotalConns: total, Running: true}})
	}

	// 首次建立基线（不算事件）
	send(0, 100)
	// 模拟日志里的刷屏场景：15 次短连接增长，在线始终在 0/1 间抖动
	for _, s := range []struct{ cur, total uint64 }{
		{1, 101}, {1, 101}, {0, 102}, {0, 103}, {2, 107}, {1, 108}, // 在线 0->N：应发 1 条上线
		{0, 109},                               // N->0：应发 1 条下线
		{0, 116}, {0, 117}, {5, 120}, {2, 121}, // 再上线：1 条
	} {
		send(s.cur, s.total)
	}

	// 15 次短连接抖动应只产生 2 条事件：首次上线 + 全部断开。
	// （再次上线在 600s 冷却窗口内，被 Notifier 冷却抑制——防抖动属预期行为）
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(texts) != 2 {
		t.Fatalf("期望恰好 2 条（上线/断开），got %d: %v", len(texts), texts)
	}
	if !contains(texts[0], "客户端已连接节点") || !contains(texts[0], "当前在线：1") {
		t.Fatalf("第1条应为上线通知: %s", texts[0])
	}
	if !contains(texts[1], "客户端已全部断开") {
		t.Fatalf("第2条应为下线通知: %s", texts[1])
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
