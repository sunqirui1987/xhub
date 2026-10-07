package provider

import (
	"strings"
	"sync"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
)

var (
	mu             sync.Mutex
	types          []Type
	modelEndpoints = map[string]string{}
)

// RegisterType adds an endpoint type. A provider file calls it from init.
// 参数 t（Type）：登记类型使用的Type。
// 返回：无。这种端点类型已登记。id 为空或没有动作时不登记。
// 调用：provider/openai/chat.go、provider/qiniu/seedance.go、provider/volcengine/seedance.go
// 测试：无直接单测
func RegisterType(t Type) {
	t.ID = strings.TrimSpace(t.ID)
	t.Kind = Kind(strings.TrimSpace(string(t.Kind)))
	if t.ID == "" || len(t.Actions) == 0 {
		return
	}
	for i := range t.Actions {
		t.Actions[i].Method = strings.ToUpper(strings.TrimSpace(t.Actions[i].Method))
	}
	mu.Lock()
	types = append(types, t)
	mu.Unlock()
}

// Model is a price-map row shown in the add-model picker.
type Model struct {
	ID           string
	Provider     string
	Official     string
	Mode         string
	EndpointType string
	Source       string
	Input        float64
	Output       float64
	Priced       bool
}

// RegisterModel adds a picker row. EndpointType is the type the form selects when this model id is chosen.
// 参数 m（Model）：正在累加或展示的Model。
// 返回：无。这条可选模型已登记。选中这个模型 id 时，表单会带上它的端点类型。id 为空时不登记。
// 调用：provider/qiniu/seedance.go、provider/volcengine/seedance.go
// 测试：无直接单测
func RegisterModel(m Model) {
	m.ID = strings.TrimSpace(m.ID)
	if m.ID == "" {
		return
	}
	if m.EndpointType != "" {
		mu.Lock()
		modelEndpoints[m.ID] = m.EndpointType
		mu.Unlock()
	}
	mode := m.Mode
	if mode == "" {
		mode = m.EndpointType
	}
	catalog.Contribute(catalog.Row{
		ID:           m.ID,
		Provider:     m.Provider,
		Mode:         mode,
		EndpointType: m.EndpointType,
		Source:       m.Source,
		Input:        m.Input,
		Output:       m.Output,
		Priced:       m.Priced,
		Official:     m.Official,
	})
}

// ProviderField is one credential input for a supplier the dropdown does not
// already list.
type ProviderField struct {
	Key      string
	Label    string
	Type     string
	Required bool
	Default  string
}

// Supplier is a dropdown entry. An existing name is left as it is.
type Supplier struct {
	Name        string
	Slug        string
	Display     string
	APIBase     string
	Placeholder string
	Fields      []ProviderField
}

// RegisterSupplier stores a default API root and, when the name is new, the add-model credential fields.
// 参数 s（Supplier）：登记Supplier使用的Supplier。
// 返回：无。默认 API 根已记下。名字是新的时，添加模型用的凭据字段也记下。slug 为空时不登记。
// 调用：provider/qiniu/seedance.go、provider/volcengine/seedance.go
// 测试：无直接单测
func RegisterSupplier(s Supplier) {
	s.Slug = strings.TrimSpace(s.Slug)
	if s.Slug == "" {
		return
	}
	fields := make([]catalog.ProviderField, 0, len(s.Fields))
	for _, f := range s.Fields {
		fields = append(fields, catalog.ProviderField{
			Key: f.Key, Label: f.Label, Type: f.Type, Required: f.Required, Default: f.Default,
		})
	}
	catalog.ContributeProvider(catalog.ProviderRow{
		Name: s.Name, Slug: s.Slug, Display: s.Display, Placeholder: s.Placeholder, Fields: fields,
	})
	if base := strings.TrimRight(strings.TrimSpace(s.APIBase), "/"); base != "" {
		mu.Lock()
		if bases == nil {
			bases = map[string]string{}
		}
		bases[s.Slug] = base
		mu.Unlock()
	}
}

var bases = map[string]string{}

// APIBase returns the deployment base, or the supplier default when that is empty.
// 参数 slug（string）：供应商 slug，例如 openai、volcengine、qiniu；configured（string）：APIBase使用的configured。空串表示调用方没有提供这项。
// 返回 string（string）：部署上的 api_base。部署没填时用供应商登记的默认根。
// 调用：dataplane/official.go
// 测试：无直接单测
func APIBase(slug, configured string) string {
	configured = strings.TrimRight(strings.TrimSpace(configured), "/")
	if configured != "" {
		return configured
	}
	mu.Lock()
	defer mu.Unlock()
	return bases[strings.TrimSpace(slug)]
}

// Types returns the registered endpoint types.
// 参数：无。
// 调用：仅在 registry.go 内使用
// 测试：chat_test.go、registry_test.go、seedance_test.go
// 返回 []Type（[]Type）：已登记的端点类型。
func Types() []Type {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Type, len(types))
	copy(out, types)
	return out
}

// ModelEndpoints maps a price-map model id to the endpoint type the form preselects.
// 参数：无。
// 返回 map[string]string（map[string]string）：模型Endpoints的字符串表。没有该键表示这项没有填。
// 调用：仅在 registry.go 内使用
// 测试：match_test.go、seedance_test.go
func ModelEndpoints() map[string]string {
	mu.Lock()
	defer mu.Unlock()
	out := make(map[string]string, len(modelEndpoints))
	for k, v := range modelEndpoints {
		out[k] = v
	}
	return out
}

// PublicBody is the add-model payload: the type list and the model defaults.
// 参数：无。
// 返回 map[string]any（map[string]any）：价目表模型 id 到默认端点类型。没有该键表示选中模型时不预选端点。
// 调用：catalog/classify.go、gateway/bypass.go、gateway/catalog.go
// 测试：无直接单测
func PublicBody() map[string]any {
	return map[string]any{"types": Types(), "models": ModelEndpoints()}
}

// Match finds the bypass action for this method and path. A custom endpoint stored on a deployment is visible only for that deployment. Adapted types are not matched here; those paths stay on the existing handlers.
// 参数 method（string）：HTTP 方法，例如 GET 或 POST；path（string）：URL 路径，用来匹配路由；models（[]config.ModelEntry）：候选部署列表，后面按策略挑一条。
// 返回 Hit（Hit）：匹配到的 bypass 动作和路径参数。没有命中时是零值；bool（bool）：这个方法和路径匹配到一条 bypass 动作时返回真。适配类型不在这里匹配。
// 调用：gateway/bypass.go
// 测试：bypass_logic_test.go、chat_test.go、match_test.go
func Match(method, path string, models []config.ModelEntry) (Hit, bool) {
	method = strings.ToUpper(method)
	path = strings.TrimSuffix(path, "/")
	var custom Hit
	customOK := false
	for _, m := range models {
		customType, ok := overrideType(m)
		if !ok {
			continue
		}
		for _, action := range customType.Actions {
			names, ok := matchPath(action.PublicPath, path)
			if !ok || action.Method != method {
				continue
			}
			custom = Hit{Type: customType, Action: action, Names: names, DeploymentName: m.ModelName}
			customOK = true
		}
	}
	for _, t := range Types() {
		if t.Kind != KindBypass {
			continue
		}
		for _, action := range t.Actions {
			if action.Method != method {
				continue
			}
			names, ok := matchPath(action.PublicPath, path)
			if !ok {
				continue
			}
			return Hit{Type: t, Action: action, Names: names}, true
		}
	}
	if customOK {
		return custom, true
	}
	return Hit{}, false
}

// 把路径模板和真实路径逐段比较，抽出花括号里的参数。
// 参数 pattern（string）：要匹配的路径模板或正则；path（string）：URL 路径，用来匹配路由。
// 返回 map[string]string（map[string]string）：路径模板里抽出的参数，例如任务 id。不匹配时为 nil，同时布尔值为假；bool（bool）：路径和模板逐段对上、并抽出花括号参数时返回真。
// 调用：仅在 registry.go 内使用
// 测试：无直接单测
func matchPath(pattern, path string) (map[string]string, bool) {
	pattern = strings.TrimSuffix(pattern, "/")
	pp := strings.Split(strings.Trim(pattern, "/"), "/")
	aa := strings.Split(strings.Trim(path, "/"), "/")
	if pattern == "" || path == "" {
		return nil, false
	}
	if len(pp) != len(aa) {
		return nil, false
	}
	names := map[string]string{}
	for i := range pp {
		if strings.HasPrefix(pp[i], "{") && strings.HasSuffix(pp[i], "}") {
			name := strings.Trim(pp[i], "{}")
			if aa[i] == "" {
				return nil, false
			}
			names[name] = aa[i]
			continue
		}
		if pp[i] != aa[i] {
			return nil, false
		}
	}
	return names, true
}

// overrideType 读取部署上自定义的 bypass 端点。endpoint 不是带 action 的 bypass 映射时，返回假。
// 参数 m：一条部署。endpoint 必须是 map，且 kind 为 bypass，并至少有一个 action。
// 返回：解析出的自定义端点类型，以及是否真有这份覆盖。
// 调用：仅 registry.go 内的 Match、ApplyOverride、BoundTypes。测试：registry_test.go TestCustomBypassReadsTheDocumentFields。
func overrideType(m config.ModelEntry) (Type, bool) {
	if m.LiteLLMParams == nil {
		return Type{}, false
	}
	raw, ok := m.LiteLLMParams["endpoint"]
	if !ok || raw == nil {
		return Type{}, false
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return Type{}, false
	}
	t := Type{
		ID:          "custom",
		Kind:        KindBypass,
		APIBase:     str(obj["api_base"]),
		ModelField:  str(obj["model_field"]),
		TaskID:      str(obj["task_id"]),
		StripPrefix: str(obj["strip_prefix"]),
	}
	if kind := str(obj["kind"]); kind != "" {
		t.Kind = Kind(kind)
	}
	if t.Kind != KindBypass {
		return Type{}, false
	}
	actions, _ := obj["actions"].([]any)
	for _, item := range actions {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t.Actions = append(t.Actions, Action{
			Name:         str(row["name"]),
			Method:       strings.ToUpper(str(row["method"])),
			PublicPath:   str(row["public_path"]),
			UpstreamPath: str(row["upstream_path"]),
			TaskQuery:    str(row["task_query"]),
		})
	}
	if len(t.Actions) == 0 {
		return Type{}, false
	}
	return t, true
}

// 把动态值收成去掉空白的字符串。不是字符串时为空串。
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 string（string）：按字符串读出的值。不是字符串或没有该键时为空串，不 panic。
// 调用：仅在 registry.go 内使用。
// 测试：无直接单测
func str(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// ApplyOverride copies a deployment's saved endpoint onto the matched type. Public paths stay on the type. Edited fields and upstream paths come from the deployment.
// 参数 m（config.ModelEntry）：一条模型部署，含对外名、供应商参数和价格覆盖；hit（Hit）：这次路由命中的端点类型。Type 决定上游动作，路径参数在 Params 里。
// 返回 Hit（Hit）：路径匹配到的端点类型和动作。
// 调用：dataplane/official.go
// 测试：无直接单测
func ApplyOverride(m config.ModelEntry, hit Hit) Hit {
	custom, ok := overrideType(m)
	if !ok {
		return hit
	}
	if custom.ModelField != "" {
		hit.Type.ModelField = custom.ModelField
	}
	if custom.TaskID != "" {
		hit.Type.TaskID = custom.TaskID
	}
	if custom.StripPrefix != "" {
		hit.Type.StripPrefix = custom.StripPrefix
	}
	if custom.APIBase != "" {
		hit.Type.APIBase = custom.APIBase
	}
	for _, action := range custom.Actions {
		if action.Name != hit.Action.Name {
			continue
		}
		if action.UpstreamPath != "" {
			hit.Action.UpstreamPath = action.UpstreamPath
		}
		if action.TaskQuery != "" {
			hit.Action.TaskQuery = action.TaskQuery
		}
	}
	return hit
}

// SelectedTypes is the endpoint types a deployment answers. endpoint_types wins when it is set, so one model can answer more than one type. An empty model info means chat.
// 参数 m（config.ModelEntry）：一条模型部署，含对外名、供应商参数和价格覆盖。
// 返回 []string（[]string）：SelectedTypes。没有匹配时为空切片。
// 调用：仅在 registry.go 内使用
// 测试：registry_test.go
func SelectedTypes(m config.ModelEntry) []string {
	if m.ModelInfo != nil {
		if list := stringList(m.ModelInfo["endpoint_types"]); len(list) > 0 {
			return list
		}
		if mode := str(m.ModelInfo["mode"]); mode != "" {
			return []string{mode}
		}
	}
	return []string{"chat"}
}

// Includes reports that this deployment selected typeID.
// 参数 m（config.ModelEntry）：一条模型部署，含对外名、供应商参数和价格覆盖；typeID（string）：Includes使用的类型标识。空串表示调用方没有提供这项。
// 返回 bool（bool）：这条部署选中了这个端点类型时返回真。
// 调用：dataplane/official.go
// 测试：registry_test.go
func Includes(m config.ModelEntry, typeID string) bool {
	for _, id := range SelectedTypes(m) {
		if id == typeID {
			return true
		}
	}
	return false
}

// BoundTypes resolves the selected ids to their registered actions. A custom bypass stored on the deployment is included when "custom" is selected.
// 参数 m（config.ModelEntry）：一条模型部署，含对外名、供应商参数和价格覆盖。
// 返回 []Type（[]Type）：已登记的端点类型。
// 调用：仅在 registry.go 内使用
// 测试：bypass_logic_test.go、registry_test.go
func BoundTypes(m config.ModelEntry) []Type {
	var out []Type
	for _, id := range SelectedTypes(m) {
		if id == "custom" {
			if custom, ok := overrideType(m); ok {
				out = append(out, custom)
			}
			continue
		}
		for _, t := range Types() {
			if t.ID == id {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

// ModeOf is the first selected endpoint type. An empty mode is chat.
// 参数 m（config.ModelEntry）：一条模型部署，含对外名、供应商参数和价格覆盖。
// 返回 string（string）：模型选中的第一个端点类型。没有选择时按 chat。
// 调用：仅在 registry.go 内使用
// 测试：无直接单测
func ModeOf(m config.ModelEntry) string {
	return SelectedTypes(m)[0]
}

// 把配置里的字符串或数组收成 []string。
// 参数 v（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 []string（[]string）：字符串列表。没有匹配时为 nil 或空切片，调用方按长度判断。
// 调用：SelectedTypes 在读取 endpoint_types 时。
// 测试：无直接单测
func stringList(v any) []string {
	switch list := v.(type) {
	case []string:
		return list
	case []any:
		var out []string
		for _, item := range list {
			if s := str(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
