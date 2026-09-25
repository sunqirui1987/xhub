package suite

import (
	"strings"
	"testing"
)

func TestCatalogCreateThenListConsumed(t *testing.T) {
	s, master := testEnv(t)
	h := s.Handler()

	type tc struct {
		createMethod, createPath string
		body                     map[string]any
		listPath                 string
		needle                   string
	}
	cases := []tc{
		{"POST", "/guardrails", map[string]any{"guardrail_name": "e2e-gr", "litellm_params": map[string]any{"guardrail": "custom"}}, "/guardrails/list", "e2e-gr"},
		{"POST", "/v1/access_group", map[string]any{"access_group_name": "e2e-ag"}, "/v1/access_group", "e2e-ag"},
		{"POST", "/credentials", map[string]any{"credential_name": "e2e-cred", "credential_values": map[string]any{"api_key": "sk-x"}}, "/credentials", "e2e-cred"},
	}
	for _, c := range cases {
		unauth := doJSON(t, h, c.createMethod, c.createPath, "", c.body)
		if unauth.Code != 401 {
			t.Fatalf("%s unauth want 401 got %d %s", c.createPath, unauth.Code, unauth.Body.String())
		}
		created := doJSON(t, h, c.createMethod, c.createPath, master, c.body)
		if created.Code != 200 {
			t.Fatalf("%s create %d %s", c.createPath, created.Code, created.Body.String())
		}
		if !strings.Contains(created.Body.String(), c.needle) {
			t.Fatalf("%s create body missing %q: %s", c.createPath, c.needle, created.Body.String())
		}
		listed := doJSON(t, h, "GET", c.listPath, master, nil)
		if listed.Code != 200 {
			t.Fatalf("%s list %d %s", c.listPath, listed.Code, listed.Body.String())
		}
		if !strings.Contains(listed.Body.String(), c.needle) {
			t.Fatalf("%s list missing created %q: %s", c.listPath, c.needle, listed.Body.String())
		}
	}
}
