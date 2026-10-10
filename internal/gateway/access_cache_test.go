package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFlushCacheHandler 验证管理处理器清空进程缓存，匿名调用不修改缓存，重复清空安全成功。
// 前置私有 PostgreSQL 及真实管理员会话；直接调用被测处理器，不启动数据面或上游。
// 结果由 HTTP 状态和缓存读取断言；登录服务、连接及 schema 自动清理。
func TestFlushCacheHandler(t *testing.T) {
	cfg, st, db := testGatewayStores(t)
	s := New(cfg, st, db)
	server := httptest.NewServer(s.Handler())
	t.Cleanup(server.Close)
	admin := loginAdmin(t, server.URL, db)
	s.Cache.Set("test-entry", []byte("cached"))
	anonymous := httptest.NewRecorder()
	s.flushCache(anonymous, httptest.NewRequest(http.MethodPost, "/flushall", nil))
	if _, exists := s.Cache.Get("test-entry"); anonymous.Code != http.StatusUnauthorized || !exists {
		t.Fatalf("匿名清空缓存未拒绝或修改数据: status=%d exists=%v", anonymous.Code, exists)
	}
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest(http.MethodPost, "/flushall", nil)
		r.Header.Set("Authorization", "Bearer "+admin)
		w := httptest.NewRecorder()
		s.flushCache(w, r)
		if _, exists := s.Cache.Get("test-entry"); w.Code != http.StatusOK || exists {
			t.Fatalf("管理清空缓存第%d次未生效: status=%d exists=%v body=%s", i+1, w.Code, exists, w.Body.String())
		}
	}
}
