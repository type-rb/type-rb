# React and JSX

TypeRB can emit ordinary TSX for existing React tools. This browser integration
is an initial alpha feature and may evolve before TypeRB reaches 1.0.

## Run a first component

Create a browser TypeScript project:

```sh
trb init --mode typescript --runtime browser react-app
cd react-app
```

Create `main.trb`. Import the explicit React platform package and use a record
for component props:

<!-- trb-doc-test: react-first-component kind=program modes=typescript -->
```trb
import { ReactNode, mount } from trb/platform/typescript/react

record GreetingProps
	name: String
end

def Greeting(props: GreetingProps): ReactNode
	return <p>Hello, {props.name}</p>
end

def App(): ReactNode
	return <main><Greeting name="TypeRB" /></main>
end

def main()
	mount(<App />, "root")
	return
end
```

Create `index.html` for Vite:

```html
<!doctype html>
<html lang="en">
	<head>
		<meta charset="UTF-8" />
		<meta name="viewport" content="width=device-width, initial-scale=1.0" />
		<title>TypeRB React</title>
	</head>
	<body>
		<div id="root"></div>
		<script type="module" src="/build/main.tsx"></script>
	</body>
</html>
```

Add Vite, install the generated React dependencies, and build the TypeRB
source:

```sh
trb add --native --dev vite
trb install
trb fmt
trb check
trb build
npm exec vite
```

Open the URL printed by Vite, then press `Ctrl-C` to stop it. The default
TypeScript browser project uses npm and therefore requires Node.js. Set
`typescript.packageManager` to `"bun"` if the project uses Bun instead.

The compiler checks required, unknown, and mistyped props, including components
imported from another TypeRB module. Files containing JSX are generated as
`.tsx`; other TypeScript modules remain `.ts`.

## Component state

Component-local state uses a typed wrapper around React `useState`:

```trb
import { MouseEvent, ReactNode, use_state } from trb/platform/typescript/react

def Counter(): ReactNode
	count := use_state(0)
	increment := fn(_event: MouseEvent)
		count.set(count.value + 1)
		return
	end
	return <button onClick={increment}>Count: {count.value}</button>
end
```

`use_state(initial)` infers `ReactState<T>` from its initial value. `value` and
`set(value)` retain that type through checking and completion, while generated
TSX uses React's ordinary `useState`. The call follows the Rules of Hooks and
therefore stays at the top level of a component or custom hook.

## Native React packages

TypeScript projects can use representable named exports from npm packages
without application-owned TypeRB declarations:

```sh
trb add --native react-spinners ^0.17.0
trb install
```

```trb
import { ReactNode } from trb/platform/typescript/react
import { ClipLoader } from "react-spinners"

def Loading(): ReactNode
	return <ClipLoader color="#4f46e5" loading size={24} />
end
```

`trb install` reads the installed package's `.d.ts` declarations, including
dependency subpaths imported by project source, and writes an ignored
`.trb/native-types.json` index. Builds and editor completion read that index
without starting TypeScript. Index generation currently requires the supported
TypeScript 6.x toolchain. Runtime output imports the original npm package
unchanged.

Automatic `.d.ts` indexing supports ordinary functions and React components
whose signatures can be represented safely by TypeRB types. Unsupported
overloads, conditional types, `any`, and individual complex props produce a
diagnostic instead of becoming `Any`. A TypeRB package may provide declarative
generic functions, classes with explicit instance and class members, records,
and discriminated result aliases for a complex native package. Application
code still imports the original npm package and writes explicit TypeRB type
arguments; it does not maintain a local signature file. See the
[package guide](packages.md).

Event attributes use purpose-specific React types instead of one untyped event:

```trb
import { ChangeEvent, FormEvent, ReactNode } from trb/platform/typescript/react

def TodoForm(): ReactNode
	on_change := fn(event: ChangeEvent)
		puts(event.currentTarget.value)
		return
	end
	on_submit := fn(event: FormEvent)
		event.preventDefault()
		return
	end
	return <form onSubmit={on_submit}>
		<input onChange={on_change} />
	</form>
end
```

The initial event set includes `MouseEvent`, `ChangeEvent`, `FormEvent`, and
`KeyboardEvent`. React and DOM spellings such as `onClick`, `preventDefault`,
and `currentTarget` are preserved without automatic name conversion.

Generated TSX and managed React package dependencies are compatible with the
normal React and Vite ecosystem. Use the separate
[browser HTTP client](browser-http.md) for typed requests; suspension remains
a TypeScript backend concern and does not add `async` syntax to TypeRB.
