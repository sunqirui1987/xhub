// 把控制台静态资源或上游 UI 反向代理出去。API 路径不会进这里。
package ui

import (
	"html"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/sunqirui1987/xhub/internal/auth"
)

const defaultUIOrigin = "http://127.0.0.1:3000"

// Origin 控制台地址。XHUB_UI_ORIGIN 优先，否则是 http://127.0.0.1:3000。末尾斜杠会被去掉。
func Origin() string {
	if v := strings.TrimSpace(os.Getenv("XHUB_UI_ORIGIN")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultUIOrigin
}

// NewProxy 按 Origin 建反向代理。地址无法解析时返回 nil，请求会落到控制台不可用的 HTML。
func NewProxy() http.Handler {
	u, err := url.Parse(Origin())
	if err != nil {
		return nil
	}
	p := httputil.NewSingleHostReverseProxy(u)
	origin := u.String()
	p.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		writeDashboardUnavailable(w, origin)
	}
	return p
}

// 尝试把请求交给控制台静态资源。不是 UI 路径时返回 false。
func Try(proxy http.Handler, w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	path := r.URL.Path
	if path == "" {
		path = "/"
	}

	if isDashboardPath(path) {
		proxyDashboard(proxy, w, r)
		return true
	}
	if path == "/login" || path == "/login/" {
		dest := "/ui/login/"
		if q := r.URL.RawQuery; q != "" {
			dest += "?" + q
		}
		http.Redirect(w, r, dest, http.StatusFound)
		return true
	}
	if path != "/" || !wantsBrowserDashboard(r) {
		return false
	}
	dest := "/ui/"
	if q := r.URL.RawQuery; q != "" {
		dest += "?" + q
	}
	http.Redirect(w, r, dest, http.StatusFound)
	return true
}

// wantsBrowserDashboard 根路径是否应跳到控制台。带了 API 密钥，或 Accept 只要 JSON 不要 HTML 时，返回 false，留给 API。
func wantsBrowserDashboard(r *http.Request) bool {
	if auth.APIKeyFrom(r) != "" {
		return false
	}
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html") {
		return false
	}
	return true
}

// isDashboardPath 是否为 /ui、/_next、/favicon.ico 或 /assets。这些路径转给控制台进程，其余 API 路径不转。
func isDashboardPath(path string) bool {
	switch {
	case path == "/ui", strings.HasPrefix(path, "/ui/"):
		return true
	case path == "/_next", strings.HasPrefix(path, "/_next/"):
		return true
	case path == "/favicon.ico":
		return true
	case path == "/assets", strings.HasPrefix(path, "/assets/"):
		return true
	default:
		return false
	}
}

// 反向代理控制台。API 路径不应进来。
func proxyDashboard(proxy http.Handler, w http.ResponseWriter, r *http.Request) {
	if proxy == nil {
		writeDashboardUnavailable(w, Origin())
		return
	}
	proxy.ServeHTTP(w, r)
}

// writeDashboardUnavailable 控制台进程没连上时返回 503 HTML。origin 会做 HTML 转义，避免环境变量里的字符破页面。
func writeDashboardUnavailable(w http.ResponseWriter, origin string) {
	safe := html.EscapeString(origin)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>XHub</title></head>
<body>
<h1>XHub 控制台</h1>
<p>网关已启动，但还没有连上控制台进程（默认 <code>` + safe + `</code>）。</p>
<p>另开一个终端启动 UI：</p>
<pre>cd frontend &amp;&amp; npm install &amp;&amp; npm run dev</pre>
<p>然后打开 <a href="` + safe + `/ui/login/">` + safe + `/ui/login/</a>，或刷新本页。</p>
</body>
</html>`))
}
