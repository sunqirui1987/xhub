package models

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

const (
	BuiltinFenno = "fennoai"
	BuiltinQiniu = "qiniu"
)

// Builtin is a provider credential installed with the gateway. Models added from it use the same row as a hand-added model.
type Builtin struct {
	ID     string
	Base   string
	KeyEnv string
}

// Builtins are the two providers a new install can add models from. Fenno calls https://api.fenno.ai. Qiniu calls the OpenAI bypass root. Catalogs stay on /v1/models.
// 参数：无。
// 返回 []Builtin（[]Builtin）：新安装可以添加模型的两个供应商。Fenno 走 https://api.fenno.ai，七牛走 OpenAI 兼容根。
// 调用：gateway/limits.go
// 测试：无直接单测
func Builtins() []Builtin {
	return []Builtin{
		{ID: BuiltinFenno, Base: "https://api.fenno.ai", KeyEnv: "FENNOAI_API_KEY"},
		{ID: BuiltinQiniu, Base: llm.QiniuBypassBase, KeyEnv: "QINIU_API_KEY"},
	}
}

// BuiltinsEnabled reports whether first install should add fennoai and qiniu. XHUB_BUILTIN_PROVIDERS defaults to on. 0, false, off, no, or disabled turns it off.
// 参数：无。
// 返回 bool（bool）：首次安装应加入 fennoai 和 qiniu 时返回真。XHUB_BUILTIN_PROVIDERS 设为 0、false、off、no 或 disabled 时返回假。
// 调用：仅在 builtin.go 内使用
// 测试：builtin_test.go
func BuiltinsEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("XHUB_BUILTIN_PROVIDERS"))) {
	case "0", "false", "off", "no", "disable", "disabled":
		return false
	default:
		return true
	}
}

// CatalogModel is one card in the provider catalog. Prices are copied from the payload and stay nil when absent.
type CatalogModel struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	InputPrice  *float64 `json:"input_price"`
	OutputPrice *float64 `json:"output_price"`
	Added       bool     `json:"added"`
}

// ParseModelIDs reads an OpenAI models list. A body with no ids returns an empty slice.
// 参数 body（[]byte）：原始正文。可能是 JSON，也可能是 SSE，由调用方按内容解析。
// 返回 []string（[]string）：解析模型标识列表。没有匹配时为 nil 或空切片，调用方按长度判断。
// 调用：仅在 builtin.go 内使用
// 测试：builtin_test.go
func ParseModelIDs(body []byte) []string {
	items := ParseCatalog(body)
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// ParseCatalog maps an OpenAI-style models document to catalog cards.
// 参数 body（[]byte）：原始正文。可能是 JSON，也可能是 SSE，由调用方按内容解析。
// 返回 []CatalogModel（[]CatalogModel）：OpenAI 风格模型目录转成的卡片。同一个 id 只留一张。
// 调用：仅在 builtin.go 内使用
// 测试：builtin_test.go
func ParseCatalog(body []byte) []CatalogModel {
	rows := catalogObjects(body)
	seen := map[string]bool{}
	var items []CatalogModel
	for _, row := range rows {
		id := strings.TrimSpace(str(row["id"]))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		category := catalogCategory(id, row)
		item := CatalogModel{ID: id, Category: category}
		if tokenPricedCategory(category) {
			item.InputPrice = catalogTokenPrice(row, "input_price", "input_cost_per_token", "input", "prompt")
			item.OutputPrice = catalogTokenPrice(row, "output_price", "output_cost_per_token", "output", "completion")
		}
		items = append(items, item)
	}
	return items
}

// tokenPricedCategory reports whether catalog prices for this category use token units.
// 参数 category（string）：供应商目录返回的模型类别。
// 返回 bool（bool）：前端可按每百万 token 展示输入输出价格时为真。
// 调用：ParseCatalog、fillFromCostMap。
// 测试：builtin_test.go。
func tokenPricedCategory(category string) bool {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "llm", "chat", "vision", "embedding":
		return true
	default:
		return false
	}
}

// catalogTokenPrice returns USD per million tokens. Provider catalogs commonly
// expose input_price/output_price in that display unit, while normalized API
// fields are USD per token and must be scaled before reaching the frontend.
// 参数 row（map[string]any）：供应商目录中的一行；displayKey（string）：已按每百万 token 表示的字段；normalizedKeys（...string）：按单 token 表示的候选字段。
// 返回 *float64（*float64）：每百万 token 的美元价格，缺失时为 nil。
// 调用：ParseCatalog。
// 测试：builtin_test.go。
func catalogTokenPrice(row map[string]any, displayKey string, normalizedKeys ...string) *float64 {
	if price := catalogPrice(row, displayKey); price != nil {
		return price
	}
	if price := catalogPrice(row, normalizedKeys...); price != nil {
		scaled := *price * 1_000_000
		return &scaled
	}
	return nil
}

// 从目录响应里取出模型数组。既接受 data 包裹，也接受裸数组。
// 参数 body（[]byte）：原始正文。可能是 JSON，也可能是 SSE，由调用方按内容解析。
// 返回 []map[string]any（[]map[string]any）：一组map[string]any。没有匹配时为空切片，不是 nil 分页。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func catalogObjects(body []byte) []map[string]any {
	var doc struct {
		Data []map[string]any `json:"data"`
	}
	if json.Unmarshal(body, &doc) != nil || doc.Data == nil {
		var bare []map[string]any
		if json.Unmarshal(body, &bare) != nil {
			return nil
		}
		return bare
	}
	return doc.Data
}

// 确定目录模型的分类。没有明确分类时按 id 推断。
// 参数 id（string）：目录分类使用的主键。空串表示调用方没有指定记录；row（map[string]any）：一行价格或模型字段。缺键表示价目表没有这项。
// 返回 string（string）：目录行的分类。没有分类字段时按模型 id 推断。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func catalogCategory(id string, row map[string]any) string {
	for _, key := range []string{"category", "type", "modality"} {
		if value := strings.TrimSpace(str(row[key])); value != "" && value != "model" {
			return value
		}
	}
	lower := strings.ToLower(id)
	switch {
	case strings.Contains(lower, "embed"):
		return "embedding"
	case strings.Contains(lower, "vision") || strings.Contains(lower, "-vl"):
		return "vision"
	case strings.Contains(lower, "whisper") || strings.Contains(lower, "tts") || strings.Contains(lower, "audio"):
		return "audio"
	}
	if i := strings.Index(id, "/"); i > 0 {
		return id[:i]
	}
	return "llm"
}

// 从目录行或其 pricing 对象里取出第一个价格。没有时为 nil。
// 参数 row（map[string]any）：一行价格或模型字段。缺键表示价目表没有这项；keys（...string）：目录价格使用的string。
// 返回 *float64（*float64）：从目录行或其 pricing 对象里取出第一个价格。找不到或这一步失败时为 nil。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func catalogPrice(row map[string]any, keys ...string) *float64 {
	if price := firstNumber(row, keys...); price != nil {
		return price
	}
	pricing, _ := row["pricing"].(map[string]any)
	if pricing == nil {
		return nil
	}
	return firstNumber(pricing, keys...)
}

// 按键的顺序取出第一个能解析的数字。没有时为 nil。
// 参数 row（map[string]any）：一行价格或模型字段。缺键表示价目表没有这项；keys（...string）：首个数字使用的string。
// 返回 *float64（*float64）：按键的顺序取出第一个能解析的数字。找不到或这一步失败时为 nil。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func firstNumber(row map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			if number := asFloat(value); number != nil {
				return number
			}
		}
	}
	return nil
}

// 把 JSON 数字收成 *float64。解析失败时为 nil。
// 参数 value（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 *float64（*float64）：把 JSON 数字收成 *float64。找不到或这一步失败时为 nil。
// 调用：仅在 builtin.go 内使用。
// 测试：无直接单测
func asFloat(value any) *float64 {
	switch number := value.(type) {
	case float64:
		return &number
	case json.Number:
		parsed, err := number.Float64()
		if err != nil {
			return nil
		}
		return &parsed
	default:
		return nil
	}
}

// ModelsURL 拼出拉取上游模型目录的地址。供应商标识决定默认根路径。
// 参数 provider（string）：供应商标识，例如 openai 或 volcengine；base（string）：根地址或完整 URL。空串表示改用供应商默认根，末尾斜杠会去掉。
// 返回 string（string）：拉取上游模型目录的地址。fenno 和七牛用固定地址，其他供应商用 base 拼上 /models。
// 调用：仅在 builtin.go 内使用
// 测试：builtin_test.go
func ModelsURL(provider, base string) string {
	if provider == BuiltinFenno {
		return "https://api.fenno.ai/v1/models"
	}
	if provider == BuiltinQiniu {
		return "https://api.qnaigc.com/v1/models"
	}
	return strings.TrimRight(strings.TrimSpace(base), "/") + "/models"
}

// builtinClient is the HTTP client that fetches provider model lists. Tests replace it.
var builtinClient = http.DefaultClient

// SetBuiltinClient replaces the client used to fetch builtin model lists.
// 参数 c（*http.Client）：写入内置客户端使用的客户端。
// 返回：无。拉取内置模型目录用的客户端已换成传入的值。传入 nil 时恢复成 http.DefaultClient。
// 调用：仅在 builtin.go 内使用
// 测试：builtin_providers_test.go
func SetBuiltinClient(c *http.Client) {
	if c == nil {
		builtinClient = http.DefaultClient
		return
	}
	builtinClient = c
}

// SeedBuiltins removes leftover provider rows and creates the two credentials when they are missing. It does not insert models. A model added later is the same row a person would save by hand.
// 参数 s（Host）：写入初始Builtins使用的数据面宿主。
// 返回：无。误存成模型的供应商行已删掉，缺失的 Fenno 和七牛凭据已补上。不插入模型。
// 调用：gateway/models/admin.go
// 测试：无直接单测
func SeedBuiltins(s Host) {
	if s == nil || s.RecordStore() == nil || !BuiltinsEnabled() {
		return
	}
	dropProviderShells(s)
	for _, spec := range Builtins() {
		seedCredential(s, spec, strings.TrimSpace(os.Getenv(spec.KeyEnv)))
	}
}

// dropProviderShells deletes fennoai and qiniu rows that were stored as models. They are credentials. Leaving them in the model table makes the playground list the provider name.
// 参数 s（Host）：丢掉供应商Shells使用的数据面宿主。
// 返回：无。存成模型的 fennoai 和 qiniu 行已删除。它们应是凭据，留在模型表里会被当成可调用模型。
// 调用：gateway/models/admin.go
// 测试：无直接单测
func dropProviderShells(s Host) {
	rows, err := s.RecordStore().ListProxyModels()
	if err != nil {
		logx.Error("builtin providers list: %v", err)
		return
	}
	for _, row := range rows {
		if str(row.Info["role"]) != "provider" {
			continue
		}
		if _, ok := builtinByID(str(row.Info["builtin"])); !ok {
			continue
		}
		if err := s.RecordStore().DeleteProxyModel(row.ID); err != nil {
			logx.Error("builtin provider %s: %v", row.ID, err)
		}
	}
}

// RefreshBuiltin reloads the catalog. It does not add or delete models.
// 参数 s（Host）：Refresh内置使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func RefreshBuiltin(s Host, w http.ResponseWriter, r *http.Request) {
	ListBuiltin(s, w, r)
}

// ListBuiltin returns the specialized builtin catalog or model IDs discovered
// through a saved OpenAI-compatible credential. It does not write.
// 参数 s（Host）：列出内置使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func ListBuiltin(s Host, w http.ResponseWriter, r *http.Request) {
	source, ok := openCatalog(s, w, r)
	if !ok {
		return
	}
	already := savedNames(source.rows, source.spec.ID)
	var available []CatalogModel
	var fetchErr string
	urlProvider := ""
	if source.specialized {
		urlProvider = source.spec.ID
	}
	items, err := fetchCatalog(ModelsURL(urlProvider, source.spec.Base), source.key)
	if err != nil {
		fetchErr = err.Error()
	} else {
		available = items
	}
	if available == nil {
		available = []CatalogModel{}
	}
	models := []map[string]any{}
	modelIDs := []string{}
	for _, item := range available {
		modelIDs = append(modelIDs, item.ID)
		if source.specialized {
			fillFromCostMap(&item)
			item.Added = already[item.ID]
			models = append(models, catalogJSON(item))
		} else {
			models = append(models, map[string]any{"id": item.ID})
		}
	}
	out := map[string]any{
		"provider":        source.spec.ID,
		"credential_name": source.credentialName,
		"api_base":        source.spec.Base,
		"models":          models,
		"model_ids":       modelIDs,
	}
	if fetchErr != "" {
		out["error"] = fetchErr
	}
	httpx.WriteJSON(w, 200, out)
}

// AddBuiltinModels is the retired catalog-to-deployment shortcut. Catalog rows
// now enter /price/model and deployments are created through /model/new, so a
// caller cannot bypass the unified pricing editor.
// 参数 s（Host）：累加内置模型使用的数据面宿主；w（http.ResponseWriter）：调用方的 HTTP 响应，状态和正文写在这里；r（*http.Request）：入站 HTTP 请求。
// 返回：无。状态码和正文写进调用方的响应。
// 调用：gateway/models/mount.go
// 测试：无直接单测
func AddBuiltinModels(s Host, w http.ResponseWriter, r *http.Request) {
	if s.RequireManage(w, r) == nil {
		return
	}
	httpx.WriteError(w, http.StatusGone, "gone", "catalog deployment import moved to /price/model followed by /model/new")
}

// addedModel 判断这个目录模型是否已经在当前部署表里。
// 参数 provider（string）：供应商标识，例如 openai。name（string）：目录里的模型 id，按表单填写的方式保存。
// 返回：一条可写入模型表的部署。
// 调用：内置供应商往目录里加模型之前。
// 测试：无直接单测
func addedModel(provider, name string) config.ModelEntry {
	return config.ModelEntry{
		ModelName: name,
		LiteLLMParams: map[string]any{
			"model":                   name,
			"custom_llm_provider":     "openai",
			"litellm_credential_name": provider,
		},
		ModelInfo: map[string]any{
			"id":         "model_" + httpx.CallID()[:12],
			"db_model":   true,
			"created_at": time.Now().UTC().Format(time.RFC3339),
		},
	}
}

type catalogSource struct {
	spec           Builtin
	rows           []store.ProxyModel
	key            string
	credentialName string
	specialized    bool
}

// openCatalog resolves either a specialized builtin catalog or a generic saved
// OpenAI-compatible credential. Generic discovery always takes its base URL and
// secret from the saved credential, never from request fields.
// 参数 s（Host）：目录发现使用的数据面宿主；w（http.ResponseWriter）：错误响应写入这里；r（*http.Request）：包含 provider 或 credential_name 的管理请求。
// 返回 catalogSource（catalogSource）：已解析的目录连接和现有部署；bool（bool）：请求有效且可以发起目录请求时为真。
// 调用：ListBuiltin。
// 测试：builtin_providers_test.go。
func openCatalog(s Host, w http.ResponseWriter, r *http.Request) (catalogSource, bool) {
	if s.RequireManage(w, r) == nil {
		return catalogSource{}, false
	}
	body := readBody(r)
	rows, err := s.RecordStore().ListProxyModels()
	if err != nil {
		httpx.WriteError(w, 500, "internal", err.Error())
		return catalogSource{}, false
	}
	provider := strings.TrimSpace(str(body["provider"]))
	name := strings.TrimSpace(str(body["credential_name"]))
	if spec, found := builtinByID(provider); found && name == "" {
		key := providerKey(s, spec, str(body["api_key"]))
		return catalogSource{spec: spec, rows: rows, key: key, credentialName: name, specialized: true}, true
	}
	if name == "" {
		httpx.WriteError(w, 400, "invalid_request", "credential_name is required for generic model discovery")
		return catalogSource{}, false
	}
	record, ok := openAICompatibleCredential(s, w, name)
	if !ok {
		return catalogSource{}, false
	}
	info, _ := record["credential_info"].(map[string]any)
	values, _ := record["credential_values"].(map[string]any)
	base := credentialText(values, "api_base")
	if base == "" {
		base = credentialText(info, "api_base")
	}
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	// Default builtin connections have their own discovery endpoint. A custom
	// saved address must still be used directly instead of that default.
	if builtin, found := builtinByID(str(info["builtin"])); found && strings.TrimRight(base, "/") == builtin.Base {
		return catalogSource{
			spec: builtin, rows: rows, key: credentialSecret(record, "api_key"), credentialName: name, specialized: true,
		}, true
	}
	// Saved credentials always determine the discovery URL, even when the
	// request includes a builtin provider hint or the credential has that name.
	spec := Builtin{ID: name, Base: base}
	return catalogSource{
		spec: spec, rows: rows, key: credentialSecret(record, "api_key"), credentialName: name,
	}, true
}

// openAICompatibleCredential reads one saved credential and verifies that its
// declared wire protocol can use an OpenAI /models endpoint.
// 参数 s（Host）：凭据存储宿主；w（http.ResponseWriter）：错误响应写入这里；name（string）：保存的凭据名称。
// 返回 map[string]any（map[string]any）：未脱敏的服务端凭据；bool（bool）：凭据存在且协议兼容时为真。
// 调用：openCatalog。
// 测试：builtin_providers_test.go。
func openAICompatibleCredential(s Host, w http.ResponseWriter, name string) (map[string]any, bool) {
	record, err := s.RecordStore().GetKV("credentials", name)
	if err != nil || record == nil {
		httpx.WriteError(w, 400, "invalid_request", "model provider is not configured")
		return nil, false
	}
	info, _ := record["credential_info"].(map[string]any)
	values, _ := record["credential_values"].(map[string]any)
	protocol := credentialText(info, "custom_llm_provider")
	if protocol == "" {
		protocol = credentialText(values, "custom_llm_provider")
	}
	if protocol != "" && !strings.EqualFold(protocol, "openai") {
		httpx.WriteError(w, 400, "invalid_request", "credential protocol is not compatible with OpenAI model discovery")
		return nil, false
	}
	return record, true
}

// credentialSecret returns a credential value after resolving the supported
// os.environ/NAME indirection. It is used only for the outbound request.
// 参数 record（map[string]any）：服务端凭据对象；key（string）：要读取的秘密字段。
// 返回 string（string）：解析后的秘密，缺失时为空串。
// 调用：openCatalog。
// 测试：builtin_providers_test.go。
func credentialSecret(record map[string]any, key string) string {
	values, _ := record["credential_values"].(map[string]any)
	return credentialText(values, key)
}

// credentialText trims a saved credential string and resolves os.environ/NAME.
// 参数 fields（map[string]any）：credential_info 或 credential_values；key（string）：要读取的字段。
// 返回 string（string）：解析后的文本，缺失时为空串。
// 调用：openCatalog、openAICompatibleCredential、credentialSecret。
// 测试：builtin_providers_test.go。
func credentialText(fields map[string]any, key string) string {
	value := strings.TrimSpace(str(fields[key]))
	if strings.HasPrefix(value, "os.environ/") {
		return strings.TrimSpace(os.Getenv(strings.TrimPrefix(value, "os.environ/")))
	}
	return value
}

// 读完请求正文并解析成对象。空正文得到空对象，不返回 nil。
// 参数 r（*http.Request）：入站 HTTP 请求，用来读路径、头和正文。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func readBody(r *http.Request) map[string]any {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	if body == nil {
		body = map[string]any{}
	}
	return body
}

// providerKey is the key used only to read the catalog. The saved model does not copy it.
// 参数 s（Host）：供应商密钥使用的数据面宿主；spec（Builtin）：供应商密钥使用的内置供应商；requestKey（string）：供应商密钥使用的请求密钥。空串表示调用方没有提供这项。
// 返回 string（string）：只用来读目录的密钥。请求里的真实密钥优先，否则用已存凭据，再否则用供应商环境变量。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func providerKey(s Host, spec Builtin, requestKey string) string {
	if key := realKey(requestKey); key != "" {
		return key
	}
	if key := credentialAPIKey(s, spec.ID); key != "" {
		return key
	}
	return realKey(os.Getenv(spec.KeyEnv))
}

// 判断这是不是可以保存的真实密钥。掩码占位符返回空串。
// 参数 raw（string）：真实密钥使用的原始内容。空串表示调用方没有提供这项。
// 返回 string（string）：去掉空白后的真实密钥。空串或带掩码的占位符返回空串，表示不要覆盖原密钥。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func realKey(raw string) string {
	key := strings.TrimSpace(raw)
	if key == "" || strings.Contains(key, "****") {
		return ""
	}
	return key
}

// 按凭据 id 读取保存的 api_key。
// 参数 s（Host）：凭据APIKey使用的数据面宿主；id（string）：密钥 id。空串表示没有指定密钥。
// 返回 string（string）：凭据表里的 api_key。库不可用或没有这条凭据时为空串。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func credentialAPIKey(s Host, id string) string {
	if s.RecordStore() == nil {
		return ""
	}
	rec, err := s.RecordStore().GetKV("credentials", id)
	if err != nil {
		return ""
	}
	values, _ := rec["credential_values"].(map[string]any)
	return strings.TrimSpace(str(values["api_key"]))
}

// 用密钥拉取上游模型目录。有密钥时带 Bearer。
// 参数 rawURL（string）：根地址或完整 URL。空串表示改用供应商默认根，末尾斜杠会去掉；key（string）：上游或调用方的密钥。空串表示还不能转发或还没有密钥。
// 返回 []CatalogModel（[]CatalogModel）：用密钥拉取上游模型目录。没有行时为空切片；error（error）：失败原因，nil 表示成功。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func fetchCatalog(rawURL, key string) ([]CatalogModel, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("x-api-key", key)
	}
	resp, err := builtinClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, errString(resp.Status)
	}
	return ParseCatalog(body), nil
}

// fillFromCostMap copies input/output prices from the price-data map when the catalog payload omitted them. The stored rates are per token; the card shows the same per-million-token dollars as Price Data Management.
// 参数 item（*CatalogModel）：填充来源费用表使用的目录中的模型。
// 返回：无。目录没带的输入或输出单价已从价格表补上。两边都有价格时不改。单价按每个 token 计。
// 调用：仅在 builtin.go 内使用
// 测试：builtin_test.go
func fillFromCostMap(item *CatalogModel) {
	if item == nil || !tokenPricedCategory(item.Category) {
		return
	}
	if item.InputPrice != nil && item.OutputPrice != nil {
		return
	}
	row := costRow(item.ID)
	if row == nil {
		return
	}
	if item.InputPrice == nil {
		item.InputPrice = perMillion(row["input_cost_per_token"])
	}
	if item.OutputPrice == nil {
		item.OutputPrice = perMillion(row["output_cost_per_token"])
	}
}

// 从内置价目表取价格行。带前缀找不到时再试去掉一个前缀。
// 参数 id（string）：费用行使用的主键。空串表示调用方没有指定记录。
// 返回 map[string]any（map[string]any）：费用行的字段表。缺键表示上游或库里没有这个字段。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func costRow(id string) map[string]any {
	prices := catalog.CostMap()
	if row := prices[id]; row != nil {
		return row
	}
	if i := strings.LastIndex(id, "/"); i >= 0 {
		if row := prices[id[i+1:]]; row != nil {
			return row
		}
	}
	return nil
}

// 把每 token 单价换成每百万 token。类型不符时为 nil。
// 参数 value（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 *float64（*float64）：把每 token 单价换成每百万 token。找不到或这一步失败时为 nil。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func perMillion(value any) *float64 {
	rate, ok := value.(float64)
	if !ok {
		return nil
	}
	scaled := rate * 1_000_000
	return &scaled
}

// 把目录模型收成返回给控制台的对象。
// 参数 item（CatalogModel）：目录JSON使用的目录中的模型。
// 返回 map[string]any（map[string]any）：给响应或报表用的 JSON 对象。键是前端已经约定的字段，缺键表示这项没有数据。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func catalogJSON(item CatalogModel) map[string]any {
	return map[string]any{
		"id":           item.ID,
		"category":     item.Category,
		"input_price":  item.InputPrice,
		"output_price": item.OutputPrice,
		"added":        item.Added,
	}
}

type errString string

// 实现 error 接口，返回写进日志或 HTTP 错误体的文本。
// 参数：无。
// 返回 string（string）：error 接口的文本，给日志和 HTTP 错误体使用。
// 调用：拉取目录失败时返回它，经 error 接口读取。
// 测试：无直接单测
func (e errString) Error() string { return string(e) }

// 按 id 查找内置供应商。找不到时布尔值为假。
// 参数 id（string）：内置按标识使用的主键。空串表示调用方没有指定记录。
// 返回 Builtin（Builtin）：按 id 查找内置供应商。没有命中时为零值；bool（bool）：按 id 找到了内置供应商时返回真。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func builtinByID(id string) (Builtin, bool) {
	for _, b := range Builtins() {
		if b.ID == id {
			return b, true
		}
	}
	return Builtin{}, false
}

// savedNames reports catalog ids already stored for this credential, including older builtin rows.
// 参数 rows（[]store.ProxyModel）：从用量或目录读出的ProxyModel；provider（string）：供应商标识，例如 openai 或 volcengine。
// 返回 map[string]bool（map[string]bool）：saved名称。没有该键表示假，不要当成缺省 JSON。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func savedNames(rows []store.ProxyModel, provider string) map[string]bool {
	out := map[string]bool{}
	for _, row := range rows {
		if str(row.Info["role"]) == "provider" {
			continue
		}
		if str(row.Params["litellm_credential_name"]) == provider && row.ModelName != "" {
			out[row.ModelName] = true
		}
		if str(row.Info["builtin"]) == provider {
			if id := str(row.Info["upstream_id"]); id != "" {
				out[id] = true
			}
		}
	}
	return out
}

// clearCopiedMode removes a mode that older imports copied onto the model. A hand-added model has no mode, so the playground treats it as chat.
// 参数 row（*store.ProxyModel）：从用量或目录读出的ProxyModel。
// 返回 bool（bool）：删掉了旧导入抄到模型上的 mode 时返回真。手加的模型没有 mode，游乐场会把它当成 chat。
// 调用：gateway/models/admin.go
// 测试：builtin_test.go
func clearCopiedMode(row *store.ProxyModel) bool {
	if row.Info == nil || str(row.Info["builtin"]) == "" {
		return false
	}
	mode := str(row.Info["mode"])
	if mode != "chat" && mode != "responses" {
		return false
	}
	delete(row.Info, "mode")
	return true
}

// 在凭据不存在时写入一条初始凭据。已经存在则不覆盖。
// 参数 s（Host）：写入初始凭据使用的数据面宿主；spec（Builtin）：写入初始凭据使用的内置供应商；key（string）：上游或调用方的密钥。空串表示还不能转发或还没有密钥。
// 返回：无。凭据不存在时写入了一条初始凭据。已经存在则不覆盖。
// 调用：仅在 builtin.go 内使用
// 测试：无直接单测
func seedCredential(s Host, spec Builtin, key string) {
	if _, err := s.RecordStore().GetKV("credentials", spec.ID); err == nil {
		return
	}
	raw, err := json.Marshal(map[string]any{
		"credential_name": spec.ID,
		"credential_info": map[string]any{
			"custom_llm_provider": "openai",
			"builtin":             spec.ID,
			"api_base":            spec.Base,
		},
		"credential_values": map[string]any{
			"api_key":  key,
			"api_base": spec.Base,
		},
	})
	if err != nil {
		return
	}
	if err := s.RecordStore().PutKV("credentials", spec.ID, string(raw)); err != nil {
		logx.Error("builtin credential %s: %v", spec.ID, err)
	}
}
