# Raw enum conversions and ordinary enum methods

The reference enum contract allows explicit, unique String or Integer raw values
on every payloadless variant. `raw_value()` returns that representation and
`from_raw(value)` returns the canonical `Result<Enum, EnumValueError>`. Ordinary
and payload enums may also declare instance methods after their variants.

## Frontend and MIR ownership

Keep raw literal values and their origins in frontend declaration records.
Validate literal spelling before optimization can erase whether an expression
was authored: folded String concatenation is still an expression, not an enum
literal. Reject duplicate values, mixed representations, missing raw values,
payload/raw combinations, reserved method names and unsupported method forms.

Resolve the compiler-owned Result and error record by module identity before
freezing MIR type catalogs. An authored record with the same leaf name cannot
replace the generated error payload. Generated Result patterns may use the
canonical declaration without granting visibility to unrelated authored types.

Lower both conversion directions to existing verified enum tests/construction,
scalar/String comparisons, control-flow joins, records and union injection.
The backend consumes those typed operations and their existing lifetime facts;
it does not inspect raw declarations or dispatch on method spellings. Preserve
one evaluation of the receiver and each argument in authored order.

Resolve each instance method to an ordinary function with an explicit hidden
`self` parameter. Ordinary signature checks, default/named arguments, privacy,
call MIR and closure capture then apply to methods as well. Do not add a second
backend method-dispatch implementation for this family. Implicit method calls
inside closures capture the hidden receiver. Names such as `map` and `select`
select collection syntax only when followed by an actual block suffix.

The REPL consumes checked call/type projections, preserves nominal identities
across retained values and closures, and constructs the same canonical Result
and error payloads. Conversion failures are values, not evaluator exceptions.

## Verification and remaining boundaries

Shared cases cover both raw types, negative/portable integer boundaries, UTF-8
and NUL, conversion failures, aliases/imports, defaults, privacy, closures,
nullable calls, Result propagation/recovery and invalid declarations/calls.
Independent MIR tests erase source and frontend raw catalogs, reorder blocks,
force GC, forge nominal/union operands and remove required error-payload roots.
Dedicated REPL checks retain conversion results and returned closures across
new declarations, rejected submissions and reload, including a shadowed error
record name.

Methods inheriting generic enum parameters are covered by
[decision 0071](0071-generic-enum-method-mir.md). Method-specific parameters and
enum attributes remain explicit inventory work.
Reference [initializer replay](https://github.com/type-rb/type-rb/issues/768),
[standard error shadowing](https://github.com/type-rb/type-rb/issues/769), and
presentation differences remain visible
as differences, rather than successful parity. This family does not complete
all basic-language coverage or establish a performance claim.

The immutable bootstrap seed remains unchanged. Adopting enum methods in the
compiler's own source requires a separately accepted seed that parses them;
the current implementation keeps its state records and helper functions within
the verified seed source subset.
