package models

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunqirui1987/xhub/internal/catalog"
)

// TestReloadRequiresExplicitFeed 验证缺少源与错误源的价格刷新边界。
// 参数 t：测试上下文；返回：无。核对失败不修改当前目录，本地 HTTP 服务自动关闭，不访问外部服务。
func TestReloadRequiresExplicitFeed(t *testing.T) {
	before := catalog.PriceSource()
	t.Setenv("XHUB_PRICE_FEED_URL", "")
	if _, err := reloadNow(context.Background()); err == nil {
		t.Fatal("未配置源仍然刷新价格")
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer up.Close()
	t.Setenv("XHUB_PRICE_FEED_URL", up.URL)
	if _, err := reloadNow(context.Background()); err == nil {
		t.Fatal("503源未报告失败")
	}
	if catalog.PriceSource() != before {
		t.Fatal("失败源改变了当前价格")
	}
}
