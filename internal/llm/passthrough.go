// 原样转发的路径如何拼到上游。不改写已经是厂商协议的正文。
package llm

import (
	"net/url"
	"path"
	"strings"
)

// PassthroughURL 是 LiteLLM _join_url_paths 的出站地址。
// 先用 join_base_and_endpoint_path 把子路径接在 api_base 的 path 上，并挡住 ".."。
// OpenAI 的结果里如果还没有 /v1/，会在 api.openai.com/ 后面插入 v1。
func PassthroughURL(base, endpoint, provider string) string {
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

// PassthroughSubpath 是 HttpPassThroughEndpointHelpers.construct_target_url_with_subpath。
// include 为 false，或子路径为空时，原样返回 base。否则规范化子路径再拼接。
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

// 拼接上游基址和端点路径。避免出现双斜杠或丢掉基址上的前缀。
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
