package gateway

import (
	"errors"
	"testing"

	"github.com/sunqirui1987/xhub/internal/config"
	_ "github.com/sunqirui1987/xhub/internal/provider/qiniu"
)

// TestCredentialCatalogHydration 验证内联目录校验、未注册中转目录兼容、缺凭据库失败及复制隔离。
// 参数 t 为单测上下文；前置编译时七牛注册，无外部服务或数据库；仅内存数据，无需清理。
func TestCredentialCatalogHydration(t *testing.T) {
	for _, tc := range []struct {
		name, catalog, model, transport, credential string
		want                                        error
	}{
		{"registered", "qiniu", "bytedance/doubao-seedance-2-0-260128", "qiniu_contents_generation", "", nil},
		{"unregistered relay", "missing", "unknown", "qiniu_contents_generation", "", nil},
		{"wrong implementation", "qiniu", "bytedance/seedance-2.0/text-to-video", "qiniu_contents_generation", "", errCredentialInvalid},
		{"unknown model", "qiniu", "unknown", "qiniu_contents_generation", "", errCredentialInvalid},
		{"legacy", "", "unknown", "qiniu_contents_generation", "", nil},
		{"missing store", "qiniu", "unknown", "qiniu_contents_generation", "saved", errCredentialUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dep := config.ModelEntry{LiteLLMParams: map[string]any{"model": tc.model, "litellm_credential_name": tc.credential}, ModelInfo: map[string]any{"catalog_id": tc.catalog, "transport": tc.transport}}
			got, err := (&Server{}).withCredential(dep)
			if !errors.Is(err, tc.want) {
				t.Fatalf("目录水合错误：得到 %v，期望 %v", err, tc.want)
			}
			if err != nil {
				return
			}
			if got.ModelInfo["catalog_id"] != tc.catalog {
				t.Fatal("内联目录未保留")
			}
			if _, exists := got.LiteLLMParams["catalog_id"]; exists {
				t.Fatal("目录元数据泄漏到上游参数")
			}
			got.ModelInfo["catalog_id"] = "changed"
			if dep.ModelInfo["catalog_id"] != tc.catalog {
				t.Fatal("水合修改了原部署")
			}
		})
	}
}
