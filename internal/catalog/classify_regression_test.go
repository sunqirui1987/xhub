package catalog

import "testing"

func TestProviderResourceAliasesRequireInferenceIdentity(t *testing.T) {
	for _, collection := range []string{"assistants", "threads", "fine_tuning", "containers", "videos", "vector_stores"} {
		for _, prefix := range []string{"/", "/v1/"} {
			for _, suffix := range []string{"", "/resource-id", "/resource-id/messages"} {
				path := prefix + collection + suffix
				if AuthOf(path) != AuthData || !IsDataPlanePath(path) {
					t.Errorf("%s bypasses inference resource isolation", path)
				}
			}
		}
		if AuthOf("/"+collection+"_settings") != AuthManagement {
			t.Errorf("collection %s matched a different management prefix", collection)
		}
	}
}
