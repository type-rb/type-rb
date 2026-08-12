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

Use `src/routes/_middleware.trb` for root middleware and nested
`_middleware.trb` files for route-scoped middleware. `trb/web/testing` exposes
the same dispatcher without opening a socket, so route and middleware tests can
construct a `Request` from the shared [`trb/http`](http.md) values.

`text`, `bytes`, `json`, `empty`, and `redirect` remain the standard response
builders. Server host, port, body limit, and shutdown timeout are configured
with `configure_server`.
