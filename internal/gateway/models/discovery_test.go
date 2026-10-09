package models

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// TestCatalogURLs 验证供应商默认目录、自定义前缀、查询参数及非法地址边界。
// 参数 t：测试上下文；返回：无。纯函数测试不发网络请求，无需清理数据。
func TestCatalogURLs(t *testing.T) {
	for _, tc := range []struct {
		provider, base string
		want           []string
	}{
		{"ignored", "https://provider.example", []string{"https://provider.example/models", "https://provider.example/v1/models"}},
		{"qiniu", "https://configured.example/v1", []string{"https://configured.example/v1/models", "https://configured.example/models"}},
		{"", " https://relay.example/ ", []string{"https://relay.example/models", "https://relay.example/v1/models"}},
		{"", "https://relay.example/v1/", []string{"https://relay.example/v1/models", "https://relay.example/models"}},
		{"", "https://relay.example/proxy/models/", []string{"https://relay.example/proxy/models", "https://relay.example/proxy/v1/models"}},
		{"", "https://relay.example/proxy/v1?tenant=one", []string{"https://relay.example/proxy/v1/models?tenant=one", "https://relay.example/proxy/models?tenant=one"}},
	} {
		t.Run(tc.provider+tc.base, func(t *testing.T) {
			got, err := catalogURLs(ModelsURL(tc.provider, tc.base))
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("目录候选错误：got=%v want=%v err=%v", got, tc.want, err)
			}
		})
	}
	for _, base := range []string{"", "/v1", "ftp://relay.example", "https://user:secret@relay.example", "https://%"} {
		if _, err := catalogURLs(ModelsURL("", base)); err == nil {
			t.Errorf("非法地址未被拒绝")
		}
	}
}

// TestFetchCatalogFallback 验证正常、空目录及双向 404 回退；其他错误不得重复请求。
// 参数 t：测试上下文；返回：无。使用本地上游核对顺序、鉴权和脱敏错误，服务器自动关闭。
func TestFetchCatalogFallback(t *testing.T) {
	for _, tc := range []struct {
		name, base, body string
		first, second    int
		wantPaths        []string
		wantErr          string
	}{
		{"root success", "", `{"data":[{"id":"chat"}]}`, 200, 200, []string{"/models"}, ""},
		{"root to v1", "", `{"data":[{"id":"chat"}]}`, 404, 200, []string{"/models", "/v1/models"}, ""},
		{"v1 to root", "/v1", `[{"id":"chat"}]`, 404, 200, []string{"/v1/models", "/models"}, ""},
		{"custom prefix", "/proxy", `{"data":[{"id":"chat"}]}`, 404, 200, []string{"/proxy/models", "/proxy/v1/models"}, ""},
		{"both missing", "", ``, 404, 404, []string{"/models", "/v1/models"}, "/models: 404 Not Found; /v1/models: 404 Not Found"},
		{"unauthorized", "", ``, 401, 200, []string{"/models"}, "401 Unauthorized"},
		{"forbidden", "", ``, 403, 200, []string{"/models"}, "403 Forbidden"},
		{"rate limited", "", ``, 429, 200, []string{"/models"}, "429 Too Many Requests"},
		{"server error", "", ``, 500, 200, []string{"/models"}, "500 Internal Server Error"},
		{"fallback unauthorized", "", ``, 404, 401, []string{"/models", "/v1/models"}, "401 Unauthorized"},
		{"HTML response", "", `<html>login</html>`, 200, 200, []string{"/models"}, "invalid model discovery response"},
		{"error object", "", `{"error":"secret"}`, 200, 200, []string{"/models"}, "invalid model discovery response"},
		{"empty data", "", `{"data":[]}`, 200, 200, []string{"/models"}, ""},
		{"empty array", "", `[]`, 200, 200, []string{"/models"}, ""},
		{"redirect", "", ``, 302, 200, []string{"/models"}, "302 Found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.Header.Get("Authorization") != "Bearer server-secret" || r.Header.Get("x-api-key") != "server-secret" || r.URL.Query().Get("token") != "query-secret" {
					t.Error("目录请求未保留供应商鉴权或查询参数")
				}
				status := tc.first
				if len(paths) > 1 {
					status = tc.second
				}
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer upstream.Close()
			items, err := fetchCatalog(context.Background(), ModelsURL("", upstream.URL+tc.base+"?token=query-secret"), "server-secret")
			if !reflect.DeepEqual(paths, tc.wantPaths) {
				t.Fatalf("目录请求顺序错误：%v want=%v", paths, tc.wantPaths)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(tc.body, "chat") && (len(items) != 1 || items[0].ID != "chat") {
					t.Fatalf("目录解析失败：%v", items)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("目录失败契约错误：%v", err)
			}
		})
	}
}

// TestFetchCatalogCanceled 验证已取消的管理请求不会继续发起供应商目录读取。
// 参数 t：测试上下文；返回：无。使用本地服务，关闭服务清理，无持久数据。
func TestFetchCatalogCanceled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("取消后仍请求上游") }))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fetchCatalog(ctx, upstream.URL+"/models", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("未保留取消错误：%v", err)
	}
}
