package gostmgr

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Manager 负责写配置文件、守护 gost 进程、提供 API 客户端。
type Manager struct {
	bin      string
	confFile string
	logFile  string
	log      *slog.Logger
	client   *Client

	mu        sync.RWMutex
	cmd       *exec.Cmd
	startedAt time.Time
	lastExit  string
	restarts  int
	stopping  atomic.Bool

	onStarted atomic.Value // func()

	writeMu sync.Mutex
}

// New 创建 Manager。
func New(bin, confFile, logFile, apiAddr, apiUser, apiPass string, log *slog.Logger) *Manager {
	return &Manager{
		bin:      bin,
		confFile: confFile,
		logFile:  logFile,
		log:      log,
		client:   NewClient(apiAddr, apiUser, apiPass),
	}
}

// Client 返回 API 客户端。
func (m *Manager) Client() *Client { return m.client }

// SetOnStarted 注册 gost 进程启动后的回调（用于重新同步配置与配额）。
func (m *Manager) SetOnStarted(fn func()) {
	if fn == nil {
		return
	}
	m.onStarted.Store(fn)
}

// ConfigFile 返回配置文件路径。
func (m *Manager) ConfigFile() string { return m.confFile }

// StatusInfo 是 gost 进程状态。
type StatusInfo struct {
	Running   bool   `json:"running"`
	PID       int    `json:"pid"`
	StartedAt int64  `json:"startedAt"`
	Restarts  int    `json:"restarts"`
	LastExit  string `json:"lastExit"`
	Bin       string `json:"bin"`
	Config    string `json:"config"`
}

// Status 返回进程状态。
func (m *Manager) Status() StatusInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	st := StatusInfo{
		Running:  m.cmd != nil && m.cmd.Process != nil,
		Restarts: m.restarts,
		LastExit: m.lastExit,
		Bin:      m.bin,
		Config:   m.confFile,
	}
	if st.Running {
		st.PID = m.cmd.Process.Pid
		st.StartedAt = m.startedAt.Unix()
	}
	return st
}

// WriteConfig 原子写入 gost 配置文件。
func (m *Manager) WriteConfig(cfg *Config) error {
	b, err := cfg.Marshal()
	if err != nil {
		return err
	}
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(m.confFile), 0o755); err != nil {
		return err
	}
	tmp := m.confFile + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.confFile)
}

// Start 启动守护循环（幂等）。
func (m *Manager) Start(ctx context.Context) {
	if !m.stopping.CompareAndSwap(false, false) {
		return
	}
	go m.supervise(ctx)
}

func (m *Manager) supervise(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil || m.stopping.Load() {
			return
		}
		m.log.Info("启动 gost", "bin", m.bin, "config", m.confFile)
		out := newRotatingWriter(m.logFile, 8<<20)
		cmd := exec.Command(m.bin, "-C", m.confFile)
		cmd.Stdout = out
		cmd.Stderr = out
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		err := cmd.Start()
		if err == nil {
			m.mu.Lock()
			m.cmd = cmd
			m.startedAt = time.Now()
			m.mu.Unlock()
			if fn, _ := m.onStarted.Load().(func()); fn != nil {
				go fn()
			}
			err = cmd.Wait()
		}

		m.mu.Lock()
		m.cmd = nil
		if err != nil {
			m.lastExit = err.Error()
		} else {
			m.lastExit = "进程正常退出"
		}
		m.restarts++
		exited := m.restarts
		m.mu.Unlock()

		if m.stopping.Load() || ctx.Err() != nil {
			return
		}
		m.log.Warn("gost 退出，准备重启", "err", err, "第几次", exited)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// Stop 停止 gost 进程。
func (m *Manager) Stop() {
	m.stopping.Store(true)
	m.mu.RLock()
	cmd := m.cmd
	m.mu.RUnlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
	}
}

// Restart 重启 gost（重置守护状态）。
func (m *Manager) Restart(ctx context.Context) {
	m.Stop()
	m.stopping.Store(false)
	m.mu.Lock()
	m.restarts = 0
	m.lastExit = ""
	m.mu.Unlock()
	m.Start(ctx)
}

// WaitReady 等待 gost API 可用。
func (m *Manager) WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := m.client.Ping(c)
		cancel()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("等待 gost API 超时: %w", err)
		}
		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// ApplyConfig 写入配置文件并让 gost 重载。
func (m *Manager) ApplyConfig(ctx context.Context, cfg *Config) error {
	if err := m.WriteConfig(cfg); err != nil {
		return err
	}
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := m.client.Reload(c); err != nil {
		m.log.Warn("gost 重载失败，尝试重启进程", "err", err)
		m.Restart(ctx)
		if err := m.WaitReady(ctx, 15*time.Second); err != nil {
			return err
		}
	}
	return nil
}

// ---------- 日志滚动 ----------

type rotatingWriter struct {
	mu   sync.Mutex
	path string
	max  int64
	f    *os.File
	size int64
}

func newRotatingWriter(path string, max int64) io.Writer {
	return &rotatingWriter{path: path, max: max}
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
			return len(p), nil
		}
		f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return len(p), nil
		}
		st, _ := f.Stat()
		w.f = f
		if st != nil {
			w.size = st.Size()
		}
	}
	if w.size > w.max {
		_ = w.f.Close()
		_ = os.Rename(w.path, w.path+".1")
		f, err := os.OpenFile(w.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err == nil {
			w.f = f
			w.size = 0
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}
