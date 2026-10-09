package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFetchMarketExplicitSource 验证价格读取只访问显式地址，覆盖成功、空地址和失败响应。
// 参数 t：测试上下文；返回：无。调用 FetchMarket，不修改共享目录，本地服务自动关闭，无外部依赖。
func TestFetchMarketExplicitSource(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantError bool
	}{
		{"有效价格", 200, `{"status":true,"data":[{"id":"test-chat","issuer":{"name":"OpenAI"},"architecture":{"output_modalities":["text"]}}]}`, false},
		{"上游不可用", 503, `{}`, true},
		{"无效JSON", 200, `invalid`, true},
		{"空价格", 200, `{"status":true,"data":[]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/configured-prices" || r.Header.Get("Accept") != "application/json" {
					t.Errorf("未按显式源契约读取：%s", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer up.Close()
			url := up.URL + "/configured-prices"
			doc, err := FetchMarket(context.Background(), url)
			if (err != nil) != tc.wantError {
				t.Fatalf("价格源结果与预期不符：%v", err)
			}
			if !tc.wantError && (doc.Source != url || doc.Models["test-chat"] == nil) {
				t.Fatalf("源地址或模型丢失：%+v", doc)
			}
		})
	}
	if _, err := FetchMarket(context.Background(), "  "); err == nil {
		t.Fatal("空地址不应回退到内置远程源")
	}
}
