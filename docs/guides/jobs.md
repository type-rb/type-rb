# `trb/jobs`

`trb/jobs` is an experimental portable contract for durable background work.
The same typed Job source runs in generated Go, Ruby, and Bun applications.
Storage is selected separately; the official `trb/jobs/sql` adapter supports
SQLite, PostgreSQL, and MySQL.

## Define and enqueue a Job

Job argument types come from the instance `perform` method. The initial
portable payload contract accepts Boolean, Integer, Float, and String values.

```trb
import { Job, maximum_attempts, priority, queue } from trb/jobs

class SendReceiptJob < Job
	queue("mail")
	priority(10)
	maximum_attempts(3)

	def perform(order_id: Integer, destination: String)
		puts("sending order " + order_id.to_s() + " to " + destination)
	end
end
```

The compiler derives typed enqueue methods from `perform`:

```trb
import { EnqueueError, JobReference } from trb/jobs
import trb/std/result
import { Duration, Instant } from trb/std/time

def enqueue_receipt(order_id: Integer): Result<JobReference, EnqueueError>
	reference := try SendReceiptJob.perform_later(order_id, "ada@example.test")
	puts(reference.id)

	try SendReceiptJob.perform_in(Duration.minutes(5), order_id, "later@example.test")
	return SendReceiptJob.perform_at(
		Instant.now().add(Duration.hours(2)),
		order_id,
		"scheduled@example.test",
	)
end
```

Enqueue operations return `Result<JobReference, EnqueueError>`. Prefix `try`
propagates an enqueue error from another Result-returning function, while
`catch` handles it at a boundary that returns another type. `EnqueueErrorKind`
distinguishes serialization, invalid arguments, cancellation, and adapter
failures.

Cancellation is checked before an adapter submits durable work. Once the
native storage operation reports a successful commit, the adapter returns
`Ok` even if the execution scope is cancelled immediately afterward; a known
success must not be rewritten as `Cancelled`. A storage client can still
report cancellation after a request has reached the server while its commit
outcome is unknown. `Cancelled` therefore does not prove that no Job was
enqueued, and callers must not assume that an unconditional retry cannot
duplicate work.

`perform_in` schedules relative to now; `perform_at` accepts an absolute
portable `Instant`. A past `Instant` is ready immediately. `queue`, `priority`,
and `maximum_attempts` are compile-time Job settings. The
default queue is `default`, the default priority is `0`, and lower priority
numbers run first. If a Job omits `maximum_attempts`, the adapter default is
used.

The derived methods are portable TypeRB wrappers rather than backend-specific
enqueue implementations. They encode the typed arguments as JSON, build an
`EnqueueRequest`, reject a negative relative delay, and normalize `perform_in`
to an absolute `Instant`. Applications normally call the derived methods and
do not construct an `EnqueueRequest` directly.

## Fallible Jobs

An infallible `perform` method omits its return type. A Job that needs worker
retry returns the exact `JobResult` contract instead:

```trb
import { Job, JobError, JobResult } from trb/jobs
import trb/std/unit

class ImportReceiptJob < Job
	def perform(source: String): JobResult
		if source.empty?()
			return JobResult::Err(JobError.new(message: "receipt source is empty"))
		end

		import_receipt(source)
		return JobResult::Ok(Unit.new())
	end
end
```

`JobResult` is `Result<Unit, JobError>`. `Ok(Unit.new())` acknowledges the Job
after `perform` completes. `Err(error)` stores exactly `error.message` and uses
the configured retry, backoff, and maximum-attempt policy. A Job maps database,
HTTP, and domain errors to `JobError` at this boundary so every backend records
the same operational message.

## Configure the SQL adapter

Add a typed composition module to the project configuration:

```jsonc
{
	"jobs": {
		"configuration": "config/jobs"
	}
}
```

Then select the adapter without coupling application Job modules to SQL:

```trb
import { JobAdapter } from trb/jobs
import { SQLAdapter, SQLDialect } from trb/jobs/sql
import { Duration } from trb/std/time

JOBS_ADAPTER: JobAdapter := SQLAdapter.new(
	dialect: SQLDialect::PostgreSQL,
	source: "postgres://localhost/jobs",
	source_environment: "JOBS_DATABASE_URL",
	poll_interval: Duration.seconds(1),
	lease_timeout: Duration.seconds(60),
	default_maximum_attempts: 5,
	retry_base_delay: Duration.seconds(1),
)
```

When `source_environment` is present, its environment variable is required at
runtime and replaces `source`. A missing or empty value fails startup instead
of silently connecting to another database.
MySQL source URLs use the same authentication and TLS options documented in
the [database schema guide](database.md).
The configuration module is also the native dependency boundary: application
Job source imports only `trb/jobs`, while an adapter package owns its target
drivers and worker implementation.
`JOBS_ADAPTER` is an ordinary immutable TypeRB constant. Its initializer runs
once when the configuration module loads, and every generated enqueue wrapper
reuses that application-scoped adapter instance. This explicit lifetime also
allows a future stateful adapter to own a connection pool or metrics state;
the configuration is not called as a per-enqueue factory.

## Adapter contract

`JobAdapter` deliberately has only two enqueue operations:

```trb
interface JobAdapter
	enqueue(request: EnqueueRequest): Result<JobReference, EnqueueError>
	enqueue_at(request: EnqueueRequest, scheduled_at: Instant): Result<JobReference, EnqueueError>
end
```

`EnqueueRequest` carries the stable Job name, serialized payload and payload
version, queue, priority, and an optional maximum-attempt override. `nil`
leaves maximum attempts to the adapter default. An adapter owns ID generation,
durable persistence, and conversion of native cancellation or storage failures
to `EnqueueError`. Worker claims, acknowledgements, retries, administration,
and process lifecycle are intentionally outside this small enqueue contract.

The official SQL adapter implements these methods in ordinary TypeRB source
and delegates only the final persistence operation to an internal native
primitive. That primitive remains a bundled implementation detail rather than
a generic external runtime ABI. The alpha compiler still recognizes a direct
`SQLAdapter.new(...)` composition so native dependencies and worker generation
remain deterministic.

## Run and inspect workers

```sh
trb jobs start
trb jobs start --queue mail
trb jobs start --once
trb jobs list
trb jobs retry JOB_ID
trb jobs discard JOB_ID
```

One command process runs one worker. PostgreSQL and MySQL support multiple
worker processes through short atomic claims. SQLite is for local and small
single-worker use only. `--once` claims at most one ready Job and is useful for
tests and operational scripts.

Workers retry failures with adapter-configured backoff, move exhausted Jobs to
`failed`, heartbeat active claims, and recover stale claims. Shutdown stops new
claims and lets the current Job return before releasing it. Delivery is at
least once, so Job implementations must be idempotent.

`trb jobs list` shows persisted state. `retry` returns a failed Job to the ready
queue, and `discard` removes a non-running Job.

## Application and REPL behavior

Jobs may call other portable packages, including `trb/orm`. The compiler-owned
execution scope crosses the worker dispatch boundary, so signal cancellation
can reach nested database and HTTP operations without adding a public context
parameter to `perform`. Payload encoding, relative-delay validation, adapter
dispatch, payload decoding, and typed Job selection are generated once as
portable TypeRB and compiled in every mode. The selected adapter and backend
retain queue persistence, claims, retries, signals, and process lifecycle.

In a configured project REPL, import a Job and call the same derived methods:

```console
trb:go> import { SendReceiptJob } from jobs/send_receipt_job
trb:go> SendReceiptJob.perform_later(42, "ada@example.test")
Ok(...) : Result<JobReference, EnqueueError>
```

The REPL persists through the configured adapter. Run `trb jobs start --once`
from another terminal to perform the queued Job.

Queue storage is intentionally separate from application transactions in this
initial contract. It provides durable enqueue after a successful call, not an
implicit cross-database transactional outbox.
