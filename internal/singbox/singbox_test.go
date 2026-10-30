package singbox

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGeneratedConfigValid 保证面板生成的 reality 配置能被 sing-box 官方二进制接受。
// 若本机 /tmp 下没有测试用 sing-box 则跳过（CI/预览环境手动放置）。
func TestGeneratedConfigValid(t *testing.T) {
	bin := os.Getenv("SINGBOX_TEST_BIN")
	if bin == "" {
		t.Skip("SINGBOX_TEST_BIN not set")
	}
	priv, pub, err := GenerateRealityKeypair()
	if err != nil {
		t.Fatal(err)
	}
	cred := &RealityCred{
		UUID: GenerateUUID(), PrivateKey: priv, PublicKey: pub,
		ShortID: GenerateShortID(), ServerName: "www.microsoft.com",
	}
	b, err := buildInstanceConfig("node-test", 24443, cred, "127.0.0.1:19399")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "config.json")
	if err := os.WriteFile(f, b, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "check", "-c", f).CombinedOutput()
	if err != nil {
		t.Fatalf("sing-box check failed: %v\n%s\nconfig:\n%s", err, out, b)
	}
}

// TestRealityKeypairRoundtrip 校验密钥对生成格式与 sing-box/xray 兼容（base64url 43 字符）。
func TestRealityKeypairRoundtrip(t *testing.T) {
	priv, pub, err := GenerateRealityKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if len(priv) != 43 || len(pub) != 43 {
		t.Fatalf("unexpected key lengths: priv=%d pub=%d", len(priv), len(pub))
	}
	sid := GenerateShortID()
	if len(sid) != 8 {
		t.Fatalf("short id len = %d, want 8", len(sid))
	}
	u := GenerateUUID()
	if len(u) != 36 || u[8] != '-' {
		t.Fatalf("bad uuid: %s", u)
	}
}

func TestClientURL(t *testing.T) {
	c := &RealityCred{UUID: "u-1", PublicKey: "PUB", ShortID: "ab12cd34", ServerName: "www.microsoft.com"}
	got := ClientURL(c, 8443, "1.2.3.4", "新加坡 节点")
	want := "vless://u-1@1.2.3.4:8443?encryption=none&security=reality&type=tcp&sni=www.microsoft.com&fp=chrome&pbk=PUB&sid=ab12cd34&flow=xtls-rprx-vision#%E6%96%B0%E5%8A%A0%E5%9D%A1%20%E8%8A%82%E7%82%B9"
	if got != want {
		t.Fatalf("ClientURL:\n got %s\nwant %s", got, want)
	}
}

// TestClientURLIPv6 确认 IPv6 地址写成 [addr]:port。
func TestClientURLIPv6(t *testing.T) {
	c := &RealityCred{UUID: "u-1", PublicKey: "PUB", ShortID: "ab12cd34", ServerName: "www.microsoft.com"}
	got := ClientURL(c, 8443, "2001:db8::1", "n")
	want := "vless://u-1@[2001:db8::1]:8443?encryption=none&security=reality&type=tcp&sni=www.microsoft.com&fp=chrome&pbk=PUB&sid=ab12cd34&flow=xtls-rprx-vision#n"
	if got != want {
		t.Fatalf("ClientURL ipv6:\n got %s\nwant %s", got, want)
	}
}

// TestBuildInstanceConfigListenAddr 校验地址族决定入站监听地址，出站不受影响。
func TestBuildInstanceConfigListenAddr(t *testing.T) {
	priv, pub, err := GenerateRealityKeypair()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ mode, want string }{
		{"", "::"},
		{"auto", "::"},
		{"ipv4", "0.0.0.0"},
		{"ipv6", "::"},
	}
	for _, tc := range cases {
		cred := &RealityCred{
			UUID: GenerateUUID(), PrivateKey: priv, PublicKey: pub,
			ShortID: GenerateShortID(), ServerName: "www.microsoft.com", IPMode: tc.mode,
		}
		b, err := buildInstanceConfig("node-test", 24443, cred, "127.0.0.1:19399")
		if err != nil {
			t.Fatalf("mode %q: %v", tc.mode, err)
		}
		var cfg struct {
			Inbounds []struct {
				Listen string `json:"listen"`
			} `json:"inbounds"`
			Outbounds []struct {
				Type string `json:"type"`
			} `json:"outbounds"`
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			t.Fatalf("mode %q: %v", tc.mode, err)
		}
		if len(cfg.Inbounds) != 1 || cfg.Inbounds[0].Listen != tc.want {
			t.Fatalf("mode %q listen mismatch: %s", tc.mode, b)
		}
		if len(cfg.Outbounds) != 1 || cfg.Outbounds[0].Type != "direct" {
			t.Fatalf("mode %q outbound changed: %s", tc.mode, b)
		}
	}
}
