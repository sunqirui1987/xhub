package usage

import "testing"

func TestCollapseSessionsSeparatesCallersWithTheSameSessionID(t *testing.T) {
	rows := []map[string]any{
		{"request_id": "key-a-1", "session_id": "shared", "api_key": "key-a", "user": "user-1", "spend": 1.0, "total_tokens": 10},
		{"request_id": "key-b-1", "session_id": "shared", "api_key": "key-b", "user": "user-1", "spend": 2.0, "total_tokens": 20},
		{"request_id": "key-a-2", "session_id": "shared", "api_key": "key-a", "user": "user-1", "spend": 3.0, "total_tokens": 30},
		{"request_id": "user-2-1", "session_id": "shared", "api_key": "", "user": "user-2", "spend": 4.0, "total_tokens": 40},
	}

	got := collapseSessions(rows)
	if len(got) != 3 {
		t.Fatalf("collapsed rows = %d, want 3: %#v", len(got), got)
	}
	assertSessionTotals(t, got[0], 2, 4, 40)
	assertSessionTotals(t, got[1], 1, 2, 20)
	assertSessionTotals(t, got[2], 1, 4, 40)
}

func TestCollapseSessionsUsesUserForKeylessCalls(t *testing.T) {
	rows := []map[string]any{
		{"request_id": "u1", "session_id": "shared", "user": "user-1"},
		{"request_id": "u2", "session_id": "shared", "user": "user-2"},
	}

	got := collapseSessions(rows)
	if len(got) != 2 {
		t.Fatalf("collapsed rows = %d, want 2: %#v", len(got), got)
	}
}

func TestCollapseSessionsDoesNotJoinRowsWithoutCaller(t *testing.T) {
	rows := []map[string]any{
		{"request_id": "first", "session_id": "shared"},
		{"request_id": "second", "session_id": "shared"},
		{"session_id": "shared"},
		{"session_id": "shared"},
	}
	got := collapseSessions(rows)
	if len(got) != 4 {
		t.Fatalf("anonymous calls were merged: %#v", got)
	}
}

func assertSessionTotals(t *testing.T, row map[string]any, count int, spend float64, tokens int) {
	t.Helper()
	if row["session_total_count"] != count || row["session_total_spend"] != spend || row["session_total_tokens"] != tokens {
		t.Fatalf("session totals = count %#v spend %#v tokens %#v", row["session_total_count"], row["session_total_spend"], row["session_total_tokens"])
	}
}
