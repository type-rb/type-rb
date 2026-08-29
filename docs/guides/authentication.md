# OIDC Bearer Authentication

The experimental `trb/auth/oidc` and `trb/web/auth/bearer` packages validate
OpenID Connect bearer tokens in Go, Ruby, and TypeScript server applications.
The public contract is portable; backend-specific networking and cryptography
remain behind the package boundary.

Create `src/auth/config.trb` from the provider issuer and API audience. Keeping
configuration in its own module lets nested route packages import it without a
generated Go `main` package cycle:

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
import { Context, Response, text } from trb/web
import trb/web/auth/bearer

def get(context: Context): Response
	case Bearer.principal(context)
	when Result::Ok(principal)
		return text(principal.subject)
	when Result::Err(_error)
		return text("unauthorized", 401)
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

Server-managed browser sessions, authorization-code PKCE, cookie-key rotation,
and pluggable session stores are not part of this first bearer profile. Keep
production issuers and JWKS endpoints on HTTPS and apply authorization policy
to the verified principal separately from authentication.
