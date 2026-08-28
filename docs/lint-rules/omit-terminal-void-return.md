# `trb/omit-terminal-void-return`

Omit a final bare `return` from a function or method with no return value.

- Default: `warning`
- Recommended: yes
- First available: TypeRB 0.3.48
- Safe fix: yes, for the cases reported by the initial rule

## Why

TypeRB permits both fallthrough and a bare `return` in a Void function. A bare
`return` remains useful for an early exit, but writing one immediately before
the callable's closing `end` adds no control-flow information. Omitting it is
the single recommended terminal style.

## Examples

The rule reports:

```trb
def print_name(name: String)
	puts(name)
	return
end
```

Use:

```trb
def print_name(name: String)
	puts(name)
end
```

An early return remains unchanged:

```trb
def print_name(name: String?)
	return if name == nil
	puts(name)
end
```

The same rule applies to Void `fn` values.

## Fix boundary

The initial rule reports only a direct final bare `return` that occupies its
own line. The safe fix removes that complete line while retaining comments and
blank lines before the callable's closing `end`.

A return with a value, an early return, a conditional return, a compact
semicolon form, and a return carrying a trailing comment remain valid and are
left unchanged. The compiler continues to accept a final bare `return` even
when this lint rule is disabled.
