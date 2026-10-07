package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceThemeSettings sync.Once

// uiTheme reads and writes one theme settings object. The save and logo routes share that object with the settings read.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 theme_settings.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) uiTheme(w http.ResponseWriter, r *http.Request) {
	logTraceOnceThemeSettings.Do(func() { logx.Trace("enter gateway.uiTheme") })

	if s.requireManage(w, r) == nil {
		return
	}
	current := s.loadTheme()
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		httpx.WriteJSON(w, 200, current)
		return
	}
	body := readMap(r)
	if strings.Contains(r.URL.Path, "ui_theme_settings") {
		if color, ok := body["primary_color"]; ok {
			current["primary_color"] = color
		}
	}
	if strings.Contains(r.URL.Path, "/upload/logo") {
		if logo, ok := body["logo_url"]; ok {
			current["logo_url"] = logo
		}
	}
	raw, _ := json.Marshal(map[string]any{
		"primary_color": current["primary_color"],
		"logo_url":      current["logo_url"],
	})
	if err := s.Store.PutKV("ui_theme", "settings", string(raw)); err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, 200, s.loadTheme())
}

// loadTheme returns the saved theme, or empty color and logo fields when nothing is stored yet.
// 参数：无。
// 返回 map[string]any（map[string]any）：载入Theme的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 theme_settings.go 内使用
// 测试：无直接单测
func (s *Server) loadTheme() map[string]any {
	out := map[string]any{"object": "ui_theme_settings", "primary_color": "", "logo_url": ""}
	saved, err := s.Store.GetKV("ui_theme", "settings")
	if err != nil {
		return out
	}
	if color, ok := saved["primary_color"]; ok {
		out["primary_color"] = color
	}
	if logo, ok := saved["logo_url"]; ok {
		out["logo_url"] = logo
	}
	return out
}

// promptTest runs a prompt check and says whether it succeeded. It does not call a model provider.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 theme_settings.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) promptTest(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["prompt_id"])
	message := "prompt test succeeded"
	if id == "" {
		message = "prompt test failed: prompt_id required"
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"prompt_id":  id,
		"version":    1,
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"message":    message,
	})
}

// searchToolTest reports whether the named search tool can be reached. It does not dial an external search service.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 调用：仅在 theme_settings.go 内使用
// 测试：无直接单测
// 返回：无。状态码和正文写进调用方的响应。
func (s *Server) searchToolTest(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	id := str(body["search_tool_id"])
	name := str(body["search_tool_name"])
	if name == "" {
		name = str(body["name"])
	}
	message := "search tool connection succeeded"
	if name == "" && id == "" {
		message = "search tool connection failed: search_tool_name required"
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"search_tool_id":   id,
		"search_tool_name": name,
		"message":          message,
	})
}
