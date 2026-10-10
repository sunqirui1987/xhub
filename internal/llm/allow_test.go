package llm

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAllowWithoutPublicSchema 验证数据面允许表排除公共接口定义。
// 参数 t 为单元测试上下文，无返回值；前置无需外部服务，覆盖合法调用、空路径和已移除路径，无持久数据需清理。
func TestAllowWithoutPublicSchema(t *testing.T) {
	for _, tc := range []struct {
		path  string
		mount bool
		want  bool
	}{
		{"/v1/chat/completions", false, true},
		{"/health/liveness", false, true},
		{"/metrics", true, true},
		{"", false, false},
		{"/openapi.json", false, false},
		{"/openapi.json/", false, false},
		{"/openapi.json", true, false},
	} {
		if got := Allow(tc.path, tc.mount); got != tc.want {
			t.Errorf("路径 %q mount=%v 允许结果=%v，期望=%v", tc.path, tc.mount, got, tc.want)
		}
	}
}

// TestFilterRejectsPublicSchema 验证实际过滤器返回 404 且不调用下游，合法数据面仍可放行。
// 参数 t 为单元测试上下文，无返回值；前置内存 HTTP 请求，验证正常和拒绝分支，响应记录器随测试释放。
func TestFilterRejectsPublicSchema(t *testing.T) {
	calls := 0
	filtered := Filter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
		w := httptest.NewRecorder()
		filtered.ServeHTTP(w, httptest.NewRequest(method, "/openapi.json", nil))
		if w.Code != http.StatusNotFound || calls != 0 {
			t.Fatalf("%s 已移除定义入口应返回 404 且不执行下游，status=%d calls=%d", method, w.Code, calls)
		}
	}
	w := httptest.NewRecorder()
	filtered.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if w.Code != http.StatusNoContent || calls != 1 {
		t.Fatalf("合法数据面应继续放行，status=%d calls=%d", w.Code, calls)
	}
}
