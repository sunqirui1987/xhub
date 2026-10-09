package dataplane

import (
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/provider"
	"net/url"
	"strings"
)

// bypassURL 将供应商根地址与协议路径组合，避免根路径重复。
// 参数 base、path（string）：供应商地址和已登记上游路径。返回 string：完整请求地址。
// 调用：bypassDeploymentURL。测试：native_bypass_test.go。
func bypassURL(base, path string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base + path
	}
	root := strings.TrimRight(u.Path, "/")
	if root != "" && (path == root || strings.HasPrefix(path, root+"/")) {
		u.Path = path
	} else if strings.HasSuffix(root, "/v1") && strings.HasPrefix(path, "/v1/") {
		u.Path = root + strings.TrimPrefix(path, "/v1")
	} else {
		u.Path = root + path
	}
	u.RawPath = ""
	return u.String()
}

// bypassDeploymentURL 按供应商协议前缀生成当前操作的上游 URL。
// 参数 base：凭据根地址；hit：匹配的已登记操作与路径参数；dep：附加凭据后的部署。
// 返回 string：只由已登记路径及编码后的任务参数构成的地址。
// 调用：原生创建、查询。测试：TestQiniuNativeNamespace。
func bypassDeploymentURL(base string, hit provider.Hit, dep config.ModelEntry) string {
	prefix := hit.Transport.SupplierPrefixes[bypassSupplier(base, dep)]
	path := provider.Expand(hit.Action.UpstreamPath, hit.Names)
	// 兼容适配接口凭据的 /v1 根地址；原生协议前缀必须插在版本路径之前。
	if prefix != "" {
		if u, err := url.Parse(base); err == nil && strings.HasPrefix(path, "/v1/") {
			u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1")
			u.RawPath = ""
			base = u.String()
		}
	}
	return bypassURL(base, prefix+path)
}

// bypassSupplier 识别凭据的执行供应商；七牛的 Qnaigc、Modelink 地址共用原生协议前缀，凭据也可能按兼容协议标记为 openai。
// 参数 base：实际上游地址；dep：附加凭据后的部署。返回：供应商标识。
// 调用：原生地址和模型名生成。测试：TestNativeSupplierURLs。
func bypassSupplier(base string, dep config.ModelEntry) string {
	if u, err := url.Parse(base); err == nil {
		// 只匹配供应商的完整主机名，避免把名称相似的其他中转站误判为七牛。
		switch strings.ToLower(u.Hostname()) {
		case "api.qnaigc.com", "api.modelink.ai":
			return "qiniu"
		}
	}
	return dep.ParamString("custom_llm_provider", "")
}

// bypassUpstreamModel 去掉网关路由前缀，保留中转供应商要求的原厂模型命名空间。
// 参数 transport：传输注册；model：部署中的上游模型名；supplier：凭据所属供应商。
// 返回：写入原生请求的模型 ID。七牛原生接口要求 openai/ 或 anthropic/ 前缀，不能把它删掉。
// 调用：serveBypassCreate。测试：TestQiniuNativeNamespace。
func bypassUpstreamModel(transport provider.Transport, model, supplier string) string {
	if transport.SupplierPrefixes[supplier] != "" {
		return provider.OfficialID(supplier, model)
	}
	return provider.OfficialID(transport.StripPrefix, model)
}
