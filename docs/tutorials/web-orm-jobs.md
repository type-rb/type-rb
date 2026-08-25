# Build a report API with Web, ORM, and Jobs

This tutorial builds one portable backend slice: an HTTP endpoint stores a
report with `trb/orm`, enqueues durable work with `trb/jobs`, and a worker
updates the same report. The TypeRB source is shared by the Go, Ruby, and
TypeScript backends.

The complete, executable project lives in
[`examples/tutorials/web-orm-jobs`](https://github.com/type-rb/type-rb/tree/main/examples/tutorials/web-orm-jobs).
Repository tests run the request and worker flow in all three modes.

The application has this flow:

```text
POST /reports
  -> Context#bind
  -> create_report
  -> application database transaction commits a pending report
  -> GenerateReportJob is enqueued
  -> 202 Accepted

trb jobs start --once
  -> GenerateReportJob#perform
  -> report status becomes ready

GET /reports/:id
  -> returns the current report
```

## Prerequisites

Install `trb`, the toolchain for the selected backend, and the `sqlite3`
command-line program. The checked-in project defaults to Go mode.

Copy the example or work in its directory:

```sh
cd examples/tutorials/web-orm-jobs
mkdir -p tmp
```

## Configure the project and database

The project selects SQLite for ORM access, reads the application database from
`DATABASE_URL`, and points the Jobs compiler integration at `config/jobs`.

`trbconfig.jsonc`:

<!-- trb-doc-file: examples/tutorials/web-orm-jobs/trbconfig.jsonc -->
```jsonc
{
  "name": "report-api",
  "mode": "go",
  "sourceDir": "src",
  "packageOptions": {
    "trb/orm": {
      "adapter": "sqlite",
      "database": {
        "environment": "DATABASE_URL"
      }
    }
  },
  "db": {
    "adapter": "sqlite",
    "database": {
      "environment": "DATABASE_URL"
    },
    "schema": "db/schema.sql",
    "lock": "db/schema.lock.json"
  },
  "jobs": {
    "configuration": "config/jobs"
  },
  "go": {
    "module": "example.com/report-api"
  }
}
```

`db/schema.sql`:

<!-- trb-doc-file: examples/tutorials/web-orm-jobs/db/schema.sql -->
```sql
CREATE TABLE reports (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  status TEXT NOT NULL
);
```

The example also commits the generated `db/schema.lock.json`. Check that the
lock still describes the SQL schema, then initialize a local database:

```sh
trb db check
sqlite3 tmp/application.sqlite3 < db/schema.sql
export DATABASE_URL="$PWD/tmp/application.sqlite3"
export JOBS_DATABASE_URL="$PWD/tmp/jobs.sqlite3"
```

The two environment variables deliberately name different databases. The
application owns `reports`; the SQL Jobs adapter owns its queue tables.

## Configure the queue

The composition module selects the SQLite Jobs adapter. Other application
modules import only `trb/jobs`, so storage selection remains at this boundary.

`src/config/jobs.trb`:

<!-- trb-doc-file: examples/tutorials/web-orm-jobs/src/config/jobs.trb -->
```trb
import { JobAdapter } from trb/jobs
import { SQLAdapter, SQLDialect } from trb/jobs/sql

JOBS_ADAPTER: JobAdapter := SQLAdapter.new(
	dialect: SQLDialect::SQLite,
	source_environment: "JOBS_DATABASE_URL",
)
```

## Define the model and Job

The schema lock supplies the generated fields on the otherwise empty model.

`src/models/report.trb`:

<!-- trb-doc-file: examples/tutorials/web-orm-jobs/src/models/report.trb -->
```trb
import { Model } from trb/orm

class Report < Model
end
```

The Job loads the report, produces an immutable changed value with `with`, and
persists it. It maps ORM failures to `JobError`, which tells the worker to
apply its retry policy.

`src/jobs/generate_report_job.trb`:

<!-- trb-doc-file: examples/tutorials/web-orm-jobs/src/jobs/generate_report_job.trb -->
```trb
import { Report } from models/report
import { Job, JobError, JobResult, maximum_attempts, queue } from trb/jobs
import { Unit } from trb/std/unit

class GenerateReportJob < Job
	queue("reports")
	maximum_attempts(3)

	def perform(report_id: Integer): JobResult
		report := Report.find(report_id) catch |error|
			return JobResult::Err(JobError.new(message: "report lookup failed: " + error.message))
		end
		_saved := report.with(status: "ready").save() catch |error|
			return JobResult::Err(JobError.new(message: "report update failed: " + error.message))
		end
		return JobResult::Ok(Unit.new())
	end
end
```

Jobs use at-least-once delivery. Real work should therefore be idempotent; this
example is safe to repeat because setting `status` to `ready` has the same
result each time.

## Put the transaction in an application service

The service owns the use case and its error vocabulary. The route does not
need to know how the report is stored or how the Job is persisted.

`src/services/create_report.trb`:

<!-- trb-doc-file: examples/tutorials/web-orm-jobs/src/services/create_report.trb -->
```trb
import { GenerateReportJob } from jobs/generate_report_job
import { Report } from models/report
import { EnqueueError } from trb/jobs
import { Database, DbError } from trb/orm
import { Result } from trb/std/result

enum CreateReportError
	Database(error: DbError)
	Queue(error: EnqueueError)
end

record AcceptedReport
	id: Integer
	job_id: String
	status: String
end

def create_report(title: String): Result<AcceptedReport, CreateReportError>
	report := Database.transaction() do |transaction|
		reports := Report.using(transaction)
		created := try reports.create(title: title, status: "pending")
		created
	end catch |error|
		return Result<AcceptedReport, CreateReportError>::Err(CreateReportError::Database(error))
	end

	reference := GenerateReportJob.perform_later(report.id) catch |error|
		return Result<AcceptedReport, CreateReportError>::Err(CreateReportError::Queue(error))
	end
	return Result<AcceptedReport, CreateReportError>::Ok(
		AcceptedReport.new(id: report.id, job_id: reference.id, status: report.status),
	)
end
```

`try` propagates `DbError` inside the transaction callback. The postfix
`catch` translates the transaction and enqueue failures into application
errors at the two boundaries where their meaning changes.

The enqueue intentionally happens after the application transaction commits.
The initial Jobs contract does not provide a transaction spanning the
application database and queue storage. If enqueueing fails, this example
returns `503` while leaving a pending report behind. A production application
that must close that gap needs an explicit retry/reconciliation process or a
transactional outbox; moving `perform_later` inside this transaction would not
make the two stores atomic.

## Map errors once at the HTTP boundary

The shared mapper converts input, database, and queue errors into HTTP
responses. Both routes reuse it instead of repeating transport policy.

`src/http/errors.trb`:

<!-- trb-doc-file: examples/tutorials/web-orm-jobs/src/http/errors.trb -->
```trb
import { CreateReportError } from services/create_report
import { DbError } from trb/orm
import { EndpointInputError, Response, json } from trb/web

record ErrorResponse
	code: String
	message: String
end

def endpoint_input_error_response(error: EndpointInputError): Response
	case error
	when EndpointInputError::Params(_error)
		return json(ErrorResponse.new(code: "invalid_path", message: "path parameters are invalid"), 400)
	when EndpointInputError::Query(_error)
		return json(ErrorResponse.new(code: "invalid_query", message: "query parameters are invalid"), 400)
	when EndpointInputError::Body(_error)
		return json(ErrorResponse.new(code: "invalid_body", message: "request body is invalid"), 400)
	end
end

def create_report_error_response(error: CreateReportError): Response
	case error
	when CreateReportError::Database(database_error)
		return database_error_response(database_error)
	when CreateReportError::Queue(queue_error)
		return json(ErrorResponse.new(code: "queue_unavailable", message: queue_error.message), 503)
	end
end

def database_error_response(error: DbError): Response
	return json(ErrorResponse.new(code: "database_error", message: error.message), 500)
end
```

## Add file-based routes

`Context#bind<T>()` decodes the request sections named by the endpoint contract
and returns `Result<T, EndpointInputError>`. The POST route binds a body; the
dynamic GET route binds the `:id` path parameter.

`src/routes/reports.trb`:

<!-- trb-doc-file: examples/tutorials/web-orm-jobs/src/routes/reports.trb -->
```trb
import { create_report_error_response, endpoint_input_error_response } from http/errors
import { create_report } from services/create_report
import { Context, Response, json } from trb/web

record CreateReportBody
	title: String
end

record CreateReportEndpointInput
	body: CreateReportBody
end

def post(context: Context): Response
	input := context.bind<CreateReportEndpointInput>() catch |error|
		return endpoint_input_error_response(error)
	end
	accepted := create_report(input.body.title) catch |error|
		return create_report_error_response(error)
	end
	return json(accepted, 202)
end
```

`src/routes/reports/[id].trb`:

<!-- trb-doc-file: examples/tutorials/web-orm-jobs/src/routes/reports/[id].trb -->
```trb
import { database_error_response, endpoint_input_error_response } from http/errors
import { Report } from models/report
import { Context, Response, json } from trb/web

record ReportParams
	id: Integer
end

record ShowReportEndpointInput
	params: ReportParams
end

record ReportResponse
	id: Integer
	status: String
	title: String
end

def get(context: Context): Response
	input := context.bind<ShowReportEndpointInput>() catch |error|
		return endpoint_input_error_response(error)
	end
	report := Report.find(input.params.id) catch |error|
		return database_error_response(error)
	end
	return json(ReportResponse.new(id: report.id, status: report.status, title: report.title))
end
```

The static `/reports` route and dynamic `/reports/:id` route coexist; static
segments take precedence when the router builds the file-routing table.

## Run the server and worker

The runnable root only starts the generated Web server.

`src/main.trb`:

<!-- trb-doc-file: examples/tutorials/web-orm-jobs/src/main.trb -->
```trb
import { configure_server, serve } from trb/web

def main()
	serve(configure_server(host: "127.0.0.1", port: 3000))
end
```

Check the complete project, then start it:

```sh
trb check
trb lint
trb run
```

From another terminal with the same environment variables, create a report:

```sh
curl -i \
  -H 'content-type: application/json' \
  -d '{"title":"August report"}' \
  http://127.0.0.1:3000/reports
```

The response is `202 Accepted` and contains an application record ID, a queue
Job ID, and `"status":"pending"`. Before the worker runs, this returns the
pending report:

```sh
curl http://127.0.0.1:3000/reports/1
```

Process one report Job, then repeat the GET request:

```sh
trb jobs start --once --queue reports
curl http://127.0.0.1:3000/reports/1
```

The status is now `ready`.

## Test the slice without a network server

`trb/web/testing` dispatches a request through the generated router in-process.

`src/report_api_test.trb`:

<!-- trb-doc-file: examples/tutorials/web-orm-jobs/src/report_api_test.trb -->
```trb
import { Body, Header, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing
import { describe, expect, test } from trb/std/test

describe("Report API") do
	test("accepts a report for background processing") do
		request := Request.new(
			method: HttpMethod.post(),
			path: "/reports",
			query_string: "",
			headers: Headers.new([Header.new(name: "content-type", value: "application/json")]),
			body: Body.new("{\"title\":\"August report\"}".to_bytes()),
		)
		response := dispatch(request)
		expect(response.status).to_equal(202)

		show_request := Request.new(
			method: HttpMethod.get(),
			path: "/reports/1",
			query_string: "",
			headers: Headers.new([]),
			body: Body.empty(),
		)
		show_response := dispatch(show_request)
		expect(show_response.status).to_equal(200)
	end
end
```

Run it against fresh local databases:

```sh
trb test
trb jobs start --once --queue reports
```

The repository's integration test goes further: for each of Go, Ruby, and
TypeScript/Bun, it creates fresh databases, runs this request test, checks the
pending row and queued Job, runs one worker, and checks the ready row and empty
queue.

## Switch backends

All files under `src/` and `db/` stay unchanged. Select one target in
`trbconfig.jsonc`, replace the target-specific section, then synchronize its
native manifest. For Ruby, use a `ruby` section and the `require_relative`
loader. For TypeScript server execution, select Bun:

```jsonc
{
  "mode": "typescript",
  "devDependencies": {
    "typescript": "^6.0.0"
  },
  "typescript": {
    "runtime": "bun",
    "packageManager": "bun",
    "moduleType": "module"
  }
}
```

Run `trb sync` and `trb install`, then repeat the same database, test, server,
and worker commands. See
[Project configuration](../configuration.md) for the complete Go, Ruby, and
TypeScript target settings.

Continue with the focused [Web](../guides/web.md),
[ORM](../guides/orm.md), [Jobs](../guides/jobs.md), and
[testing](../guides/testing.md) guides when extending this slice.
