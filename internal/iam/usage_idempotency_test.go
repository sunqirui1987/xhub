package iam

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"
)

// These tests pin the invariant RecordUsage documents: a replayed batch must
// not count twice.
//
// The replay is not hypothetical. The gateway buffers spend rows and flushes
// them, and the flush is retried both after a failure and when a second process
// replays the same buffered entry. Before this was keyed on request_id, each
// replay added the call again: the usage event existed once, but the roll-up
// and the scope counters kept climbing. Nothing in the gateway can detect that
// after the fact, because the only record is the numbers themselves.

// usageRecord builds one event for a key that exists in the fixture.
func usageRecord(requestID, userID string, cost float64) UsageRecord {
	return UsageRecord{
		RequestID:        requestID,
		TS:               time.Now().UTC(),
		OwnerType:        OwnerPersonal,
		UserID:           userID,
		Model:            "idem-model",
		CallType:         "chat",
		Status:           "success",
		PromptTokens:     20,
		CompletionTokens: 10,
		Cost:             cost,
		DurationMS:       40,
	}
}

func TestRecordUsageRejectsInvalidBatchBeforeWriting(t *testing.T) {
	db := testDB(t)
	good := usageRecord("valid-before-invalid", "", 1)
	negative := -1
	invalid := []UsageRecord{
		{Cost: 1},
		usageRecord("negative", "", -1),
		usageRecord("nan", "", math.NaN()),
		usageRecord("infinity", "", math.Inf(1)),
		{RequestID: "tokens", PromptTokens: -1},
		{RequestID: "cached", CachedTokens: &negative},
	}
	for _, bad := range invalid {
		if err := db.RecordUsage(context.Background(), []UsageRecord{good, bad}); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
		if countEvents(t, db, good.RequestID) != 0 {
			t.Fatal("invalid batch partially persisted")
		}
	}
}

func TestRecordUsageConcurrentSettlement(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	u, err := db.CreateUser(ctx, Actor{Kind: "system"}, UserInput{Email: "concurrent@example.com", Name: "Concurrent", Password: "password123", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	row := usageRecord("official-settlement:scoped-task", u.ID, 1.25)
	row.TS = time.Time{}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := db.RecordUsage(ctx, []UsageRecord{row}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if countEvents(t, db, row.RequestID) != 1 || userSpend(t, db, u.ID) != 1.25 || dailyRequests(t, db, u.ID) != 1 || dailyCost(t, db, u.ID) != 1.25 {
		t.Fatal("concurrent settlement charged or aggregated twice")
	}
	if !row.TS.IsZero() {
		t.Fatal("RecordUsage mutated caller record")
	}
}

// userSpend reads the live counter the budget check consults, for one account.
func userSpend(t *testing.T, db *DB, userID string) float64 {
	t.Helper()
	var spend float64
	has, err := db.session(context.Background()).Table("users").
		Where("id = ?", userID).Cols("spend").Get(&spend)
	if err != nil {
		t.Fatalf("read user spend: %v", err)
	}
	if !has {
		t.Fatalf("no user %s", userID)
	}
	return spend
}

func countEvents(t *testing.T, db *DB, requestID string) int64 {
	t.Helper()
	n, err := db.session(context.Background()).Table("usage_events").
		Where("request_id = ?", requestID).Count()
	if err != nil {
		t.Fatalf("count usage events: %v", err)
	}
	return n
}

// dailyCost sums the roll-up rows, which is what the usage report reads.
func dailyCost(t *testing.T, db *DB, userID string) float64 {
	t.Helper()
	var total float64
	for _, r := range dailyRows(t, db, userID) {
		total += r.Cost
	}
	return total
}

func dailyRequests(t *testing.T, db *DB, userID string) int64 {
	t.Helper()
	var requests int64
	for _, r := range dailyRows(t, db, userID) {
		requests += r.Requests
	}
	return requests
}

func dailyRows(t *testing.T, db *DB, userID string) []UsageDaily {
	t.Helper()
	var rows []UsageDaily
	err := db.session(context.Background()).Table("usage_daily").
		Where("user_id = ?", userID).Find(&rows)
	if err != nil {
		t.Fatalf("read usage_daily: %v", err)
	}
	return rows
}

// TestRecordUsageIsIdempotentOnRequestID is the regression test for the replay
// bug: the same batch delivered twice has to land once.
func TestRecordUsageIsIdempotentOnRequestID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	u, err := db.CreateUser(ctx, Actor{Kind: "system"}, UserInput{
		Email: "idem@example.com", Name: "Idem", Password: "password123", Role: RoleUser,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	rec := usageRecord("idem-replay-1", u.ID, 1.25)
	if err := db.RecordUsage(ctx, []UsageRecord{rec}); err != nil {
		t.Fatalf("first flush: %v", err)
	}
	if got := userSpend(t, db, u.ID); got != 1.25 {
		t.Fatalf("after one flush spend = %v, want 1.25", got)
	}

	// The same batch again, which is what a retry or a second flusher does.
	if err := db.RecordUsage(ctx, []UsageRecord{rec}); err != nil {
		t.Fatalf("replayed flush: %v", err)
	}
	// A third time, because an at-least-once queue can deliver more than twice.
	if err := db.RecordUsage(ctx, []UsageRecord{rec}); err != nil {
		t.Fatalf("third flush: %v", err)
	}

	if got := countEvents(t, db, "idem-replay-1"); got != 1 {
		t.Fatalf("usage_events has %d rows for one request_id, want 1", got)
	}
	if got := userSpend(t, db, u.ID); got != 1.25 {
		t.Fatalf("replay charged again: spend = %v, want 1.25", got)
	}
	if got := dailyRequests(t, db, u.ID); got != 1 {
		t.Fatalf("replay incremented the roll-up: requests = %d, want 1", got)
	}
	if got := dailyCost(t, db, u.ID); got != 1.25 {
		t.Fatalf("replay added to the reported cost: cost = %v, want 1.25", got)
	}
}

// TestRecordUsageDropsDuplicatesWithinOneBatch covers the other shape of the
// same bug: one batch that names the same request twice.
//
// A batch is not a set. The flusher builds it by reading a buffer, and a buffer
// can hold two entries for one request — the same call recorded once when it
// started and once when it finished, with the second meant to correct the
// first. Deduplicating only against rows already in the table would let the
// second entry through and charge the call twice inside a single transaction.
func TestRecordUsageDropsDuplicatesWithinOneBatch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	u, err := db.CreateUser(ctx, Actor{Kind: "system"}, UserInput{
		Email: "idem-batch@example.com", Name: "Batch", Password: "password123", Role: RoleUser,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	rec := usageRecord("idem-batch-1", u.ID, 2.00)
	if err := db.RecordUsage(ctx, []UsageRecord{rec, rec}); err != nil {
		t.Fatalf("batched duplicate: %v", err)
	}

	if got := countEvents(t, db, "idem-batch-1"); got != 1 {
		t.Fatalf("usage_events has %d rows, want 1", got)
	}
	if got := userSpend(t, db, u.ID); got != 2.00 {
		t.Fatalf("in-batch duplicate charged twice: spend = %v, want 2", got)
	}
	if got := dailyRequests(t, db, u.ID); got != 1 {
		t.Fatalf("in-batch duplicate counted twice: requests = %d, want 1", got)
	}
}

// TestRecordUsageCountsDistinctRequests guards the other direction. An
// idempotency rule that is too broad is as wrong as none: two different calls
// must both be charged.
func TestRecordUsageCountsDistinctRequests(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	u, err := db.CreateUser(ctx, Actor{Kind: "system"}, UserInput{
		Email: "idem-distinct@example.com", Name: "Distinct", Password: "password123", Role: RoleUser,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := db.RecordUsage(ctx, []UsageRecord{
		usageRecord("idem-distinct-1", u.ID, 1.00),
		usageRecord("idem-distinct-2", u.ID, 3.00),
	}); err != nil {
		t.Fatalf("two distinct requests: %v", err)
	}

	if got := userSpend(t, db, u.ID); got != 4.00 {
		t.Fatalf("spend = %v, want 4", got)
	}
	if got := dailyRequests(t, db, u.ID); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

// TestRecordUsageRollsBackTheWholeBatch pins that a failing row does not leave
// a partial charge behind.
//
// The event, the roll-up and the scope counter are one transaction. If they
// were not, a batch that failed halfway would leave a call counted in the
// report but not in the budget, or the reverse, and neither number could be
// trusted afterwards.
//
// The failure is injected through owner_type, which the schema constrains with
// a CHECK. It is the one column here that the database rejects rather than
// ignores, which matters: naming a key or user that does not exist is a silent
// no-op, because addScopeSpend's UPDATE simply matches no row. Only a
// constraint violation proves the surrounding transaction unwinds.
func TestRecordUsageRollsBackTheWholeBatch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	u, err := db.CreateUser(ctx, Actor{Kind: "system"}, UserInput{
		Email: "idem-rollback@example.com", Name: "Rollback", Password: "password123", Role: RoleUser,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	good := usageRecord("idem-rollback-1", u.ID, 5.00)
	bad := usageRecord("idem-rollback-2", u.ID, 5.00)
	bad.OwnerType = "not-a-real-owner-type"

	if err := db.RecordUsage(ctx, []UsageRecord{good, bad}); err == nil {
		t.Fatal("a batch with an invalid owner_type was accepted")
	}

	if got := userSpend(t, db, u.ID); got != 0 {
		t.Fatalf("a rolled-back batch left a charge: spend = %v, want 0", got)
	}
	if got := dailyRequests(t, db, u.ID); got != 0 {
		t.Fatalf("a rolled-back batch left a roll-up row: requests = %d, want 0", got)
	}
	if got := countEvents(t, db, "idem-rollback-1"); got != 0 {
		t.Fatalf("a rolled-back batch left the row that preceded the failure: %d rows, want 0", got)
	}

	// And the batch must be replayable once the bad row is fixed, rather than
	// leaving the good row permanently wedged behind it.
	if err := db.RecordUsage(ctx, []UsageRecord{usageRecord("idem-rollback-1", u.ID, 5.00)}); err != nil {
		t.Fatalf("replay after rollback: %v", err)
	}
	if got := userSpend(t, db, u.ID); got != 5.00 {
		t.Fatalf("after a clean replay spend = %v, want 5", got)
	}
}
