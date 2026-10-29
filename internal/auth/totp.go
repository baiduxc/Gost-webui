// Package auth 实现面板的两步验证基础算法：
// RFC 6238 TOTP 动态口令、一次性恢复码与随机令牌。
//
// 这里只做纯计算，不涉及存储与 HTTP，便于单元测试与复用。
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"
)

const (
	// Digits 是动态码位数（Google Authenticator 等 App 通用 6 位）。
	Digits = 6
	// Period 是动态码有效期。
	Period = 30 * time.Second
	// Skew 允许前后各一个时间窗，抵消客户端与服务器时钟偏差。
	Skew = 1
	// secretBytes 是密钥长度：160 bit（RFC 4226 推荐）。
	secretBytes = 20

	// RecoveryCodeLen 是恢复码有效字符数。
	RecoveryCodeLen = 10
	// recoveryAlphabet 去掉了 0/O/1/I 等易混淆字符。
	recoveryAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret 生成 Base32（无填充）编码的随机密钥。
func GenerateSecret() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

// NormalizeSecret 规范化用户/配置传入的密钥：转大写并去掉空格与短横线。
func NormalizeSecret(secret string) string {
	return strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "\t", "").Replace(secret))
}

// FormatSecret 把密钥按 4 位分组，便于人工抄写。
func FormatSecret(secret string) string {
	s := NormalizeSecret(secret)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// URI 生成 otpauth:// 链接，供验证器 App 扫码或手动录入。
func URI(issuer, account, secret string) string {
	q := url.Values{}
	q.Set("secret", NormalizeSecret(secret))
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(Digits))
	q.Set("period", fmt.Sprint(int(Period/time.Second)))
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + q.Encode()
}

// counter 返回 t 对应的时间步。
func counter(t time.Time) int64 {
	return t.Unix() / int64(Period/time.Second)
}

// hotp 按 RFC 4226 计算 counter 时刻的动态码。
func hotp(key []byte, c int64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(c))
	m := hmac.New(sha1.New, key)
	m.Write(buf[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", Digits, v%1_000_000)
}

// Code 计算 t 时刻的动态码，并返回所用时间步。
func Code(secret string, t time.Time) (string, int64, error) {
	key, err := b32.DecodeString(NormalizeSecret(secret))
	if err != nil {
		return "", 0, fmt.Errorf("密钥格式不正确: %w", err)
	}
	c := counter(t)
	return hotp(key, c), c, nil
}

// Verify 校验动态码，返回命中的时间步。
// 允许 ±Skew 个时间窗；调用方需用返回的时间步做防重放。
func Verify(secret, code string, t time.Time) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return 0, false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	key, err := b32.DecodeString(NormalizeSecret(secret))
	if err != nil || len(key) == 0 {
		return 0, false
	}
	c := counter(t)
	for d := int64(-Skew); d <= Skew; d++ {
		if subtle.ConstantTimeCompare([]byte(hotp(key, c+d)), []byte(code)) == 1 {
			return c + d, true
		}
	}
	return 0, false
}

// Step 返回 t 当前所处的时间步。
func Step(t time.Time) int64 { return counter(t) }

// NewRecoveryCodes 生成 n 个一次性恢复码（10 位大写字符）。
func NewRecoveryCodes(n int) ([]string, error) {
	if n <= 0 {
		n = 10
	}
	out := make([]string, 0, n)
	max := big.NewInt(int64(len(recoveryAlphabet)))
	for i := 0; i < n; i++ {
		var buf [RecoveryCodeLen]byte
		for j := range buf {
			v, err := rand.Int(rand.Reader, max)
			if err != nil {
				return nil, err
			}
			buf[j] = recoveryAlphabet[v.Int64()]
		}
		out = append(out, string(buf[:]))
	}
	return out, nil
}

// NormalizeRecoveryCode 规范化恢复码：去空格/短横线并转大写。
func NormalizeRecoveryCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(code) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// FormatRecoveryCode 把恢复码按 5 位分组展示。
func FormatRecoveryCode(code string) string {
	s := NormalizeRecoveryCode(code)
	if len(s) <= 5 {
		return s
	}
	return s[:5] + "-" + s[5:]
}

// HashRecoveryCode 返回恢复码的 SHA-256 十六进制摘要。
// 恢复码本身有约 50 bit 熵，且登录失败受严格限流，摘要存储足以防止明文泄露。
func HashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(NormalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}

// MatchRecoveryCode 在 hashes 中查找恢复码，返回下标（未命中返回 -1）。
// 遍历全部条目并做常量时间比较，避免通过耗时泄露命中位置。
func MatchRecoveryCode(code string, hashes []string) int {
	want := HashRecoveryCode(code)
	idx := -1
	for i, h := range hashes {
		if subtle.ConstantTimeCompare([]byte(want), []byte(strings.ToLower(strings.TrimSpace(h)))) == 1 && idx < 0 {
			idx = i
		}
	}
	return idx
}

// NewToken 生成 n 字节随机数的十六进制令牌。
func NewToken(n int) (string, error) {
	if n <= 0 {
		n = 32
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
