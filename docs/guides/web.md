# Portable Web Applications

`trb/web` provides compile-time file-based routing and the same request,
response, middleware, and server behavior in Go, Ruby, and TypeScript modes.
Create a Go project with:

```sh
trb init --mode go --module example.com/api --template web api
cd api
trb run
```

The generated server listens on `http://localhost:3000`. Open that URL to see
`{"message":"Hello, TypeRB!"}`, then press Ctrl-C to stop the server. The same
template supports Ruby and TypeScript when their corresponding mode and
toolchain are selected instead; one project needs only its selected target
toolchain.

Route files live below `src/routes`. File names determine paths, and exported
functions determine HTTP methods. For example,
`src/routes/todos/[id].trb` can contain:

<!-- trb-doc-test: web-update-todo -->
```trb
import { Context, Response } from trb/web
import trb/std/result

record UpdateTodo
	title: String
end

record Todo
	id: String
	title: String
end

def post(context: Context): Response
	id := context.path_value("id")
	case context.request.json<UpdateTodo>()
	when Result::Ok(input)
		return Response.json(Todo.new(id: id, title: input.title), 201)
	when Result::Err(_error)
		return Response.json({"error" => "invalid_request"}, 400)
	end
end
```

Static route segments take precedence over parameter segments. For example,
`src/routes/todos/new.trb` and `src/routes/todos/[id].trb` can coexist:
`/todos/new` selects the static route, while `/todos/42` binds `id` in the
parameter route. Routing selects the most specific path before its HTTP
handler, so an unsupported method on `/todos/new` returns 405 instead of
falling through to `[id].trb`. Patterns that cannot be ordered consistently,
such as sibling `[id].trb` and `[slug].trb` files, remain build errors.

`Request`, `Response`, and `Context` are immutable classes. Request methods
handle query parameters, headers, cookies, text, bytes, and typed JSON. Response
methods change status, headers, `Vary`, and cookies by returning a new response:

```trb
response := Response.json({"ok" => true})
	.with_status(202)
	.with_header("cache-control", "no-store")
	.vary("accept")
```

Path and query values can be bound to records without giving the target
runtime permission to reflect over application types:

```trb
import { Context, Response } from trb/web
import trb/std/result

record TodoParams
	id: Integer
end

record TodoQuery
	page: Integer?
	tag: Array<String>
end

def get(context: Context): Response
	case context.params<TodoParams>()
	when Result::Err(_error)
		return Response.text("invalid path", 400)
	when Result::Ok(params)
		case context.request.query<TodoQuery>()
		when Result::Err(_error)
			return Response.text("invalid query", 400)
		when Result::Ok(query)
			return Response.text(params.id.to_s() + ":" + query.tag.size().to_s())
		end
	end
end
```

Record field names are wire names. A path record must contain exactly the
parameters declared by its route file, which is checked during the build.
Query scalars accept one value, nullable fields use `nil` when missing, and
arrays preserve repeated keys and use an empty array when missing. Unknown
query keys are ignored. Boolean, numeric, raw-value enum, and date/time fields
use their portable parsers. Malformed encoding, missing or duplicate scalar
values, and invalid conversions return `ParameterError`; applications retain
control over the corresponding error response.

An endpoint can combine those explicit bindings into one optional endpoint
input record. The record's supported fields are
`params`, `query`, and `body`; each field may be omitted when the endpoint does
not use that input source. `params` and `query` name binding records, while
`body` names the typed JSON value:

```trb
import { Context, EndpointInputError, Response } from trb/web

record TodoInput
	params: TodoParams
	query: TodoQuery
	body: UpdateTodo
end

def invalid_input(error: EndpointInputError): Response
	case error
	when EndpointInputError::Params(_error)
		return Response.text("invalid path", 400)
	when EndpointInputError::Query(_error)
		return Response.text("invalid query", 400)
	when EndpointInputError::Body(_error)
		return Response.text("invalid body", 400)
	end
end

def post(context: Context): Response
	input := context.bind<TodoInput>() catch |error|
		return invalid_input(error)
	end
	return Response.text(input.params.id.to_s() + ":" + input.body.title)
end
```

`bind<T>()` checks path parameters, query parameters, then the JSON body and
returns the first failure as `EndpointInputError::Params`, `Query`, or `Body`.
Each variant preserves the original `ParameterError` or `RequestError`, so the
application still chooses its validation response and logging policy through
an ordinary mapper function; `trb/web` does not install a global error mapper
or choose an application response body. The
compiler validates an endpoint input record's `params` field against the
file-based route in the same way as a direct `params<T>()` call. Combined
input records are optional; handlers may continue to use the individual
request methods. `bind<T>()` is most useful when one handler consumes more than
one input source, or when the application wants a named endpoint input. A
handler that reads only path, query, or body input does not need a wrapper
record merely for uniformity.

File routes may also publish an optional typed endpoint contract for tooling.
The route file remains the source of the HTTP method and path. A contract class
in that same module connects the route handler to its input and status-specific
response types:

```trb
import { Context, Endpoint, Response, handles, input, response } from trb/web

record CreateTodoBody
	title: String
end

record CreateTodoInput
	body: CreateTodoBody
end

record CreateTodoResponse
	id: Integer
	title: String
end

record ErrorResponse
	message: String
end

def post(context: Context): Response
	request := context.bind<CreateTodoInput>() catch |_error|
		return Response.json(ErrorResponse.new(message: "invalid request"), 400)
	end
	return Response.json(CreateTodoResponse.new(id: 42, title: request.body.title), 202)
end

class CreateTodoEndpoint < Endpoint
	handles(post)
	input<CreateTodoInput>()
	response<CreateTodoResponse>(status: 202)
	response<ErrorResponse>(status: 400)
end
```

`handles` accepts the actual top-level function rather than its name as a
string, so the ordinary `(Context) -> Response` signature is checked. A
contract directly inherits `Endpoint`, declares exactly one local file-route
handler, may declare one `input<T>()`, and declares one or more unique literal
HTTP statuses from 100 through 599. These calls are compile-time declarations:
Go, Ruby, and TypeScript output does not execute them. The resulting versioned
endpoint catalog retains portable type identities for downstream tooling.

The contract does not call `bind<T>()`, validate request data, infer schemas
from the handler body, or prove that every runtime response has the documented
status and payload. Those remain explicit application behavior. Routes and
handlers without a contract continue to compile; contracts are useful when an
application wants generated descriptions or clients without making that
tooling mandatory for every route.

Generate an OpenAPI 3.1 JSON document from the contracts in the configured
project with:

```sh
# Write deterministic JSON to standard output.
trb web openapi

# Write below the project root and override document metadata.
trb web openapi \
  --output api/openapi.json \
  --title "Todo API" \
  --api-version 2026-08
```

The project `name` and `version` are the default OpenAPI title and API version.
Generation compiles and checks the project but does not start or invoke the
selected Go, Ruby, or TypeScript toolchain. The same endpoint contracts
therefore produce the same document in every mode.

An endpoint input remains the `Context#bind<T>()` envelope rather than a wire
object. Its `params` fields become required OpenAPI path parameters, `query`
fields become query parameters, and `body` becomes a required
`application/json` request body. Path fields must match the dynamic file-route
segments exactly. A non-nullable query scalar is required; a nullable scalar
is optional; and an Array is a repeated optional query parameter because a
missing value binds to an empty Array. JSON `@json` names apply to body and
response records, not URL parameters.

The initial schema generator supports Boolean, portable Integer, Float,
String, Array, `Hash<String, V>`, records, String- or Integer-backed raw enums,
transparent aliases, nominal newtypes, nullable values, and the portable time
types. It rejects unsupported JSON shapes, generic schemas, recursive records,
and catch-all routes with source-located diagnostics when this command is
invoked. Those OpenAPI-only restrictions do not make `trb check` fail.
`Unit` declares a response without content and is required for 1xx, 204, 205,
and 304 statuses.

The generator deliberately does not inspect handler bodies, infer undocumented
responses, run validation, or invent summaries, tags, authentication, or
application error schemas. Only routes with an explicit endpoint contract are
published.

Generate a checked browser client from the same endpoint catalog with:

```sh
# Write formatted TypeRB source to standard output.
trb web client

# Write below the project root and choose the exported class name.
trb web client \
  --output generated/todo_api_client.trb \
  --name TodoApiClient
```

The generated source targets `trb/platform/typescript/browser` and wraps its
existing `HttpClient`; it does not introduce another transport abstraction.
It serializes declared path, query, and JSON body input, decodes every declared
status into an endpoint-specific enum variant, and returns
`Result<EndpointResult, RequestError>`. Undeclared statuses are explicit
`RequestErrorKind::Contract` failures that retain the original response.
Callers can still supply headers and an optional timeout to every generated
method.

Client-visible user-defined input and response types must be imported into the
route from a shared module. The generated source imports those declarations
rather than copying records, enums, aliases, or newtypes. A route-local contract type
continues to work for endpoint checking and OpenAPI generation but is rejected
by `trb web client`, because a separate browser project cannot import that type
without also importing the route handler. The command produces identical
source when the server project is configured for Go, Ruby, or TypeScript; the
resulting client itself is intentionally browser-target-specific.

Middleware can attach request-scoped values without a string-keyed cast at
the handler boundary. Create one `ContextKey<T>` and share that key between
the producer and consumer:

```trb
import { Context, ContextKey, Next, Response } from trb/web
import trb/std/result

record CurrentUser
	id: Integer
	name: String
end

CURRENT_USER := ContextKey<CurrentUser>.new("current_user")

def authenticate(context: Context, next_handler: Next): Response
	user := CurrentUser.new(id: 42, name: "Ada")
	return next_handler.call(context.with(CURRENT_USER, user))
end

def get(context: Context): Response
	case context.fetch(CURRENT_USER)
	when Result::Ok(user)
		return Response.text(user.name)
	when Result::Err(_error)
		return Response.text("unauthorized", 401)
	end
end
```

The key supplies the value type to both operations, so `with` rejects a value
of another type and `fetch` infers `Result<CurrentUser, ContextValueError>`.
Keys use instance identity: two keys with the same diagnostic name remain
independent. `Context` stays immutable; `with` returns a new context, replacing
the value only for that key, and `with_request` retains attached values.

Use `src/routes/_middleware.trb` for root middleware and nested
`_middleware.trb` files for route-scoped middleware. `trb/web/testing` exposes
the same dispatcher without opening a socket, so route and middleware tests can
construct a `Request` from the shared [`trb/http`](http.md) values. A request
test that imports `dispatch` belongs in a `*_test.trb` file at the configured
source root, which owns the complete file-route manifest. Tests inside a
subpackage should call that package's ordinary functions instead of importing
the project dispatcher.

Response compression is opt-in middleware:

```trb
import { Context, Next, Response } from trb/web
import trb/web/middleware/compression

def call(context: Context, next_handler: Next): Response
	return Compression.call(context, next_handler)
end
```

The default compresses eligible responses of at least 1 KiB with gzip when the
request's `Accept-Encoding` allows it. `CompressionOptions` can change the
minimum size. The shared implementation respects quality values,
`Cache-Control: no-transform`, partial and bodyless responses, existing content
encodings, and compressible media types. It also maintains `Vary` and removes
representation metadata invalidated by compression.

Request deadlines are also opt-in middleware:

```trb
import { Context, Next, Response } from trb/web
import trb/web/middleware/timeout
import { TimeoutOptions } from trb/web/middleware/timeout

OPTIONS := TimeoutOptions.new(milliseconds: 5000)

def call(context: Context, next_handler: Next): Response
	return Timeout.call(context, next_handler, OPTIONS)
end
```

The default deadline is 30 seconds. An expired request returns a portable JSON
504 response with `{"error":"gateway_timeout"}`. TypeRB keeps cancellation out
of application signatures: the compiler forwards a hidden execution scope to
downstream handlers, ORM operations, and browser HTTP requests. Generated loop
checkpoints stop TypeRB CPU work cooperatively. Native operations receive the
target runtime's cancellation signal when their API supports one and otherwise
observe cancellation at the next generated boundary.

`Response.text`, `Response.bytes`, `Response.json`, `Response.empty`, and
`Response.redirect` are the standard response builders. Pass a
`Web::ServerConfig` to `Web.serve` to set the host, port, body limit, or
shutdown timeout; omitted fields use the portable defaults.

```trb
import trb/web

def main()
	Web.serve(Web::ServerConfig.new(port: 8080))
	return
end
```

Construct response cookies directly with `ResponseCookie.new(name: ..., value:
...)`; omit `attributes` when no cookie attributes are needed.
