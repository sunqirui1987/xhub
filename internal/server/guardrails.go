package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
)

func (s *Server) applyGuardrailHTTP(w http.ResponseWriter, r *http.Request) {
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	name := str(body["guardrail_name"])
	if name == "" {
		name = str(body["guardrail"])
	}
	text := guardrailText(body)
	action, out := s.evalNamedGuardrail(name, text)
	httpx.WriteJSON(w, 200, map[string]any{
		"status":         "success",
		"guardrail_name": name,
		"guardrail_id":   name,
		"created_at":     time.Now().UTC().Format(time.RFC3339),
		"action":         action,
		"blocked":        action == "block",
		"text":           out,
		"output":         map[string]any{"text": out},
	})
}

func (s *Server) preCallGuardrails(body map[string]any) (bool, string) {
	text := guardrailText(body)
	list := s.listGuardrails()
	for _, g := range list {
		params, _ := g["litellm_params"].(map[string]any)
		if params == nil {
			params = map[string]any{}
		}
		if !boolOf(params["default_on"]) && !boolOf(g["default_on"]) {
			continue
		}
		mode := str(params["mode"])
		if mode != "" && mode != "pre_call" {
			continue
		}
		action, _ := matchGuardrail(g, text)
		if action == "block" {
			name := str(g["guardrail_name"])
			if name == "" {
				name = str(g["name"])
			}
			if name == "" {
				name = "guardrail"
			}
			return true, "Guardrail blocked the request: " + name
		}
	}
	return false, ""
}

func (s *Server) evalNamedGuardrail(name, text string) (action, out string) {
	if name != "" {
		for _, g := range s.listGuardrails() {
			n := str(g["guardrail_name"])
			if n == "" {
				n = str(g["name"])
			}
			if n == name || str(g["id"]) == name || str(g["guardrail_id"]) == name {
				return matchGuardrail(g, text)
			}
		}
	}
	for _, g := range s.listGuardrails() {
		if boolOf(g["default_on"]) {
			return matchGuardrail(g, text)
		}
		if params, _ := g["litellm_params"].(map[string]any); boolOf(params["default_on"]) {
			return matchGuardrail(g, text)
		}
	}
	return "allow", text
}

func (s *Server) listGuardrails() []map[string]any {
	var out []map[string]any
	for _, kind := range []string{"guardrails", "guardrail"} {
		list, _ := s.Store.ListKV(kind)
		out = append(out, list...)
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out
}

func matchGuardrail(g map[string]any, text string) (action, out string) {
	params, _ := g["litellm_params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	}
	kind := str(params["guardrail"])
	if kind == "" {
		kind = str(g["guardrail"])
	}
	low := strings.ToLower(text)
	out = text
	if kind == "block" || kind == "always_block" {
		return "block", out
	}
	words := extraWords(params["blocked_words"])
	words = append(words, extraWords(params["keywords"])...)
	words = append(words, extraWords(g["blocked_words"])...)
	for _, w := range words {
		if w != "" && strings.Contains(low, strings.ToLower(w)) {
			if kind == "redact" || str(params["mode"]) == "redact" {
				return "redact", strings.ReplaceAll(out, w, "[REDACTED]")
			}
			return "block", out
		}
	}
	return "allow", out
}

func extraWords(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, t...)
	case string:
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func guardrailText(body map[string]any) string {
	if t := str(body["text"]); t != "" {
		return t
	}
	if t := str(body["input"]); t != "" {
		return t
	}
	if t := str(body["prompt"]); t != "" {
		return t
	}
	var b strings.Builder
	if msgs, ok := body["messages"].([]any); ok {
		for _, raw := range msgs {
			if m, ok := raw.(map[string]any); ok {
				b.WriteString(str(m["content"]))
				b.WriteByte(' ')
			}
		}
	}
	return strings.TrimSpace(b.String())
}
