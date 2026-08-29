# Browser HTTP Client

`trb/platform/typescript/browser` is the official low-level HTTP client for
TypeScript browser applications. It keeps target-specific Fetch behavior in an
explicit platform package while exposing checked TypeRB request, response,
body, and error types.

Construct one `HttpClient` at the application boundary and pass or export the
higher-level API objects that use it:

```trb
import {
	HttpClient,
	RequestError,
	Response,
	json_body,
} from trb/platform/typescript/browser
import { Header, Headers, HttpMethod } from trb/http
import trb/std/result
import trb/std/url

record Todo
	id: Integer
	title: String
end

record CreateTodoInput
	title: String
end

def fetch_todo(client: HttpClient, id: Integer): Result<Response<Todo>, RequestError>
	raw := try client.request(
		"/todos",
		query: [URL::QueryParameter.new(name: "id", value: id.to_s())],
		headers: Headers.new([Header.new(name: "accept", value: "application/json")]),
		timeout_milliseconds: 2000,
	)
	return raw.json<Todo>()
end

def create_todo(client: HttpClient, input: CreateTodoInput): Result<Response<Todo>, RequestError>
	body := try json_body(input)
	raw := try client.request("/todos", method: HttpMethod.post(), body: body)
	return raw.json<Todo>()
end
```

TypeRB source does not add `async`. The TypeScript backend identifies the
suspending Fetch operation and generates the required `Promise` and `await`
boundaries.

## Response model

`request()` returns `Result<Response<Body>, RequestError>`. Prefix `try`
propagates an error from a Result-returning function and yields the response on
success. The response buffers its body once and preserves the status, final
URL, ordered headers, and bytes. Decode it explicitly when no endpoint contract
is available:

- `response.json<T>()` validates JSON against `T` and returns
  `Result<Response<T>, RequestError>`.
- `response.text()` returns `Response<String>`.
- `response.bytes()` returns `Response<Bytes>`.
- `response.no_body()` validates an empty body and returns
  `Result<Response<NoBody>, RequestError>`.

`response.headers.first(name)` performs case-insensitive lookup,
`response.headers.values(name)` preserves repeated values, and
`response.headers.entries()` exposes the ordered header list. These types come
from portable `trb/http`, so server and browser packages share the same HTTP
value model.

Declared HTTP statuses, including non-2xx statuses, remain ordinary responses.
They are not converted into transport errors. Literal status fields and union
narrowing can express a contract response such as
`CreatedResponse | InvalidResponse`, then narrow the complete value with
`case response.status`.

## Errors and bodies

Fallible operations return `Result` with one `RequestError` type. Its `kind` is `Network`,
`Timeout`, `Abort`, or `Contract`. A JSON or empty-body contract failure keeps
the original `Response<Body>` in `error.response`, so diagnostics and explicit
fallback handling do not lose the status, headers, or body.

Use `RequestBody::Text`, `RequestBody::Bytes`, or `RequestBody::Form` for those
wire formats. `RequestBody::File` sends a browser `File` as the request body and
uses its media type as the default `Content-Type`; explicitly supplied headers
still take precedence. The platform `File` type exposes checked `name`, `size`,
`type`, and `lastModified` fields and can cross a supported native component
callback boundary. `json_body(value)` runs the checked JSON encoder and
produces a JSON request body. Query parameters and headers are ordered arrays,
so repeated names have an unambiguous representation.

The initial client is buffered and intentionally omits streaming, retry policy,
progress events, and interceptors. Contractless external APIs and generated
endpoint clients use the same transport and `Response` model.

## Generated clients for trb/web

When the server publishes explicit `trb/web` endpoint contracts, generate a
higher-level client instead of repeating path, query, body, and status handling:

```sh
trb web client --output generated/api_client.trb --name ApiClient
```

The generated TypeRB class accepts an ordinary `HttpClient`, so base URL and
application composition remain explicit:

```trb
import generated/api_client
import { HttpClient } from trb/platform/typescript/browser

api := ApiClient.new(HttpClient.new("https://api.example.com"))
```

Each method returns an endpoint-specific status enum inside `Result`, rather
than treating every non-2xx response as a transport error. Contract types stay
in an authored shared module imported by both route and browser projects. The
generator copies no data model, runs no target toolchain, and adds no retry,
authentication, caching, or global error policy. Those concerns compose around
the generated class or the underlying `HttpClient`.
