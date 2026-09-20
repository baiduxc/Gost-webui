package link

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestParseVmessWS(t *testing.T) {
	obj := map[string]any{
		"v": "2", "ps": "US-01", "add": "us.example.com", "port": "443",
		"id": "b831381d-6324-4d53-ad4f-8cda48b30811", "aid": "0", "scy": "auto",
		"net": "ws", "type": "none", "host": "us.example.com", "path": "/ws",
		"tls": "tls", "sni": "us.example.com",
	}
	b, _ := json.Marshal(obj)
	raw := "vmess://" + base64.StdEncoding.EncodeToString(b)

	info, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if info.Scheme != "vmess" || info.Host != "us.example.com" || info.Port != 443 {
		t.Fatalf("解析错误: %+v", info)
	}
	if info.Transport != "ws" || !info.TLS || info.UDP {
		t.Fatalf("属性错误: %+v", info)
	}

	out, err := info.Rewrite("1.2.3.4", 20001, "US-01-中转", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "vmess://") {
		t.Fatalf("前缀错误: %s", out)
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(out, "vmess://"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["add"] != "1.2.3.4" || m["port"] != "20001" || m["ps"] != "US-01-中转" {
		t.Fatalf("改写错误: %v", m)
	}
	if m["sni"] != "us.example.com" || m["host"] != "us.example.com" {
		t.Fatalf("SNI/Host 应保持原域名: %v", m)
	}
	if m["id"] != obj["id"] || m["path"] != "/ws" {
		t.Fatalf("其它字段被破坏: %v", m)
	}
}

func TestParseVmessNoPadding(t *testing.T) {
	obj := map[string]any{"v": "2", "ps": "n", "add": "1.1.1.1", "port": 8443, "id": "x", "net": "tcp", "tls": "none"}
	b, _ := json.Marshal(obj)
	raw := "vmess://" + base64.RawStdEncoding.EncodeToString(b)
	info, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if info.Port != 8443 || info.TLS {
		t.Fatalf("%+v", info)
	}
}

func TestParseVlessReality(t *testing.T) {
	raw := "vless://11111111-2222-3333-4444-555555555555@us.example.com:443" +
		"?type=tcp&security=reality&sni=www.microsoft.com&fp=chrome&pbk=abcdef&sid=00&flow=xtls-rprx-vision#%F0%9F%87%BA%F0%9F%87%B8%20US-02"
	info, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if info.Scheme != "vless" || info.Port != 443 || !info.TLS {
		t.Fatalf("%+v", info)
	}
	if info.Name != "🇺🇸 US-02" {
		t.Fatalf("备注解析错误: %q", info.Name)
	}
	out, err := info.Rewrite("relay.example.com", 30001, "中转-US-02", "", "")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "relay.example.com:30001" {
		t.Fatalf("host 改写错误: %s", u.Host)
	}
	q := u.Query()
	if q.Get("sni") != "www.microsoft.com" || q.Get("pbk") != "abcdef" || q.Get("flow") != "xtls-rprx-vision" {
		t.Fatalf("参数丢失: %v", q)
	}
	if u.User.Username() != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("uuid 丢失: %v", u.User)
	}
	if u.Fragment != "中转-US-02" {
		t.Fatalf("备注错误: %q", u.Fragment)
	}
}

func TestParseVlessWSNoSNI(t *testing.T) {
	raw := "vless://uuid@cdn.example.com:443?type=ws&security=tls&path=%2Fws#name"
	info, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	out, err := info.Rewrite("1.2.3.4", 10086, "relay", "", "")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(out)
	q := u.Query()
	if q.Get("sni") != "cdn.example.com" || q.Get("host") != "cdn.example.com" {
		t.Fatalf("应自动补 SNI/Host: %s", out)
	}
	if q.Get("path") != "/ws" || q.Get("type") != "ws" {
		t.Fatalf("原有参数丢失: %s", out)
	}
}

func TestParseTrojan(t *testing.T) {
	raw := "trojan://mypassword@us.example.com:443?sni=us.example.com&type=tcp#US-Trojan"
	info, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !info.TLS || info.Scheme != "trojan" {
		t.Fatalf("%+v", info)
	}
	out, err := info.Rewrite("2.2.2.2", 40001, "trojan-relay", "", "")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(out)
	if u.Host != "2.2.2.2:40001" || u.User.Username() != "mypassword" {
		t.Fatalf("改写错误: %s", out)
	}
}

func TestParseSSSIP002(t *testing.T) {
	cred := base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:pass123"))
	raw := "ss://" + cred + "@us.example.com:8388#SS-Node"
	info, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if info.Scheme != "ss" || info.Port != 8388 || info.TLS {
		t.Fatalf("%+v", info)
	}
	out, err := info.Rewrite("3.3.3.3", 50001, "ss-relay", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "@3.3.3.3:50001") {
		t.Fatalf("改写错误: %s", out)
	}
	if !strings.Contains(out, url.QueryEscape("SS-Node")) && !strings.Contains(out, "ss-relay") {
		t.Fatalf("备注错误: %s", out)
	}
}

func TestParseSSLegacy(t *testing.T) {
	raw := "ss://" + base64.StdEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:pw@us.example.com:8388")) + "#legacy"
	info, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if info.Host != "us.example.com" || info.Port != 8388 {
		t.Fatalf("%+v", info)
	}
	out, err := info.Rewrite("4.4.4.4", 50002, "legacy-relay", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "@4.4.4.4:50002") {
		t.Fatalf("改写错误: %s", out)
	}
	// 凭据应可解出
	head := strings.TrimPrefix(out, "ss://")
	head = head[:strings.IndexByte(head, '@')]
	dec, err := base64.StdEncoding.DecodeString(head)
	if err != nil {
		t.Fatal(err)
	}
	if string(dec) != "chacha20-ietf-poly1305:pw" {
		t.Fatalf("凭据错误: %s", dec)
	}
}

func TestParseHysteria2(t *testing.T) {
	raw := "hysteria2://mypass@us.example.com:8443?sni=us.example.com&insecure=1#HY2"
	info, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !info.UDP {
		t.Fatal("hysteria2 应该需要 UDP 转发")
	}
	out, err := info.Rewrite("5.5.5.5", 60001, "hy2-relay", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "5.5.5.5:60001") || !strings.Contains(out, "insecure=1") {
		t.Fatalf("改写错误: %s", out)
	}
}

func TestParseTuic(t *testing.T) {
	raw := "tuic://uuid:password@us.example.com:443?congestion_control=bbr&alpn=h3#TUIC"
	info, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !info.UDP || info.Scheme != "tuic" {
		t.Fatalf("%+v", info)
	}
	out, err := info.Rewrite("6.6.6.6", 60002, "tuic-relay", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "6.6.6.6:60002") || !strings.Contains(out, "congestion_control=bbr") {
		t.Fatalf("改写错误: %s", out)
	}
}

func TestParseIPv6(t *testing.T) {
	raw := "vless://uuid@[2001:db8::1]:443?security=tls&sni=example.com#v6"
	info, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if info.Host != "2001:db8::1" || info.Port != 443 {
		t.Fatalf("%+v", info)
	}
	out, err := info.Rewrite("2001:db8::2", 2443, "v6-relay", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[2001:db8::2]:2443") {
		t.Fatalf("IPv6 改写错误: %s", out)
	}
}

func TestInvalid(t *testing.T) {
	for _, raw := range []string{"", "hello", "vmess://not-base64!!!", "vless://uuid@host"} {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("应该报错: %q", raw)
		}
	}
}
