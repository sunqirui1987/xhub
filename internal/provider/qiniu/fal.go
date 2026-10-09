package qiniu

import (
	"strings"

	"github.com/sunqirui1987/xhub/internal/provider"
)

// falEndpoint 保存一个明确的 Fal 创建路径、目录定价 ID、文档来源与计价事实。
// 每个队列模型组拥有自己的查询路径和任务作用域；白名单防止任意路径转发。
type falEndpoint struct {
	path, price, doc string
	billing          provider.FalModel
}

// registerFal 注册一个可由 Custom 凭据显式选择的七牛 Fal 模型组及其创建、状态、结果操作。
// 参数 id：传输 ID；label：展示名；queue：查询路径所属模型组；endpoints：允许的具体模型路径。
// 返回：无。模型目录与传输注册表同时更新，URL 模型 ID 与定价 ID 分别保存。
// 调用：本包 init。测试：fal_test.go、billing_test.go。
func registerFal(id, label, queue string, endpoints []falEndpoint) {
	specs := make(map[string]provider.FalModel, len(endpoints))
	actions := []provider.Action{
		{Name: "get", Method: "GET", PublicPath: "/queue/" + queue + "/requests/{request_id}", UpstreamPath: "/queue/" + queue + "/requests/{request_id}"},
		{Name: "status", Method: "GET", PublicPath: "/queue/" + queue + "/requests/{request_id}/status", UpstreamPath: "/queue/" + queue + "/requests/{request_id}/status"},
	}
	for _, e := range endpoints {
		specs[e.path] = e.billing
		actions = append(actions, provider.Action{Name: "create", Method: "POST", PublicPath: "/queue/" + e.path, UpstreamPath: "/queue/" + e.path, Model: e.path})
		provider.RegisterModel(provider.Model{
			ID: "qiniu/" + e.path, Provider: "qiniu", Official: e.path, Mode: "video", TransportID: id,
			Source:     "https://docs.modelink.ai/api/video-fal-" + e.doc,
			PriceModel: e.price, PriceSource: "https://api.modelink.ai/v1/market/models",
		})
	}
	provider.RegisterTransport(provider.Transport{
		EndpointID: "bypass:fal-video", Protocol: "fal", Family: "video", ModelGroup: label,
		ID: id, Label: "Bypass - 七牛 Fal " + label, Kind: provider.KindBypass, Providers: []string{"custom", "custom_openai"},
		StripPrefix: "qiniu", Auth: provider.AuthConfig{Header: "Authorization", Prefix: "Key"}, TaskID: "request_id", QueueURLs: true,
		Billing: provider.FalBilling(specs), Actions: actions,
	})
}

// init 将 Seedance、Kling、Vidu、Veo、MiniMax 的已登记模型加入七牛 Fal 注册表。
// 参数：无。返回：无。每次扩展必须声明真实路径和计价变体，不能把所有视频模型套用同一费率。
// 调用：Go 包初始化。测试：fal_test.go。
func init() {
	for _, brand := range []struct{ path, market, doc, label string }{
		{"bytedance", "bytedance/doubao", "doubao", "Doubao Seedance"},
		{"byteplus", "byteplus/dreamina", "dreamina", "Dreamina Seedance"},
	} {
		for _, version := range []struct{ path, market, doc string }{
			{"2.0", "2-0-260128", "20"}, {"2.5", "2-5-260628", "25"},
		} {
			queue := brand.path + "/seedance-" + version.path
			var endpoints []falEndpoint
			variants := []struct{ path, market, doc string }{{"", version.market, ""}}
			if version.path == "2.0" {
				variants = append(variants, struct{ path, market, doc string }{"/fast", "2-0-fast-260128", "-fast"}, struct{ path, market, doc string }{"/mini", "2-0-mini-260615", "-mini"})
			}
			for _, v := range variants {
				for _, op := range []string{"text-to-video", "image-to-video", "reference-to-video"} {
					endpoints = append(endpoints, falEndpoint{queue + v.path + "/" + op, brand.market + "-seedance-" + v.market, brand.doc + "-seedance-" + version.doc + v.doc, provider.FalModel{Seedance: true, Resolution: "720p"}})
				}
			}
			registerFal("qiniu_fal_"+brand.doc+"_"+version.doc, brand.label+" "+version.path, queue, endpoints)
		}
	}
	var kling []falEndpoint
	addKling := func(version, price, doc, op string, modes []string, audio bool, variant string) {
		for _, mode := range modes {
			band := mode
			if mode == "standard" {
				band = "std"
			}
			kling = append(kling, falEndpoint{"fal-ai/kling-video/" + version + "/" + mode + "/" + op, price, doc, provider.FalModel{Variant: strings.ReplaceAll(variant, "{mode}", band), AudioDefault: audio}})
		}
	}
	addKling("v2.5-turbo", "kling-v2-5-turbo", "kling-v25-turbo", "text-to-video", []string{"pro"}, false, "{mode}_norefv_v_duration")
	addKling("v2.5-turbo", "kling-v2-5-turbo", "kling-v25-turbo", "image-to-video", []string{"standard", "pro"}, false, "{mode}_norefv_v_duration")
	for _, op := range []string{"text-to-video", "image-to-video"} {
		addKling("v2.6", "kling-v2-6", "kling-v26", op, []string{"pro"}, true, "{mode}_{sound}_duration")
	}
	addKling("v2.6", "kling-v2-6", "kling-v26", "motion-control", []string{"standard", "pro"}, false, "mc_{mode}_v_duration")
	for _, op := range []string{"text-to-video", "image-to-video"} {
		addKling("v3", "kling-v3", "kling-v3", op, []string{"standard", "pro", "4k"}, true, "{mode}_{sound}_duration")
	}
	addKling("v3", "kling-v3", "kling-v3", "motion-control", []string{"standard", "pro", "4k"}, false, "mc_{mode}_v_duration")
	for _, op := range []string{"text-to-video", "image-to-video", "reference-to-video"} {
		addKling("o3", "kling-v3-omni", "kling-v3-omni", op, []string{"standard", "pro", "4k"}, false, "{mode}_{sound}_duration")
	}
	for _, op := range []string{"video-to-video/edit", "video-to-video/reference"} {
		addKling("o3", "kling-v3-omni", "kling-v3-omni", op, []string{"standard", "std", "pro"}, false, "{mode}_{ref}_v_duration")
	}
	for _, op := range []string{"text-to-video", "image-to-video", "reference-to-video", "video-to-video/edit", "video-to-video/reference"} {
		addKling("o1", "kling-video-o1", "kling-video-o1", op, []string{"standard", "pro"}, false, "{mode}_{ref}_v_duration")
	}
	for _, op := range []string{"text-to-video", "image-to-video"} {
		for _, mode := range []string{"standard", "pro"} {
			resolution := "720p"
			if mode == "pro" {
				resolution = "1080p"
			}
			kling = append(kling, falEndpoint{"fal-ai/kling-video/v3/turbo/" + mode + "/" + op, "kling-video/kling-3.0-turbo", "kling-30-turbo", provider.FalModel{Variant: resolution + "_av_duration"}})
		}
	}
	registerFal("qiniu_fal_kling", "Kling", "fal-ai/kling-video", kling)
	var vidu []falEndpoint
	for _, v := range []struct {
		version, suffix, price, doc string
		ops                         []string
	}{
		{"q1", "", "viduq1", "vidu-q1", []string{"text-to-video", "image-to-video", "reference-to-video", "start-end-to-video"}},
		{"q2", "", "viduq2", "vidu-q2", []string{"text-to-video", "reference-to-video"}},
		{"q2", "/pro", "viduq2-pro", "vidu-q2-pro", []string{"image-to-video", "start-end-to-video", "reference-to-video"}},
		{"q2", "/turbo", "viduq2-turbo", "vidu-q2-turbo", []string{"image-to-video", "start-end-to-video"}},
		{"q3", "", "vidu/viduq3", "vidu-q3", []string{"reference-to-video"}},
		{"q3", "/pro", "viduq3-pro", "vidu-q3-pro", []string{"text-to-video", "image-to-video", "start-end-to-video"}},
		{"q3", "/turbo", "viduq3-turbo", "vidu-q3-turbo", []string{"text-to-video", "image-to-video", "start-end-to-video", "reference-to-video"}},
	} {
		for _, op := range v.ops {
			mode := "i2v"
			if op == "text-to-video" {
				mode = "t2v"
			}
			if op == "reference-to-video" {
				mode = "r2v"
			}
			spec := provider.FalModel{Resolution: "720p", Variant: "{resolution}_" + mode + "_duration"}
			if v.version == "q1" {
				spec.Resolution = "1080p"
			}
			if v.version == "q2" {
				spec.PricingBlocked = "vidu_duration_tiers"
			}
			vidu = append(vidu, falEndpoint{"fal-ai/vidu/" + v.version + "/" + op + v.suffix, v.price, v.doc, spec})
		}
	}
	registerFal("qiniu_fal_vidu", "Vidu", "fal-ai/vidu", vidu)
	var veo []falEndpoint
	for _, variant := range []struct{ suffix, price, doc string }{{"", "veo-3.1-generate-001", "veo-31"}, {"/fast", "veo-3.1-fast-generate-001", "veo-31-fast"}} {
		for _, op := range []string{"", "/image-to-video", "/first-last-frame-to-video"} {
			veo = append(veo, falEndpoint{"fal-ai/veo3.1" + variant.suffix + op, variant.price, variant.doc, provider.FalModel{Resolution: "720p", Variant: "{audio}", AudioDefault: true}})
		}
	}
	registerFal("qiniu_fal_veo31", "Veo 3.1", "fal-ai/veo3.1", veo)
	for _, model := range []string{"h3", "h3-max"} {
		var endpoints []falEndpoint
		for _, op := range []string{"text-to-video", "image-to-video", "reference-to-video"} {
			spec := provider.FalModel{Resolution: "2k", Variant: "{resolution}_v_duration", PricingBlocked: "minimax_composite_usage"}
			if model == "h3-max" {
				spec.Resolution = "768p"
				if op != "reference-to-video" {
					spec.PricingBlocked = ""
				}
			}
			endpoints = append(endpoints, falEndpoint{"minimax/" + model + "/" + op, "minimax/minimax-" + model, "minimax-" + model, spec})
		}
		registerFal("qiniu_fal_minimax_"+strings.ReplaceAll(model, "-", "_"), "MiniMax "+model, "minimax/"+model, endpoints)
	}
}
