// Package prefs merges configuration documents. A database key overrides YAML, and a partial update does not drop keys that were not mentioned.
package prefs

// Overlay copies the YAML base and then writes every database key on top. A database value wins even when it is a list or null. Keys absent from the database stay as they were.
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

// MergePatch folds a partial update into the current document. Nested objects are merged, while lists and scalars replace the previous value. Keys that the patch does not mention stay.
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
