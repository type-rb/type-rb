# 0009: Semantic record construction IR

## Context

TypeRB spells both class and record construction with `Type.new(...)`, as Ruby
does. Go emits record composite literals or compiler-owned default helpers,
Ruby can use `Data` construction or a helper, and TypeScript emits object
literals or generated functions. These are representation choices, not
different source semantics.

Record construction was previously lowered as an ordinary callable `Call`
with optional record-only fields attached to it. Effect analysis and each
backend then inspected the callee and target expression again to decide
whether the call was a record constructor. Qualified, imported, generic, and
same-leaf declarations made that reconstruction increasingly fragile even
after canonical declaration identity became available.

## Decision

Every checked ordinary record construction lowers to a dedicated
`RecordConstruct` expression. The node retains:

- the canonical record declaration identity;
- the authored or imported target projection needed to select a backend name;
- explicit generic type arguments;
- record fields in declaration order, including whether each field has a
  constructor default;
- supplied arguments in authored evaluation order; and
- the checked result type and source span.

The canonical declaration decides what is being constructed. The target
projection does not become a second semantic identity; it preserves such
details as a namespace import alias that a backend must render.

An ordinary `Call` no longer carries record-only target, declaration, or field
side channels. Class construction and function or method invocation remain
ordinary calls.

The checker continues to own keyword binding, required-field diagnostics,
assignability, and default eligibility. Default expressions remain on the
record declaration. Effect analysis combines that declaration with the fields
actually omitted at each `RecordConstruct`: an omitted effectful default uses
the effectful helper path, while an explicitly supplied field can keep a
synchronous construction path.

Backends consume `RecordConstruct` directly and choose their native literal or
generated default helper without rediscovering record semantics from a
`.new`-shaped call. The typed-IR evaluator likewise resolves the exact record
declaration before applying defaults.

This is an internal compiler representation change. It does not change source
syntax, record-default behavior, JSON or ORM decoding, Project Declaration
Input v6, or CLI behavior.

## Consequences

- Record construction has one target-independent semantic boundary across Go,
  Ruby, TypeScript, and the REPL.
- Canonical declaration identity reaches construction effect analysis without
  leaf-name or callee-shape reconstruction.
- Argument evaluation order and field declaration order remain separate and
  explicit in typed IR.
- Future compiler consumers can distinguish construction from invocation
  exhaustively instead of testing optional fields on `Call`.
- Backend-generated helper and type names remain backend-owned projections.

## Alternatives considered

- Keeping record fields on `Call` was rejected because an optional side
  channel permits invalid combinations and requires every consumer to repeat
  constructor detection.
- Lowering immediately to a generic record literal was rejected because
  omitted defaults can be effectful and require a definition-owned helper and
  execution scope.
- Encoding a generated helper name in shared IR was rejected because Go,
  Ruby, and TypeScript use different representations and naming rules.
- Expanding Project Declaration Input together with this change was rejected
  because provider visibility and wire identity are a separate versioned
  contract.
