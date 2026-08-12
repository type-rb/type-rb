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
generic functions, classes, records, and discriminated result aliases for a
complex native package. Application code still imports the original npm
package and writes explicit TypeRB type arguments; it does not maintain a local
signature file. See the [package guide](packages.md).

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
