package auth

import (
	"strings"
	"testing"
	"time"
)

// RFC 6238 附录 B 的测试密钥（ASCII "12345678901234567890"）。
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestCodeRFC6238(t *testing.T) {
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	}
	for _, c := range cases {
		got, _, err := Code(rfcSecret, time.Unix(c.unix, 0).UTC())
		if err != nil {
			t.Fatalf("Code(%d) 出错: %v", c.unix, err)
		}
		if got != c.want {
			t.Errorf("Code(%d) = %s, 期望 %s", c.unix, got, c.want)
		}
	}
}

func TestVerifySkewAndReplay(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	code, step, err := Code(rfcSecret, now)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := Verify(rfcSecret, code, now); !ok || got != step {
		t.Fatalf("当前时间窗校验失败: ok=%v step=%d want=%d", ok, got, step)
	}

	next, _, _ := Code(rfcSecret, now.Add(Period))
	if got, ok := Verify(rfcSecret, next, now); !ok || got != step+1 {
		t.Fatalf("允许 +1 时间窗: ok=%v step=%d", ok, got)
	}

	prev, _, _ := Code(rfcSecret, now.Add(-Period))
	if got, ok := Verify(rfcSecret, prev, now); !ok || got != step-1 {
		t.Fatalf("允许 -1 时间窗: ok=%v step=%d", ok, got)
	}

	far, _, _ := Code(rfcSecret, now.Add(3*Period))
	if _, ok := Verify(rfcSecret, far, now); ok {
		t.Fatal("超出容差的时间窗不应通过")
	}

	for _, bad := range []string{"", "12345", "1234567", "abcdef", "12345 6"} {
		if _, ok := Verify(rfcSecret, bad, now); ok {
			t.Errorf("非法验证码 %q 不应通过", bad)
		}
	}

	if _, ok := Verify("not-base32!!", code, now); ok {
		t.Fatal("非法密钥不应通过")
	}
}

func TestSecretFormatAndURI(t *testing.T) {
	s, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 32 {
		t.Fatalf("密钥长度异常: %d", len(s))
	}
	if f := FormatSecret(s); strings.Contains(f, "-") || len(strings.ReplaceAll(f, " ", "")) != 32 {
		t.Fatalf("分组格式异常: %s", f)
	}
	if NormalizeSecret("abcd efgh-ijkl") != "ABCDEFGHIJKL" {
		t.Fatal("NormalizeSecret 结果异常")
	}
	uri := URI("Gost-WebUI", "admin", s)
	if !strings.HasPrefix(uri, "otpauth://totp/") || !strings.Contains(uri, "secret="+s) {
		t.Fatalf("otpauth 链接异常: %s", uri)
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, err := NewRecoveryCodes(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 10 {
		t.Fatalf("期望 10 个恢复码，得到 %d", len(codes))
	}
	seen := map[string]bool{}
	hashes := make([]string, 0, len(codes))
	for _, c := range codes {
		if len(c) != RecoveryCodeLen {
			t.Fatalf("恢复码长度异常: %s", c)
		}
		if seen[c] {
			t.Fatalf("恢复码重复: %s", c)
		}
		seen[c] = true
		hashes = append(hashes, HashRecoveryCode(c))
	}
	// 手输容错：大小写、空格、短横线都应能匹配
	idx := MatchRecoveryCode(strings.ToLower(FormatRecoveryCode(codes[3])), hashes)
	if idx != 3 {
		t.Fatalf("恢复码匹配失败: idx=%d", idx)
	}
	if MatchRecoveryCode("AAAAAAAAAA", hashes) != -1 {
		t.Fatal("不存在的恢复码不应匹配")
	}
	if strings.Count(FormatRecoveryCode(codes[0]), "-") != 1 {
		t.Fatal("展示格式应带一个短横线")
	}
}
