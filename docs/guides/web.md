# Portable Web Applications

`trb/web` provides compile-time file-based routing and the same request,
response, middleware, and server behavior in Go, Ruby, and TypeScript modes.
Create a project with:

```sh
trb init --template web
trb run
```

Route files live below `src/routes`. File names determine paths, and exported
functions determine HTTP methods. For example,
`src/routes/todos/[id].trb` can contain:

```trb
import { Context, Response, json } from trb/web
import { Result } from trb/std/result

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
		return json(Todo.new(id: id, title: input.title), 201)
	when Result::Err(_error)
		return json({"error" => "invalid_request"}, 400)
	end
end
```

`Request`, `Response`, and `Context` are immutable classes. Request methods
handle query parameters, headers, cookies, text, bytes, and typed JSON. Response
methods change status, headers, `Vary`, and cookies by returning a new response:

```trb
response := json({"ok" => true})
	.with_status(202)
	.with_header("cache-control", "no-store")
	.vary("accept")
```

Path and query values can be bound to records without giving the target
runtime permission to reflect over application types:

```trb
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
		return text("invalid path", 400)
	when Result::Ok(params)
		case context.request.query<TodoQuery>()
		when Result::Err(_error)
			return text("invalid query", 400)
		when Result::Ok(query)
			return text(params.id.to_s() + ":" + query.tag.size().to_s())
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

An endpoint can combine those explicit bindings into one optional input
contract. The contract is an ordinary record whose supported fields are
`params`, `query`, and `body`; each field may be omitted when the endpoint does
not use that input source. `params` and `query` name binding records, while
`body` names the typed JSON value:

```trb
import { Context, EndpointInputError, Response, text } from trb/web
import { Result } from trb/std/result

record TodoInput
	params: TodoParams
	query: TodoQuery
	body: UpdateTodo
end

def invalid_input(_error: EndpointInputError): Response
	return text("invalid request", 400)
end

def post(context: Context): Response
	case context.bind<TodoInput>()
	when Result::Ok(input)
		return text(input.params.id.to_s() + ":" + input.body.title)
	when Result::Err(error)
		return invalid_input(error)
	end
end
```

`bind<T>()` checks path parameters, query parameters, then the JSON body and
returns the first failure as `EndpointInputError::Params`, `Query`, or `Body`.
Each variant preserves the original `ParameterError` or `RequestError`, so the
application still chooses its validation response and logging policy. The
compiler validates a contract's `params` record against the file-based route
in the same way as a direct `params<T>()` call. Contracts are optional;
handlers may continue to use the individual request methods.

Middleware can attach request-scoped values without a string-keyed cast at
the handler boundary. Create one `ContextKey<T>` and share that key between
the producer and consumer:

```trb
import { Context, ContextKey, Next, Response, text } from trb/web
import { Result } from trb/std/result

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
		return text(user.name)
	when Result::Err(_error)
		return text("unauthorized", 401)
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
construct a `Request` from the shared [`trb/http`](http.md) values.

Response compression is opt-in middleware:

```trb
import { Context, Next, Response } from trb/web
import trb/web/middleware/compression

def call(context: Context, next_handler: Next): Response
	return compression.call(context, next_handler)
end
```

The default compresses eligible responses of at least 1 KiB with gzip when the
request's `Accept-Encoding` allows it. `CompressionOptions` can change the
minimum size. The shared implementation respects quality values,
`Cache-Control: no-transform`, partial and bodyless responses, existing content
encodings, and compressible media types. It also maintains `Vary` and removes
representation metadata invalidated by compression.

`text`, `bytes`, `json`, `empty`, and `redirect` remain the standard response
builders. Server host, port, body limit, and shutdown timeout are configured
with `configure_server`.
