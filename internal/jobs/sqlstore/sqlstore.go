// Package sqlstore defines the durable state model and dialect-specific SQL
// used by the portable SQL job adapter. Backend runtimes own their database
// clients, while this package keeps queue semantics consistent.
package sqlstore

import (
	"fmt"
	"time"
)

type Dialect string

const (
	SQLite     Dialect = "sqlite"
	PostgreSQL Dialect = "postgresql"
	MySQL      Dialect = "mysql"
)

type State string

const (
	Ready   State = "ready"
	Running State = "running"
	Failed  State = "failed"
)

type Record struct {
	ID              string
	Queue           string
	JobName         string
	Payload         string
	PayloadVersion  int
	Priority        int
	RunAt           time.Time
	State           State
	Attempts        int
	MaximumAttempts int
	ClaimedBy       string
	ClaimedAt       *time.Time
	LastError       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewRecord(id, jobName, payload string, maximumAttempts int, now time.Time) Record {
	return Record{
		ID: id, Queue: "default", JobName: jobName, Payload: payload,
		PayloadVersion: 1, RunAt: now, State: Ready,
		MaximumAttempts: maximumAttempts, CreatedAt: now, UpdatedAt: now,
	}
}

func (record *Record) Claim(workerID string, now time.Time) error {
	if record.State != Ready || record.RunAt.After(now) {
		return fmt.Errorf("job %s is not ready", record.ID)
	}
	record.State = Running
	record.Attempts++
	record.ClaimedBy = workerID
	record.ClaimedAt = pointer(now)
	record.UpdatedAt = now
	return nil
}

func (record *Record) Acknowledge(workerID string) error {
	if record.State != Running || record.ClaimedBy != workerID {
		return fmt.Errorf("job %s is not claimed by worker %s", record.ID, workerID)
	}
	return nil
}

// Fail records an execution failure. It returns true when the job remains
// retryable and false when it has reached its failed terminal state.
func (record *Record) Fail(workerID, message string, retryAt, now time.Time) (bool, error) {
	if record.State != Running || record.ClaimedBy != workerID {
		return false, fmt.Errorf("job %s is not claimed by worker %s", record.ID, workerID)
	}
	record.LastError = message
	record.ClaimedBy = ""
	record.ClaimedAt = nil
	record.UpdatedAt = now
	if record.Attempts >= record.MaximumAttempts {
		record.State = Failed
		return false, nil
	}
	record.State = Ready
	record.RunAt = retryAt
	return true, nil
}

func (record *Record) Release(now time.Time) bool {
	if record.State != Running {
		return false
	}
	record.State = Ready
	record.ClaimedBy = ""
	record.ClaimedAt = nil
	record.UpdatedAt = now
	return true
}

func (record Record) Stale(cutoff time.Time) bool {
	return record.State == Running && record.ClaimedAt != nil && record.ClaimedAt.Before(cutoff)
}

func Schema(dialect Dialect) ([]string, error) {
	switch dialect {
	case SQLite:
		return sqliteSchema, nil
	case PostgreSQL:
		return postgresqlSchema, nil
	case MySQL:
		return mysqlSchema, nil
	default:
		return nil, fmt.Errorf("unsupported job SQL dialect %q", dialect)
	}
}

func ClaimSelection(dialect Dialect) (string, error) {
	return claimSelection(dialect, "")
}

// ClaimSelectionForQueue returns the atomic claim selection restricted to one
// queue. The backend supplies the placeholder because SQL drivers differ.
func ClaimSelectionForQueue(dialect Dialect, placeholder string) (string, error) {
	if placeholder == "" {
		return "", fmt.Errorf("queue placeholder must not be empty")
	}
	return claimSelection(dialect, " AND queue_name = "+placeholder)
}

func claimSelection(dialect Dialect, queueCondition string) (string, error) {
	switch dialect {
	case SQLite:
		return `SELECT id FROM trb_jobs WHERE state = 'ready' AND run_at <= strftime('%Y-%m-%d %H:%M:%f', 'now')` + queueCondition + ` ORDER BY priority ASC, run_at ASC, id ASC LIMIT 1`, nil
	case PostgreSQL:
		return `SELECT id FROM trb_jobs WHERE state = 'ready' AND run_at <= CURRENT_TIMESTAMP` + queueCondition + ` ORDER BY priority ASC, run_at ASC, id ASC LIMIT 1 FOR UPDATE SKIP LOCKED`, nil
	case MySQL:
		return `SELECT id FROM trb_jobs WHERE state = 'ready' AND run_at <= CURRENT_TIMESTAMP(6)` + queueCondition + ` ORDER BY priority ASC, run_at ASC, id ASC LIMIT 1 FOR UPDATE SKIP LOCKED`, nil
	default:
		return "", fmt.Errorf("unsupported job SQL dialect %q", dialect)
	}
}

func pointer(value time.Time) *time.Time { return &value }

var sqliteSchema = []string{
	`CREATE TABLE IF NOT EXISTS trb_jobs (id TEXT PRIMARY KEY, queue_name TEXT NOT NULL, job_name TEXT NOT NULL, payload TEXT NOT NULL, payload_version INTEGER NOT NULL, priority INTEGER NOT NULL, run_at TEXT NOT NULL, state TEXT NOT NULL, attempts INTEGER NOT NULL, maximum_attempts INTEGER NOT NULL, claimed_by TEXT, claimed_at TEXT, last_error TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS trb_jobs_ready ON trb_jobs (state, priority, run_at, id)`,
}

var postgresqlSchema = []string{
	`CREATE TABLE IF NOT EXISTS trb_jobs (id TEXT PRIMARY KEY, queue_name TEXT NOT NULL, job_name TEXT NOT NULL, payload JSONB NOT NULL, payload_version INTEGER NOT NULL, priority INTEGER NOT NULL, run_at TIMESTAMPTZ NOT NULL, state TEXT NOT NULL, attempts INTEGER NOT NULL, maximum_attempts INTEGER NOT NULL, claimed_by TEXT, claimed_at TIMESTAMPTZ, last_error TEXT, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL)`,
	`CREATE INDEX IF NOT EXISTS trb_jobs_ready ON trb_jobs (state, priority, run_at, id)`,
}

var mysqlSchema = []string{
	`CREATE TABLE IF NOT EXISTS trb_jobs (id VARCHAR(64) PRIMARY KEY, queue_name VARCHAR(255) NOT NULL, job_name VARCHAR(255) NOT NULL, payload JSON NOT NULL, payload_version INTEGER NOT NULL, priority INTEGER NOT NULL, run_at DATETIME(6) NOT NULL, state VARCHAR(32) NOT NULL, attempts INTEGER NOT NULL, maximum_attempts INTEGER NOT NULL, claimed_by VARCHAR(255), claimed_at DATETIME(6), last_error TEXT, created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL, INDEX trb_jobs_ready (state, priority, run_at, id))`,
}
