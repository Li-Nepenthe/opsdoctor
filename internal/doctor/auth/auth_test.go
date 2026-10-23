// 本文件：认证规则的边界与伪造令牌测试。
//
// 这些测试直接调用认证包，不启动登录服务。先看合法输入如何建立基线，再看算法、签发方、受众和有效期被逐项改坏后为什么必须拒绝。
//
// 主要内容：
//   - TestEmailAndPasswordValidation：保留旧仓库的输入边界和密码摘要回归测试，避免迁移时弱化认证。
//   - TestTokenConfiguration：验证启动时拒绝短密钥和无效有效期，防止签出无法使用的令牌。
//   - TestJWTRejectsAlgorithmIssuerAudienceExpiryAndTampering：验证算法、签发方、受众、有效期和篡改均被拒绝。

package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TestEmailAndPasswordValidation 保留旧仓库的输入边界和密码摘要回归测试，避免迁移时弱化认证。
func TestEmailAndPasswordValidation(t *testing.T) {
	// 表格驱动测试将有效和无效输入放在一起，便于逐项比较边界。
	for _, test := range []struct {
		input string // input 是用户输入的测试邮箱。
		want  string // want 是规范化后的预期地址。
		valid bool   // valid 表示该格式是否应该接受。
	}{
		// 允许用户误输首尾空格，并把邮箱统一成小写。
		{"  ALICE@example.com  ", "alice@example.com", true},
		// 邮件展示名不是登录账号，不能悄悄从中提取地址。
		{"Alice <alice@example.com>", "", false},
		// 没有 @ 的字符串不能被当成邮箱。
		{"missing-at", "", false},
		// 空输入必须拒绝，避免创建无法识别的账号。
		{"", "", false},
	} {
		got, err := NormalizeEmail(test.input)
		if got != test.want || (err == nil) != test.valid {
			t.Fatalf("%q=%q/%v", test.input, got, err)
		}
	}
	for _, password := range []string{
		"short",                 // 少于 8 字节，不满足本项目的最低长度。
		strings.Repeat("x", 73), // 超过 bcrypt 的 72 字节上限，不能截断后继续计算。
	} {
		if _, err := HashPassword(password); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid password accepted")
		}
	}
	// 摘要不能等于明文。同一密码能验证通过，错误密码必须失败。
	hash, err := HashPassword("valid-password")
	if err != nil || hash == "valid-password" || !VerifyPassword(hash, "valid-password") || VerifyPassword(hash, "wrong-password") {
		t.Fatal("bcrypt contract failed")
	}
}

// TestTokenConfiguration 验证启动时拒绝短密钥和无效有效期，防止签出无法使用的令牌。
func TestTokenConfiguration(t *testing.T) {
	// 先逐项破坏构造参数，再用有效配置验证签发时仍会拒绝空用户 ID。
	secret := strings.Repeat("s", minimumSecretBytes)
	if _, err := NewTokens("short", time.Hour); err == nil {
		t.Fatal("短密钥被接受")
	}
	if _, err := NewTokens(secret, 0); err == nil {
		t.Fatal("零有效期被接受")
	}
	tokens, err := NewTokens(secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.Issue(" "); !errors.Is(err, ErrInvalid) {
		t.Fatal("空主体被接受")
	}
}

// TestJWTRejectsAlgorithmIssuerAudienceExpiryAndTampering 验证算法、签发方、受众、有效期和篡改均被拒绝。
func TestJWTRejectsAlgorithmIssuerAudienceExpiryAndTampering(t *testing.T) {
	// 密钥是固定的测试假值。不会读取真实 JWT_SECRET，也不会访问网络。
	secret := strings.Repeat("s", 32)
	tokens, err := NewTokens(secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := tokens.Issue("user-a")
	if err != nil {
		t.Fatal(err)
	}
	if id, err := tokens.Parse(valid); id != "user-a" || err != nil {
		t.Fatalf("parse=%q/%v", id, err)
	}
	for _, name := range []string{"algorithm", "issuer", "audience", "expired", "missing-expiry", "empty-subject"} {
		t.Run(name, func(t *testing.T) {
			claims := jwt.RegisteredClaims{Subject: "user-a", Issuer: issuer, Audience: jwt.ClaimStrings{audience}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
			method := jwt.SigningMethodHS256
			switch name {
			case "algorithm":
				// 即使密钥正确，也不能接受服务端白名单之外的签名算法。
				method = jwt.SigningMethodHS384
			case "issuer":
				// 其他系统签发的令牌不能拿来登录 OpsDoctor。
				claims.Issuer = "elsewhere"
			case "audience":
				// 签发方正确但接收对象不同，也不能用于医生服务。
				claims.Audience = jwt.ClaimStrings{"elsewhere"}
			case "expired":
				// 过期一小时，避开时钟精度边界，确认过期令牌会被拒绝。
				claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
			case "missing-expiry":
				// 缺少过期时间不能被当作永不过期的令牌。
				claims.ExpiresAt = nil
			case "empty-subject":
				// 验签成功也必须有用户主体，否则无法知道是谁登录。
				claims.Subject = ""
			}
			token, err := jwt.NewWithClaims(method, claims).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tokens.Parse(token); !errors.Is(err, ErrUnauthorized) {
				t.Fatal("invalid JWT accepted")
			}
		})
	}
	// 保留原签名却把载荷换成空 JSON（e30），验证改过内容的令牌无法蒙混过关。
	parts := strings.Split(valid, ".")
	parts[1] = "e30"
	if _, err := tokens.Parse(strings.Join(parts, ".")); err == nil {
		t.Fatal("tampered token accepted")
	}
}
