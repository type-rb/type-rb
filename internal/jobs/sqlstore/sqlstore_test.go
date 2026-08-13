package sqlstore

import (
	"strings"
	"testing"
	"time"
)

func TestRecordLifecycleRetriesThenFails(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	record := NewRecord("job-1", "ReceiptJob", `[1]`, 2, now)
	if err := record.Claim("worker-1", now); err != nil {
		t.Fatal(err)
	}
	retryAt := now.Add(time.Second)
	retry, err := record.Fail("worker-1", "temporary", retryAt, now)
	if err != nil || !retry || record.State != Ready || !record.RunAt.Equal(retryAt) {
		t.Fatalf("unexpected retry transition: retry=%v err=%v record=%#v", retry, err, record)
	}
	if err := record.Claim("worker-2", retryAt); err != nil {
		t.Fatal(err)
	}
	retry, err = record.Fail("worker-2", "permanent", retryAt, retryAt)
	if err != nil || retry || record.State != Failed {
		t.Fatalf("unexpected failed transition: retry=%v err=%v record=%#v", retry, err, record)
	}
}

func TestRecordReleasesStaleClaim(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	record := NewRecord("job-1", "ReceiptJob", `[]`, 5, now)
	if err := record.Claim("worker-1", now); err != nil {
		t.Fatal(err)
	}
	if !record.Stale(now.Add(time.Second)) || !record.Release(now.Add(time.Second)) || record.State != Ready {
		t.Fatalf("stale claim was not released: %#v", record)
	}
}

func TestServerDatabaseClaimsUseSkipLocked(t *testing.T) {
	for _, dialect := range []Dialect{PostgreSQL, MySQL} {
		query, err := ClaimSelection(dialect)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(query, "FOR UPDATE SKIP LOCKED") {
			t.Fatalf("%s claim is not multi-worker safe: %s", dialect, query)
		}
	}
	query, err := ClaimSelection(SQLite)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(query, "SKIP LOCKED") {
		t.Fatalf("SQLite must retain its explicit single-worker path: %s", query)
	}
	if !strings.Contains(query, `strftime('%Y-%m-%d %H:%M:%f', 'now')`) {
		t.Fatalf("SQLite must compare scheduled jobs with fractional-second precision: %s", query)
	}
}

func TestClaimSelectionCanFilterOneQueue(t *testing.T) {
	for _, dialect := range []Dialect{SQLite, PostgreSQL, MySQL} {
		query, err := ClaimSelectionForQueue(dialect, "?")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(query, "queue_name = ?") {
			t.Fatalf("%s selection does not filter its queue: %s", dialect, query)
		}
	}
}

func TestAllDialectsCreateDurableJobSchema(t *testing.T) {
	for _, dialect := range []Dialect{SQLite, PostgreSQL, MySQL} {
		statements, err := Schema(dialect)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(statements, "\n")
		for _, required := range []string{"payload_version", "maximum_attempts", "claimed_by", "claimed_at", "last_error"} {
			if !strings.Contains(joined, required) {
				t.Fatalf("%s schema is missing %s: %s", dialect, required, joined)
			}
		}
	}
}
