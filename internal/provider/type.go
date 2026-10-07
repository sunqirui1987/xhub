// Package provider 是端点目录。供应商目录登记能力、转发方式和模型，
// 人在添加模型时从这些里选。网关按能力决定一条请求能不能打到一条部署，
// 按转发方式决定怎么把它送到上游。它不为每一家供应商长一个 switch。
//
// 目录里有两件互相独立的事，它们曾经被压成同一个下拉：
//
//   - 能力（Capability）：这条模型应答哪些入口。文本进文本出是 chat，
//     向量是 embedding，生图是 image。一种能力对应一组入口路径，
//     同一组里的路径是同一件事的不同拼法。
//   - 转发方式（Transport）：网关怎么把请求送到上游。协议适配由网关按
//     (op, 供应商) 自己编译；内置 Bypass 按目录里登记的路径原样转发；
//     自定义 Bypass 按操作员照文档填的路径转发。
package provider

import "github.com/sunqirui1987/xhub/internal/logx"

import "strings"

// Action 是一个端点类型里的一次 HTTP 调用，例如 create 或 get。
type Action struct {
	Name         string `json:"name"`
	Method       string `json:"method"`
	PublicPath   string `json:"public_path"`
	UpstreamPath string `json:"upstream_path"`
	// TaskQuery 是携带任务 id 的查询参数。
	// 空表示 id 是路径里的 {name} 占位符。
	TaskQuery string `json:"task_query,omitempty"`
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

// Transport 是网关把请求送到上游的方式。
//
//   - adapted：网关按 (op, 供应商) 编译请求，走 llm.Endpoint / llm.Build。
//     这一种不进目录：它的行为由供应商名决定，不是一条可以登记的数据。
//   - bypass：内置的原样转发。字段是 APIBase、ModelField、TaskID、
//     StripPrefix 和 Actions。
type Transport struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Kind 只有 adapted 和 bypass。custom 不是一种 Kind：它是部署上自己带的
	// 一份 bypass 文档，由 overrideTransport 读出来，ID 是 "custom"。
	Kind Kind `json:"kind"`
	// Providers 限制这个转发方式对哪些供应商显示。空表示任何供应商都能选。
	Providers   []string `json:"providers,omitempty"`
	APIBase     string   `json:"api_base,omitempty"`
	ModelField  string   `json:"model_field,omitempty"`
	TaskID      string   `json:"task_id,omitempty"`
	StripPrefix string   `json:"strip_prefix,omitempty"`
	Actions     []Action `json:"actions,omitempty"`
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
// DeploymentName 在路径属于某条部署自己的自定义端点时才有值。
type Hit struct {
	Transport      Transport
	Action         Action
	Names          map[string]string
	DeploymentName string
}

// OfficialID 从存储的模型 id 上剥掉一层 "<前缀>/"。后面的斜杠保留，
// 所以 qiniu/bytedance/doubao-... 会变成 bytedance/doubao-...
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
func init() { logx.Trace("enter provider types") }
