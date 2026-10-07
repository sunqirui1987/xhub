// compliance.go serves the EU AI Act and GDPR checks the request-log panel
// reads. The catalog still lists these paths, and the generic family writer
// used to answer them with a status stub that has no checks array. The panel
// maps that array, so the stub crashed the logs page.

package gateway

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
)

var logTraceOnceCompliance sync.Once

// complianceCheck is one article result. Field names match the log panel.
type complianceCheck struct {
	CheckName string `json:"check_name"`
	Article   string `json:"article"`
	Passed    bool   `json:"passed"`
	Detail    string `json:"detail"`
}

// complianceResponse is the body POST /compliance/eu-ai-act and POST /compliance/gdpr return.
type complianceResponse struct {
	Compliant  bool              `json:"compliant"`
	Regulation string            `json:"regulation"`
	Checks     []complianceCheck `json:"checks"`
}

// complianceModule mounts the two compliance checks ahead of the catalog so the family stub cannot answer them.
// 参数 s（*Server）：合规模块使用的网关进程。
// 返回 httpx.Module（httpx.Module）：可挂到网关上的模块。
// 调用：gateway/routes.go
// 测试：compliance_test.go
func complianceModule(s *Server) httpx.Module {
	return httpx.Bind("compliance", func(reg httpx.Registrar) {
		reg.Handle("POST /compliance/eu-ai-act", s.complianceEU)
		reg.Handle("POST /compliance/gdpr", s.complianceGDPR)
	})
}

// complianceEU serves POST /compliance/eu-ai-act.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func (s *Server) complianceEU(w http.ResponseWriter, r *http.Request) {
	s.writeCompliance(w, r, "EU AI Act", euAIActChecks)
}

// complianceGDPR serves POST /compliance/gdpr.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func (s *Server) complianceGDPR(w http.ResponseWriter, r *http.Request) {
	s.writeCompliance(w, r, "GDPR", gdprChecks)
}

// writeCompliance requires a management session, then writes the regulation result. A body without request_id is rejected and nothing is stored.
// 参数 w（http.ResponseWriter）：调用方的 HTTP 响应，状态码和正文写在这里；r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文；regulation（string）：写进响应的法规名；build（func(map[string]any) []complianceCheck）：用请求正文算出的检查列表。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func (s *Server) writeCompliance(w http.ResponseWriter, r *http.Request, regulation string, build func(map[string]any) []complianceCheck) {
	logTraceOnceCompliance.Do(func() { logx.Trace("enter gateway.writeCompliance") })

	httpx.SetCallID(w, httpx.CallID())
	if s.requireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	if str(body["request_id"]) == "" {
		httpx.WriteError(w, 400, "invalid_request", "request_id is required")
		return
	}
	checks := build(body)
	if checks == nil {
		checks = []complianceCheck{}
	}
	passed := true
	for _, check := range checks {
		if !check.Passed {
			passed = false
			break
		}
	}
	httpx.WriteJSON(w, 200, complianceResponse{Compliant: passed, Regulation: regulation, Checks: checks})
}

// euAIActChecks evaluates Art. 9, Art. 5, and Art. 12 from the guardrails and audit fields in the body.
// 参数 body（map[string]any）：已解析的合规请求。缺字段按未提供处理。
// 返回 []complianceCheck（[]complianceCheck）：三条欧盟人工智能法案检查。不会是 nil。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func euAIActChecks(body map[string]any) []complianceCheck {
	guardrails := activeGuardrails(body)
	return []complianceCheck{
		guardrailsApplied(guardrails),
		contentScreened(guardrails),
		auditComplete(body, guardrails, "Art. 12"),
	}
}

// gdprChecks evaluates Art. 32, Art. 5(1)(c), and Art. 30 from the same request fields.
// 参数 body（map[string]any）：已解析的合规请求。缺字段按未提供处理。
// 返回 []complianceCheck（[]complianceCheck）：三条 GDPR 检查。不会是 nil。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func gdprChecks(body map[string]any) []complianceCheck {
	guardrails := activeGuardrails(body)
	return []complianceCheck{
		dataProtection(guardrails),
		sensitiveDataProtected(guardrails),
		auditComplete(body, guardrails, "Art. 30"),
	}
}

// guardrailsApplied is EU AI Act Art. 9: at least one guardrail ran.
// 参数 guardrails（[]map[string]any）：这次请求里实际跑过的护栏。空切片表示没有。
// 返回 complianceCheck（complianceCheck）：Art. 9 的通过与否和说明。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func guardrailsApplied(guardrails []map[string]any) complianceCheck {
	n := len(guardrails)
	detail := "No guardrails applied"
	if n > 0 {
		detail = fmt.Sprintf("%d guardrail(s) applied", n)
	}
	return complianceCheck{CheckName: "Guardrails applied", Article: "Art. 9", Passed: n > 0, Detail: detail}
}

// contentScreened is EU AI Act Art. 5: content was screened before the model call.
// 参数 guardrails（[]map[string]any）：这次请求里实际跑过的护栏。空切片表示没有。
// 返回 complianceCheck（complianceCheck）：Art. 5 的通过与否和说明。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func contentScreened(guardrails []map[string]any) complianceCheck {
	pre := guardrailsByMode(guardrails, "pre_call")
	detail := "No pre-call screening applied"
	if len(pre) > 0 {
		detail = fmt.Sprintf("%d pre-call guardrail(s) screened content", len(pre))
	}
	return complianceCheck{CheckName: "Content screened before LLM", Article: "Art. 5", Passed: len(pre) > 0, Detail: detail}
}

// dataProtection is GDPR Art. 32: a pre-call guardrail protected the request.
// 参数 guardrails（[]map[string]any）：这次请求里实际跑过的护栏。空切片表示没有。
// 返回 complianceCheck（complianceCheck）：Art. 32 的通过与否和说明。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func dataProtection(guardrails []map[string]any) complianceCheck {
	pre := guardrailsByMode(guardrails, "pre_call")
	detail := "No pre-call data protection applied"
	if len(pre) > 0 {
		detail = fmt.Sprintf("%d pre-call guardrail(s) protect data", len(pre))
	}
	return complianceCheck{CheckName: "Data protection applied", Article: "Art. 32", Passed: len(pre) > 0, Detail: detail}
}

// sensitiveDataProtected is GDPR Art. 5(1)(c). A pre-call intervention or a clean pre-call pass counts. No pre-call guardrail does not.
// 参数 guardrails（[]map[string]any）：这次请求里实际跑过的护栏。空切片表示没有。
// 返回 complianceCheck（complianceCheck）：Art. 5(1)(c) 的通过与否和说明。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func sensitiveDataProtected(guardrails []map[string]any) complianceCheck {
	pre := guardrailsByMode(guardrails, "pre_call")
	detail := "No pre-call guardrails to protect sensitive data"
	passed := false
	switch {
	case guardrailIntervened(pre):
		detail = "Guardrail intervened to protect sensitive data"
		passed = true
	case allGuardrailsPassed(pre):
		detail = "No sensitive data detected"
		passed = true
	}
	return complianceCheck{CheckName: "Sensitive data protected", Article: "Art. 5(1)(c)", Passed: passed, Detail: detail}
}

// auditComplete requires user_id, model, timestamp, and at least one guardrail result.
// 参数 body（map[string]any）：已解析的合规请求。缺字段按未提供处理；guardrails（[]map[string]any）：这次请求里实际跑过的护栏；article（string）：写进结果的条款号。
// 返回 complianceCheck（complianceCheck）：审计完整性的通过与否和缺失字段。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func auditComplete(body map[string]any, guardrails []map[string]any, article string) complianceCheck {
	var missing []string
	if str(body["user_id"]) == "" {
		missing = append(missing, "user_id")
	}
	if str(body["model"]) == "" {
		missing = append(missing, "model")
	}
	if str(body["timestamp"]) == "" {
		missing = append(missing, "timestamp")
	}
	if len(guardrails) == 0 {
		missing = append(missing, "guardrail_results")
	}
	detail := "All required audit fields present"
	if len(missing) > 0 {
		detail = "Missing: " + strings.Join(missing, ", ")
	}
	return complianceCheck{CheckName: "Audit record complete", Article: article, Passed: len(missing) == 0, Detail: detail}
}

// activeGuardrails reads guardrail_information. A single object is accepted as a one-item list. Entries marked not_run are left out.
// 参数 body（map[string]any）：已解析的合规请求。guardrail_information 可以是数组或单个对象。
// 返回 []map[string]any（[]map[string]any）：实际跑过的护栏。没有时为空切片，不是 nil。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func activeGuardrails(body map[string]any) []map[string]any {
	var raw []any
	switch info := body["guardrail_information"].(type) {
	case []any:
		raw = info
	case map[string]any:
		raw = []any{info}
	}
	out := []map[string]any{}
	for _, item := range raw {
		g, ok := item.(map[string]any)
		if !ok || str(g["guardrail_status"]) == "not_run" {
			continue
		}
		out = append(out, g)
	}
	return out
}

// guardrailsByMode keeps guardrails whose logged mode is guaranteed to have run in mode.
// 参数 guardrails（[]map[string]any）：这次请求里实际跑过的护栏；mode（string）：要匹配的钩子，例如 pre_call。
// 返回 []map[string]any（[]map[string]any）：模式对得上的护栏。没有时为空切片，不是 nil。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func guardrailsByMode(guardrails []map[string]any, mode string) []map[string]any {
	out := []map[string]any{}
	for _, g := range guardrails {
		if modeMatches(g["guardrail_mode"], mode) {
			out = append(out, g)
		}
	}
	return out
}

// modeMatches reports whether a logged guardrail_mode is guaranteed to have run in mode. A missing mode counts as pre_call. A list or tag map counts only when every branch is that one mode, so a mixed list is not treated as pre-call.
// 参数 gMode（any）：日志里的 guardrail_mode。缺省、字符串、字符串列表和带 default 的对象都要接住；mode（string）：要匹配的钩子，例如 pre_call。
// 返回 bool（bool）：能确定这次请求在该钩子上跑过时返回真。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func modeMatches(gMode any, mode string) bool {
	if gMode == nil {
		return mode == "pre_call"
	}
	switch m := gMode.(type) {
	case string:
		return m == mode
	case []any:
		return branchRunsInMode(m, mode)
	case map[string]any:
		defaultMode, ok := m["default"]
		if !ok || defaultMode == nil {
			return false
		}
		branches := []any{defaultMode}
		if tags, ok := m["tags"].(map[string]any); ok {
			for _, branch := range tags {
				branches = append(branches, branch)
			}
		}
		for _, branch := range branches {
			if !branchRunsInMode(branch, mode) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// branchRunsInMode reports whether one mode branch is exactly mode. A list matches only when it is non-empty and every item is that mode.
// 参数 branch（any）：一个模式或模式列表；mode（string）：要匹配的钩子，例如 pre_call。
// 返回 bool（bool）：这一支确定只在该钩子上跑时返回真。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func branchRunsInMode(branch any, mode string) bool {
	switch b := branch.(type) {
	case string:
		return b == mode
	case []any:
		if len(b) == 0 {
			return false
		}
		for _, item := range b {
			s, ok := item.(string)
			if !ok || s != mode {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// guardrailIntervened reports whether any guardrail blocked, failed, or otherwise intervened.
// 参数 guardrails（[]map[string]any）：要查看的护栏。空切片表示没有。
// 返回 bool（bool）：有护栏介入时返回真。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func guardrailIntervened(guardrails []map[string]any) bool {
	for _, g := range guardrails {
		switch str(g["guardrail_status"]) {
		case "guardrail_intervened", "failed", "blocked":
			return true
		}
	}
	return false
}

// allGuardrailsPassed reports whether every guardrail succeeded. An empty list does not pass.
// 参数 guardrails（[]map[string]any）：要查看的护栏。空切片表示没有。
// 返回 bool（bool）：列表非空且全部成功时返回真。
// 调用：仅在 compliance.go 内使用
// 测试：compliance_test.go
func allGuardrailsPassed(guardrails []map[string]any) bool {
	if len(guardrails) == 0 {
		return false
	}
	for _, g := range guardrails {
		if str(g["guardrail_status"]) != "success" {
			return false
		}
	}
	return true
}
