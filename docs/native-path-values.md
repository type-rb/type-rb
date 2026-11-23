# Native path values

`trb/std/path` supplies nominal path values to ordinary Native programs and the
REPL. Import `Path`, `RelativePath` and `RelativePathError` by name as needed.

```trb
import { Path, RelativePath } from trb/std/path

def main()
	child := RelativePath.parse("images/icon.svg") catch |error|
		puts(error.to_s())
		return
	end
	puts(Path.new("./public").join(child).to_s())
end
```

`Path.new(text)` preserves its String without validation or normalization.
`value()` and `to_s()` return that text. `Path#join` accepts a `RelativePath`:
an empty parent yields the child; a parent ending in `/` appends the child
directly; other parents receive one `/` between parent and child. Existing dot
components, repeated separators, Unicode and other parent bytes remain intact.
This is the host grammar of the supported Darwin and Linux Native targets.

`RelativePath.parse(text)` returns `Result<RelativePath, RelativePathError>`.
The constructor is private. Validation rejects an empty path, empty components,
`.` and `..` components, NUL, colon and backslash. Leading/trailing slash and
double slash produce `EmptyComponent` before component validation. Components
are otherwise checked in order. `child(name)` rejects slash with
`MultipleComponents` before applying the same validation to the single name.

`RelativePath#join` composes two validated descendants. `parent()` returns the
preceding components, or `nil` for a single component. `value()` and `to_s()`
preserve the validated text, including Unicode without normalization.
`RelativePathError#to_s` exposes the portable error messages.

These operations do not inspect files, resolve symlinks or establish filesystem
access authority. File and directory resource APIs require their own providers
and lifecycle checks. Source-backend execution and other target platforms remain
separate coverage work.
