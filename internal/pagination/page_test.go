package pagination

import "testing"

// TestOffsetAndBounds 验证普通、末页、空数据、非法输入和最大整数边界；纯函数无需清理。
// 前置仅内存数据；结果必须非负、在长度内、超大页为空而不是回绕或 panic。
func TestOffsetAndBounds(t *testing.T) {
	max := int(^uint(0) >> 1)
	for _, c := range []struct{ page, size, want int }{{1, 25, 0}, {2, 25, 25}, {0, 25, 0}, {-max, 25, 0}, {2, 0, 0}, {max, 25, max}, {max, 1, max - 1}} {
		if got := Offset(c.page, c.size); got != c.want {
			t.Fatalf("偏移 page=%d size=%d: %d != %d", c.page, c.size, got, c.want)
		}
	}
	for _, c := range []struct{ total, page, size, start, end int }{{26, 1, 25, 0, 25}, {26, 2, 25, 25, 26}, {26, 3, 25, 26, 26}, {0, 1, 25, 0, 0}, {26, -1, 25, 0, 25}, {26, max, 25, 26, 26}, {26, 1, max, 0, 26}, {26, 2, max, 26, 26}, {26, 2, 0, 0, 0}, {max, 2, max, max, max}} {
		a, b := Bounds(c.total, c.page, c.size)
		if a != c.start || b != c.end {
			t.Fatalf("边界 %+v 得到 [%d,%d)", c, a, b)
		}
	}
}
