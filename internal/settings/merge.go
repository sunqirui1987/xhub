// 设置文档的合并规则。数据库覆盖 YAML，局部更新不会清掉没提到的键。
package settings

// 数据库里的键覆盖 YAML 基线。即使值是列表或 null 也覆盖。数据库没有的键保持原样。
// Overlay applies database keys on top of the YAML base. A database key wins
// even when its value is a list or null. Keys absent from the database stay.
func Overlay(base, db map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range db {
		out[k] = v
	}
	return out
}

// 把局部更新折进当前文档。嵌套对象继续合并，列表和标量直接替换。没提到的键保留。
// MergePatch folds a partial update into the current document. Nested objects
// merge; lists and scalars replace. Unmentioned keys stay.
func MergePatch(current, patch map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range current {
		out[k] = v
	}
	for k, v := range patch {
		cur, curMap := out[k].(map[string]any)
		pv, patchMap := v.(map[string]any)
		if curMap && patchMap {
			out[k] = MergePatch(cur, pv)
			continue
		}
		out[k] = v
	}
	return out
}
