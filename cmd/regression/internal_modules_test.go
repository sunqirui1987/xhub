package regression

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/cache"
	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/live"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/llm/estimate"
	"github.com/sunqirui1987/xhub/internal/plugin"
	"github.com/sunqirui1987/xhub/internal/provider"
	_ "github.com/sunqirui1987/xhub/internal/provider/all"
)

// regressionExtension 是内部扩展注册表的可观察替身，记录收到的调用并返回固定决定。
// 参数由 BeforeUpstream 接收；返回预置的 plugin.Decision。调用场景是验证具名扩展边界；无外部调用和持久化副作用。
type regressionExtension struct {
	name  string
	seen  *plugin.Call
	value plugin.Decision
}

// Name 返回扩展的稳定注册名。
// 参数：无；返回注册名。仅由 plugin.Registry 注册和查找时调用；无副作用。
func (e *regressionExtension) Name() string { return e.name }

// BeforeUpstream 保存网关准备发往上游的调用并返回预置决定。
// 参数 call 是模型调用上下文；返回预置决定。仅在注册表调用边界使用；会更新 seen，不访问外部服务。
func (e *regressionExtension) BeforeUpstream(call plugin.Call) plugin.Decision {
	*e.seen = call
	return e.value
}

// TestInternalLLMRouteBoundary 验证内部 LLM 路由过滤器只放行数据面、健康检查和明确公开入口。
// 前置条件是内存 HTTP handler；验证普通路由、挂载路由、拒绝路径和 allowlist 副本隔离；无测试数据需要清理。
func TestInternalLLMRouteBoundary(t *testing.T) {
	prefixes := llm.PathPrefixes()
	if len(prefixes) == 0 {
		t.Fatal("LLM 路径白名单为空")
	}
	original := prefixes[0]
	prefixes[0] = "/mutated/"
	if llm.PathPrefixes()[0] != original {
		t.Fatal("调用方修改了 LLM 路径白名单源数据")
	}

	for _, tc := range []struct {
		path  string
		mount bool
		want  bool
	}{
		{path: "/v1/chat/completions", want: true},
		{path: "/health", want: true},
		{path: "/", want: true},
		{path: "/key/generate", want: false},
		{path: "", want: false},
		{path: "/metrics", mount: true, want: true},
		{path: "/docs", mount: true, want: false},
	} {
		if got := llm.Allow(tc.path, tc.mount); got != tc.want {
			t.Errorf("Allow(%q, mount=%v)=%v，期望 %v", tc.path, tc.mount, got, tc.want)
		}
	}

	filtered := llm.Filter(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		path string
		want int
	}{
		{path: "/v1/embeddings", want: http.StatusNoContent},
		{path: "/team/new", want: http.StatusNotFound},
	} {
		r := httptest.NewRequest(http.MethodPost, tc.path, nil)
		w := httptest.NewRecorder()
		filtered.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("过滤路径 %q 得到状态码 %d，期望 %d", tc.path, w.Code, tc.want)
		}
	}
}

// TestInternalLLMProtocolHelpersBoundary 验证响应消息整形、流选项、透传 URL 和上游错误映射的协议边界。
// 前置条件仅为内存对象；验证输入不被改写、目录前缀不丢失、越界子路径被约束及未知状态失败；无清理需求。
func TestInternalLLMProtocolHelpersBoundary(t *testing.T) {
	message := map[string]any{
		"role": "user",
		"content": []any{
			map[string]any{"type": "text", "text": "hello"},
			map[string]any{"type": "image_url", "image_url": "https://example.invalid/a.png"},
		},
	}
	shaped := llm.ShapeResponsesMessage(message)
	parts := shaped["content"].([]any)
	if parts[0].(map[string]any)["type"] != "input_text" || parts[1].(map[string]any)["type"] != "image_url" {
		t.Fatalf("Responses 消息整形错误: %#v", parts)
	}
	if message["content"].([]any)[0].(map[string]any)["type"] != "text" {
		t.Fatal("Responses 消息整形修改了调用方输入")
	}
	assistant := map[string]any{"role": "assistant", "content": "done"}
	if got := llm.ShapeResponsesMessage(assistant); !reflect.DeepEqual(got, assistant) {
		t.Fatalf("助手消息不应被改写: %#v", got)
	}
	if got := llm.ShapeResponsesMessage(nil); got != nil {
		t.Fatalf("nil 消息应保持 nil: %#v", got)
	}

	if flag, ok := llm.NormalizeStreamOptions(map[string]any{"include_obfuscation": true}); !ok || !flag {
		t.Fatalf("合法流选项未被识别: flag=%v ok=%v", flag, ok)
	}
	for _, opts := range []map[string]any{nil, {}, {"include_obfuscation": "true"}} {
		if _, ok := llm.NormalizeStreamOptions(opts); ok {
			t.Fatalf("非法流选项被当作显式布尔值: %#v", opts)
		}
	}

	if got := llm.PassthroughURL("https://api.openai.com/proxy", "chat/completions", "openai"); got != "https://api.openai.com/v1/proxy/chat/completions" {
		t.Fatalf("OpenAI 透传地址错误: %q", got)
	}
	if got := llm.PassthroughURL("https://vendor.invalid/root/", "/v1/models/", "custom"); got != "https://vendor.invalid/root/v1/models/" {
		t.Fatalf("供应商根路径或末尾斜杠丢失: %q", got)
	}
	if got := llm.PassthroughSubpath("https://vendor.invalid/root", "../escape/", true); got != "https://vendor.invalid/root/escape/" {
		t.Fatalf("透传子路径未规范化: %q", got)
	}
	if got := llm.PassthroughSubpath("https://vendor.invalid/root", "child", false); got != "https://vendor.invalid/root" {
		t.Fatalf("关闭子路径拼接后仍改写地址: %q", got)
	}

	for status, want := range map[int]string{400: "BadRequestError", 401: "AuthenticationError", 404: "NotFoundError", 408: "Timeout", 429: "RateLimitError", 500: "InternalServerError", 502: "BadGatewayError", 503: "ServiceUnavailableError", 504: "Timeout"} {
		if got, ok := llm.ExceptionForStatus(status); !ok || got != want {
			t.Errorf("状态码 %d 映射为 %q/%v，期望 %q/true", status, got, ok, want)
		}
	}
	if got, ok := llm.ExceptionForStatus(418); ok || got != "" {
		t.Fatalf("未知状态码不应伪装成已知异常: %q/%v", got, ok)
	}
}

// TestInternalEstimateBoundary 验证折扣、加价、计费档位和本地 token 估算的组合契约。
// 前置条件是固定价格和短文本；验证供应商规则优先级、未知值降级及模型名归一化；不访问供应商且无清理需求。
func TestInternalEstimateBoundary(t *testing.T) {
	final, percent, amount := estimate.ApplyDiscount(10, "vendor", map[string]float64{"vendor": 0.2})
	if final != 8 || percent != 0.2 || amount != 2 {
		t.Fatalf("折扣结果错误: final=%v percent=%v amount=%v", final, percent, amount)
	}
	if final, percent, amount = estimate.ApplyDiscount(10, "", map[string]float64{"": 0.9}); final != 10 || percent != 0 || amount != 0 {
		t.Fatalf("空供应商不应套用折扣: %v %v %v", final, percent, amount)
	}
	margins := map[string]estimate.Margin{
		"global": {Percent: 0.1, IsPercent: true},
		"vendor": {Percent: 0.2, FixedAmount: 1, HasPercent: true, HasFixed: true},
	}
	marked, pct, fixed, total := estimate.ApplyMargin(10, "vendor", margins)
	if marked != 13 || pct != 0.2 || fixed != 1 || total != 3 {
		t.Fatalf("供应商加价结果错误: %v %v %v %v", marked, pct, fixed, total)
	}
	marked, pct, fixed, total = estimate.ApplyMargin(10, "other", margins)
	if marked != 11 || pct != 0.1 || fixed != 0 || total != 1 {
		t.Fatalf("全局加价回退错误: %v %v %v %v", marked, pct, fixed, total)
	}
	if in, out := estimate.TokenCost(3, 2, 0.5, 2); in != 1.5 || out != 4 {
		t.Fatalf("token 分侧计费错误: input=%v output=%v", in, out)
	}

	for _, tc := range []struct {
		value    string
		tier     string
		known    bool
		standard bool
	}{
		{value: "on_demand_priority", tier: "priority", known: true},
		{value: "batch", tier: "flex", known: true},
		{value: "ON_DEMAND", known: true, standard: true},
		{value: "unknown"},
		{value: ""},
	} {
		tier, known, standard := estimate.MapTrafficType(tc.value)
		if tier != tc.tier || known != tc.known || standard != tc.standard {
			t.Errorf("traffic_type %q 得到 %q/%v/%v", tc.value, tier, known, standard)
		}
	}
	if tier, ok := estimate.NormalizeServiceTier("auto", true); ok || tier != "" {
		t.Fatalf("auto 不应成为计费档位: %q/%v", tier, ok)
	}
	if tier, ok := estimate.NormalizeServiceTier("priority", true); !ok || tier != "priority" {
		t.Fatalf("显式计费档位丢失: %q/%v", tier, ok)
	}
	if _, ok := estimate.NormalizeServiceTier("priority", false); ok {
		t.Fatal("非字符串 service_tier 不应可计价")
	}

	count, tokenizerName, err := estimate.CountTokens("unknown-model", "hello world", nil)
	if err != nil || count <= 0 || tokenizerName != "openai_tokenizer" {
		t.Fatalf("未知模型没有回退到本地 tokenizer: count=%d tokenizer=%q err=%v", count, tokenizerName, err)
	}
	messages := []map[string]any{{"role": "user", "content": "hello", "name": "alice"}}
	count, _, err = estimate.CountTokens("gpt-4", "", messages)
	if err != nil || count <= 7 {
		t.Fatalf("消息 token 估算未计入消息开销: count=%d err=%v", count, err)
	}
	if got := estimate.ModelUsedForCount("public", "openai/gpt-4"); got != "gpt-4" {
		t.Fatalf("部署模型前缀未去除: %q", got)
	}
	if got := estimate.ModelUsedForCount("public", "  "); got != "public" {
		t.Fatalf("空部署模型未回退请求模型: %q", got)
	}
	modern := estimate.OpenAISupportedParams("gpt-4o", true)
	legacy := estimate.OpenAISupportedParams("gpt-4", false)
	if !contains(modern, "response_format") || !contains(modern, "user") || contains(legacy, "response_format") || contains(legacy, "user") {
		t.Fatalf("OpenAI 参数能力边界错误: modern=%v legacy=%v", modern, legacy)
	}
}

// TestInternalCatalogClassificationBoundary 验证目录对公开、数据面、已移除入口和路径模板的分类。
// 前置条件是内嵌目录；验证段边界避免相似前缀误命中，并检查公开响应形状；无外部依赖和清理需求。
func TestInternalCatalogClassificationBoundary(t *testing.T) {
	for _, tc := range []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodGet, "/public/providers/fields", true},
		{http.MethodHead, "/.well-known/openid-configuration", true},
		{http.MethodGet, "/model_hub", true},
		{http.MethodPost, "/public/providers/fields", false},
		{http.MethodGet, "/private", false},
	} {
		if got := catalog.IsPublicPath(tc.method, tc.path); got != tc.want {
			t.Errorf("公开路径分类 %s %s=%v，期望 %v", tc.method, tc.path, got, tc.want)
		}
	}
	if catalog.AuthOf("/v1/chat/completions") != catalog.AuthData || catalog.AuthOf("/team/new") != catalog.AuthManagement || catalog.AuthOf("/v1/mcp/tools") != catalog.AuthMixed {
		t.Fatal("目录鉴权分类未区分数据面、管理面和已移除入口")
	}
	if !catalog.IsDataPlanePath("/v1/mcp/tools") || catalog.IsLLMPrefix("/v1/chatcompletions") || catalog.HasPrefix("/v1/chatcompletions", "/v1/chat") {
		t.Fatal("路径段边界分类错误")
	}
	for _, tc := range []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/v1/models/{model}", "/v1/models/gpt-4", true},
		{"/files/{path:path}", "/files/a/b/c", true},
		{"/v1/models/{model}", "/v1/models/a/extra", true},
		{"/v1/models/{model}", "/v1/other/a", false},
		{"/v1/models", "/v1/models/extra", false},
	} {
		if got := catalog.PathMatch(tc.pattern, tc.path); got != tc.want {
			t.Errorf("PathMatch(%q, %q)=%v，期望 %v", tc.pattern, tc.path, got, tc.want)
		}
	}
	if got := catalog.Split("///v1/models///"); !reflect.DeepEqual(got, []string{"v1", "models"}) {
		t.Fatalf("路径拆分错误: %#v", got)
	}
	if got := catalog.Split("/"); got != nil {
		t.Fatalf("根路径应拆成 nil: %#v", got)
	}
	if body, ok := catalog.PublicBody("/public/skill_hub").(map[string]any); !ok || body["count"] != 0 {
		t.Fatalf("公开 skill hub 响应形状错误: %#v", body)
	}
	if total, input, output, ok := catalog.Cost("definitely-not-a-catalog-model", 1, 1); ok || total != 0 || input != 0 || output != 0 {
		t.Fatalf("未知模型被错误计成免费模型: %v %v %v %v", total, input, output, ok)
	}
}

// TestInternalCacheAndPluginBoundary 验证缓存清空与具名扩展调用这两个进程内模块的实际边界。
// 前置条件是新建内存缓存和注册表；验证副本隔离、Flush、调用透传及缺失扩展错误；所有状态随测试进程释放。
func TestInternalCacheAndPluginBoundary(t *testing.T) {
	c := cache.New()
	source := []byte("answer")
	c.Set("key", source)
	source[0] = 'X'
	got, ok := c.Get("key")
	if !ok || string(got) != "answer" {
		t.Fatalf("缓存未隔离写入切片: %q/%v", got, ok)
	}
	got[0] = 'Y'
	again, _ := c.Get("key")
	if string(again) != "answer" {
		t.Fatalf("缓存未隔离读取切片: %q", again)
	}
	c.Flush()
	if _, ok := c.Get("key"); ok {
		t.Fatal("Flush 后缓存仍命中")
	}
	if cache.Key("ab", "c") == cache.Key("a", "bc") {
		t.Fatal("缓存键分段发生碰撞")
	}

	registry := plugin.New()
	var seen plugin.Call
	ext := &regressionExtension{
		name:  "audit",
		seen:  &seen,
		value: plugin.Decision{Refuse: true, Status: http.StatusForbidden, Code: "blocked", Message: "policy", Header: map[string]string{"X-Audit": "yes"}},
	}
	if err := registry.Register(ext); err != nil {
		t.Fatalf("注册扩展失败: %v", err)
	}
	names := registry.Names()
	if !reflect.DeepEqual(names, []string{"audit"}) {
		t.Fatalf("扩展注册顺序错误: %#v", names)
	}
	names[0] = "mutated"
	if registry.Names()[0] != "audit" {
		t.Fatal("调用方修改了扩展注册表名称切片")
	}
	call := plugin.Call{Op: "chat", Model: "demo", Path: "/v1/chat/completions"}
	decision, err := registry.Invoke("audit", call)
	if err != nil || !reflect.DeepEqual(seen, call) || !decision.Refuse || decision.Status != http.StatusForbidden || decision.Header["X-Audit"] != "yes" {
		t.Fatalf("具名扩展调用边界错误: seen=%#v decision=%#v err=%v", seen, decision, err)
	}
	if _, err := registry.Invoke("missing", call); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("缺失扩展未返回可定位错误: %v", err)
	}
	var nilRegistry *plugin.Registry
	if _, err := nilRegistry.Invoke("missing", call); err == nil {
		t.Fatal("nil 注册表未返回错误")
	}
}

// TestInternalLiveUnavailableBoundary 验证未配置 Redis 时热路径模块保持可预测降级，同时拒绝需要可靠入队的写入。
// 前置条件是 nil 客户端和语法非法的 Redis URL；验证计数、查询、日志及字符串操作边界；不会连接或清理任何 Redis。
func TestInternalLiveUnavailableBoundary(t *testing.T) {
	if got := live.SpendRef("team", "team-1"); got != "team/team-1" {
		t.Fatalf("热花费引用未隔离主体类型: %q", got)
	}
	if got := live.SpendRef("", "raw"); got != "raw" {
		t.Fatalf("空主体类型不应改写引用: %q", got)
	}
	if client, err := live.Open("://invalid redis url"); err == nil || client != nil {
		t.Fatalf("非法 Redis URL 未在连接前失败: client=%v err=%v", client, err)
	}

	var client *live.Client
	if err := client.Close(); err != nil {
		t.Fatalf("nil Redis 客户端关闭失败: %v", err)
	}
	if err := client.RecordFailure("deployment", 1, time.Minute); err != nil {
		t.Fatalf("无 Redis 时失败计数未降级: %v", err)
	}
	if len(client.Cooled([]string{"deployment"})) != 0 || len(client.Latencies([]string{"deployment"})) != 0 || len(client.Usages([]string{"deployment"})) != 0 {
		t.Fatal("无 Redis 时路由观测应返回空结果")
	}
	if err := client.AddLatency("deployment", 12); err != nil {
		t.Fatalf("无 Redis 时延迟写入未降级: %v", err)
	}
	if err := client.AddUsage("deployment", 5); err != nil {
		t.Fatalf("无 Redis 时用量写入未降级: %v", err)
	}
	if n, err := client.HitRPM("key"); err != nil || n != 0 {
		t.Fatalf("无 Redis 时 RPM 计数错误: n=%d err=%v", n, err)
	}
	if n, err := client.HitTPM("key", 7); err != nil || n != 0 {
		t.Fatalf("无 Redis 时 TPM 计数错误: n=%d err=%v", n, err)
	}
	if err := client.ChargeSpend("key/id", 1.25); err != nil || client.HotSpend("key/id") != 0 {
		t.Fatalf("无 Redis 时热花费未降级: err=%v", err)
	}
	if len(client.PeekSpend()) != 0 || len(client.TakeSpend()) != 0 {
		t.Fatal("无 Redis 时花费队列应为空")
	}
	if err := client.AckSpend(map[string]float64{"key/id": 1}); err != nil || client.ClearSpendQueue() != nil {
		t.Fatalf("无 Redis 时花费确认或清空失败: %v", err)
	}
	if err := client.EnqueueSpend(live.SpendLog{RequestID: "request-1"}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("可靠花费入队未拒绝缺失 Redis: %v", err)
	}
	if err := client.EnqueueLog(live.SpendLog{RequestID: "request-1"}); err != nil {
		t.Fatalf("兼容日志入队未按 nil 客户端降级: %v", err)
	}
	if rows, raw := client.PeekLogs(3); rows != nil || raw != nil {
		t.Fatalf("无 Redis 时日志预览非空: rows=%#v raw=%#v", rows, raw)
	}
	if client.AckFlushed(nil, 1, "head") != nil || client.AckLogs(1) != nil || client.DrainLogs(1) != nil {
		t.Fatal("无 Redis 时日志确认或提取未降级")
	}
	if value, ok := client.GetString(context.Background(), "pin"); ok || value != "" {
		t.Fatalf("无 Redis 时字符串读取错误: %q/%v", value, ok)
	}
	client.SetString(context.Background(), "pin", "deployment", time.Minute)
}

// TestInternalProviderProtocolBoundary 验证已登记供应商协议从目录选择到 SSE 终态和实测用量的组合边界。
// 前置条件是内建供应商注册表和内存事件；验证能力去重、流终态、视频完成计量及非法结果拒绝；不发起供应商调用。
func TestInternalProviderProtocolBoundary(t *testing.T) {
	entry := config.ModelEntry{ModelInfo: map[string]any{"endpoint_types": []any{" chat ", "video", "chat", "unknown"}}}
	if got := provider.SelectedCapabilities(entry); !reflect.DeepEqual(got, []string{"chat", "video"}) {
		t.Fatalf("部署能力未按目录去重过滤: %#v", got)
	}
	if !provider.KnownEndpoint("video") || provider.KnownEndpoint("unknown") || !provider.IncludesCapability(entry, "chat") || provider.IncludesCapability(entry, "embedding") {
		t.Fatal("公开端点目录与部署能力判断不一致")
	}

	raw := []byte("event: response.completed\ndata: {\"response\":{\"id\":\"resp-1\",\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n")
	usage := provider.NativeStreamUsage("openai-responses", raw, nil)
	if usage["input_tokens"] != float64(3) || usage["output_tokens"] != float64(2) {
		t.Fatalf("Responses SSE 用量提取错误: %#v", usage)
	}
	state := provider.NativeStreamState{}
	state.Observe("openai-responses", raw)
	if !state.Completed || state.Failed || state.ResponseID != "resp-1" {
		t.Fatalf("Responses SSE 终态识别错误: %#v", state)
	}
	state.Observe("openai-responses", []byte("event: response.failed\ndata: {\"type\":\"response.failed\",\"error\":{\"message\":\"failed\"}}\n\n"))
	if !state.Failed {
		t.Fatal("失败事件被后续状态观察遗漏")
	}

	var video *provider.Transport
	transports := provider.Transports()
	for i := range transports {
		if transports[i].ID == "openai_videos" {
			video = &transports[i]
			break
		}
	}
	if video == nil || video.Billing == nil {
		t.Fatal("OpenAI 视频传输未登记计量边界")
	}
	ctx := video.Billing.Context(map[string]any{"seconds": 99})
	if got := video.Billing.Usage(map[string]any{"status": "completed", "seconds": json.Number("8.5")}, ctx); got["seconds"] != 8.5 {
		t.Fatalf("视频完成结果未使用实测秒数: %#v", got)
	}
	for _, doc := range []map[string]any{
		{"status": "in_progress", "seconds": 8},
		{"status": "completed", "seconds": -1},
		{"status": "completed", "seconds": "invalid"},
		{"status": "completed", "seconds": 8, "error": map[string]any{"message": "failed"}},
	} {
		if got := video.Billing.Usage(doc, ctx); got != nil {
			t.Errorf("视频非成功或非法结果被错误计量: doc=%#v usage=%#v", doc, got)
		}
	}
}

// TestInternalProxyParameterBoundary 验证代理控制字段不会泄漏给上游，同时保留模型协议自己的字段。
// 前置条件是内存请求正文；验证已知代理字段、未知扩展和 nil 正文边界；只修改测试局部 map，无清理需求。
func TestInternalProxyParameterBoundary(t *testing.T) {
	body := map[string]any{"model": "gpt-4o", "messages": []any{}, "api_base": "https://secret.invalid", "rpm": 10, "vendor_extension": true}
	llm.StripProxyParams(body)
	if _, ok := body["api_base"]; ok {
		t.Fatal("代理上游地址字段泄漏到供应商正文")
	}
	if _, ok := body["rpm"]; ok {
		t.Fatal("代理限流字段泄漏到供应商正文")
	}
	if body["model"] != "gpt-4o" || body["vendor_extension"] != true {
		t.Fatalf("供应商协议字段被误删: %#v", body)
	}
	llm.StripProxyParams(nil)
}

// TestInternalConfigLoadBoundary 验证磁盘配置从环境引用解析到运行默认值和原始扩展字段的服务启动边界。
// 前置条件是临时 YAML 与测试环境变量；验证 PostgreSQL 限制、默认值和模型凭据展开；临时目录由 testing 自动清理。
func TestInternalConfigLoadBoundary(t *testing.T) {
	t.Setenv("XHUB_REGRESSION_DB", "postgresql://user:pass@localhost/xhub")
	t.Setenv("XHUB_REGRESSION_KEY", "resolved-secret")
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte("general_settings:\n  database_url: os.environ/XHUB_REGRESSION_DB\n  custom_flag: kept\nrouter_settings:\n  custom_route: kept\nmodel_list:\n  - model_name: demo\n    litellm_params:\n      api_key: os.environ/XHUB_REGRESSION_KEY\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("写入临时配置失败: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("加载合法 PostgreSQL 配置失败: %v", err)
	}
	if loaded.GeneralSettings.DatabaseURL != "postgresql://user:pass@localhost/xhub" || loaded.ModelList[0].LiteLLMParams["api_key"] != "resolved-secret" {
		t.Fatalf("配置环境引用未展开: %#v", loaded)
	}
	if loaded.RouterSettings.RoutingStrategy != "random" || loaded.RouterSettings.NumRetries != 2 || loaded.RouterSettings.Timeout != 60 {
		t.Fatalf("路由默认值未建立: %#v", loaded.RouterSettings)
	}
	if loaded.GeneralRaw["custom_flag"] != "kept" || loaded.RouterRaw["custom_route"] != "kept" {
		t.Fatalf("未声明扩展配置未保留: general=%#v router=%#v", loaded.GeneralRaw, loaded.RouterRaw)
	}

	invalid := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(invalid, []byte("general_settings:\n  database_url: sqlite:///tmp/xhub.db\n"), 0o600); err != nil {
		t.Fatalf("写入非法配置失败: %v", err)
	}
	if _, err := config.Load(invalid); err == nil || !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("非 PostgreSQL 配置未被拒绝: %v", err)
	}
}

// TestInternalGeminiNativeStreamBoundary 验证 Gemini 原生流只在完成候选出现后确认终态，并合并候选与思考 token。
// 前置条件是内存 SSE 事件；验证数值类型归一、缓存 token 和完成标记；不访问 Google 或 Vertex 服务。
func TestInternalGeminiNativeStreamBoundary(t *testing.T) {
	raw := []byte("data: {\"candidates\":[{\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":4,\"candidatesTokenCount\":3,\"thoughtsTokenCount\":2,\"cachedContentTokenCount\":1}}\n\n")
	usage := provider.NativeStreamUsage("gemini", raw, nil)
	if usage["prompt_tokens"] != float64(4) || usage["completion_tokens"] != float64(5) {
		t.Fatalf("Gemini 原生流 token 汇总错误: %#v", usage)
	}
	details, _ := usage["prompt_tokens_details"].(map[string]any)
	if details["cached_tokens"] != float64(1) {
		t.Fatalf("Gemini 缓存 token 丢失: %#v", details)
	}
	state := provider.NativeStreamState{}
	state.Observe("gemini", raw)
	if !state.Completed || state.Failed {
		t.Fatalf("Gemini 完成候选终态错误: %#v", state)
	}
}
