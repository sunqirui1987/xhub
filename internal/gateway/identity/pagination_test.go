package identity

import (
	"net/http/httptest"
	"reflect"
	"testing"
)

// TestDirectoryPaginationInputs 验证普通/非法/极大页偏移、CSV空项和空角色；纯函数不写数据库，无需清理。
// 前置请求字符串；结果不会溢出回到首页，缺失筛选不会意外限制角色。
func TestDirectoryPaginationInputs(t *testing.T) {
	for _, c := range []struct {
		query string
		want  int
	}{{"", 0}, {"page=2", 25}, {"page=-1", 0}, {"page=bad", 0}, {"page=9223372036854775807", int(^uint(0) >> 1)}} {
		if got := pageOffset(httptest.NewRequest("GET", "/?"+c.query, nil), 25); got != c.want {
			t.Fatalf("目录偏移 %q: %d != %d", c.query, got, c.want)
		}
	}
	if got := stringListCSV(" a, ,b "); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("标识列表解析: %v", got)
	}
	if got := stringListCSV(""); len(got) != 0 {
		t.Fatalf("空标识: %v", got)
	}
	for _, c := range []struct{ raw, want string }{{"", ""}, {"internal_user", "user"}, {"proxy_admin", "admin"}, {"invalid", "invalid"}} {
		if got := listRole(c.raw); got != c.want {
			t.Fatalf("角色筛选 %q: %q", c.raw, got)
		}
	}
}
