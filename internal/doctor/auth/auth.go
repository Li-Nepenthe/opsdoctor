// 本文件：HTTP 登录入口通过本包校验密码与登录令牌。
// Issue 签发已确认的身份，Parse 按医生服务的规则重新验证。

package auth

import (
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Error 表示 HTTP 层可以区分的认证失败类别。
type Error string

// Error 返回固定的领域文本。
func (err Error) Error() string { return string(err) }

// 调用方按错误类别决定响应，底层失败原因不向客户端公开。
const (
	ErrUnauthorized Error = "认证失败"
	ErrInvalid      Error = "认证参数不合法"
)

// 签发方和受众固定为本项目，其他服务签发的令牌不能用于登录。
const (
	issuer               = "opsdoctor"
	audience             = "opsdoctor-doctor"
	minimumSecretBytes   = 32  // HS256 至少使用 256 位密钥材料；生产环境还必须保证随机性。
	minimumPasswordBytes = 8   // 演示登录的最低输入长度；按字节计数，与 bcrypt 上限保持一致。
	maximumPasswordBytes = 72  // bcrypt 的硬上限，不能静默截断，否则不同密码可能等价。
	maximumEmailBytes    = 254 // 防止异常大的邮箱输入；与常用邮件地址长度上限对齐。
)

// Tokens 保存 JWT 签名配置。字段构造后不变，Issue 和 Parse 可以并发调用。
type Tokens struct {
	secret []byte // 构造时复制，禁止记录到日志。
	ttl    time.Duration
}

// NewTokens 在程序启动时拒绝过短密钥或不合理有效期，避免请求来了才发现签名配置不能用。
func NewTokens(secret string, ttl time.Duration) (*Tokens, error) {
	if len(secret) < minimumSecretBytes {
		return nil, fmt.Errorf("JWT_SECRET 必须至少 %d 字节", minimumSecretBytes)
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("JWT 有效期必须大于 0")
	}
	return &Tokens{secret: []byte(secret), ttl: ttl}, nil
}

// Issue 把已经确认的用户 ID 写进签名令牌。拒绝空 ID，避免生成没有身份含义的登录凭据。
func (tokens *Tokens) Issue(userID string) (string, error) {
	// 调用方只提供用户标识，签发方、受众和算法都由服务端决定。
	if strings.TrimSpace(userID) == "" {
		return "", fmt.Errorf("签发令牌：%w", ErrInvalid)
	}
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		Issuer:    issuer,
		Audience:  jwt.ClaimStrings{audience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(tokens.ttl)),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(tokens.secret)
	if err != nil {
		return "", fmt.Errorf("签发令牌：%w", err)
	}
	return signed, nil
}

// Parse 同时校验签名算法、签发方、受众、有效期和用户标识，成功返回 userID。
// 任何无效令牌都映射为 ErrUnauthorized，客户端不应据此区分签名错误还是用户不存在。
func (tokens *Tokens) Parse(token string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	// 算法由服务端白名单决定，不能听任令牌头告诉我们“该怎么验证它自己”。
	parsed, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return tokens.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(issuer), jwt.WithAudience(audience),
		jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || !parsed.Valid || strings.TrimSpace(claims.Subject) == "" {
		// 签名有效但用户 ID 为空仍不能登录。对外统一报认证失败，不透露是哪一项校验没过。
		return "", fmt.Errorf("解析令牌：%w", ErrUnauthorized)
	}
	return claims.Subject, nil
}

// NormalizeEmail 返回规范化邮箱。展示名、非法地址和过长输入返回 ErrInvalid。
// 账户邮箱统一转小写，不区分大小写。
func NormalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) > maximumEmailBytes {
		return "", fmt.Errorf("校验邮箱：%w", ErrInvalid)
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || !strings.Contains(email, "@") {
		// ParseAddress 也接受“名字 <邮箱>”。比较解析结果与原输入，确保登录只接受单纯的邮箱地址。
		return "", fmt.Errorf("校验邮箱：%w", ErrInvalid)
	}
	return email, nil
}

// HashPassword 只为合法长度的密码生成摘要，避免截断后不同密码变得等价。
func HashPassword(password string) (string, error) {
	if len(password) < minimumPasswordBytes || len(password) > maximumPasswordBytes {
		return "", fmt.Errorf("校验密码长度：%w", ErrInvalid)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("计算密码摘要：%w", err)
	}
	return string(hash), nil
}

// VerifyPassword 比较摘要和输入。不区分错误密码与损坏摘要，统一返回 false 避免泄露细节。
func VerifyPassword(hash, password string) bool {
	if len(password) < minimumPasswordBytes || len(password) > maximumPasswordBytes {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
