// Package provider is the catalog of endpoint types. A provider directory
// registers the types, models, and prices a person can pick when adding a model.
// The gateway matches a request to one of those types. It does not grow a switch
// for each new supplier.
package provider

import "strings"

// Kind says how a request for this type is handled.
// adapted uses the existing chat and family loops.
// bypass copies the provider's own official request through.
type Kind string

const (
	KindAdapted Kind = "adapted"
	KindBypass  Kind = "bypass"
)

// Action is one HTTP call in an endpoint type, such as create or get.
type Action struct {
	Name         string `json:"name"`
	Method       string `json:"method"`
	PublicPath   string `json:"public_path"`
	UpstreamPath string `json:"upstream_path"`
	// TaskQuery is the query parameter that carries the task id.
	// Empty means the id is a {name} placeholder in the path.
	TaskQuery string `json:"task_query,omitempty"`
}

// Type is one choice in the add-model endpoint list.
type Type struct {
	ID          string   `json:"id"`
	Kind        Kind     `json:"kind"`
	Label       string   `json:"label"`
	Providers   []string `json:"providers,omitempty"`
	Operation   string   `json:"operation,omitempty"`
	APIBase     string   `json:"api_base,omitempty"`
	ModelField  string   `json:"model_field,omitempty"`
	TaskID      string   `json:"task_id,omitempty"`
	StripPrefix string   `json:"strip_prefix,omitempty"`
	Actions     []Action `json:"actions"`
}

// Hit is the type and action a request path selected.
// DeploymentName is set when the path belongs to one deployment's custom endpoint.
type Hit struct {
	Type           Type
	Action         Action
	Names          map[string]string
	DeploymentName string
}

// OfficialID strips one "<prefix>/" from a stored model id. A later slash stays, so qiniu/bytedance/doubao-... becomes bytedance/doubao-...
// 参数 prefix（string）：要匹配或要去掉的前缀。空串表示不处理前缀；stored（string）：官方标识使用的stored。空串表示调用方没有提供这项。
// 返回 string（string）：去掉一个供应商标前缀后的模型 id。后面的斜杠保留，例如 bytedance/doubao-...。
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

// ReadTaskID reads a dotted field from a JSON object, for example "data.task_id".
// 参数 doc（map[string]any）：已经解析的 JSON 对象。缺字段表示上游或调用方没有给这项；path（string）：点分字段路径，例如 data.task_id，用来从 JSON 里取值。
// 返回 string（string）：按点分路径取出的任务 id。路径不存在时为空串。
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

// Expand fills {name} placeholders in an upstream path.
// 参数 pattern（string）：要匹配的路径模板或正则；names（map[string]string）：字符串到字符串的表。缺键表示这项没有填，不要补成空 JSON。
// 返回 string（string）：把 {name} 替换成路径参数后的上游路径。
// 调用：dataplane/official.go
// 测试：无直接单测
func Expand(pattern string, names map[string]string) string {
	out := pattern
	for k, v := range names {
		out = strings.ReplaceAll(out, "{"+k+"}", v)
	}
	return out
}
