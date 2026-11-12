# OIDC Bearer Authentication

The experimental `trb/auth/oidc` and `trb/web/auth/bearer` packages validate
OpenID Connect bearer tokens in Go, Ruby, and TypeScript server applications.
The public contract is portable; backend-specific networking and cryptography
remain behind the package boundary.

Create `src/auth/config.trb` from the provider issuer and API audience. A dedicated
configuration module lets routes share the same settings:

```trb
import { bearer_options } from trb/auth/oidc

OIDC := bearer_options(
	issuer: "https://identity.example.com/",
	audience: "type-rb-api",
)
```

`bearer_options` discovers the JWKS URI from the issuer's OpenID Provider
Configuration. Construct `OidcBearerOptions` directly to select a fixed JWKS
URI, a provider-specific roles claim, or a different non-negative clock skew.
The default roles claim is `roles`, and the default clock skew is 60 seconds.

Protect a file-route subtree with `_middleware.trb`:

```trb
import { Context, Next, Response } from trb/web
import trb/web/auth/bearer
import { OIDC } from auth/config

def call(context: Context, next_handler: Next): Response
	return Bearer.authenticate(context, next_handler, OIDC)
end
```

The middleware returns a JSON 401 response with `WWW-Authenticate: Bearer`
when credentials are missing or invalid. A verified `OidcPrincipal` is stored
in the immutable request context. Handlers retrieve it without sharing a
string key or performing a cast:

```trb
import trb/std/result
import { Context, Response } from trb/web
import trb/web/auth/bearer

def get(context: Context): Response
	case Bearer.principal(context)
	when Result::Ok(principal)
		return Response.text(principal.subject)
	when Result::Err(_error)
		return Response.text("unauthorized", 401)
	end
end
```

Use `Bearer.verify(request, options)` when an application needs verification
without the standard middleware response. It returns
`Result<OidcPrincipal, OidcAuthError>` and distinguishes missing credentials,
invalid credentials, provider failures, and invalid configuration.

The initial profile accepts RS256 JWTs with a signing key ID. It validates the
signature, issuer, audience, expiration, optional not-before time, subject, and
the configured roles claim. Provider metadata and JWKS documents are bounded
to 1 MiB and cached for five minutes. An unknown key ID triggers one
rate-limited JWKS refresh so normal signing-key rotation does not create an
unbounded provider request path.

Endpoint hostnames are normalized before connecting, including discovery,
JWKS URLs and redirect targets. Every output uses non-transitional UTS #46
with Unicode 17.0.0 tables and explicit hostname validation. For example,
`ＢＵＣＨＥＲ.example.invalid` connects to `bucher.example.invalid`, while
`bücher.example.invalid` connects to `xn--bcher-kva.example.invalid`.
Invalid names fail as provider errors before a connection to that target.
Paths and queries retain their contents. A terminal DNS root dot and ordinary
IPv4/IPv6 literals are supported; ambiguous numeric URL forms such as `127.1`
are rejected.

This normalization applies to network addresses. The configured issuer,
discovery issuer and JWT `iss` claim still require exact string equality;
Unicode and ASCII spellings do not become interchangeable identity values.
Redirects use the same hostname validation and one five-second request budget,
with at most ten requests. HTTP and HTTPS are the supported endpoint schemes.
Environment proxies continue to follow the selected runtime's transport policy;
Node requires its explicit environment-proxy option. Hostname normalization
also precedes the transport's proxy exclusion and TLS name checks.

See the [provider pins and qualification](../../internal/oidchost/README.md)
for dependency identities, validation flags and the Ruby source/table update
procedure. Run `trb install` to install managed dependencies before running
generated Go or TypeScript sources directly.

Server-managed browser sessions, authorization-code PKCE, cookie-key rotation,
and pluggable session stores are not part of this first bearer profile. Keep
production issuers and JWKS endpoints on HTTPS and apply authorization policy
to the verified principal separately from authentication.
