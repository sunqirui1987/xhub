package server

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
