package server

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

func uiOrigin() string {
	if v := strings.TrimSpace(os.Getenv("XHUB_UI_ORIGIN")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultUIOrigin
}

func newUIProxy() http.Handler {
	u, err := url.Parse(uiOrigin())
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

func (s *Server) tryDashboard(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	path := r.URL.Path
	if path == "" {
		path = "/"
	}

	if isDashboardPath(path) {
		s.proxyDashboard(w, r)
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

func (s *Server) proxyDashboard(w http.ResponseWriter, r *http.Request) {
	if s.uiProxy == nil {
		writeDashboardUnavailable(w, uiOrigin())
		return
	}
	s.uiProxy.ServeHTTP(w, r)
}

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
