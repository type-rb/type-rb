<!-- Generated from tools/native-language-cases.json; do not edit by hand. -->

Each count describes registered probes, not language coverage percentages.

| Family | Phase | Probes with differences | Uncovered contracts | Reference syntax nodes |
| --- | --- | --- | --- | --- |
| Portable Process operations | packages | 0 / 1 | Complete source output and release host qualification |  |
| Source text and lexical forms | basic | 0 / 64 | Invalid UTF-8 source and diagnostic positions; CRLF and source normalization; Remaining reserved contexts and malformed literal contracts | CommentStatement, BlankStatement, Identifier |
| Strings and UTF-8 | basic | 2 / 182 | Reference output-mode Unicode case differences (TypeRB #791); Embedded-expression boundaries beyond the reviewed interpolation probes | Literal, InterpolatedString |
| Immutable Bytes | basic | 0 / 22 | Bytes values across remaining nominal wrappers and imported declarations |  |
| Numeric and Boolean operations | basic | 1 / 86 | Remaining operators and assignment forms; mixed widening and portable failure boundaries; Lazy evaluation with remaining transfer, nullable invalidation and projection combinations | UnaryExpression, BinaryExpression |
| Bindings, constants and mutation | basic | 5 / 163 | Remaining collection-inference combinations; qualified namespace binding members (TypeRB #787) | VariableStatement, AssignmentStatement |
| Function declarations and calls | basic | 0 / 49 | none registered | MethodStatement, ReturnStatement, CallExpression, ExpressionStatement |
| Defaults and named arguments | basic | 0 / 63 | Imported method-default shadowing across execution modes (reference issue #1134) |  |
| Conditions and value-producing branches | basic | 0 / 16 | Nullable/union common result types and broader expression nesting boundaries | IfStatement |
| Loop control and transfers | basic | 0 / 10 | Additional nested transfer and diagnostic-origin differential probes | WhileStatement, BreakStatement, NextStatement |
| Case statements and expressions | basic | 0 / 16 | none registered | CaseStatement |
| Records and field bindings | basic | 0 / 41 | Nominal cycles and defaults involving remaining unsupported value types | RecordStatement, RecordFieldStatement |
| Member access and projections | basic | 1 / 12 | Safe member access, chained field narrowing and receiver replacement | MemberExpression |
| Nullable values and narrowing | basic | 12 / 100 | Class/callback and general pattern narrowing; remaining collection/union optional conversions; unguarded nullable indexing (issue #1120); Nullable standard-library methods beyond the currently implemented Native APIs |  |
| Enums and tagged values | basic | 4 / 176 | Method-specific type parameters, attribute output-mode effects and remaining pattern/control combinations; Reference REPL initializer replay (TypeRB #768) and shadowed standard error identity (#769); imported/nested REPL presentation parity | EnumStatement, EnumMemberStatement |
| Transparent aliases | basic | 3 / 110 | Authored alias spelling in diagnostics, nominal import display and nullable union alternative ordering (#455); Transparent alias compatibility at conditional branch joins (proposal #1128) | TypeAliasStatement |
| Nominal newtypes | basic | 1 / 119 | Class/interface representation dependencies; Method references and wider callable/generic combinations | NewtypeStatement |
| Generic declarations and applications | basic | 2 / 197 | Remaining method-specific generic combinations and deferred generic class methods; enum ordinary/REPL contract decision (#1160); Wider constraints and type applications; Generic output of concrete aggregate values; Nullable type-argument output differs from the reference Go representation (#691) | GenericExpression |
| Union values, literal types and discriminated unions | basic | 10 / 218 | Reference grouped annotation and nullable-alternative boundaries (TypeRB #764 and #765); composite type patterns remain staged in the reference |  |
| Function values and lexical capture | basic | 0 / 116 | Method references and remaining parameter-capability/narrowing combinations | LambdaExpression |
| Arrays and checked indexes | basic | 11 / 324 | Remaining receiver and inference combinations; unguarded nullable indexing (issue #1120); Wider invalid element/mutation combinations | ArrayLiteral, IndexExpression |
| Hash values and operations | basic | 12 / 69 | Wider key/value representations, dynamic Any operations, remaining inference combinations and managed lifetimes; nullable Hash reference Go lowering and unguarded indexing (issue #1120) | HashLiteral |
| Structured and value-producing iteration | basic | 0 / 176 | Remaining receiver APIs that shorten or reorder an Array during traversal | IterationExpression, BlockExpression |
| Range values and boundaries | basic | 0 / 19 | none registered | RangeExpression |
| Result and typed propagation | basic | 0 / 81 | General union errors and compiler-declared structured package boundaries | TryExpression, CatchExpression |
| Classes, fields and methods | basic | 0 / 89 | Initialized superclass storage and constructor chaining; inherited overrides (TypeRB #797) | ClassStatement, FieldStatement |
| Explicit interface conformance | basic | 0 / 21 | Variance and method-specific generic contracts with the reference object specification; Initialized-superclass and override dependencies | InterfaceStatement |
| Modules and constant lookup | basic | 1 / 40 | Remaining module method and qualified mutable namespace-member boundaries | ModuleStatement |
| Symbol values | basic | 1 / 36 | Remaining reference operator/control framing and multiline interpolation boundaries | SymbolLiteral |
| Project imports and declaration identity | basic | 1 / 83 | Bare/named aliases, graph conflicts, missing/unused imports and file/project/REPL identity | ImportStatement |
| Target source interop | toolchain | 0 / 0 | Go/Ruby/TypeScript source emission and platform interop | NativeStatement, NativeBlock, NativeExpression |
| Portable HTTP values | packages | 0 / 29 | Networking, streaming, transport adapters and Web semantic/type providers |  |
| Web values and middleware | packages | 0 / 71 | HTTP serving, whole-buffer gzip, OIDC and ordinary Web deadlines have dedicated runtime acceptance; logger and full deadline execution in the retained REPL remain incomplete. |  |
| JSX and web bindings | packages | 0 / 0 | Typed JSX and official web-package boundaries | JSXElement |
| Package capability activation | packages | 0 / 0 | Package-owned activation and resolution | ActivateStatement |
| Retired authored attempt syntax | retired | 0 / 0 | none registered | AttemptExpression |
| JSON values and comments | packages | 0 / 33 | Generic record schemas remain incomplete; Deterministic precedence for multiple invalid object members requires the decision in #1138 |  |
| Typed Web path and query records | packages | 3 / 44 | Integer-backed raw enums and multiple non-nullable Array fields await a cross-mode reference decision (#1175); Time range, parsing and codec-import restrictions remain tracked in the time-services family |  |
| Typed JSON codecs and Web JSON | packages | 1 / 68 | Generic record schemas remain incomplete; recursive schemas and closed inbound newtypes retain the reference rejection; Response.json REPL invocation awaits the adapter decision in #1178; Non-finite JSON error message differs between compiled and REPL reference paths; #1179 owns the unresolved parity gap |  |
| Combined Web endpoint inputs | packages | 1 / 55 | Renamed imported envelope records remain rejected pending canonical-identity decision #1183; Calling bind without a type argument is rejected; reference check-only acceptance differs and awaits #1184; HTTP server lifecycle remains incomplete; typed Context storage is tracked by the web-context family |  |
| Portable time values and Web time codecs | packages | 17 / 92 | Portable constructor/parser bounds and Duration integer limits await consistent reference semantics (#1187); TimeOfDay/DateTime error prefixes and Duration leading-plus parsing differ between reference execution modes (#1188); Renamed time codec imports retain the reference restriction pending #1189; implicitly required but otherwise unused imports also intersect #1180. Project-startup versus interactive-submission rejection remains an imports-family REPL gap; System zone data is required; TZ/ZONEINFO overrides, zip or embedded databases, TZif v4 and broader environment parity remain unqualified; Monotonic clocks and scheduling remain incomplete |  |
| Typed Web context values | packages | 0 / 56 | REPL binding display of opaque Context storage awaits the portable inspection decision in #1192; deterministic value-operation probes do not establish display parity |  |
| Hex, Base64 and secure random bytes | packages | 0 / 74 | Host-failure diagnostic equivalence across implementations remains unqualified |  |
| File routes and in-process Web dispatch | packages | 0 / 75 | Network serving, streaming, cancellation and shutdown remain incomplete; Wider generic and helper schema combinations remain unqualified |  |
| Checked endpoint declarations and Web generators | packages | 3 / 68 | Ordinary unowned class-body calls remain incomplete pending #1208; local namesakes, indirect descendants and namespaced contracts are explicit gaps; Entry-free configured project checking remains incomplete; the pinned OpenAPI command itself requires a main entry; Native source-backend execution and server lifecycle remain incomplete; The pinned renamed shared-import browser consumer failure remains explicit under #1183; generator ownership review is #1211 |  |
| Path and validated relative path values | packages | 0 / 39 | none registered |  |
| Scoped File and ambient directory operations | packages | 1 / 6 | Opened directory capabilities and cooperative lock scopes remain separate coverage work; Serial concurrent_map File bodies, helper calls and borrowed-write rejection have focused controls; broader package-resource combinations remain unqualified.; Pinned reference REPL implicit-import replay remains visible in the directory case under #1127; directory multi-error ordering review is #1287; Complete four-host release qualification and source-backend execution remain outstanding |  |
| Buffered string construction | packages | 0 / 3 | Aliased new factory code generation in the pinned reference.; Generic collection display containing builders.; Explicit puts across compiled execution and REPL: specification proposal #1249; current mode-specific reference behavior is preserved. |  |
| OIDC Bearer authentication | packages | 0 / 2 | Complete transport edge compatibility and release artifact qualification. |  |
| Structured serial concurrent mapping | basic | 0 / 12 | Broader combinations of package-owned native values and execution-scope adapters remain unqualified; #626 tracks the remaining scope coverage. Imported authored-call safety is a specification proposal in #1333, not an adopted change. |  |
