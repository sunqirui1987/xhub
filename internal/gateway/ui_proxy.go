// Package gateway reverse-proxies dashboard assets to the upstream UI. API paths do not enter this file.
package gateway

import (
	"html"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

const defaultUIOrigin = "http://127.0.0.1:3000"

var logTraceOnceUiProxy sync.Once

// Origin is the dashboard address. XHUB_UI_ORIGIN wins, otherwise it is http://127.0.0.1:3000. A trailing slash is removed.
func Origin() string {
	logTraceOnceUiProxy.Do(func() { logx.Trace("enter gateway.Origin") })

	if v := strings.TrimSpace(os.Getenv("XHUB_UI_ORIGIN")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultUIOrigin
}

// NewProxy builds a reverse proxy for Origin. An unparseable address returns nil, and requests then get the dashboard-unavailable HTML.
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

// Try hands the request to the dashboard assets. It returns false when the path is not a UI path.
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

// wantsBrowserDashboard reports whether the root path should go to the dashboard. An API key, or an Accept that wants JSON and not HTML, returns false so the API keeps the request.
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

// isDashboardPath reports /ui, /_next, /favicon.ico, and /assets. Those paths go to the console process. Other API paths do not.
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

// proxyDashboard reverse-proxies the dashboard. API paths should not reach it.
func proxyDashboard(proxy http.Handler, w http.ResponseWriter, r *http.Request) {
	if proxy == nil {
		writeDashboardUnavailable(w, Origin())
		return
	}
	proxy.ServeHTTP(w, r)
}

// writeDashboardUnavailable returns 503 HTML when the console process is unreachable. origin is HTML-escaped so a character in the environment variable cannot break the page.
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
