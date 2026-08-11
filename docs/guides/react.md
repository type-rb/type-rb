# Experimental React and JSX

TypeRB can emit ordinary TSX for existing React tools. This browser integration
is experimental and belongs to the application-ready prototype line rather
than the stable language surface.

Import the explicit React platform package and use a record for component
props:

```trb
import { ReactNode } from trb/platform/typescript/react

record GreetingProps
	name: String
end

def Greeting(props: GreetingProps): ReactNode
	return <p>Hello, {props.name}</p>
end
```

The compiler checks required, unknown, and mistyped props, including components
imported from another TypeRB module. Files containing JSX are generated as
`.tsx`; other TypeScript modules remain `.ts`.

Typed browser JSON calls use ordinary `fails` and `attempt` semantics:

```trb
import { FetchError, get_json, patch_json, post_json } from trb/platform/typescript/browser

record Todo
	id: Integer
	title: String
	completed: Boolean
end

def load_todos(): Array<Todo> fails FetchError
	return get_json<Array<Todo>>("/api/todos")
end

def save_todo(todo: Todo): Todo fails FetchError
	if todo.id == 0
		return post_json<Todo>("/api/todos", todo)
	end
	return patch_json<Todo>("/api/todos/" + todo.id.to_s(), todo)
end
```

`get_json<T>`, `post_json<T>`, `put_json<T>`, `patch_json<T>`, and
`delete_json<T>` encode request values and validate response JSON against the
same TypeRB type. HTTP, network, and invalid-response failures remain explicit
through `FetchError`.

Browser suspension remains a TypeScript backend concern; TypeRB does not add
`async` syntax. Generated TSX and package dependencies are compatible with the
normal React and Vite ecosystem.
