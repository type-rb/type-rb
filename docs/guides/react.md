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

Use a typed `fn` value for component callbacks and event handlers:

```trb
import { ReactEvent, ReactNode, prevent_default } from trb/platform/typescript/react

record ButtonProps
	on_click: (ReactEvent) -> Void
end

def Button(props: ButtonProps): ReactNode
	return <button onClick={props.on_click}>Save</button>
end

def Page(): ReactNode
	handle_click := fn(event: ReactEvent)
		prevent_default(event)
		return
	end
	return <Button on_click={handle_click} />
end
```

The callback keeps the ordinary TypeRB function type across module boundaries
and lowers to a React-compatible TypeScript function.

Component-local state uses a typed wrapper around React `useState`:

```trb
import { ReactEvent, ReactNode, use_state } from trb/platform/typescript/react

def Counter(): ReactNode
	count := use_state(0)
	increment := fn(_event: ReactEvent)
		count.set(count.value + 1)
		return
	end
	return <button onClick={increment}>Count: {count.value}</button>
end
```

`use_state(initial)` infers `ReactState<T>` from the initial value. Its `value`
property and `set(value)` method preserve that type, while generated TSX uses
an ordinary React hook. Calls therefore follow the normal Rules of Hooks: keep
them at the top level of a component or custom hook and do not place them in
conditional control flow.

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
