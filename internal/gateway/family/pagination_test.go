package family

import "testing"

// TestSliceMapsPagination 验证正常、末页、空列表、非法条数和极大页码安全切片；纯内存夹具无需清理。
func TestSliceMapsPagination(t *testing.T) {
	rows := []map[string]any{{"id": 1}, {"id": 2}, {"id": 3}}
	for _, c := range []struct{ page, size, want int }{{1, 2, 2}, {2, 2, 1}, {3, 2, 0}, {-1, 2, 2}, {1, 0, 3}, {int(^uint(0) >> 1), 2, 0}, {2, int(^uint(0) >> 1), 0}} {
		if got := sliceMaps(rows, c.page, c.size); len(got) != c.want {
			t.Fatalf("切片 page=%d size=%d: %v", c.page, c.size, got)
		}
	}
	if got := sliceMaps(nil, 1, 25); got == nil || len(got) != 0 {
		t.Fatalf("空列表必须为非nil空切片: %v", got)
	}
}
