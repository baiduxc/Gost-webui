package gostmgr

import (
	"strings"
	"testing"
)

func TestBuildLandingSS_TCP(t *testing.T) {
	yaml, run := BuildLandingSS("abc123", "aes-256-gcm", "pass", 8388, false)
	if !strings.Contains(yaml, "type: ss") {
		t.Fatalf("缺少 ss 服务:\n%s", yaml)
	}
	if strings.Contains(yaml, "type: ssu") {
		t.Fatalf("TCP-only 不应包含 ssu:\n%s", yaml)
	}
	if !strings.Contains(yaml, `addr: ":8388"`) {
		t.Fatalf("端口错误:\n%s", yaml)
	}
	if !strings.Contains(yaml, "username: aes-256-gcm") || !strings.Contains(yaml, `password: "pass"`) {
		t.Fatalf("凭据错误:\n%s", yaml)
	}
	if run != `gost -L "ss://aes-256-gcm:pass@:8388"` {
		t.Fatalf("启动命令错误: %s", run)
	}
}

func TestBuildLandingSS_UDP(t *testing.T) {
	yaml, run := BuildLandingSS("id2", "chacha20-ietf-poly1305", "pw", 443, true)
	if !strings.Contains(yaml, "type: ssu") {
		t.Fatalf("缺少 ssu 服务:\n%s", yaml)
	}
	if !strings.Contains(run, `ssu://chacha20-ietf-poly1305:pw@:443`) {
		t.Fatalf("UDP 启动命令错误: %s", run)
	}
}
