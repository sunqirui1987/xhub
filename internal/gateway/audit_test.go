package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/sunqirui1987/xhub/internal/iam"
)

func TestAuditLogsConsoleFiltersAndPagination(t *testing.T) {
	f := newPermFixture(t)
	for _, action := range []string{"key.create", "key.update", "key.delete"} {
		if err := f.db.RecordAudit(context.Background(), iam.Actor{ID: f.admin.user.ID, Kind: "session"}, iam.Audit{
			Action: action, ObjectType: "key", ObjectID: "audit-key", TeamID: "audit-team",
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		query, action      string
		total, page, pages int
	}{
		{"page=2&page_size=1&table_name=LiteLLM_VerificationToken&object_team_id=audit-team", "key.update", 3, 2, 3},
		{"page_size=1&action=deleted&object_key_hash=audit-key", "key.delete", 1, 1, 1},
		{"page_size=1&search=missing-audit-record", "", 0, 1, 0},
	} {
		status, body := f.call(f.admin, http.MethodGet, "/audit/logs?"+tc.query, nil)
		if status != http.StatusOK {
			t.Fatalf("audit query %s: %d %s", tc.query, status, body)
		}
		var response struct {
			Rows     []iam.AuditEntry `json:"audit_logs"`
			Total    int              `json:"total"`
			Page     int              `json:"page"`
			PageSize int              `json:"page_size"`
			Pages    int              `json:"total_pages"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatal(err)
		}
		if response.Total != tc.total || response.Page != tc.page || response.PageSize != 1 || response.Pages != tc.pages {
			t.Fatalf("audit metadata for %s: %+v", tc.query, response)
		}
		if tc.total == 0 {
			if response.Rows == nil || len(response.Rows) != 0 {
				t.Fatalf("empty audit query: %s", body)
			}
		} else if len(response.Rows) != 1 || response.Rows[0].Action != tc.action || response.Rows[0].ActorID != f.admin.user.ID {
			t.Fatalf("audit record for %s: %s", tc.query, body)
		}
	}
}

func TestAuditLogsOnlyPlatformAdmin(t *testing.T) {
	f := newPermFixture(t)
	if _, err := f.db.AddOrgAdmin(context.Background(), iam.Actor{Kind: "system"}, f.orgA.ID, f.outsider.user.Email); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		caller actor
	}{
		{"member", f.member},
		{"team administrator", f.teamAdmin},
		{"organization administrator", f.outsider},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, body := f.call(tc.caller, http.MethodGet, "/audit/logs?page=1&page_size=50", nil)
			if status != http.StatusForbidden {
				t.Fatalf("expected forbidden, got %d %s", status, body)
			}
		})
	}
	status, body := f.call(f.admin, http.MethodGet, "/audit/logs", nil)
	if status != http.StatusOK {
		t.Fatalf("platform administrator: %d %s", status, body)
	}
}
