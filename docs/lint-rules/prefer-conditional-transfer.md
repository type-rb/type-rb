# `trb/prefer-conditional-transfer`

Prefer a conditional control-transfer statement when a simple `if` guard
contains exactly one `return`, `break`, or `next`.

- Default: `warning`
- Recommended: yes
- First available: TypeRB 0.3.25
- Safe fix: yes, for the cases reported by the initial rule

## Why

Guard clauses keep the main path at the surrounding indentation level. The
conditional-transfer syntax expresses that shape directly without adding a
general Ruby-style modifier `if` to calls or assignments.

## Examples

The rule reports:

```trb
if cached != nil
	return cached
end
```

Use:

```trb
return cached if cached != nil
```

The same rule applies inside a loop:

```trb
next if item.hidden?()
break if complete
```

## Fix boundary

The initial rule reports only a block that can be rewritten without moving or
discarding comments, joining multiline expressions, or producing a line over
120 display columns. Tabs and Unicode display width are included. The edit
replaces the complete `if`/`end` block, preserves
the existing leading indentation, and retains the original transfer and
condition text. More complex guards remain valid and are left unchanged.

The 120-column boundary is a conservative limit on when this rule may
collapse a block into one line. It is not a general maximum line length for
TypeRB source.

This rule does not report `if` with `else` or `elsif`, multiple body
statements, value-producing `if`, or an already-authored conditional transfer.
