// 本文件：为浏览器提供 Cookie 登录和防跨站请求保护，命令行继续使用 Bearer。
//
// EventSource 不能设置 Authorization 头，令牌又不能放进地址。因此浏览器只通过
// HttpOnly Cookie 携带身份；修改请求另带 CSRF 值，外站读不到这个值就不能冒用登录。
//
// 主要内容：sameOrigin 拒绝跨站来源；csrfToken 绑定会话；session 支持刷新页面；logout 退出。
// 阅读建议：页面不保存 JWT 到 localStorage，也不会把它放进 SSE 地址。

package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/gin-gonic/gin"
)

// sessionCookie 是同一站点的会话名，不包含用户标识或任何凭据。
const sessionCookie = "opsdoctor_session"

// sameOrigin 在浏览器提供 Origin 时严格匹配；无 Origin 的 CLI 仍须通过正文格式和身份校验。
func sameOrigin(ctx *gin.Context) bool {
	if ctx.GetHeader("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin := ctx.GetHeader("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	scheme := "http"
	if ctx.Request.TLS != nil {
		scheme = "https"
	}
	return err == nil && parsed.Scheme == scheme && parsed.Host == ctx.Request.Host && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && parsed.Path == ""
}

// csrfToken 派生值只绑定当前不可猜测的签名 JWT；知道自己的值不能推导另一个会话的值。
func csrfToken(token string) string {
	sum := sha256.Sum256([]byte("opsdoctor/csrf/v1\x00" + token))
	return hex.EncodeToString(sum[:])
}

// validCSRF 使用固定时间比较，不按错误位置提前退出。
func validCSRF(token, value string) bool {
	return subtle.ConstantTimeCompare([]byte(csrfToken(token)), []byte(value)) == 1
}

// setSession 的 HttpOnly 禁止脚本读身份，SameSite=Strict 让跨站导航也不自动带登录。
func (server *api) setSession(ctx *gin.Context, token string, age int) {
	http.SetCookie(ctx.Writer, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: age, HttpOnly: true, Secure: server.options.SecureCookie || ctx.Request.TLS != nil, SameSite: http.SameSiteStrictMode})
}

// session 返回修改请求所需的派生值，浏览器刷新后仍不用把 JWT 暴露给脚本。
func (server *api) session(ctx *gin.Context) {
	httpx.Success(ctx, 200, map[string]string{"csrf_token": csrfToken(ctx.GetString("login_token"))})
}

// logout 通过同样的 CSRF 校验清 Cookie，防止外站强迫用户退出。
func (server *api) logout(ctx *gin.Context) {
	server.setSession(ctx, "", -1)
	httpx.Success(ctx, 200, map[string]bool{"logged_out": true})
}

// browserHeaders 禁止缓存含凭据的响应，也禁止把不可信正文当脚本或嵌入其他站点。
func browserHeaders(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("X-Content-Type-Options", "nosniff")
	ctx.Header("Referrer-Policy", "no-referrer")
	ctx.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	ctx.Next()
}
