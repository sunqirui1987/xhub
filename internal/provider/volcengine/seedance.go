// Package volcengine registers Ark's Seedance API as a bypass endpoint type.
package volcengine

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/provider"
)

// 进程启动时登记这个目录的供应商、端点类型和模型价格。添加模型时就能选到它们。
// 参数：无。
// 返回：无。火山方舟供应商和内容生成端点已登记。
// 调用：Go 在载入这个包时自动执行。
// 测试：无直接单测
func init() {
	logx.Debug("provider volcengine loading its registered transport and models")
	provider.RegisterSupplier(provider.Supplier{
		Name: "VolcEngine", Slug: "volcengine", Display: "VolcEngine",
	})
	provider.RegisterTransport(provider.Transport{
		CatalogID:  "volcengine",
		EndpointID: "bypass:ark-video", Protocol: "ark", Family: "video", ModelGroup: "Seedance",
		Auth: provider.AuthConfig{Header: "Authorization", Prefix: "Bearer"},
		ID:   "ark_contents_generation", Kind: provider.KindBypass,
		Label:     "Bypass - 方舟内容生成 /api/v3/contents/generations/tasks",
		Providers: []string{"volcengine"}, ModelField: "model", TaskID: "id", StripPrefix: "volcengine",
		Billing: provider.SeedanceBilling(),
		Actions: []provider.Action{
			{Name: "create", Method: "POST", PublicPath: "/api/v3/contents/generations/tasks", UpstreamPath: "/api/v3/contents/generations/tasks"},
			{Name: "get", Method: "GET", PublicPath: "/api/v3/contents/generations/tasks/{id}", UpstreamPath: "/api/v3/contents/generations/tasks/{id}"},
			{Name: "list", Method: "GET", PublicPath: "/api/v3/contents/generations/tasks", UpstreamPath: "/api/v3/contents/generations/tasks"},
		},
	})
	// Configure actual Ark account prices on the deployment.
	provider.RegisterModel(provider.Model{
		ID: "volcengine/doubao-seedance-2-0-260128", Provider: "volcengine",
		Official: "doubao-seedance-2-0-260128", TransportID: "ark_contents_generation",
		Source: "https://www.volcengine.com/docs/82379/1544106",
	})
	provider.RegisterModel(provider.Model{
		ID: "volcengine/doubao-seedance-2-0-fast-260128", Provider: "volcengine",
		Official: "doubao-seedance-2-0-fast-260128", TransportID: "ark_contents_generation",
		Source: "https://www.volcengine.com/docs/82379/1520757",
	})
}
