// Package singbox 以"每节点一个 sing-box 进程"的方式承载 VLESS+REALITY 节点：
// 每个实例拥有独立配置目录与 clash API 端口，进程级 dlTotal/upTotal 即节点精确
// 流量，配额暂停=停进程，节点间互不影响。与 gost 引擎并存。
package singbox

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- 凭据 ----------

// RealityCred 是 reality 节点的凭据（存于 model.Node）。
type RealityCred struct {
	UUID       string `json:"uuid"`
	PrivateKey string `json:"privKey"`
	PublicKey  string `json:"pubKey"`
	ShortID    string `json:"shortId"`
	ServerName string `json:"serverName"`
}

func GenerateRealityKeypair() (priv, pub string, err error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	priv = base64.RawURLEncoding.EncodeToString(key.Bytes())
	pub = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	return priv, pub, nil
}

func GenerateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func GenerateShortID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func ValidServerName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 253 || strings.ContainsAny(s, " \t:/@") {
		return false
	}
	return true
}

// ClientURL 生成 vless:// 分享链接（host 为面板公网地址）。
func ClientURL(c *RealityCred, port int, host, name string) string {
	return fmt.Sprintf(
		"vless://%s@%s:%d?encryption=none&security=reality&type=tcp&sni=%s&fp=chrome&pbk=%s&sid=%s&flow=xtls-rprx-vision#%s",
		c.UUID, host, port, c.ServerName, c.PublicKey, c.ShortID, urlEscape(name))
}

func urlEscape(s string) string {
	const hd = "0123456789ABCDEF"
	var b []byte
	for _, c := range []byte(s) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b = append(b, c)
			continue
		}
		b = append(b, '%', hd[c>>4], hd[c&0xf])
	}
	return string(b)
}

// ---------- sing-box 配置 ----------

type instanceConfig struct {
	Log struct {
		Level     string `json:"level"`
		Timestamp bool   `json:"timestamp"`
	} `json:"log"`
	Inbounds  []json.RawMessage `json:"inbounds"`
	Outbounds []json.RawMessage `json:"outbounds"`
	Route     struct {
		Final string `json:"final"`
	} `json:"route"`
	Experimental struct {
		ClashAPI struct {
			ExternalController string `json:"external_controller"`
		} `json:"clash_api"`
	} `json:"experimental"`
}

// buildInstanceConfig 生成单节点 config.json。
func buildInstanceConfig(id string, port int, c *RealityCred, apiAddr string) ([]byte, error) {
	ic := instanceConfig{}
	ic.Log.Level = "warning"
	ic.Log.Timestamp = true
	ic.Route.Final = "out"
	ic.Experimental.ClashAPI.ExternalController = apiAddr

	inbound := map[string]any{
		"type":        "vless",
		"tag":         id,
		"listen":      "::",
		"listen_port": port,
		"users":       []map[string]any{{"uuid": c.UUID, "flow": "xtls-rprx-vision"}},
		"tls": map[string]any{
			"enabled":     true,
			"server_name": c.ServerName,
			"reality": map[string]any{
				"enabled":     true,
				"handshake":   map[string]any{"server": c.ServerName, "server_port": 443},
				"private_key": c.PrivateKey,
				"short_id":    []string{c.ShortID},
			},
		},
	}
	ib, err := json.Marshal(inbound)
	if err != nil {
		return nil, err
	}
	ob, err := json.Marshal(map[string]any{"type": "direct", "tag": "out"})
	if err != nil {
		return nil, err
	}
	ic.Inbounds = []json.RawMessage{ib}
	ic.Outbounds = []json.RawMessage{ob}
	return json.MarshalIndent(&ic, "", "  ")
}

// ---------- 实例池 ----------

// Stats 是实例运行态。
type Stats struct {
	Running bool
	PID     int
	Down    uint64
	Up      uint64
	Conns   int
	Ready   bool
	LastErr string
}

type instance struct {
	id       string
	dir      string
	confFile string
	logFile  string
	apiAddr  string
	bin      string
	log      *slog.Logger

	mu      sync.Mutex
	cmd     *exec.Cmd
	ready   bool
	lastErr string
	stop    chan struct{}
	dead    bool
}

// Pool 管理全部 reality 节点实例。
type Pool struct {
	dataDir string // 例：/var/lib/gost-webui/singbox
	bin     string
	log     *slog.Logger

	mu   sync.Mutex
	inst map[string]*instance
}

func NewPool(dataDir, bin string, log *slog.Logger) *Pool {
	return &Pool{dataDir: dataDir, bin: bin, log: log, inst: map[string]*instance{}}
}

// Installed 判断二进制可用（版本输出包含 sing-box）。
func Installed(bin string) bool {
	if bin == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "version").Output()
	return err == nil && strings.Contains(string(out), "sing-box")
}

func apiPortFor(id string) int {
	// 由节点 ID 散列到 19100-19899 的本地回环端口，稳定且不与常规端口冲突。
	var h uint32
	for _, ch := range id {
		h = h*131 + uint32(ch)
	}
	return 19100 + int(h%800)
}

func (p *Pool) nodeDir(id string) string { return filepath.Join(p.dataDir, id) }

// Apply 写入/更新节点配置并确保进程按期望状态运行（enabled 决定是否拉起）。
func (p *Pool) Apply(ctx context.Context, id string, port int, cred *RealityCred, enabled bool) error {
	dir := p.nodeDir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	apiAddr := "127.0.0.1:" + strconv.Itoa(apiPortFor(id))
	cfgBytes, err := buildInstanceConfig(id, port, cred, apiAddr)
	if err != nil {
		return err
	}
	confFile := filepath.Join(dir, "config.json")

	p.mu.Lock()
	inst := p.inst[id]
	if inst == nil {
		inst = &instance{id: id, dir: dir, confFile: confFile, logFile: filepath.Join(dir, "sb.log"), apiAddr: apiAddr, bin: p.bin, log: p.log}
		p.inst[id] = inst
	}
	changed := false
	if old, err := os.ReadFile(confFile); err != nil || string(old) != string(cfgBytes) {
		tmp := confFile + ".tmp"
		if err := os.WriteFile(tmp, cfgBytes, 0o600); err != nil {
			p.mu.Unlock()
			return err
		}
		if err := os.Rename(tmp, confFile); err != nil {
			p.mu.Unlock()
			return err
		}
		changed = true
	}
	p.mu.Unlock()

	if !enabled {
		inst.stopProcess()
		return nil
	}
	if changed || inst.isDead() {
		inst.restart(ctx)
	}
	return nil
}

// Remove 停止并删除节点实例目录。
func (p *Pool) Remove(id string) {
	p.mu.Lock()
	inst := p.inst[id]
	delete(p.inst, id)
	p.mu.Unlock()
	if inst != nil {
		inst.stopProcess()
		_ = os.RemoveAll(inst.dir)
	}
}

// StopAll 面板退出时调用。
func (p *Pool) StopAll() {
	p.mu.Lock()
	all := make([]*instance, 0, len(p.inst))
	for _, i := range p.inst {
		all = append(all, i)
	}
	p.mu.Unlock()
	for _, i := range all {
		i.stopProcess()
	}
}

// Stats 返回实例运行态；实例不存在返回零值。
func (p *Pool) Stats(ctx context.Context, id string) Stats {
	p.mu.Lock()
	inst := p.inst[id]
	p.mu.Unlock()
	if inst == nil {
		return Stats{}
	}
	return inst.stats(ctx)
}

// AnyStats 批量取回。
func (p *Pool) AnyStats(ctx context.Context, ids []string) map[string]Stats {
	out := make(map[string]Stats, len(ids))
	for _, id := range ids {
		out[id] = p.Stats(ctx, id)
	}
	return out
}

// IsRealityInstalled 暴露给 handler 判断能否添加 reality 节点。
func (p *Pool) RealityInstalled() bool { return Installed(p.bin) }

// ---------- 单实例进程 ----------

func (i *instance) isDead() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.cmd == nil || i.cmd.Process == nil
}

func (i *instance) stopProcess() {
	i.mu.Lock()
	if i.stop != nil {
		close(i.stop)
		i.stop = nil
	}
	cmd := i.cmd
	i.cmd = nil
	i.ready = false
	i.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { _, _ = cmd.Process.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
	}
}

// restart 停止旧进程并重新拉起 + 守护。
func (i *instance) restart(ctx context.Context) {
	i.stopProcess()
	go i.supervise()
}

func (i *instance) supervise() {
	stop := make(chan struct{})
	i.mu.Lock()
	i.stop = stop
	i.mu.Unlock()

	backoff := time.Second
	for {
		select {
		case <-stop:
			return
		default:
		}
		i.log.Info("启动 sing-box 实例", "node", i.id, "api", i.apiAddr)
		out := newRotatingWriter(i.logFile, 4<<20)
		cmd := exec.Command(i.bin, "run", "-D", i.dir, "-c", i.confFile)
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Start(); err != nil {
			i.setErr(err.Error())
			i.log.Warn("sing-box 启动失败", "node", i.id, "err", err)
			select {
			case <-time.After(backoff):
			case <-stop:
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		i.mu.Lock()
		i.cmd = cmd
		i.mu.Unlock()
		backoff = time.Second

		err := cmd.Wait()
		i.mu.Lock()
		if i.cmd == cmd {
			i.cmd = nil
			i.ready = false
		}
		i.mu.Unlock()
		if err != nil {
			i.setErr(err.Error())
		}
		select {
		case <-stop:
			return
		default:
		}
		i.log.Warn("sing-box 实例退出，准备重启", "node", i.id, "err", err)
		select {
		case <-time.After(backoff):
		case <-stop:
			return
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (i *instance) setErr(s string) {
	i.mu.Lock()
	i.lastErr = s
	i.mu.Unlock()
}

type connectionsResp struct {
	Connections []struct {
		Download uint64 `json:"download"`
		Upload   uint64 `json:"upload"`
	} `json:"connections"`
	DownloadTotal uint64 `json:"downloadTotal"`
	UploadTotal   uint64 `json:"uploadTotal"`
}

func (i *instance) stats(ctx context.Context) Stats {
	i.mu.Lock()
	procRunning := i.cmd != nil && i.cmd.Process != nil
	pid := 0
	if procRunning {
		pid = i.cmd.Process.Pid
	}
	api := i.apiAddr
	lastErr := i.lastErr
	i.mu.Unlock()

	st := Stats{PID: pid, LastErr: lastErr}
	if !procRunning {
		return st
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, "http://"+api+"/connections", nil)
	if err != nil {
		return st
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return st
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return st
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return st
	}
	var cr connectionsResp
	if err := json.Unmarshal(body, &cr); err != nil {
		return st
	}
	st.Running = true
	st.Ready = true
	st.Down = cr.DownloadTotal
	st.Up = cr.UploadTotal
	st.Conns = len(cr.Connections)
	return st
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
		f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
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
		f, err := os.OpenFile(w.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err == nil {
			w.f = f
			w.size = 0
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}
