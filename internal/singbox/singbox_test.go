package singbox

import (
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
