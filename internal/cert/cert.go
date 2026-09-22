// Package cert 管理订阅 HTTPS 证书：内置 ACME(Let's Encrypt) 自动签发与续期，
// 并支持手动上传证书作为备选。证书按域名缓存于 DataDir/certs，跨重启持久。
package cert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

// Status 描述当前证书状态，用于前端展示。
type Status struct {
	Mode     string `json:"mode"`          // acme / manual / none
	Domain   string `json:"domain"`        // 证书对应域名
	NotAfter int64  `json:"notAfter"`      // 到期时间（Unix 秒），0 表示无
	Issuer   string `json:"issuer"`        // 颁发者
	Err      string `json:"err,omitempty"` // 附加说明（未签发 / 挑战端口不可用等）
}

// Provider 提供订阅监听所需的 TLS 证书：手动证书优先，其次委托 ACME 自动管理。
type Provider struct {
	log      *slog.Logger
	cacheDir string

	mu      sync.Mutex
	ctx     context.Context
	domain  string
	email   string
	manual  *tls.Certificate
	mgr     *autocert.Manager
	acmeSrv *http.Server
	chalErr string
}

// New 创建证书管理器。cacheDir 为 ACME 证书缓存目录。
func New(log *slog.Logger, cacheDir string) *Provider {
	if log == nil {
		log = slog.Default()
	}
	return &Provider{log: log, cacheDir: cacheDir, ctx: context.Background()}
}

// Reload 更新域名/邮箱/手动证书，并据此（重建 ACME 管理器、）启停 :80 挑战监听。
// 手动证书 certPEM/keyPEM 均为空表示不使用手动证书。
func (p *Provider) Reload(domain, email, certPEM, keyPEM string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	email = strings.TrimSpace(email)

	var manual *tls.Certificate
	if strings.TrimSpace(certPEM) != "" && strings.TrimSpace(keyPEM) != "" {
		c, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
		if err != nil {
			return fmt.Errorf("手动证书无效: %w", err)
		}
		manual = &c
	}

	acmeEnabled := domain != "" && email != ""

	p.mu.Lock()
	changed := domain != p.domain || email != p.email
	p.domain = domain
	p.email = email
	p.manual = manual
	p.chalErr = ""
	if acmeEnabled && (changed || p.mgr == nil) {
		p.mgr = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Email:      email,
			HostPolicy: autocert.HostWhitelist(domain),
			Cache:      autocert.DirCache(p.cacheDir),
		}
	}
	if !acmeEnabled {
		p.mgr = nil
	}
	oldSrv := p.acmeSrv
	var newSrv *http.Server
	if acmeEnabled && p.mgr != nil {
		newSrv = &http.Server{
			Addr:              ":80",
			Handler:           p.mgr.HTTPHandler(nil),
			ReadHeaderTimeout: 10 * time.Second,
		}
	}
	p.acmeSrv = newSrv
	p.mu.Unlock()

	// 停止旧的 :80 挑战监听（在锁外执行，避免阻塞）
	if oldSrv != nil && oldSrv != newSrv {
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = oldSrv.Shutdown(shutCtx)
		cancel()
	}
	if newSrv != nil {
		go func() {
			if err := newSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				p.log.Warn("ACME :80 挑战监听启动失败，HTTP-01 可能无法完成", "err", err)
				p.mu.Lock()
				p.chalErr = err.Error()
				p.mu.Unlock()
			}
		}()
	}
	return nil
}

// Start 启动后台自动签发/续期循环（首次延迟，等待配置与 :80 监听就绪）。
func (p *Provider) Start(ctx context.Context) {
	p.mu.Lock()
	p.ctx = ctx
	p.mu.Unlock()
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Second):
		}
		p.autoIssue(ctx)
		t := time.NewTicker(12 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				p.autoIssue(ctx)
			}
		}
	}()
}

func (p *Provider) autoIssue(ctx context.Context) {
	p.mu.Lock()
	enabled := p.mgr != nil && p.domain != "" && p.email != ""
	p.mu.Unlock()
	if !enabled {
		return
	}
	ictx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if _, err := p.issue(ictx); err != nil {
		p.log.Warn("证书自动签发/续期失败", "err", err)
	}
}

// GetCertificate 供 TLS 监听使用：手动证书优先，否则委托 ACME。
func (p *Provider) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	p.mu.Lock()
	manual := p.manual
	mgr := p.mgr
	p.mu.Unlock()
	if manual != nil {
		return manual, nil
	}
	if mgr != nil {
		return mgr.GetCertificate(hello)
	}
	return nil, errors.New("无可用证书")
}

// TLSEnabled 表示是否应以 HTTPS 提供订阅（已配域名，且有手动证书或 ACME 邮箱）。
func (p *Provider) TLSEnabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.domain != "" && (p.manual != nil || p.email != "")
}

// Domain 返回当前配置的订阅域名。
func (p *Provider) Domain() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.domain
}

// IssueNow 立即为配置的域名申请/续期证书（供「申请证书」按钮调用）。
func (p *Provider) IssueNow(ctx context.Context) (Status, error) {
	p.mu.Lock()
	domain, email, mgr := p.domain, p.email, p.mgr
	p.mu.Unlock()
	if domain == "" || email == "" || mgr == nil {
		return Status{}, errors.New("请先填写订阅域名与 ACME 邮箱并保存后再申请")
	}
	ictx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if _, err := p.issue(ictx); err != nil {
		return Status{}, err
	}
	return p.Status(ctx), nil
}

func (p *Provider) issue(ctx context.Context) (*tls.Certificate, error) {
	p.mu.Lock()
	mgr, domain := p.mgr, p.domain
	p.mu.Unlock()
	if mgr == nil {
		return nil, errors.New("ACME 未启用")
	}
	hello := &tls.ClientHelloInfo{ServerName: domain, SupportedProtos: []string{"h2", "http/1.1"}}
	return mgr.GetCertificate(hello)
}

// Status 返回当前证书状态（不会触发网络签发）。
func (p *Provider) Status(ctx context.Context) Status {
	p.mu.Lock()
	domain, manual, mgr, chalErr := p.domain, p.manual, p.mgr, p.chalErr
	p.mu.Unlock()

	if manual != nil {
		st := Status{Mode: "manual", Domain: domain}
		if leaf := leafOf(manual); leaf != nil {
			st.NotAfter = leaf.NotAfter.Unix()
			st.Issuer = leaf.Issuer.CommonName
			if st.Domain == "" && len(leaf.DNSNames) > 0 {
				st.Domain = leaf.DNSNames[0]
			}
		}
		return st
	}
	if mgr != nil && domain != "" {
		if data, err := mgr.Cache.Get(ctx, domain); err == nil {
			if c, err := parseLeafPEM(data); err == nil {
				return Status{Mode: "acme", Domain: domain, NotAfter: c.NotAfter.Unix(), Issuer: c.Issuer.CommonName}
			}
		}
		st := Status{Mode: "none", Domain: domain, Err: "尚未签发，请点击「申请证书」"}
		if chalErr != "" {
			st.Err = "80 端口挑战监听不可用：" + chalErr
		}
		return st
	}
	return Status{Mode: "none", Domain: domain}
}

// Close 停止 :80 挑战监听。
func (p *Provider) Close(ctx context.Context) {
	p.mu.Lock()
	srv := p.acmeSrv
	p.acmeSrv = nil
	p.mu.Unlock()
	if srv != nil {
		_ = srv.Shutdown(ctx)
	}
}

// leafOf 返回手动证书的叶子证书（X509KeyPair 不自动解析 Leaf）。
func leafOf(c *tls.Certificate) *x509.Certificate {
	if c.Leaf != nil {
		return c.Leaf
	}
	if len(c.Certificate) > 0 {
		if leaf, err := x509.ParseCertificate(c.Certificate[0]); err == nil {
			return leaf
		}
	}
	return nil
}

// parseLeafPEM 从 PEM 束中解析出第一个证书（叶子）。
func parseLeafPEM(data []byte) (*x509.Certificate, error) {
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			return x509.ParseCertificate(block.Bytes)
		}
	}
	return nil, errors.New("未找到证书 PEM 块")
}
