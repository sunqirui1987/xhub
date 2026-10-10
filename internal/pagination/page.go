// Package pagination 为数据库偏移和内存列表提供不会整数溢出的分页边界。
package pagination

// Offset 将一基页码转换为偏移；page/size 非正返回零，溢出返回最大整数表示越界。
// 数据库分页路由调用；返回值非负，无副作用，超大页不能回到第一页。
func Offset(page, size int) int {
	if page <= 1 || size <= 0 {
		return 0
	}
	max := int(^uint(0) >> 1)
	if page-1 > max/size {
		return max
	}
	return (page - 1) * size
}

// Bounds 返回内存列表安全的半开区间；total 是长度，page 是一基页码，size 是已规范化页大小。
// 各列表路由调用；空列表、非法页大小和越界页返回空区间，计算中先比较再相乘，不溢出。
func Bounds(total, page, size int) (int, int) {
	if total <= 0 || size <= 0 {
		return 0, 0
	}
	if page < 1 {
		page = 1
	}
	if page-1 > (total-1)/size {
		return total, total
	}
	start := (page - 1) * size
	end := total
	if size < total-start {
		end = start + size
	}
	return start, end
}
