package provider

import (
	"strings"
	"sync"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/logx"
)

var (
	mu             sync.Mutex
	transports     []Transport
	modelEndpoints = map[string]string{}
)

// RegisterTransport 登记一个内置转发方式。供应商文件在 init 里调它。
// 参数 t（Transport）：要登记的内置转发方式。
// 返回：无。id 为空或没有动作时不登记——一个没有动作的转发方式无处可转。
// 调用：provider/volcengine/seedance.go
// 测试：seedance_test.go
func RegisterTransport(t Transport) {
	t.ID = strings.TrimSpace(t.ID)
	t.Kind = Kind(strings.TrimSpace(string(t.Kind)))
	if t.ID == "" || len(t.Actions) == 0 {
		return
	}
	for i := range t.Actions {
		t.Actions[i].Method = strings.ToUpper(strings.TrimSpace(t.Actions[i].Method))
	}
	mu.Lock()
	transports = append(transports, t)
	mu.Unlock()
}

// Model 是内置目录里显示在添加模型选择器上的一行。
//
// TransportID 是目录推荐的执行配置；用户入口由部署独立声明。
type Model struct {
	ID          string
	Provider    string
	Official    string
	Mode        string
	TransportID string
	Source      string
	Input       float64
	Output      float64
	Priced      bool
	PriceModel  string
	PriceSource string
}

// RegisterModel 登记一条可选模型，并记录它的默认执行传输。
// 参数 m（Model）：正在累加或展示的模型。
// 返回：无。id 为空时不登记。
// 调用：provider/volcengine/seedance.go
// 测试：无直接单测
func RegisterModel(m Model) {
	m.ID = strings.TrimSpace(m.ID)
	if m.ID == "" {
		return
	}
	if m.TransportID != "" {
		mu.Lock()
		modelEndpoints[m.ID] = m.TransportID
		mu.Unlock()
	}
	mode := m.Mode
	if mode == "" {
		mode = m.TransportID
	}
	catalog.Contribute(catalog.Row{
		ID:          m.ID,
		Provider:    m.Provider,
		Mode:        mode,
		TransportID: m.TransportID,
		Source:      m.Source,
		Input:       m.Input,
		Output:      m.Output,
		Priced:      m.Priced,
		Official:    m.Official,
		PriceModel:  m.PriceModel, PriceSource: m.PriceSource,
	})
}

// ProviderField 是下拉里还没有的供应商要用的一项凭据输入。
type ProviderField struct {
	Key      string
	Label    string
	Type     string
	Required bool
	Default  string
}

// Supplier 是一条下拉项。已有的名字保持原样。
type Supplier struct {
	Name        string
	Slug        string
	Display     string
	Placeholder string
	Fields      []ProviderField
}

// RegisterSupplier 登记展示名称和凭据字段；实际地址必须由连接显式配置。
// 参数 s（Supplier）：要登记的供应商。
// 返回：无。slug 为空时不登记。
// 调用：provider/volcengine/seedance.go
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
}

// Transports 返回已登记的内置转发方式。
// 参数：无。
// 返回 []Transport（[]Transport）：已登记的转发方式。
// 调用：PublicBody。
// 测试：seedance_test.go
func Transports() []Transport {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Transport, len(transports))
	copy(out, transports)
	return out
}

// ModelEndpoints 把内置模型 id 映射到表单要预选的默认 transport ID。
// 参数：无。
// 返回 map[string]string（map[string]string）：模型 id 到端点类型。
// 调用：PublicBody。
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

// PublicBody 是添加模型的载荷：公开能力、执行传输和模型默认值。
//
// endpoint_types 是公开协议能力；capabilities 是能力与路径说明；
// transports 是后端登记的执行传输；models 是模型到默认 transport 的映射。
// 参数：无。
// 返回 map[string]any（map[string]any）：能力、转发方式和模型默认端点。
// 调用：catalog/classify.go、gateway/bypass.go、gateway/catalog.go
// 测试：无直接单测
func PublicBody() map[string]any {
	return map[string]any{
		"endpoint_types": EndpointTypes(),
		"capabilities":   Capabilities(),
		"transports":     Transports(),
		"models":         ModelEndpoints(),
	}
}

// Match 找出这个方法和路径命中的 bypass 动作，只在已登记的转发方式里找。
//
// 它刻意不读部署上的自定义文档：bypass 是后台登记的形状，不是运维在界面上
// 随手填的一份路径表。一份填错的路径表发不出请求，也就拿不到上游的返回值，
// 预选、日志和用量都无从谈起。
//
// 参数 method（string）：HTTP 方法，例如 GET 或 POST；path（string）：URL 路径；
// models（[]config.ModelEntry）：候选部署列表，当前不参与匹配，保留给调用方复用签名。
// 返回 Hit（Hit）：命中的转发方式和动作；bool（bool）：命中时为真。
// 调用：gateway/bypass.go
// 测试：bypass_logic_test.go、match_test.go
func Match(method, path string, models []config.ModelEntry) (Hit, bool) {
	method = strings.ToUpper(method)
	path = strings.TrimSuffix(path, "/")
	for _, t := range Transports() {
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
			return Hit{Transport: t, Action: action, Names: names}, true
		}
	}
	return Hit{}, false
}

// 把路径模板和真实路径逐段比较，抽出花括号里的参数。
// 参数 pattern（string）：路径模板；path（string）：真实路径。
// 返回 map[string]string（map[string]string）：抽出的参数；bool（bool）：逐段对上时为真。
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

// 把动态值收成去掉空白的字符串。不是字符串时为空串。
// 参数 v（any）：JSON 里读出的动态值。
// 返回 string（string）：按字符串读出的值。
// 调用：仅在 registry.go 内使用
// 测试：无直接单测
func str(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// SelectedCapabilities 返回一条部署应答的能力 id 列表。
//
// 从 model_info.endpoint_types 读取能力 id；字段缺失时不声明任何能力。
//
// 认不出的 id（realtime、batch、ocr）被忽略，不放进任何能力。一项都认不出时
// 返回空切片，调用方据此拒绝这条部署——不在这里补默认 chat，那会让一条只写了
// realtime 的部署意外应答所有对话请求。
//
// 参数 m（config.ModelEntry）：一条部署。
// 返回 []string（[]string）：能力 id 列表，可能为空。
// 调用：AdaptedPool 过滤适配池。
// 测试：registry_test.go
func SelectedCapabilities(m config.ModelEntry) []string {
	if m.ModelInfo == nil {
		return nil
	}
	if list := stringList(m.ModelInfo["endpoint_types"]); len(list) > 0 {
		return normalizeCapabilities(list)
	}
	return nil
}

// normalizeCapabilities 把一批能力 id 去重并保持顺序，未知 id 丢弃。
// 参数 list（[]string）：要收的 id。
// 返回 []string（[]string）：能力 id 列表。
// 调用：SelectedCapabilities。
// 测试：registry_test.go
func normalizeCapabilities(list []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range list {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !KnownEndpoint(id) || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// SelectedTransport 返回一条部署的转发方式 id。
//
// model_info.transport 必须是登记过的内置 id，否则返回空串。
//
// 内置 Bypass 之外的 bypass 形状不存在：后台没登记过的转发方式，运维在界面上
// 也选不到、存不进。
//
// 参数 m（config.ModelEntry）：一条部署。
// 返回 string（string）：转发方式 id，未配置或无效时为空。
// 调用：AdaptedPool 过滤适配池；dataplane/official.go 选部署。
// 测试：registry_test.go
func SelectedTransport(m config.ModelEntry) string {
	if m.ModelInfo == nil {
		return ""
	}
	if id := str(m.ModelInfo["transport"]); id != "" {
		if _, ok := transportByID(id); ok {
			return id
		}
	}
	return ""
}

// transportByID 只接受当前登记的 bypass transport。
// 参数 id（string）：待查找的传输方式标识。
// 返回 string、bool：登记的 ID 与是否存在。
// 调用：SelectedTransport。
// 测试：registry_test.go。
func transportByID(id string) (string, bool) {
	for _, transport := range Transports() {
		if transport.ID == id {
			return id, true
		}
	}
	return "", false
}

// IncludesCapability 报告这条部署是否应答这个能力。
// 参数 m（config.ModelEntry）：一条部署；capability（string）：能力 id。
// 返回 bool（bool）：应答这个能力时为真。
// 调用：AdaptedPool。
// 测试：registry_test.go
func IncludesCapability(m config.ModelEntry, capability string) bool {
	for _, id := range SelectedCapabilities(m) {
		if id == capability {
			return true
		}
	}
	return false
}

// Includes 报告这条部署是否选中了这个转发方式 id。
// 参数 m（config.ModelEntry）：一条部署；typeID（string）：转发方式 id。
// 返回 bool（bool）：选中时为真。
// 调用：dataplane/official.go
// 测试：registry_test.go
func Includes(m config.ModelEntry, typeID string) bool {
	for _, t := range Transports() {
		if t.ID == typeID {
			return AllowsEndpoint(m, t.EndpointID) == nil
		}
	}
	return false
}

// BoundTransports 把这条部署声明的转发方式解析成登记好的条目。
// 参数 m（config.ModelEntry）：一条部署。
// 返回 []Transport（[]Transport）：这条部署应答的内置转发方式。
// 调用：测试。
// 测试：registry_test.go
func BoundTransports(m config.ModelEntry) []Transport {
	if ValidateDeployment(m) != nil {
		return nil
	}
	for _, t := range Transports() {
		if t.ID == SelectedTransport(m) {
			return []Transport{t}
		}
	}
	return nil
}

// 把配置里的字符串或数组收成 []string。
// 参数 v（any）：JSON 里读出的动态值。
// 返回 []string（[]string）：字符串列表。
// 调用：SelectedCapabilities、SelectedTransport。
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

// init 记一次目录载入。目录是进程启动时由各供应商包的 init 填好的，
// 这一行是"目录确实装上了"的凭证。
// 参数：无。
// 返回：无。只写一行进程日志。
// 调用：Go 在载入这个包时自动执行。
// 测试：无直接单测
func init() { logx.Debug("provider registry loaded") }
