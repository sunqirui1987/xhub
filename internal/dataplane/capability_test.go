package dataplane

import (
	"os"
	"strings"
	"testing"
)

// TestCapabilitySetsStaySeparate locks the split between the chat loop, the
// official-API forwarder, and the Redis flush. A future edit that widens one
// of these parameters back to the full Host fails here.
func TestCapabilitySetsStaySeparate(t *testing.T) {
	checks := []struct {
		file string
		sig  string
	}{
		{"serve.go", "func Serve(h Adapted,"},
		{"official.go", "func ServeBypass(h Bypass,"},
		{"live.go", "func Flush(h Runtime)"},
		{"live.go", "func State(h Runtime)"},
	}
	for _, c := range checks {
		body, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), c.sig) {
			t.Fatalf("%s does not declare %s", c.file, c.sig)
		}
	}
}
