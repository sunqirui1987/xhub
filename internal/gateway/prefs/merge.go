// Package prefs merges configuration documents. A database key overrides YAML, and a partial update does not drop keys that were not mentioned.
package prefs

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceMerge sync.Once

// Overlay copies the YAML base and then writes every database key on top. A database value wins even when it is a list or null. Keys absent from the database stay as they were.
// 参数 base（map[string]any）：当前已保存的字段表，补丁叠在它上面；db（map[string]any）：Overlay读到的 JSON 对象。缺键表示没有该字段。
// 返回 map[string]any（map[string]any）：Overlay的字段表。缺键表示上游或库里没有这个字段。
// 调用：gateway/prefs/settings.go
// 测试：无直接单测
func Overlay(base, db map[string]any) map[string]any {
	logTraceOnceMerge.Do(func() { logx.Trace("enter prefs.Overlay") })

	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range db {
		out[k] = v
	}
	return out
}

// MergePatch folds a partial update into the current document. Nested objects are merged, while lists and scalars replace the previous value. Keys that the patch does not mention stay.
// 参数 current（map[string]any）：当前已保存的字段表，补丁叠在它上面；patch（map[string]any）：调用方提交的补丁。只覆盖出现的键。
// 返回 map[string]any（map[string]any）：合并Patch的字段表。缺键表示上游或库里没有这个字段。
// 调用：gateway/prefs/settings.go
// 测试：无直接单测
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
