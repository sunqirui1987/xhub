package iam

import (
	"fmt"
	"net/url"
	"testing"
	"time"
)

// TestSessionZoneIsNamedOnTheConnection pins that the session time zone is set
// from the machine rather than left to the server. xorm reads a timestamp back
// in the application zone while pgx renders it in the session zone, so a server
// whose zone differs from the machine's would shift every stored instant.
func TestSessionZoneIsNamedOnTheConnection(t *testing.T) {
	got := dsnWithSessionZone("postgres://u:p@127.0.0.1:5432/x?sslmode=disable")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if u.Query().Get("timezone") == "" {
		t.Fatalf("no timezone on the connection: %s", got)
	}
	if u.Query().Get("sslmode") != "disable" {
		t.Fatalf("existing parameters were dropped: %s", got)
	}
	if u.Host != "127.0.0.1:5432" || u.Path != "/x" {
		t.Fatalf("dsn was rewritten: %s", got)
	}
}

// TestServerZoneUsesThePosixSign is the property that is easy to get backwards.
// PostgreSQL reads the sign of a numeric zone as the offset west of UTC, so a
// machine at UTC+8 has to be named "-08", not "+08:00". Sending the Go spelling
// would put the session at UTC-8 and move every stored timestamp by twice the
// offset, which is still a plausible-looking timestamp and so never fails loudly.
func TestServerZoneUsesThePosixSign(t *testing.T) {
	east := time.FixedZone("", 8*3600)
	if got := serverZone(east); got != "-08:00" {
		t.Fatalf("UTC+8 must be named -08:00 on the server, got %q", got)
	}
	west := time.FixedZone("", -8*3600)
	if got := serverZone(west); got != "+08:00" {
		t.Fatalf("UTC-8 must be named +08:00 on the server, got %q", got)
	}
	utc := time.FixedZone("", 0)
	if got := serverZone(utc); got != "-00:00" {
		t.Fatalf("UTC must be a zero offset, got %q", got)
	}
	// A location that knows its own name is passed through, since a named zone
	// carries its daylight-saving rules and a fixed offset does not.
	named, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skipf("zone database unavailable: %v", err)
	}
	if got := serverZone(named); got != "Asia/Shanghai" {
		t.Fatalf("a named zone must be sent by name, got %q", got)
	}
}

// TestSessionZoneIsAppliedByTheServer pins the end-to-end effect: the zone this
// package puts on the DSN is the zone the server actually runs the query in.
func TestSessionZoneIsAppliedByTheServer(t *testing.T) {
	db := testDB(t)
	// testDB connects the way Open does, so this reads back what Open asked for.
	want := serverZone(time.Local)
	q, err := db.Engine.QueryString(`SELECT current_setting('TimeZone') AS z`)
	if err != nil {
		t.Fatalf("read session zone: %v", err)
	}
	if q[0]["z"] != want {
		t.Fatalf("server zone %q does not match the requested %q", q[0]["z"], want)
	}
	// And the offset it applies is the machine's offset, which is the property
	// that keeps a stored instant equal to the one that was written.
	var delta string
	q2, err := db.Engine.QueryString(`SELECT (now() - now() AT TIME ZONE 'UTC')::interval AS d`)
	if err != nil {
		t.Fatalf("read offset: %v", err)
	}
	delta = q2[0]["d"]
	_, wantOffset := time.Now().Zone()
	wantHours := wantOffset / 3600
	wantText := fmt.Sprintf("%02d:00:00", wantHours)
	if wantHours < 0 {
		wantText = "-" + wantText[1:]
	}
	if delta != wantText {
		t.Fatalf("server offset %q, machine offset %d seconds", delta, wantOffset)
	}
}

// TestSessionZoneSurvivesAnUnparseableDSN pins that a DSN this function cannot
// parse is handed back untouched, so connection errors are reported by the
// driver rather than hidden by a rewrite.
func TestSessionZoneSurvivesAnUnparseableDSN(t *testing.T) {
	const bad = "://not a url"
	if got := dsnWithSessionZone(bad); got != bad {
		t.Fatalf("unparseable dsn was rewritten: %q", got)
	}
}
