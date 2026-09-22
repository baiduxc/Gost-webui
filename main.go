// gost-webui 是基于 gost 的中转面板：管理中转端口转发、流量统计与配额限制。
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"gost-webui/internal/alerts"
	"gost-webui/internal/cli"
	"gost-webui/internal/config"
	"gost-webui/internal/controller"
	"gost-webui/internal/gostmgr"
	"gost-webui/internal/notify"
	"gost-webui/internal/server"
	"gost-webui/internal/store"
)

const version = "1.3.0"

//go:embed all:web
var embeddedWeb embed.FS

func main() {
	confPath := flag.String("c", "/etc/gost-webui/panel.yml", "配置文件路径")
	listen := flag.String("listen", "", "监听地址（覆盖配置文件，如 :8787）")
	showVersion := flag.Bool("v", false, "打印版本号")
	setPassword := flag.String("set-password", "", "设置管理员账号密码：-set-password \"admin 新密码\"")
	setListen := flag.String("set-listen", "", "设置监听地址：-set-listen :8899")
	setBasePath := flag.String("set-base-path", "", "设置访问路径：-set-base-path /panel（空串表示根路径）")
	setPublicHost := flag.String("set-public-host", "", "设置服务器外网地址：-set-public-host 1.2.3.4")
	showConf := flag.Bool("show", false, "打印当前配置摘要")
	flag.Parse()

	if *showVersion {
		fmt.Println("gost-webui", version)
		return
	}

	// 维护命令（go-ui 菜单调用）：改密码/端口/路径/地址
	provided := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	if provided["set-password"] || provided["set-listen"] || provided["set-base-path"] ||
		provided["set-public-host"] || provided["show"] {
		opts := cli.Options{
			Password:   *setPassword,
			Listen:     *setListen,
			PublicHost: *setPublicHost,
			Show:       *showConf,
		}
		if provided["set-base-path"] {
			opts.BasePath = setBasePath
		}
		if err := cli.Run(*confPath, opts); err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			os.Exit(1)
		}
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*confPath)
	if err != nil {
		logger.Error("加载配置失败", "err", err)
		os.Exit(1)
	}
	if *listen != "" {
		cfg.Listen = *listen
	}
	if err := cfg.EnsureDirs(); err != nil {
		logger.Error("创建目录失败", "err", err)
		os.Exit(1)
	}

	st, err := store.Open(filepath.Join(cfg.DataDir, "panel.db"))
	if err != nil {
		logger.Error("打开数据库失败", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	// 运行期可调参数（数据库优先，其次配置文件）
	if v, ok := st.GetSetting("panel_listen"); ok && v != "" {
		cfg.Listen = v
	}
	if v, ok := st.GetSetting("base_path"); ok {
		cfg.BasePath = config.NormalizeBasePath(v)
	}
	sampleSeconds := cfg.SampleSeconds
	retentionDays := cfg.RetentionDays
	if v, ok := st.GetSetting("sample_seconds"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			sampleSeconds = n
		}
	}
	if v, ok := st.GetSetting("retention_days"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			retentionDays = n
		}
	}

	// 通知（Telegram）：配置文件提供初始值
	notifier := notify.New(logger)
	notifier.Update(notify.Config{
		Enabled:  cfg.Notify.Enabled,
		Token:    cfg.Notify.Token,
		ChatID:   cfg.Notify.ChatID,
		APIBase:  cfg.Notify.APIBase,
		Cooldown: cfg.Notify.Cooldown,
		Events:   notify.DefaultEvents(),
	})

	mgr := gostmgr.New(cfg.Gost.Bin, cfg.Gost.ConfigFile, cfg.Gost.LogFile,
		cfg.Gost.APIAddr, cfg.Gost.APIUser, cfg.Gost.APIPass, logger)

	// gost API 端口被占用时（例如遗留进程）自动切换，避免面板无法控制 gost
	if !addrFree(cfg.Gost.APIAddr) {
		if port, err := pickFreePort(); err == nil {
			host, _, _ := net.SplitHostPort(cfg.Gost.APIAddr)
			if host == "" {
				host = "127.0.0.1"
			}
			newAddr := net.JoinHostPort(host, strconv.Itoa(port))
			logger.Warn("gost API 端口被占用，已自动切换", "old", cfg.Gost.APIAddr, "new", newAddr)
			cfg.Gost.APIAddr = newAddr
			mgr = gostmgr.New(cfg.Gost.Bin, cfg.Gost.ConfigFile, cfg.Gost.LogFile,
				cfg.Gost.APIAddr, cfg.Gost.APIUser, cfg.Gost.APIPass, logger)
		}
	}

	ctl := controller.New(st, mgr, logger, controller.Options{
		APIAddr:   cfg.Gost.APIAddr,
		APIUser:   cfg.Gost.APIUser,
		APIPass:   cfg.Gost.APIPass,
		QuotaFile: filepath.Join(cfg.DataDir, "quota.json"),
		LogLevel:  "info",
	})
	ctl.SampleSeconds = sampleSeconds
	ctl.RetentionDays = retentionDays

	alertMgr := alerts.New(st, notifier, logger)
	ctl.Alerts = alertMgr

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ctl.Bootstrap(ctx); err != nil {
		logger.Error("启动 gost 失败", "err", err)
	}
	ctl.RunSampler(ctx)
	go alertMgr.Run(ctx)

	webFS, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		logger.Error("加载前端资源失败", "err", err)
		os.Exit(1)
	}
	srv := server.New(cfg, ctl, webFS, logger)
	server.Version = version
	server.SetConfigPath(*confPath)
	srv.SetRestartFunc(func() { restartPanel(mgr, logger) })

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	accessURL := accessURLOf(cfg)
	go func() {
		logger.Info("面板已启动",
			"listen", cfg.Listen,
			"base_path", cfg.BasePath,
			"数据目录", cfg.DataDir,
			"gost", cfg.Gost.Bin)
		alertMgr.NotifyPanelStart(context.Background(), accessURL)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP 服务退出", "err", err)
			cancel()
		}
	}()

	slog.Info("面板访问地址", "url", accessURL)
	fmt.Printf("\n  面板地址: %s\n  用户名: %s\n  初始密码见配置文件: %s\n\n",
		accessURL, srv.AdminUser(), *confPath)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
	case <-ctx.Done():
	}
	logger.Info("正在退出…")
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel2()
	_ = httpSrv.Shutdown(shutdownCtx)
	// 退出前最后采样一次，避免丢失最后一段流量（配额计数恢复依赖面板统计）
	ctl.SampleOnce(shutdownCtx)
	alertMgr.NotifyPanelStop(shutdownCtx)
	mgr.Stop()
}

// restartPanel 重启面板：优先交给 systemd，否则重新执行自身。
func restartPanel(mgr *gostmgr.Manager, logger *slog.Logger) {
	logger.Info("正在重启面板…")
	if server.SystemdAvailable("gost-webui") {
		if err := exec.Command("systemctl", "restart", "gost-webui").Start(); err == nil {
			return
		}
		logger.Warn("systemd 重启失败，改为自重启")
	}
	mgr.Stop()
	exe, err := os.Executable()
	if err != nil {
		logger.Error("获取可执行文件路径失败", "err", err)
		os.Exit(0)
	}
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
		logger.Error("自重启失败", "err", err)
		os.Exit(0)
	}
}

// accessURLOf 拼出面板访问地址（用于展示与通知）。
func accessURLOf(cfg *config.Config) string {
	host := cfg.PublicHost
	if host == "" {
		if ip, err := publicIP(); err == nil {
			host = ip
		}
	}
	if host == "" {
		host = "127.0.0.1"
	}
	port := "8787"
	if _, p, err := net.SplitHostPort(cfg.Listen); err == nil {
		port = p
	}
	return fmt.Sprintf("http://%s:%s%s/", host, port, cfg.BasePath)
}

func publicIP() (string, error) {
	endpoints := []string{
		"https://api.ipify.org",
		"https://ipv4.icanhazip.com",
		"https://ip.sb",
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, ep := range endpoints {
		res, err := client.Get(ep)
		if err != nil {
			continue
		}
		buf := make([]byte, 64)
		n, _ := res.Body.Read(buf)
		res.Body.Close()
		ip := ""
		for _, f := range string(buf[:n]) {
			if f == '\n' || f == '\r' || f == ' ' || f == '\t' {
				break
			}
			ip += string(f)
		}
		if net.ParseIP(ip) != nil {
			return ip, nil
		}
	}
	return "", fmt.Errorf("未探测到公网 IP")
}

// addrFree 判断地址是否可监听。
func addrFree(addr string) bool {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// pickFreePort 挑选一个空闲的本地端口。
func pickFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
