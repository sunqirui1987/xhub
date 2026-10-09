// Package provider 管理供应商、公开端点类型、执行传输和模型登记。
// 供应商保存连接与鉴权；端点类型定义客户端协议；传输定义具体上游动作。
// 部署显式声明 transport 与 endpoint_types，价格目录只负责计价，不推断调用能力。
// adapted 编译公开协议到供应商协议；bypass 保留供应商协议载荷与响应。
package provider

import (
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// Action 是一个端点类型里的一次 HTTP 调用，例如 create 或 get。
type Action struct {
	Name         string `json:"name"`
	Method       string `json:"method"`
	PublicPath   string `json:"public_path"`
	UpstreamPath string `json:"upstream_path"`
	// TaskQuery 是携带任务 id 的查询参数。
	// 空表示 id 是路径里的 {name} 占位符。
	TaskQuery string `json:"task_query,omitempty"`
	// Model 是固定创建路径对应的上游模型；可选请求体 model 仅用于选择网关别名。
	Model string `json:"model,omitempty"`
}

// Capability 是一种入口能力。它决定哪些路径能调用一条模型。
//
// Ops 是这条能力覆盖的数据面操作名，取值来自 gateway/family 的 inferenceOp。
// 认入口仍然用那个函数，不从 Paths 生成：子串顺序、gemini、count_tokens
// 和 /threads 都在它那份 switch 里，复制一份出来只会两边走散。
//
// Paths 只给表单和 Playground 展示用，不参与路由判断。
type Capability struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Ops   []string `json:"ops"`
	Paths []string `json:"paths,omitempty"`
}

// EndpointDescriptor 定义公开端点类型；不包含供应商地址或密钥。
// Family 用于界面分类，Protocol 标识载荷协议，Capability 关联适配层能力。
type EndpointDescriptor struct {
	Category   string `json:"category"` // openai、vertex、claude 或 bypass；分类不决定执行方式。
	ID         string `json:"id"`
	Label      string `json:"label"`
	Kind       Kind   `json:"kind"`
	Protocol   string `json:"protocol"`
	Family     string `json:"family"`
	Capability string `json:"capability,omitempty"`
	// Paths 保存用户入口路径；对话绑定读取目录，与上游执行配置独立。
	Paths []string `json:"paths,omitempty"`
}

// AuthConfig 定义供应商鉴权；先清除客户端凭据，再写入部署凭据。
// Header 指定头名称；Prefix 如 Bearer 或 Key，空值表示直接写入密钥。
type AuthConfig struct {
	Header string `json:"header"`
	Prefix string `json:"prefix,omitempty"`
}

// Transport 定义原生协议如何在供应商上执行，与公开端点类型分开登记。
// SupplierPrefixes 按供应商追加协议路径；ResponseUsage 只提取响应事实，不计算金额。
// EndpointID 关联公开协议；Protocol 决定事实提取；Family 用于展示；ModelGroup 区分队列。
// Auth、Headers 定义鉴权和协议默认头；ID 是部署引用的稳定执行标识。
// 地址和凭据只来自连接配置；ModelField 指定可替换的顶层模型字段。
// TaskID 指定响应中的任务编号；StripPrefix 只移除模型的一层供应商前缀。
// QueueURLs 要求轮询 URL 改写为网关入口；Billing 定义成功结果的异步用量提取。
// Actions 是方法和路径白名单，禁止任意地址代理；创建与查询必须固定到同一传输。
type Transport struct {
	// CatalogID 绑定编译时供应商目录，与凭据名称及认证协议独立。
	CatalogID        string                                              `json:"catalog_id,omitempty"`
	SupplierPrefixes map[string]string                                   `json:"supplier_prefixes,omitempty"`
	EndpointID       string                                              `json:"endpoint_id"`
	Protocol         string                                              `json:"protocol"`
	Family           string                                              `json:"family"`
	ModelGroup       string                                              `json:"model_group,omitempty"`
	Auth             AuthConfig                                          `json:"auth"`
	Headers          map[string]string                                   `json:"headers,omitempty"`
	ResponseUsage    func(map[string]any, map[string]any) map[string]any `json:"-"`
	ID               string                                              `json:"id"`
	Label            string                                              `json:"label"`
	// Kind 只有 adapted 和 bypass。所有 Bypass 传输必须在后台注册，部署不能上传任意路径文档。
	Kind Kind `json:"kind"`
	// Providers 限制这个转发方式对哪些供应商显示。空表示任何供应商都能选。
	Providers   []string     `json:"providers,omitempty"`
	ModelField  string       `json:"model_field,omitempty"`
	TaskID      string       `json:"task_id,omitempty"`
	StripPrefix string       `json:"strip_prefix,omitempty"`
	QueueURLs   bool         `json:"queue_urls,omitempty"`
	Billing     *TaskBilling `json:"-"`
	Actions     []Action     `json:"actions,omitempty"`
}

// TaskContext 保存异步结算需要的创建事实，不保存提示词、素材 URL 或密钥。
// StartedAt 用于选择时段价格；Model 固定上游模型；Resolution、HasVideo、Variant 保存价格维度。
// Known=false 表示上下文不完整；PricingBlocked 保留无法自动计价的明确原因。
// 请求时长不代表实际生成量，实际用量必须从成功结果提取。
type TaskContext struct {
	StartedAt      time.Time `json:"started_at"`
	Model          string    `json:"model"`
	Resolution     string    `json:"resolution"`
	HasVideo       bool      `json:"has_video"`
	Known          bool      `json:"known"`
	Variant        string    `json:"variant,omitempty"`
	PricingBlocked string    `json:"pricing_blocked,omitempty"`
}

// TaskBilling 定义供应商成功终态和实测用量，不在传输层计算金额。
// Context 接收创建请求，保存最小计价上下文；Usage 接收轮询结果与固定上下文。
// 未完成或失败返回 nil；缺失价格维度时保留 pricing_blocked，交给目录处理。
type TaskBilling struct {
	Context func(map[string]any) TaskContext
	Usage   func(map[string]any, TaskContext) map[string]any
}

// Kind 说一条请求按哪种方式处理。
type Kind string

const (
	// KindAdapted 走现有的 chat 和 family 循环。
	KindAdapted Kind = "adapted"
	// KindBypass 把供应商自己的官方请求原样转发。
	KindBypass Kind = "bypass"
)

// Hit 是一条请求路径命中的转发方式和动作。
type Hit struct {
	Transport Transport
	Action    Action
	Names     map[string]string
}

// OfficialID 从存储的模型 id 上剥掉一层 "<前缀>/"。后面的斜杠保留，
// 所以 supplier/org/model 会变成 bytedance/doubao-...
// 参数 prefix（string）：要剥掉的前缀。空串表示不处理；stored（string）：存储的模型 id。
// 返回 string（string）：去掉一层前缀后的模型 id。
// 调用：dataplane/official.go
// 测试：match_test.go、seedance_test.go
func OfficialID(prefix, stored string) string {
	stored = strings.TrimSpace(stored)
	prefix = strings.TrimSpace(prefix)
	if stored == "" || prefix == "" {
		return stored
	}
	head := prefix + "/"
	if strings.HasPrefix(stored, head) {
		return stored[len(head):]
	}
	return stored
}

// ReadTaskID 从 JSON 对象里读一个点分字段，例如 "data.task_id"。
// 参数 doc（map[string]any）：已经解析的 JSON 对象；path（string）：点分字段路径。
// 返回 string（string）：取出的任务 id。路径不存在时为空串。
// 调用：dataplane/official.go
// 测试：match_test.go、registry_test.go
func ReadTaskID(doc map[string]any, path string) string {
	if doc == nil || path == "" {
		return ""
	}
	var cur any = doc
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[part]
	}
	s, _ := cur.(string)
	return strings.TrimSpace(s)
}

// Expand 把上游路径里的 {name} 占位符填上。
// 参数 pattern（string）：带占位符的路径模板；names（map[string]string）：参数表。
// 返回 string（string）：填好的上游路径。
// 调用：dataplane/official.go
// 测试：无直接单测
func Expand(pattern string, names map[string]string) string {
	out := pattern
	for k, v := range names {
		out = strings.ReplaceAll(out, "{"+k+"}", v)
	}
	return out
}

// init 记一次类型定义文件的载入。这个文件只有类型和几个纯函数，
// 载入本身没有别的可记的事。
// 参数：无。
// 返回：无。只写一行进程日志。
// 调用：Go 在载入这个包时自动执行。
// 测试：无直接单测
func init() { logx.Trace("enter provider types") }
