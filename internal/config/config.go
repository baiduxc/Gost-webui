// Package config 定义面板的启动配置（panel.yml）。
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Gost 描述被管理的 gost 进程。
type Gost struct {
	// Bin 是 gost 可执行文件路径。
	Bin string `yaml:"bin"`
	// ConfigFile 是面板生成的 gost 配置文件路径。
	ConfigFile string `yaml:"config_file"`
	// APIAddr 是 gost REST API 监听地址（只监听回环）。
	APIAddr string `yaml:"api_addr"`
	// APIUser/APIPass 是访问 gost API 的 Basic Auth 凭据。
	APIUser string `yaml:"api_username"`
	APIPass string `yaml:"api_password"`
	// LogFile 是 gost 进程输出日志文件。
	LogFile string `yaml:"log_file"`
	// LogLevel 是 gost 进程日志级别：trace/debug/info/warn/error。
	// info 会记录每条连接的来源 IP 与目标域名；在意隐私留痕时建议设为 warn。
	LogLevel string `yaml:"log_level"`
}

// Admin 面板管理员账号（初始密码，首次启动后写入数据库）。
type Admin struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Notify 通知（Telegram）默认配置；运行期以数据库设置为准。
type Notify struct {
	Enabled  bool   `yaml:"enabled"`
	Token    string `yaml:"token"`
	ChatID   string `yaml:"chat_id"`
	APIBase  string `yaml:"api_base"`
	Cooldown int    `yaml:"cooldown_seconds"`
}

// Config 面板配置。
type Config struct {
	Listen string `yaml:"listen"`
	// BasePath 是面板访问路径前缀，例如 /panel；为空表示根路径。
	BasePath string `yaml:"base_path"`
	// DataDir 存放面板数据库与流量统计。
	DataDir string `yaml:"data_dir"`
	// LogDir 日志目录。
	LogDir string `yaml:"log_dir"`
	// PublicHost 对外展示的中转机地址；为空时面板自动探测公网 IP。
	PublicHost string `yaml:"public_host"`
	// SampleSeconds 流量采样间隔（秒）。
	SampleSeconds int `yaml:"sample_seconds"`
	// RetentionDays 流量明细保留天数。
	RetentionDays int `yaml:"retention_days"`
	// CacheDir 编译缓存等（预留）。
	Admin  Admin  `yaml:"admin"`
	Gost   Gost   `yaml:"gost"`
	Notify Notify `yaml:"notify"`
}

// Default 返回默认配置。
func Default() *Config {
	return &Config{
		Listen:        ":8787",
		BasePath:      "",
		DataDir:       "/var/lib/gost-webui",
		LogDir:        "/var/log/gost-webui",
		SampleSeconds: 15,
		RetentionDays: 90,
		Admin:         Admin{Username: "admin", Password: RandomHex(8)},
		Notify: Notify{
			APIBase:  "https://api.telegram.org",
			Cooldown: 600,
		},
		Gost: Gost{
			Bin:        "/opt/gost-webui/bin/gost",
			ConfigFile: "/etc/gost-webui/gost.yml",
			APIAddr:    "127.0.0.1:18080",
			APIUser:    "gost",
			APIPass:    RandomHex(16),
			LogFile:    "/var/log/gost-webui/gost.log",
			LogLevel:   "info",
		},
	}
}

// NormalizeBasePath 规范化访问路径前缀。
func NormalizeBasePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

// Load 从 path 加载配置；文件不存在时写入默认配置。
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		cfg := Default()
		if err := cfg.Save(path); err != nil {
			return nil, err
		}
		return cfg, nil
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}
	applyDefaults(cfg)
	return cfg, nil
}

func applyDefaults(c *Config) {
	d := Default()
	if strings.TrimSpace(c.Listen) == "" {
		c.Listen = d.Listen
	}
	c.BasePath = NormalizeBasePath(c.BasePath)
	if strings.TrimSpace(c.DataDir) == "" {
		c.DataDir = d.DataDir
	}
	if strings.TrimSpace(c.LogDir) == "" {
		c.LogDir = d.LogDir
	}
	if c.SampleSeconds <= 0 {
		c.SampleSeconds = d.SampleSeconds
	}
	if c.RetentionDays <= 0 {
		c.RetentionDays = d.RetentionDays
	}
	if c.Admin.Username == "" {
		c.Admin.Username = d.Admin.Username
	}
	if c.Gost.Bin == "" {
		c.Gost.Bin = d.Gost.Bin
	}
	if c.Gost.ConfigFile == "" {
		c.Gost.ConfigFile = d.Gost.ConfigFile
	}
	if c.Gost.APIAddr == "" {
		c.Gost.APIAddr = d.Gost.APIAddr
	}
	if c.Gost.APIUser == "" {
		c.Gost.APIUser = d.Gost.APIUser
	}
	if c.Gost.LogFile == "" {
		c.Gost.LogFile = d.Gost.LogFile
	}
	if !ValidGostLogLevel(c.Gost.LogLevel) {
		c.Gost.LogLevel = d.Gost.LogLevel
	}
	if c.Notify.APIBase == "" {
		c.Notify.APIBase = d.Notify.APIBase
	}
	if c.Notify.Cooldown <= 0 {
		c.Notify.Cooldown = d.Notify.Cooldown
	}
}

// Save 写入配置文件（权限 0600）。
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// EnsureDirs 创建运行所需目录。
func (c *Config) EnsureDirs() error {
	for _, d := range []string{c.DataDir, c.LogDir, filepath.Dir(c.Gost.ConfigFile), filepath.Dir(c.Gost.Bin)} {
		if d == "" || d == "." {
			continue
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// ValidGostLogLevel 判断 gost 日志级别是否合法（不区分大小写）。
// "off" 表示关闭日志（gost 输出丢弃，不写文件）。
func ValidGostLogLevel(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "trace", "debug", "info", "warn", "warning", "error":
		return true
	}
	return false
}

// NormalizeGostLogLevel 规范化 gost 日志级别；非法值返回 ""。
func NormalizeGostLogLevel(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "warning" {
		v = "warn"
	}
	if !ValidGostLogLevel(v) {
		return ""
	}
	return v
}

// RandomHex 生成 n 字节随机数的十六进制字符串。
func RandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// 退化方案：不应发生
		for i := range b {
			b[i] = byte(i * 7)
		}
	}
	return hex.EncodeToString(b)
}
