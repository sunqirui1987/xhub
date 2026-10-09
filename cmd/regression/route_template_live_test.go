package regression

import (
	"fmt"
	"net/http"
	"testing"
)

// TestLiveRouteTemplateSelectionAndBilling verifies the selected document and
// durable accounting while requests travel through an actual provider.
// Fault injection, exact retry counts and weighted distributions are covered
// separately by route_template_config_test.go's deterministic upstream.
func TestLiveRouteTemplateSelectionAndBilling(t *testing.T) {
	if !liveEnabled() {
		t.Skip("set XHUB_REGRESSION_LIVE=1 to call a real model")
	}
	h, admin, c, model := openLiveChat(t, "live-template")
	// Templates replace the whole platform document. Give every live stage
	// an explicit timeout, consistent with liveModelDeployment, rather than
	// relying on defaults while checking selection and durable accounting.
	liveBody := map[string]any{"routing_strategy": "simple-shuffle", "num_retries": 1, "timeout": 90}
	org := routeTemplate(t, h, admin, "live organization", liveBody)
	team := routeTemplate(t, h, admin, "live team", liveBody)
	key := routeTemplate(t, h, admin, "live key", liveBody)
	check := func(label, id, scope, scopeID string) {
		t.Helper()
		effective := effectiveTemplate(t, h, admin, "key", c.keyID)
		if got := stringField(effective, "template_id"); got != id || effective["scope_type"] != scope || stringField(effective, "scope_id") != scopeID {
			t.Fatalf("%s: effective template = %v; want %s at %s/%s", label, effective, id, scope, scopeID)
		}
		t.Logf("live template stage=%s scope=%s/%s template=%s model=%s", label, scope, scopeID, id, model)
		h.assertBilled(t, c, admin, model, fmt.Sprintf("Reply ok. Template stage %s.", label), nil)
	}
	check("platform", "", "platform", "")
	bindTemplate(t, h, admin, "organization", c.orgID, org)
	check("organization", org, "organization", c.orgID)
	bindTemplate(t, h, admin, "team", c.teamID, team)
	check("team", team, "team", c.teamID)
	bindTemplate(t, h, admin, "key", c.keyID, key)
	check("key", key, "key", c.keyID)
	h.ok(http.MethodPost, "/route_template/"+key+"/update", admin, map[string]any{"body": map[string]any{"routing_strategy": "simple-shuffle", "num_retries": 1, "timeout": 75}})
	if got := templateBody(t, h, admin, key)["timeout"]; got != float64(75) {
		t.Fatalf("edited template timeout = %v", got)
	}
	check("edited key", key, "key", c.keyID)
	bindTemplate(t, h, admin, "key", c.keyID, "")
	check("clear key", team, "team", c.teamID)
	bindTemplate(t, h, admin, "team", c.teamID, "")
	check("clear team", org, "organization", c.orgID)
	bindTemplate(t, h, admin, "organization", c.orgID, "")
	check("clear organization", "", "platform", "")
}
