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
	// 保留注册参数的转义，防止模型或任务 ID 中的斜杠变为路径分隔符。
	encoded := u.Path
	if decoded, err := url.PathUnescape(encoded); err == nil {
		u.Path = decoded
		u.RawPath = encoded
	}
	return u.String()
}

// bypassDeploymentURL 按供应商协议前缀生成当前操作的上游 URL。
// 参数 base：凭据根地址；hit：匹配的已登记操作与路径参数；dep：附加凭据后的部署。
// 返回 string：只由已登记路径及编码后的任务参数构成的地址。
// 调用：原生创建、查询。测试：TestNativeSupplierURLs。
func bypassDeploymentURL(base string, hit provider.Hit, dep config.ModelEntry) string {
	names := make(map[string]string, len(hit.Names))
	for k, v := range hit.Names {
		names[k] = url.PathEscape(v)
	}
	path := provider.Expand(hit.Action.UpstreamPath, names)
	return bypassURL(base, path)
}

// bypassSupplier 读取凭据明确声明的供应商；不通过域名、凭据名或模型名推断协议。
// 参数 base：实际上游地址；dep：附加凭据后的部署。返回：供应商标识。
// 调用：原生地址和模型名生成。测试：TestNativeSupplierURLs。
func bypassSupplier(base string, dep config.ModelEntry) string {
	return dep.ParamString("custom_llm_provider", "")
}

// bypassUpstreamModel 去掉显式注册的协议前缀，保留上游模型的其余命名空间。
// 参数 transport：传输注册；model：部署中的上游模型名；supplier：凭据所属供应商。
// 返回：写入原生请求的模型 ID。保留供应商协议明确登记的模型命名空间。
// 调用：serveBypassCreate。测试：TestNativeSupplierURLs。
func bypassUpstreamModel(transport provider.Transport, model, supplier string) string {
	return provider.OfficialID(transport.StripPrefix, model)
}
