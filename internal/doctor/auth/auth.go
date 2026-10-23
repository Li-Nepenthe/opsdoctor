package auth

import (
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Error 是可比较的领域错误；用常量而不是可被重新赋值的包级变量，方便 errors.Is 判断。
type Error string

// Error 实现标准库 error 接口；不阻塞，不包含调用方的密码或令牌。
func (err Error) Error() string { return string(err) }

// 错误分类供后续 HTTP 层映射状态码；这里不依赖 HTTP，也不泄露失败的具体凭据。
const (
	ErrUnauthorized Error = "认证失败"
	ErrInvalid      Error = "认证参数不合法"
)

// JWT 与密码边界延续旧仓库的安全约束；issuer/audience 改为当前项目，拒绝旧项目的令牌。
const (
	issuer               = "opsdoctor"
	audience             = "opsdoctor-doctor"
	minimumSecretBytes   = 32  // HS256 至少使用 256 位密钥材料；生产环境还必须保证随机性。
	minimumPasswordBytes = 8   // 演示登录的最低输入长度；按字节计数，与 bcrypt 上限保持一致。
	maximumPasswordBytes = 72  // bcrypt 的硬上限，不能静默截断，否则不同密码可能等价。
	maximumEmailBytes    = 254 // 防止异常大的邮箱输入；与常用邮件地址长度上限对齐。
)

// Tokens 保存 JWT 签名配置；字段构造后不变，Issue 和 Parse 可以并发调用。
type Tokens struct {
	secret []byte        // secret 是构造时复制的密钥，不向外暴露也不写日志。
	ttl    time.Duration // ttl 是令牌存活时长，必须大于 0。
}

// NewTokens 校验密钥长度和有效期；无效时返回配置错误，不回显密钥，不执行 IO。
func NewTokens(secret string, ttl time.Duration) (*Tokens, error) {
	// 构造时就验证这些不变量，避免服务启动后才发现无法安全签发令牌。
	if len(secret) < minimumSecretBytes {
		return nil, fmt.Errorf("JWT_SECRET 必须至少 %d 字节", minimumSecretBytes)
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("JWT 有效期必须大于 0")
	}
	return &Tokens{secret: []byte(secret), ttl: ttl}, nil
}

// Issue 为非空 userID 签发令牌；返回签名字符串或带上下文的签名错误，不执行网络请求。
func (tokens *Tokens) Issue(userID string) (string, error) {
	// 先拒绝空用户标识，再构造服务端决定的 claims；调用方不能自选 issuer 或算法。
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
// 任何无效令牌都映射为 ErrUnauthorized，避免向客户端泄露内部校验细节；可并发调用。
func (tokens *Tokens) Parse(token string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	// 算法由服务端白名单决定，不能听任令牌头告诉我们“该怎么验证它自己”。
	parsed, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return tokens.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(issuer), jwt.WithAudience(audience),
		jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || !parsed.Valid || strings.TrimSpace(claims.Subject) == "" {
		// 签名有效但用户 ID 为空仍不能登录；对外统一报认证失败，不透露是哪一项校验没过。
		return "", fmt.Errorf("解析令牌：%w", ErrUnauthorized)
	}
	return claims.Subject, nil
}

// NormalizeEmail 返回规范化邮箱；展示名、非法地址和过长输入返回 ErrInvalid。
// 为保持旧仓库的登录习惯，整个地址转小写；本项目不支持区分大小写的邮箱账户。
func NormalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) > maximumEmailBytes {
		// 在解析前拒绝异常大的输入，长度按 UTF-8 字节计算。
		return "", fmt.Errorf("校验邮箱：%w", ErrInvalid)
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || !strings.Contains(email, "@") {
		// ParseAddress 也接受“名字 <邮箱>”；比较解析结果与原输入，确保登录只接受单纯的邮箱地址。
		return "", fmt.Errorf("校验邮箱：%w", ErrInvalid)
	}
	return email, nil
}

// HashPassword 用 bcrypt 的默认成本计算摘要；输入越界返回 ErrInvalid，底层错误保留原因。
// bcrypt 故意消耗 CPU 来提高穷举成本，本函数同步等待计算完成；不同调用之间不共享状态。
func HashPassword(password string) (string, error) {
	// 先检查 bcrypt 的输入边界；尤其不能靠截断长密码来“兼容”，那会让不同密码变得等价。
	if len(password) < minimumPasswordBytes || len(password) > maximumPasswordBytes {
		return "", fmt.Errorf("校验密码长度：%w", ErrInvalid)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("计算密码摘要：%w", err)
	}
	return string(hash), nil
}

// VerifyPassword 比较摘要和输入；不区分错误密码与损坏摘要，统一返回 false 避免泄露细节。
// 它同步消耗 CPU，可并发调用；先拒绝过长输入，绝不依赖隐式截断。
func VerifyPassword(hash, password string) bool {
	if len(password) < minimumPasswordBytes || len(password) > maximumPasswordBytes {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
