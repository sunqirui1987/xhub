// Package volcengine registers Ark's Seedance API as a bypass endpoint type.
package volcengine

import "github.com/sunqirui1987/xhub/internal/provider"

// 进程启动时登记这个目录的供应商、端点类型和模型价格。添加模型时就能选到它们。
// 参数：无。
// 返回：无。火山方舟供应商和内容生成端点已登记。
// 调用：Go 在载入这个包时自动执行。
// 测试：无直接单测
func init() {
	provider.RegisterSupplier(provider.Supplier{
		Name: "VolcEngine", Slug: "volcengine", Display: "VolcEngine",
		APIBase: "https://ark.cn-beijing.volces.com",
	})
	provider.RegisterType(provider.Type{
		ID: "ark_contents_generation", Kind: provider.KindBypass,
		Label:     "Bypass - 方舟内容生成 /api/v3/contents/generations/tasks",
		Providers: []string{"volcengine"}, APIBase: "https://ark.cn-beijing.volces.com",
		ModelField: "model", TaskID: "id", StripPrefix: "volcengine",
		Actions: []provider.Action{
			{Name: "create", Method: "POST", PublicPath: "/api/v3/contents/generations/tasks", UpstreamPath: "/api/v3/contents/generations/tasks"},
			{Name: "get", Method: "GET", PublicPath: "/api/v3/contents/generations/tasks/{id}", UpstreamPath: "/api/v3/contents/generations/tasks/{id}"},
			{Name: "list", Method: "GET", PublicPath: "/api/v3/contents/generations/tasks", UpstreamPath: "/api/v3/contents/generations/tasks"},
		},
	})
	// Ark lists this band at CNY 46 per million tokens. The price map is USD,
	// so the row stores the BytePlus list of $7 per million for 480p and 720p
	// without video input. Other bands are set on the deployment.
	const perToken = 7.0 / 1_000_000
	provider.RegisterModel(provider.Model{
		ID: "volcengine/doubao-seedance-2-0-260128", Provider: "volcengine",
		Official: "doubao-seedance-2-0-260128", EndpointType: "ark_contents_generation",
		Source: "https://www.volcengine.com/docs/82379/1544106",
		Input:  perToken, Output: perToken, Priced: true,
	})
	provider.RegisterModel(provider.Model{
		ID: "volcengine/doubao-seedance-2-0-fast-260128", Provider: "volcengine",
		Official: "doubao-seedance-2-0-fast-260128", EndpointType: "ark_contents_generation",
		Source: "https://www.volcengine.com/docs/82379/1520757",
	})
}
