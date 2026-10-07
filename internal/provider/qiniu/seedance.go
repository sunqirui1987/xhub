// Package qiniu registers Qiniu Modelink's Seedance API as a bypass endpoint type.
package qiniu

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/provider"
)

// 进程启动时登记这个目录的供应商、端点类型和模型价格。添加模型时就能选到它们。
// 参数：无。
// 返回：无。七牛供应商和内容生成端点已登记，模型价格也进了目录。
// 调用：Go 在载入这个包时自动执行。
// 测试：无直接单测
func init() {
	logx.Debug("provider qiniu loading its registered transport and models")
	provider.RegisterSupplier(provider.Supplier{
		Name: "Qiniu", Slug: "qiniu", Display: "Qiniu",
		APIBase: "https://api.qnaigc.com", Placeholder: "qiniu/bytedance/doubao-seedance-2-0-260128",
		Fields: []provider.ProviderField{
			{Key: "api_base", Label: "API Base", Type: "text", Default: "https://api.qnaigc.com"},
			{Key: "api_key", Label: "API Key", Type: "password", Required: true},
		},
	})
	provider.RegisterTransport(provider.Transport{
		ID: "qiniu_contents_generation", Kind: provider.KindBypass,
		Label:     "Bypass - 七牛内容生成 /v3/contents/generations/tasks",
		Providers: []string{"qiniu"}, APIBase: "https://api.qnaigc.com",
		ModelField: "model", TaskID: "id", StripPrefix: "qiniu",
		Actions: []provider.Action{
			{Name: "create", Method: "POST", PublicPath: "/v3/contents/generations/tasks", UpstreamPath: "/v3/contents/generations/tasks"},
			{Name: "get", Method: "GET", PublicPath: "/v3/contents/generations/tasks/{id}", UpstreamPath: "/v3/contents/generations/tasks/{id}"},
		},
	})
	for _, id := range []string{
		"bytedance/doubao-seedance-2-0-260128",
		"bytedance/doubao-seedance-2-0-fast-260128",
		"bytedance/doubao-seedance-2-0-mini-260128",
	} {
		provider.RegisterModel(provider.Model{
			ID: "qiniu/" + id, Provider: "qiniu", Official: id,
			EndpointType: "qiniu_contents_generation",
			Source:       "https://docs.modelink.ai/api/video-doubao-seedance-20",
		})
	}
}
