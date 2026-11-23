# Checked Web endpoint contracts

A configured Web project can describe a file-route handler with the existing
`trb/web` endpoint declarations. In `src/routes/items.trb`:

```trb
import { Context, Response, Endpoint, handles, response } from trb/web

record Item
  id: Integer
  title: String
end

def get(_context: Context): Response
  return Response.json(Item.new(id: 1, title: "sample"))
end

class ItemsEndpoint < Endpoint
  handles(get)
  response<Item>(status: 200)
end
```

The top-level contract class directly extends the owned `Endpoint`, names exactly one
local file-route handler and declares one or more unique literal response
statuses from 100 through 599. Optional `input<T>()` declares the input envelope.
The ordinary checker resolves the handler signature, type arguments and marker
function identities. Marker calls are compile-time metadata and never execute
as route or program behavior. Runtime input binding and response construction
remain explicit application operations; declarations do not infer or validate
an implementation's responses automatically.

## OpenAPI generation

```sh
./trbn web openapi --config trbconfig.jsonc
./trbn web openapi --output docs/openapi.json --title "Sample API" --api-version 1.0
```

The target-neutral command uses the project's checked endpoint catalog. It emits
OpenAPI 3.1 in every declared project mode without invoking a compiler toolchain,
server, handler or program entry point. The pinned integration requires a project
with a `main` entry, although generation never runs it. See the
[CLI contract](native-cli.md) for defaults and atomic file output.

An input envelope is a non-nullable, non-generic record containing only `params`,
`query` and `body`. Path parameters match the route exactly and are required.
Query fields retain declaration order; nullable fields and repeated array fields
are optional. The body is required JSON when declared. Response status codes
that prohibit bodies require `Unit`.

JSON schemas cover Boolean, portable Integer, Float, String, nullable types,
arrays, String-keyed hashes, records, raw String/Integer enums, transparent aliases,
newtypes and the portable time scalars. Record properties use their JSON wire
names. Components retain declaration identity and reject colliding public names;
required fields retain declaration order. Paths, methods, status maps and component
maps are deterministic.

The pinned generator rejects recursive and generic schemas, generic aliases,
catch-all routes, unsupported parameter shapes and invalid JSON field names.
These are generation-only restrictions: an otherwise valid ordinary program
continues to check and execute. A project with no endpoint contracts cannot
produce an OpenAPI document.

## Identity restrictions and verification

The current reference accepts a renamed superclass import but does not recognize
renamed marker functions or a type alias as the superclass. Renamed time imports
are rejected by OpenAPI; transparent time aliases work. A renamed `Unit` import
is an ordinary empty-record response schema, whereas canonical `Unit` and a
transparent alias of it describe a bodyless response. Native preserves these
reference distinctions pending the [canonical identity decision](https://github.com/type-rb/type-rb-native/issues/1207).
Qualified types reached through an imported namespace are also rejected by the
pinned schema provider, including nested records, enums, newtypes and aliases.
Locally declared qualified types and aliases exported by their defining shared
module retain their existing schema behavior.

Owned contracts do not grant metadata privileges to unrelated class bodies or
local declarations with the same names. Nested contract classes do not acquire
the top-level metadata capability. General class-body call execution is
still an [ordinary language gap](https://github.com/type-rb/type-rb-native/issues/1208); it is rejected explicitly rather than silently
removed. Network serving remains a separate provider.

## Browser client generation

```sh
./trbn web client --config trbconfig.jsonc
./trbn web client --name PublicClient --output generated/api_client.trb
```

The command generates TypeRB source over `trb/platform/typescript/browser`.
It uses the same checked endpoint catalog and pinned JSON wire subset as
OpenAPI generation. It does not run handlers, the program entry point, a server,
an external formatter or a target toolchain. Generation works in every declared
project mode; executing the generated client requires a browser-target consumer.
This command does not establish Native execution of browser APIs or Native
source-backend support.

Each endpoint produces a status-tagged result enum and a client method derived
from its name. Methods encode path parameters, omit absent nullable queries,
repeat array query fields, encode JSON bodies and accept optional headers and
timeouts. Declared response statuses use typed JSON decoding or bodyless
responses. Unexpected statuses return a contract error retaining the response;
transport and decoding failures remain explicit `Result` errors.

Contract types must be imported from shared modules or packages that the browser
consumer can resolve. The generator imports them instead of copying their
definitions. Route-local types, colliding generated/imported names and duplicate
or reserved method names are generation errors. Paths, methods, response statuses
and imports have deterministic ordering.

Pinned reference limitations remain explicit. A renamed shared type import can
generate an import that the shared module does not export
([canonical imported identity](https://github.com/type-rb/type-rb-native/issues/1183)).
Shared application records named `Body` retain a different declaration identity
from the transport body. Both the original `Body` fixture and its `Payload`
control compile through the reference browser backend and exercise the same
synthetic Fetch checks. Generation success alone does not qualify consumers;
the remaining renamed-import failure is retained explicitly. Package and
generator responsibility boundaries are subject to a separate
[ownership review](https://github.com/type-rb/type-rb-native/issues/1211).

Shared ordinary cases cover checking, compilation, execution and REPL loading.
Provider units check ownership, corrupted catalog identities and types, and
source-erased MIR emission. `tools/native-web-openapi-test.py` compares complete
reference-derived documents and checks configuration modes, metadata overrides,
output replacement, failures, path constraints and absence of external tools.
`tools/native-web-client-test.py` compares complete reference-derived client
sources and the same publication boundaries. Its reference mode additionally
projects generated sources through the pinned TypeScript/browser backend and
executes a synthetic Fetch transport harness with Node. That harness uses
in-memory responses, not a network service or browser UI. The Go-free Native CLI
lane independently compares its own generated source to the same fixtures.
