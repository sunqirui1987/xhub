// Package guard 在请求发给上游前执行本地规则、XGo 脚本及外部审核；拦截结果阻止上游调用。
package guard

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceGuard sync.Once

// Apply 执行管理员护栏调试，支持已保存规则和经过校验的临时规则，不修改持久化配置。
// 参数：s：鉴权/配置宿主；w：HTTP 响应；r：含 guardrail_name 或临时 guardrail、待检查文本的请求。
// 返回：无；成功返回 action、reason、metadata 和转换后的文本；配置/运行错误返回 HTTP 错误。
// 调用：Module 注册的两个 apply_guardrail 路径。
// 测试：guard_test.go、external_test.go、gateway/guardrail_manage_test.go。
func Apply(s Host, w http.ResponseWriter, r *http.Request) {
	logTraceOnceGuard.Do(func() { logx.Trace("enter guard.Apply") })

	if s.RequireManage(w, r) == nil {
		return
	}
	body := readMap(r)
	name := str(body["guardrail_name"])
	if name == "" {
		name = str(body["guardrail"])
	}
	var rule map[string]any
	if inline, ok := body["guardrail"].(map[string]any); ok {
		if err := Validate(inline); err != nil {
			httpx.WriteError(w, 400, "invalid_request", err.Error())
			return
		}
		rule = inline
	} else {
		for _, g := range listGuardrails(s) {
			if ruleIdentifies(g, name) {
				rule = g
				break
			}
		}
		if rule == nil {
			httpx.WriteError(w, 404, "not_found", "guardrail not found")
			return
		}
	}
	action, reason, metadata, err := runRuleDetailed(rule, body)
	if err != nil {
		httpx.WriteError(w, 400, "guardrail_execution", err.Error())
		return
	}
	out := guardrailText(body)
	httpx.WriteJSON(w, 200, map[string]any{"metadata": metadata, "status": "success", "guardrail_name": findingName(rule), "action": action, "reason": reason, "blocked": action == "block", "response_text": out, "text": out, "output": map[string]any{"text": out}})

}

// PreCall 提供仅关心是否拦截的请求前检查入口；完整执行结果由 Evaluate 提供。
// 参数：s：配置和存储宿主；body：已解析正文，允许原地改写协议文本。
// 返回：bool：是否拒绝调用上游；string：拒绝原因，放行时为空。
// 调用：兼容调用方；正式数据面通过网关宿主调用 Evaluate。
// 测试：guard_test.go。
func PreCall(s Host, body map[string]any) (bool, string) {
	blocked, message, _ := Evaluate(s, body)
	return blocked, message
}

// Evaluate 合并默认启用与请求指定的规则，按优先级串行执行；拦截立即结束，只记录实际执行的规则。
// 参数：s：配置和存储宿主；body：解析后的正文，读取 guardrails/metadata.guardrails 并原地应用文本修改。
// 返回：blocked：是否拦截；message：拦截原因；findings：已执行结果及耗时，Flag 附加元数据随结果记录。
// 调用：gateway/wire.go 的数据面宿主；普通 chat/responses 在上游请求前调用。
// 测试：guard_test.go、engine_test.go、external_test.go、gateway/guardrail_block_test.go。
func Evaluate(s Host, body map[string]any) (blocked bool, message string, findings []map[string]any) {
	requested, err := requestedRules(body["guardrails"])
	if err != nil {
		return true, err.Error(), nil
	}
	if metadata, ok := body["metadata"].(map[string]any); ok {
		more, err := requestedRules(metadata["guardrails"])
		if err != nil {
			return true, err.Error(), nil
		}
		requested = append(requested, more...)
	}
	rules := listGuardrails(s)
	for _, name := range requested {
		found := false
		for _, g := range rules {
			if ruleIdentifies(g, name) {
				found = true
				break
			}
		}
		if !found {
			return true, "Unknown guardrail: " + name, []map[string]any{findingJSON(map[string]any{"guardrail_name": name}, "block", false)}
		}
	}
	for _, g := range rules {
		params, _ := g["litellm_params"].(map[string]any)
		if params == nil {
			params = map[string]any{}
		}
		selected := false
		for _, name := range requested {
			if ruleIdentifies(g, name) {
				selected = true
				break
			}
		}
		if !selected && !boolOf(params["default_on"]) && !boolOf(g["default_on"]) {
			continue
		}
		if !preCallMode(params["mode"]) {
			continue
		}
		started := time.Now()
		action, reason, metadata, err := runRuleDetailed(g, body)
		finding := findingJSON(g, action, action == "redact" || action == "modify")
		if err != nil {
			action = "block"
			reason = "Guardrail execution failed: " + findingName(g)
			finding = findingJSON(g, action, false)
			finding["error"] = err.Error()
		}
		// 本地关键词规则只返回 block 动作，没有独立原因。先补齐稳定且不泄露正文的说明，
		// 再把同一说明写入持久化结果，确保调用方错误响应与监控日志一致。
		if action == "block" && reason == "" {
			reason = "Guardrail blocked the request: " + findingName(g)
		}
		if reason != "" {
			finding["reason"] = reason
		}
		if metadata != nil {
			finding["guardrail_response"].(map[string]any)["metadata"] = metadata
		}
		finding["start_time"] = float64(started.UnixNano()) / 1e9
		finding["end_time"] = float64(time.Now().UnixNano()) / 1e9
		finding["duration"] = time.Since(started).Seconds()
		findings = append(findings, finding)
		if action == "block" {
			return true, reason, findings
		}

	}
	return false, "", findings
}

// findingName 按 guardrail_name、历史 name 的顺序读取展示名称。
// 参数：g：规则对象；缺失字段不引发 panic。
// 返回：名称字符串；均未提供时返回 guardrail。
// 调用：规则选择、管理接口和执行结果生成。
// 测试：guard_test.go、engine_test.go 通过选择和结果验证。
func findingName(g map[string]any) string {
	name := str(g["guardrail_name"])
	if name == "" {
		name = str(g["name"])
	}
	if name == "" {
		name = "guardrail"
	}
	return name
}

// findingJSON 构造前端监控兼容的护栏结果；Flag 和文本修改记录告警，block 记录拒绝。
// 参数：g：规则；action：allow/block/redact/modify/flag；redacted：是否实际执行文本修改。
// 返回：结果对象，含提供商/阶段/状态；调用方补齐实际耗时、原因及 Flag metadata。
// 调用：Evaluate 的每条执行结果及未知选择器错误。
// 测试：guard_test.go、external_test.go。
func findingJSON(g map[string]any, action string, redacted bool) map[string]any {
	status := "success"
	switch action {
	case "block":
		status = "blocked"
	case "redact", "modify", "flag":
		status = "guardrail_flagged"
	}
	id := str(g["guardrail_id"])
	if id == "" {
		id = str(g["id"])
	}
	params, _ := g["litellm_params"].(map[string]any)
	provider := str(params["guardrail"])
	if provider == "" {
		provider = "xhub"
	}
	now := float64(time.Now().UnixNano()) / 1e9
	masked := map[string]any{}
	if redacted {
		masked["redacted"] = 1
	}
	return map[string]any{
		"guardrail_id":        id,
		"guardrail_name":      findingName(g),
		"guardrail_mode":      "pre_call",
		"guardrail_provider":  provider,
		"guardrail_status":    status,
		"start_time":          now,
		"end_time":            now,
		"duration":            0,
		"guardrail_response":  map[string]any{"action": action},
		"masked_entity_count": masked,
	}
}

// evalNamed 兼容旧单规则调试：优先指定规则，未找到时使用首条默认规则，无规则时放行。
// 参数：s：配置和存储宿主；name：名称或 ID，空串表示默认规则；text：待检查文本。
// 返回：action：执行动作，失败时 block；out：原文或修改后的文本。
// 调用：兼容入口及 guard_test.go；实际数据面使用 Evaluate 执行整条规则链。
// 测试：TestEvalNamedRunsTheAskedRuleAndOtherwiseTheDefault。
func evalNamed(s Host, name, text string) (action, out string) {
	if name != "" {
		for _, g := range listGuardrails(s) {
			n := str(g["guardrail_name"])
			if n == "" {
				n = str(g["name"])
			}
			if n == name || str(g["id"]) == name || str(g["guardrail_id"]) == name {
				return matchGuardrail(g, text)
			}
		}
	}
	for _, g := range listGuardrails(s) {
		if boolOf(g["default_on"]) {
			return matchGuardrail(g, text)
		}
		if params, _ := g["litellm_params"].(map[string]any); boolOf(params["default_on"]) {
			return matchGuardrail(g, text)
		}
	}
	return "allow", text
}

// listGuardrails 合并 YAML、当前及历史 KV 规则，检查名称歧义、继承全局跳过设置并排序。
// 参数：s：配置和存储宿主；无 KV 时仍加载 YAML。
// 返回：可执行规则列表；存储错误或歧义配置返回默认拦截哨兵，无配置返回空列表。
// 调用：Apply、Evaluate、evalNamed；管理列表使用 storedRules。
// 测试：guard_regression_test.go、engine_test.go。
func listGuardrails(s Host) []map[string]any {
	if s == nil || s.RecordStore() == nil {
		logx.Debug("guardrails loading YAML only reason=no store")
		out := configuredRules(s)
		effectiveDefaults(s, out)
		sortRules(out)
		return out
	}
	out := configuredRules(s)
	for _, kind := range []string{"guardrails", "guardrail"} {
		list, err := s.RecordStore().ListKV(kind)
		if err != nil {
			logx.Error("guardrail lookup failed kind=%s err=%v", kind, err)
			return []map[string]any{{"name": "guardrail_store_unavailable", "default_on": true, "guardrail": "always_block"}}
		}
		out = append(out, list...)
	}
	if out == nil {
		out = []map[string]any{}
	}
	out = uniqueRules(out)
	effectiveDefaults(s, out)
	sortRules(out)
	return out
}

// matchGuardrail 把单段文本包装成协议正文，通过统一执行器运行一条规则。
// 参数：g：完整规则；text：待检查文本，包括空串。
// 返回：action：规则动作，执行失败则 block；out：原文或改写后的 text。
// 调用：evalNamed 和兼容单元测试。
// 测试：guard_test.go。
func matchGuardrail(g map[string]any, text string) (action, out string) {
	body := map[string]any{"text": text}
	action, _, err := runRule(g, body)
	if err != nil {
		return "block", text
	}
	return action, str(body["text"])
}

// extraWords 规范化配置中的字符串或字符串数组，忽略不支持的类型及动态数组中的空值。
// 参数：v：string、[]string、[]any 或其他配置值。
// 返回：字符串列表；单字符串包装成一项，其他类型返回空列表。
// 调用：关键词、模式及选择器兼容读取。
// 测试：TestExtraWordsAcceptsTheShapesASavedRuleUses。
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

// guardrailText 按协议字段顺序汇总文本，供调试展示，保留图片 URL、角色和其他元数据。
// 参数：body：已解析协议正文；此函数不修改文本内容。
// 返回：非空文本用空格连接的字符串；无文本为空串。
// 调用：Apply 和兼容测试；规则逐段检查使用 visitGuardedText。
// 测试：guard_test.go 的协议字段覆盖测试。
func guardrailText(body map[string]any) string {
	var parts []string
	visitRequestText(body, func(value string) string {
		if value != "" {
			parts = append(parts, value)
		}
		return value
	})
	return strings.Join(parts, " ")
}

// visitRequestText 递归遍历支持协议的文本叶子，仅改写 text/content/parts/output，不触碰角色和图片 URL。
// 参数：body：正文；visit：每个字符串叶子的转换函数，返回值原位替换。
// 返回：无；body 原地更新，数组元素数量和顺序不变。
// 调用：guardrailText；带角色跳过的规则执行另用 visitGuardedText。
// 测试：guard_test.go、guard_regression_test.go。
func visitRequestText(body map[string]any, visit func(string) string) {
	var walk func(any) any
	walk = func(value any) any {
		switch v := value.(type) {
		case string:
			return visit(v)
		case []any:
			for i := range v {
				v[i] = walk(v[i])
			}
		case map[string]any:
			for _, key := range []string{"text", "content", "parts", "output"} {
				if child, ok := v[key]; ok {
					v[key] = walk(child)
				}
			}
		}
		return value
	}
	for _, key := range []string{"text", "texts", "input", "prompt", "messages", "contents", "system", "instructions", "systemInstruction"} {
		if value, ok := body[key]; ok {
			body[key] = walk(value)
		}
	}
}

// ruleIdentifies 判断选择器是否匹配规则的名称或任一历史 ID。
// 参数：g：规则；name：客户端指定名称或 ID。
// 返回：bool：匹配且名称非空时为真。
// 调用：Evaluate、Manage、evalNamed。
// 测试：engine_test.go 的选择器及重名测试。
func ruleIdentifies(g map[string]any, name string) bool {
	if name == "" {
		return false
	}
	return findingName(g) == name || str(g["guardrail_id"]) == name || str(g["id"]) == name
}

// preCallMode 判断历史或多值模式是否包含可执行的请求前阶段。
// 参数：value：mode 动态值；缺失沿用历史请求前行为。
// 返回：bool：pre_call 或兼容 redact 为真；响应阶段为假。
// 调用：Evaluate、evalNamed。
// 测试：engine_test.go。
func preCallMode(value any) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok && text == "" {
		return true
	}
	for _, mode := range extraWords(value) {
		if mode == "pre_call" || mode == "redact" {
			return true
		}
	}
	return false
}

// sortRules 对规则按优先级升序、同优先级按 ID 稳定排序。
// 参数：rules：可原地排序的规则数组。
// 返回：无；按排序结果串行执行，拦截后停止。
// 调用：storedRules、listGuardrails。
// 测试：TestSelectionDefaultsAndOrdering。
func sortRules(rules []map[string]any) {
	sort.SliceStable(rules, func(i, j int) bool {
		a, _ := rules[i]["litellm_params"].(map[string]any)
		b, _ := rules[j]["litellm_params"].(map[string]any)
		pa := priority(a["priority"])
		pb := priority(b["priority"])
		if pa != pb {
			return pa < pb
		}
		return str(rules[i]["guardrail_id"]) < str(rules[j]["guardrail_id"])
	})
}
