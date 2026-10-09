package models

import (
	"bytes"
	"github.com/sunqirui1987/xhub/internal/config"
	"net/http/httptest"
	"testing"
)

// TestSetFallbackInvalidAndUnavailable 验证管理接口严格正文、缺失模型和存储不可用。
// 前置内存模型宿主，断言400/404/503且目录不变，无写入数据和清理需求。
func TestSetFallbackInvalidAndUnavailable(t *testing.T) {
	h := &modelTestHost{models: []config.ModelEntry{{ModelName: "a"}, {ModelName: "b"}}}
	for _, tc := range []struct {
		body   string
		status int
	}{
		{"{}", 400}, {"null", 400}, {"{\"model_name\":\"a\",\"policy\":null}", 400},
		{"{\"model_name\":\"a\",\"policy\":{}} {}", 400},
		{"{\"model_name\":\"missing\",\"policy\":{}}", 404},
		{"{\"model_name\":\"a\",\"policy\":{\"fallbacks\":[\"a\"]}}", 400},
		{"{\"model_name\":\"a\",\"policy\":{\"fallbacks\":[\"b\"]}}", 503},
	} {
		r := httptest.NewRequest("PUT", "/model/fallback", bytes.NewBufferString(tc.body))
		w := httptest.NewRecorder()
		SetFallback(h, w, r)
		if w.Code != tc.status {
			t.Fatalf("请求%s 状态%d: %s", tc.body, w.Code, w.Body.String())
		}
		if len(h.models) != 2 {
			t.Fatal("失败请求修改了模型目录")
		}
	}
}

// TestFallbackRemovalWithoutPolicy 验证无策略移除和同名其他部署边界；纯内存目录，无持久化清理。
func TestFallbackRemovalWithoutPolicy(t *testing.T) {
	h := &modelTestHost{models: []config.ModelEntry{{ModelName: "a", ModelInfo: map[string]any{"id": "a1"}}, {ModelName: "a", ModelInfo: map[string]any{"id": "a2"}}}}
	if err := checkFallbackRemoval(h, "a", "a1"); err != nil {
		t.Fatal(err)
	}
	h.models = h.models[:1]
	if err := checkFallbackRemoval(h, "a", "a1"); err != nil {
		t.Fatal(err)
	}
}
