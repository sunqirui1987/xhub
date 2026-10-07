package provider

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"strings"
	"sync"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/config"
)

var (
	mu             sync.Mutex
	transports     []Transport
	modelEndpoints = map[string]string{}
)

// RegisterTransport 登记一个内置转发方式。供应商文件在 init 里调它。
// 参数 t（Transport）：要登记的内置转发方式。
// 返回：无。id 为空或没有动作时不登记——一个没有动作的转发方式无处可转。
// 调用：provider/qiniu/seedance.go、provider/volcengine/seedance.go
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

// Model 是价目表里显示在添加模型选择器上的一行。
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

// RegisterModel 登记一条可选模型。选中这个模型 id 时，表单会带上它的端点类型。
// 参数 m（Model）：正在累加或展示的模型。
// 返回：无。id 为空时不登记。
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
	APIBase     string
	Placeholder string
	Fields      []ProviderField
}

// RegisterSupplier 记下默认 API 根；名字是新的时，连添加模型的凭据字段一起记下。
// 参数 s（Supplier）：要登记的供应商。
// 返回：无。slug 为空时不登记。
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

// APIBase 返回部署上的根地址；部署没填时用供应商登记的默认根。
// 参数 slug（string）：供应商 slug，例如 openai、volcengine、qiniu；configured（string）：部署上写的 api_base。
// 返回 string（string）：要用的根地址。两处都没有时为空串。
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

// ModelEndpoints 把价目表模型 id 映射到表单要预选的端点类型。
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

// PublicBody 是添加模型的载荷：能力表、转发方式表和模型默认值。
//
// 形状从 {types, models} 改成了 {capabilities, transports, models}，
// 因为原来的 types 正是这次要拆掉的那件东西。调用方只有添加模型表单。
// 参数：无。
// 返回 map[string]any（map[string]any）：能力、转发方式和模型默认端点。
// 调用：catalog/classify.go、gateway/bypass.go、gateway/catalog.go
// 测试：无直接单测
func PublicBody() map[string]any {
	return map[string]any{
		"capabilities": Capabilities(),
		"transports":   Transports(),
		"models":       ModelEndpoints(),
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

// ApplyOverride 当前是恒等函数。部署上不再支持自带 bypass 文档，
// 保留这个签名让 dataplane 的调用点不必改。
// 参数 m（config.ModelEntry）：一条部署，当前不读；hit（Hit）：这次命中的转发方式。
// 返回 Hit（Hit）：原样返回。
// 调用：dataplane/official.go
// 测试：无直接单测
func ApplyOverride(m config.ModelEntry, hit Hit) Hit { return hit }

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
// 读顺序：
//  1. model_info.endpoint_types：新写入是这个字段，里面是能力 id。
//  2. model_info.mode：旧行只有这个字符串，按存量 id 映射成能力。
//  3. 都没有：chat。老的部署和不带端点信息的部署都是这个意思。
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
		return []string{"chat"}
	}
	if list := stringList(m.ModelInfo["endpoint_types"]); len(list) > 0 {
		return normalizeCapabilities(list)
	}
	if mode := str(m.ModelInfo["mode"]); mode != "" {
		return normalizeCapabilities([]string{mode})
	}
	return []string{"chat"}
}

// normalizeCapabilities 把一批 id 收成能力 id，去重并保持顺序。
// 每个 id 先按能力 id 认，认不出再按存量 id 认，都不认就丢掉。
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
		capability := id
		if !isKnownCapability(capability) {
			mapped, ok := capabilityByLegacyID(id)
			if !ok {
				continue
			}
			capability = mapped
		}
		if seen[capability] {
			continue
		}
		seen[capability] = true
		out = append(out, capability)
	}
	return out
}

// SelectedTransport 返回一条部署的转发方式 id。
//
// 判定顺序：
//  1. model_info.transport 是登记过的内置 id → 那个 id。
//  2. endpoint_types 或 mode 里出现内置 Bypass id → 那个 id。旧行只写了这个。
//  3. 其余 → adapted。
//
// 内置 Bypass 之外的 bypass 形状不存在：后台没登记过的转发方式，运维在界面上
// 也选不到、存不进。
//
// 参数 m（config.ModelEntry）：一条部署。
// 返回 string（string）：转发方式 id，always 有值。
// 调用：AdaptedPool 过滤适配池；dataplane/official.go 选部署。
// 测试：registry_test.go
func SelectedTransport(m config.ModelEntry) string {
	if m.ModelInfo == nil {
		return AdaptedTransportID
	}
	if id := str(m.ModelInfo["transport"]); id != "" {
		if _, ok := transportByLegacyID(id); ok {
			return id
		}
		if id == AdaptedTransportID {
			return AdaptedTransportID
		}
	}
	for _, id := range append(appendedIDs(m), str(m.ModelInfo["mode"])) {
		if transport, ok := transportByLegacyID(id); ok {
			return transport
		}
	}
	return AdaptedTransportID
}

// appendedIDs 返回 model_info.endpoint_types 里的值。
// 参数 m（config.ModelEntry）：一条部署。
// 返回 []string（[]string）：endpoint_types 的值，没有时为空切片。
// 调用：SelectedTransport、Includes。
// 测试：无直接单测
func appendedIDs(m config.ModelEntry) []string {
	if m.ModelInfo == nil {
		return nil
	}
	return stringList(m.ModelInfo["endpoint_types"])
}

// IsAdapted 报告这条部署走协议适配。Bypass 部署不能从能力门进适配路径：
// 方舟内容生成的入口是 /api/v3/contents/generations/tasks，不是 /v1/videos，
// 把它放进适配池会让 /v1/videos 选中它然后打错地址。
// 参数 m（config.ModelEntry）：一条部署。
// 返回 bool（bool）：走协议适配时为真。
// 调用：AdaptedPool。
// 测试：registry_test.go
func IsAdapted(m config.ModelEntry) bool {
	return SelectedTransport(m) == AdaptedTransportID
}

// AdaptedPool 把适配路径的候选收敛到能应答这个 op 的部署。
//
// 两件事都做：丢掉不是协议适配的，丢掉能力不含这个 op 的。这是新行为，
// 不是把现有比较换个写法——原来适配路径完全不过滤端点类型，一条标成
// embedding 的部署现在仍能被 /v1/chat/completions 打到。
//
// 参数 models（[]config.ModelEntry）：router.Order 之后的候选；op（string）：数据面操作名。
// 返回 []config.ModelEntry（[]config.ModelEntry）：留下来的部署，可能为空。
// 调用：dataplane/serve.go
// 测试：capability_test.go
func AdaptedPool(models []config.ModelEntry, op string) []config.ModelEntry {
	capability, known := CapabilityForOp(op)
	out := make([]config.ModelEntry, 0, len(models))
	for _, m := range models {
		if !IsAdapted(m) {
			continue
		}
		if !known {
			// 认不出的 op 交给原来那条路去处理，不在这里下结论。
			out = append(out, m)
			continue
		}
		if IncludesCapability(m, capability) {
			out = append(out, m)
		}
	}
	return out
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

// Includes 报告这条部署是否选中了这个转发方式 id。Bypass 选部署用它：
// 路径先命中转发方式，再按转发方式 id 挑部署，能力不参与。
// 参数 m（config.ModelEntry）：一条部署；typeID（string）：转发方式 id。
// 返回 bool（bool）：选中时为真。
// 调用：dataplane/official.go
// 测试：registry_test.go
func Includes(m config.ModelEntry, typeID string) bool {
	if SelectedTransport(m) == typeID {
		return true
	}
	for _, id := range appendedIDs(m) {
		if id == typeID {
			return true
		}
	}
	return false
}

// SelectedTypes 返回一条部署声明的原始端点 id 列表。它只服务 Bypass 选部署：
// 能力那一路走 SelectedCapabilities。endpoint_types 优先，其次 mode，都没有则 chat。
// 参数 m（config.ModelEntry）：一条部署。
// 返回 []string（[]string）：原始 id 列表。
// 调用：BoundTransports。
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

// BoundTransports 把这条部署声明的转发方式解析成登记好的条目。
// 参数 m（config.ModelEntry）：一条部署。
// 返回 []Transport（[]Transport）：这条部署应答的内置转发方式。
// 调用：测试。
// 测试：registry_test.go
func BoundTransports(m config.ModelEntry) []Transport {
	var out []Transport
	for _, id := range SelectedTypes(m) {
		for _, t := range Transports() {
			if t.ID == id {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

// 把配置里的字符串或数组收成 []string。
// 参数 v（any）：JSON 里读出的动态值。
// 返回 []string（[]string）：字符串列表。
// 调用：SelectedCapabilities、SelectedTransport、SelectedTypes。
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

// init 记一次目录载入，并把已登记的转发方式数写下来。目录是进程启动时
// 由各供应商包的 init 填好的，这一行是"目录确实装上了"的唯一凭证。
func init() { logx.Debug("provider registry loaded") }
