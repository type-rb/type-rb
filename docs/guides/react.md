# React and JSX

TypeRB can emit ordinary TSX for existing React tools. This browser integration
is an initial alpha feature and may evolve before TypeRB reaches 1.0.

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

Mount a component with no props at the browser entry point:

```trb
import { ReactNode, mount } from trb/platform/typescript/react

def App(): ReactNode
	return <main>TypeRB</main>
end

def main()
	mount(<App />, "root")
	return
end
```

The compiler checks required, unknown, and mistyped props, including components
imported from another TypeRB module. Files containing JSX are generated as
`.tsx`; other TypeScript modules remain `.ts`.

Generated TSX and managed React package dependencies are compatible with the
normal React and Vite ecosystem. Use the separate
[browser HTTP client](browser-http.md) for typed requests; suspension remains
a TypeScript backend concern and does not add `async` syntax to TypeRB.
