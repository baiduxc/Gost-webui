package alerts

import (
	"context"
	"encoding/json"
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
		var form map[string]string
		_ = json.Unmarshal(b, &form)
		mu.Lock()
		texts = append(texts, form["text"])
		mu.Unlock()
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	m := &Manager{
		Notify:            notify.New(slog.New(slog.NewTextHandler(io.Discard, nil))),
		TrafficThresholds: []int{80, 95},
		nodeRunning:       map[string]bool{},
		nodeUDP:           map[string]bool{},
		nodeTotal:         map[string]uint64{},
		quotaLevel:        map[string]int{},
		windowKey:         map[string]int64{},
	}
	m.Notify.Update(notify.Config{
		Enabled: true, Token: "t", ChatID: "c",
		APIBase: srv.URL, Cooldown: 0,
		Events: map[string]bool{notify.EventNodeClient: true},
	})

	n := &model.Node{ID: "n1", Name: "东京节点", ListenPort: 45678, TargetHost: "1.2.3.4", TargetPort: 8443, Enabled: true}
	live1 := map[string]*model.Live{"n1": {TotalConns: 0, CurrentConns: 0, Running: true}}
	m.nodeRunning["n1"] = true // 避免上线事件干扰
	m.ObserveNodes(context.Background(), []*model.Node{n}, live1)

	live2 := map[string]*model.Live{"n1": {TotalConns: 2, CurrentConns: 1, Running: true}}
	m.ObserveNodes(context.Background(), []*model.Node{n}, live2)

	// 无变化不应再发
	m.ObserveNodes(context.Background(), []*model.Node{n}, live2)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := len(texts)
		mu.Unlock()
		if got >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(texts) != 1 {
		t.Fatalf("期望恰好 1 条连接通知, got %d: %v", len(texts), texts)
	}
	for _, want := range []string{"东京节点", "新增客户端：2", "当前在线：1"} {
		if !contains(texts[0], want) {
			t.Fatalf("通知缺少 %q: %s", want, texts[0])
		}
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
