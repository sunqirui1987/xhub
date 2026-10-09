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

// TestFetchCatalogFallback 验证正常、空目录、双向 404 及无效目录回退；鉴权、限流等错误不得重复请求。
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
		{"HTML response", "", `<html>login</html>`, 200, 200, []string{"/models", "/v1/models"}, "invalid model discovery response"},
		{"error object", "", `{"error":"secret"}`, 200, 200, []string{"/models", "/v1/models"}, "invalid model discovery response"},
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

// TestFetchCatalogInvalidResponseFallback 验证网站 HTML、错误 JSON 和畸形正文不会阻断同源目录发现。
// 参数 t：测试上下文；返回：无。前置为本地上游，验证双向及自定义前缀回退、空目录和后续错误；
// 响应正文与查询秘密不得泄露，服务器自动关闭，不写入持久数据。
func TestFetchCatalogInvalidResponseFallback(t *testing.T) {
	for _, tc := range []struct {
		name, base, firstBody, secondBody, wantErr string
		secondStatus                               int
		wantCount                                  int
	}{
		{"website to v1", "", `<html>server-secret</html>`, `{"data":[{"id":"chat"}]}`, "", 200, 1},
		{"website to root", "/v1", `<html>server-secret</html>`, `[{"id":"chat"}]`, "", 200, 1},
		{"custom prefix", "/relay", `<html>server-secret</html>`, `{"data":[{"id":"chat"}]}`, "", 200, 1},
		{"error JSON", "", `{"error":"server-secret"}`, `{"data":[{"id":"chat"}]}`, "", 200, 1},
		{"malformed JSON", "", `{`, `{"data":[]}`, "", 200, 0},
		{"empty body", "", ``, `[]`, "", 200, 0},
		{"fallback missing", "", `<html>server-secret</html>`, ``, "/v1/models: 404 Not Found", 404, 0},
		{"fallback invalid", "", `<html>server-secret</html>`, `{"error":"server-secret"}`, "/v1/models: invalid model discovery response", 200, 0},
		{"fallback unauthorized", "", `<html>server-secret</html>`, ``, "401 Unauthorized", 401, 0},
		{"fallback rate limited", "", `<html>server-secret</html>`, ``, "429 Too Many Requests", 429, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.Header.Get("Authorization") != "Bearer server-secret" || r.Header.Get("x-api-key") != "server-secret" || r.URL.Query().Get("token") != "query-secret" {
					t.Error("回退请求未保留已存供应商的鉴权和查询参数")
				}
				if len(paths) == 1 {
					_, _ = io.WriteString(w, tc.firstBody)
					return
				}
				w.WriteHeader(tc.secondStatus)
				_, _ = io.WriteString(w, tc.secondBody)
			}))
			defer upstream.Close()
			rawURL := ModelsURL("", upstream.URL+tc.base+"?token=query-secret")
			items, err := fetchCatalog(context.Background(), rawURL, "server-secret")
			wantURLs, _ := catalogURLs(rawURL)
			if len(paths) != len(wantURLs) {
				t.Fatalf("无效目录必须恰好尝试两个同源候选：%v", paths)
			}
			for i, candidate := range wantURLs {
				if !strings.HasPrefix(candidate, upstream.URL+paths[i]+"?") {
					t.Fatalf("无效目录回退顺序错误：%v", paths)
				}
			}
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || strings.Contains(err.Error(), "secret") {
					t.Fatalf("无效目录回退失败契约或脱敏错误：%v", err)
				}
			} else if err != nil || len(items) != tc.wantCount || (tc.wantCount > 0 && items[0].ID != "chat") {
				t.Fatalf("无效目录回退未返回有效模型列表：%v err=%v", items, err)
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

// TestMergeCatalogModels 验证上架列表完整、专用型号补充、去重、价格保留及空目录边界。
// 参数 t 为测试上下文；返回无；仅内存，无外部依赖和清理。
func TestMergeCatalogModels(t *testing.T) {
	price := 1.25
	items := []CatalogModel{{ID: "gpt-5.6-sol", InputPrice: &price}, {ID: "video"}, {ID: "gpt-5.6-sol"}, {ID: ""}}
	got := mergeCatalogModels(items, map[string][]string{"video": {"fal"}, "ark": {"ark"}})
	if len(got) != 3 || got[0].ID != "ark" || got[1].ID != "gpt-5.6-sol" || got[1].InputPrice != &price || got[2].ID != "video" {
		t.Fatalf("完整目录合并错误: %+v", got)
	}
	if len(mergeCatalogModels(nil, nil)) != 0 || items[0].ID != "gpt-5.6-sol" {
		t.Fatal("空目录或输入不变性错误")
	}
}
