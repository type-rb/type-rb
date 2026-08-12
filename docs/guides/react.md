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

Browser routing is an optional React Router integration:

```trb
import { ReactEvent, ReactNode } from trb/platform/typescript/react
import { browser_router, link_to, route, route_param, use_navigate } from trb/platform/typescript/react/router

def Home(): ReactNode
	return <main>{link_to("/todos/42", <span>Open todo</span>)}</main>
end

def TodoPage(): ReactNode
	id := route_param("id")
	navigate := use_navigate()
	go_home := fn(_event: ReactEvent)
		navigate("/")
		return
	end
	return <main><p>Todo {id}</p><button onClick={go_home}>Home</button></main>
end

def App(): ReactNode
	return browser_router([
		route("/", <Home />),
		route("/todos/:id", <TodoPage />),
	])
end
```

The generated application uses `react-router-dom`; `route_param(name)` returns
`String?`, and `use_navigate()` returns a typed `(String) -> Void` function.
Components without props accept no parameter and can be rendered as
`<Component />`.

Typed form state keeps the application value and validation errors as separate
records:

```trb
import { ReactEvent, ReactNode, input_value, prevent_default } from trb/platform/typescript/react
import { use_form } from trb/platform/typescript/react/form

record ProfileDraft
	name: String
end

record ProfileErrors
	name: String?
end

def ProfileForm(): ReactNode
	form := use_form(ProfileDraft.new(name: ""), ProfileErrors.new(name: nil))
	update_name := fn(event: ReactEvent)
		form.set_value(ProfileDraft.new(name: input_value(event)))
		return
	end
	submit := fn(event: ReactEvent)
		prevent_default(event)
		if form.value.name.empty?()
			form.set_errors(ProfileErrors.new(name: "Name is required"))
		else
			form.clear_errors()
		end
		return
	end
	return <form onSubmit={submit}>
		<input value={form.value.name} onChange={update_name} />
		<p>{form.errors.name}</p>
	</form>
end
```

`ReactForm<Value, Errors>` infers both record types. It also exposes `dirty`
and `submitting` flags, typed setters, `clear_errors()`, and `reset()`. Validation
policy stays in ordinary TypeRB functions, so domain constraints and
UI-specific errors can evolve independently of React.

Browser suspension remains a TypeScript backend concern; TypeRB does not add
`async` syntax. Generated TSX and package dependencies are compatible with the
normal React and Vite ecosystem.

Applications can choose either an encrypted server session or browser bearer
tokens without changing their shared principal contract. See
[Experimental OIDC authentication](authentication.md) for both profiles.
