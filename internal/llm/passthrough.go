// Package llm joins passthrough paths onto an upstream base. It does not rewrite a body that is already in the provider protocol.
package llm

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"net/url"
	"path"
	"strings"
	"sync"
)

var logTraceOncePassthrough sync.Once

// PassthroughURL is the outbound address produced by LiteLLM _join_url_paths. It first joins the subpath onto the api_base path and rejects "..". If an OpenAI result still has no /v1/, v1 is inserted after api.openai.com/.
// 参数 base（string）：根地址或完整 URL。空串表示改用供应商默认根，末尾斜杠会去掉；endpoint（string）：拼在根地址后面的路径，避免出现双斜杠；provider（string）：供应商标识，例如 openai 或 volcengine。
// 返回 string（string）：按 LiteLLM 规则拼出的出站地址。先把子路径接到 api_base 的路径上。base 解析失败时退回原 base。
// 调用：gateway/ingress.go
// 测试：无直接单测
func PassthroughURL(base, endpoint, provider string) string {
	logTraceOncePassthrough.Do(func() { logx.Trace("enter llm.PassthroughURL") })

	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	if endpoint != "" && !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	u.Path = joinBaseAndEndpoint(u.Path, endpoint)
	u.RawPath = ""
	out := u.String()
	if (provider == "openai" || provider == "openai_passthrough") && !strings.Contains(out, "/v1/") {
		out = strings.Replace(out, "api.openai.com/", "api.openai.com/v1/", 1)
	}
	return out
}

// PassthroughSubpath matches HttpPassThroughEndpointHelpers.construct_target_url_with_subpath. When include is false or the subpath is empty, base is returned unchanged. Otherwise the subpath is normalized and joined.
// 参数 base（string）：根地址或完整 URL。空串表示改用供应商默认根，末尾斜杠会去掉；subpath（string）：拼在根地址后面的路径，避免出现双斜杠；include（bool）：为真时响应里带上明文密钥。明文只在创建或轮换时出现。
// 返回 string（string）：include 为假或子路径为空时原样返回 base，否则把子路径接到 base 后面，不留下双斜杠。
// 调用：仅在 passthrough.go 内使用
// 测试：无直接单测
func PassthroughSubpath(base, subpath string, include bool) string {
	if !include || subpath == "" {
		return base
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	subpath = strings.TrimPrefix(subpath, "/")
	trailing := strings.HasSuffix(subpath, "/")
	safe := strings.TrimPrefix(path.Clean("/"+subpath), "/")
	if safe == "." {
		safe = ""
	}
	if trailing && safe != "" && !strings.HasSuffix(safe, "/") {
		safe += "/"
	}
	return base + safe
}

// joinBaseAndEndpoint joins an upstream base and an endpoint path without a double slash and without dropping a prefix already on the base.
// 参数 basePath（string）：拼接根地址和端点使用的根地址路径。空串表示调用方没有提供这项；endpointPath（string）：拼接根地址和端点使用的端点路径。空串表示调用方没有提供这项。
// 返回 string（string）：接好的路径。不会出现双斜杠，也不会丢掉 base 上已有的前缀。endpoint 原来以斜杠结尾时保留。
// 调用：仅在 passthrough.go 内使用
// 测试：无直接单测
func joinBaseAndEndpoint(basePath, endpointPath string) string {
	trailing := strings.HasSuffix(endpointPath, "/")
	if basePath == "" || basePath == "/" {
		normalized := path.Clean("/" + strings.TrimPrefix(endpointPath, "/"))
		if trailing && normalized != "/" {
			normalized += "/"
		}
		return normalized
	}
	basePath = strings.TrimRight(basePath, "/")
	combined := path.Clean(basePath + "/" + strings.TrimPrefix(endpointPath, "/"))
	if combined != basePath && !strings.HasPrefix(combined, basePath+"/") {
		return basePath + "/"
	}
	if trailing && !strings.HasSuffix(combined, "/") {
		combined += "/"
	}
	return combined
}
