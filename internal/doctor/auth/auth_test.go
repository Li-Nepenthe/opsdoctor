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
		{"  ALICE@example.com  ", "alice@example.com", true},
		{"Alice <alice@example.com>", "", false},
		{"missing-at", "", false}, {"", "", false},
	} {
		got, err := NormalizeEmail(test.input)
		if got != test.want || (err == nil) != test.valid {
			t.Fatalf("%q=%q/%v", test.input, got, err)
		}
	}
	for _, password := range []string{"short", strings.Repeat("x", 73)} {
		if _, err := HashPassword(password); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid password accepted")
		}
	}
	hash, err := HashPassword("valid-password")
	if err != nil || hash == "valid-password" || !VerifyPassword(hash, "valid-password") || VerifyPassword(hash, "wrong-password") {
		t.Fatal("bcrypt contract failed")
	}
}

// TestTokenConfiguration 验证启动时拒绝短密钥和无效有效期，防止签出无法使用的令牌。
func TestTokenConfiguration(t *testing.T) {
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
	// 密钥是固定的测试假值；不会读取真实 JWT_SECRET，也不会访问网络。
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
				method = jwt.SigningMethodHS384
			case "issuer":
				claims.Issuer = "elsewhere"
			case "audience":
				claims.Audience = jwt.ClaimStrings{"elsewhere"}
			case "expired":
				claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
			case "missing-expiry":
				claims.ExpiresAt = nil
			case "empty-subject":
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
	parts := strings.Split(valid, ".")
	parts[1] = "e30"
	if _, err := tokens.Parse(strings.Join(parts, ".")); err == nil {
		t.Fatal("tampered token accepted")
	}
}
