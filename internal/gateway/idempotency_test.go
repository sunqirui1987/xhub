package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sunqirui1987/xhub/internal/authz"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/iam"
)

func TestIdempotencyScopesHashesAndExpires(t *testing.T) {
	s, db, user, sessions := idempotencyFixture(t)
	body := []byte(`{"value":1}`)
	lease, hit, conflict := s.beginIdempotency(idemTestRequest(sessions[0]), "same", body)
	if lease == nil || hit != nil || conflict {
		t.Fatalf("first lookup lease=%v hit=%v conflict=%v", lease != nil, hit != nil, conflict)
	}
	lease.complete(&idemRec{Code: http.StatusCreated, Body: []byte("created")})
	_, hit, conflict = s.beginIdempotency(idemTestRequest(sessions[0]), "same", body)
	if hit == nil || hit.Code != http.StatusCreated || conflict {
		t.Fatalf("replay hit=%v conflict=%v", hit != nil, conflict)
	}
	_, _, conflict = s.beginIdempotency(idemTestRequest(sessions[0]), "same", []byte(`{"value":2}`))
	if !conflict {
		t.Fatal("same key with a different payload did not conflict")
	}
	other, hit, _ := s.beginIdempotency(idemTestRequest(sessions[1]), "same", body)
	if other == nil || hit != nil {
		t.Fatal("two UI sessions shared an idempotency namespace")
	}
	other.abort()
	for key := range s.idem {
		if strings.Contains(key, sessions[0]) || strings.Contains(key, sessions[1]) {
			t.Fatalf("stored session credential in idempotency key %q", key)
		}
	}

	teamID := testTeam(t, db, user.ID)
	key, plain, err := db.CreateKey(context.Background(), iam.Actor{ID: user.ID, Kind: "session"}, iam.KeyInput{Name: "idem", TeamID: teamID, OwnerType: iam.OwnerService})
	if err != nil {
		t.Fatal(err)
	}
	keyReq := idemTestRequest(plain)
	keyLease, _, _ := s.beginIdempotency(keyReq, "key-replay", body)
	if keyLease == nil {
		t.Fatal("valid key did not acquire idempotency lease")
	}
	keyLease.complete(&idemRec{Code: http.StatusOK, Body: []byte("ok")})
	for stored := range s.idem {
		if strings.Contains(stored, plain) {
			t.Fatalf("stored raw API key in idempotency key %q", stored)
		}
	}
	if _, err := db.SetKeyStatus(context.Background(), iam.Actor{ID: user.ID, Kind: "session"}, key.ID, iam.StatusRevoked); err != nil {
		t.Fatal(err)
	}
	if replayLease, replay, _ := s.beginIdempotency(idemTestRequest(plain), "key-replay", body); replayLease != nil || replay != nil {
		t.Fatal("revoked key reached its cached response")
	}

	now := time.Now()
	s.now = func() time.Time { return now }
	s.idemTTL = time.Second
	expiring, _, _ := s.beginIdempotency(idemTestRequest(sessions[0]), "expires", body)
	expiring.complete(&idemRec{Code: http.StatusOK})
	now = now.Add(2 * time.Second)
	afterExpiry, replay, _ := s.beginIdempotency(idemTestRequest(sessions[0]), "expires", body)
	if afterExpiry == nil || replay != nil {
		t.Fatal("expired response was replayed")
	}
	afterExpiry.abort()
}

func TestIdempotencyConcurrentRequestWaitsForOwner(t *testing.T) {
	s, _, _, sessions := idempotencyFixture(t)
	body := []byte(`{"value":1}`)
	owner, _, _ := s.beginIdempotency(idemTestRequest(sessions[0]), "concurrent", body)
	result := make(chan *idemRec, 1)
	go func() {
		_, replay, _ := s.beginIdempotency(idemTestRequest(sessions[0]), "concurrent", body)
		result <- replay
	}()
	select {
	case <-result:
		t.Fatal("concurrent request did not wait for the owner")
	case <-time.After(20 * time.Millisecond):
	}
	owner.complete(&idemRec{Code: http.StatusAccepted, Body: []byte("once")})
	select {
	case replay := <-result:
		if replay == nil || replay.Code != http.StatusAccepted || string(replay.Body) != "once" {
			t.Fatalf("concurrent replay %#v", replay)
		}
	case <-time.After(time.Second):
		t.Fatal("concurrent replay remained blocked")
	}
}

func idempotencyFixture(t *testing.T) (*Server, *iam.DB, *iam.User, []string) {
	t.Helper()
	db := testIdentityStore(t)
	user, err := db.CreateUser(context.Background(), iam.Actor{Kind: "system"}, iam.UserInput{Email: "idem@example.com", Name: "Idem", Password: "password123", Role: iam.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Cfg: &config.Config{}, IAM: db, Authz: authz.New(db), sessions: map[string]sessionRec{}, idem: map[string]*idemRec{}, idemTTL: defaultIdempotencyTTL, now: time.Now}
	sessions := []string{"sess-idem-a", "sess-idem-b"}
	for _, session := range sessions {
		s.sessions[session] = sessionRec{UserID: user.ID, Version: user.SessionVersion, ExpiresAt: time.Now().Add(time.Hour)}
	}
	return s, db, user, sessions
}

func idemTestRequest(credential string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/probe", nil)
	r.Header.Set("Authorization", "Bearer "+credential)
	return r
}
