# Shared HTTP Values

`trb/http` provides portable values shared by server and client packages. It
does not start a server or send a request.

<!-- trb-doc-test: shared-http-values -->
```trb
import { Body, Header, Headers, HttpMethod } from trb/http

method := HttpMethod.post()
headers := Headers.new([
	Header.new(name: "content-type", value: "application/json"),
	Header.new(name: "set-cookie", value: "theme=dark"),
	Header.new(name: "set-cookie", value: "session=abc"),
])
body := Body.new("{}".to_bytes())

puts(method.to_s())
puts(headers.first("Content-Type"))
puts(headers.values("set-cookie").size())
puts(body.to_s())
```

`HttpMethod.new(value)` accepts extension methods in addition to the named
constructors. `Headers` preserves insertion order and repeated fields while
performing lookup case-insensitively. Its `with`, `add`, and `without` methods
return new values; `entries()` returns a copy. `Body` is the initial buffered
body representation and exposes its portable `Bytes` through `bytes()`.

`trb/web` uses these values for inbound requests and outbound server responses.
`trb/platform/typescript/browser` uses them for browser requests and buffered
Fetch responses. Query parameter encoding remains in `trb/std/url`.
