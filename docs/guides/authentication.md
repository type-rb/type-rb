# Experimental OIDC authentication

TypeRB provides two explicit OpenID Connect profiles. Both use the portable
`trb/auth/oidc` contract and the same `OidcPrincipal` type.

- Server sessions are the recommended default when one application owns the
  browser and API. Tokens stay behind the HTTP boundary, while the browser uses
  an encrypted `HttpOnly` session cookie and a CSRF token.
- Browser bearer tokens support SPAs that call a separately deployed API or
  migrate an existing React/OIDC application. This profile has the usual
  browser-token exposure tradeoff.

The server packages work in Go, Ruby, and TypeScript. React and browser
transport packages are explicit TypeScript platform packages.

## Server-session profile

Create the session configuration from the provider issuer. TypeRB loads the
standard OpenID Provider Configuration document, so applications do not repeat
the authorization, token, JWKS, and logout endpoints. Load secrets from the
deployment environment. `cookie_secret` must be a base64url-encoded 32-byte
key; `secure` defaults to `true`.

```trb
import { session_options } from trb/auth/oidc

SESSION_AUTH := session_options(
	issuer: "https://identity.example.com/",
	client_id: "type-rb-web",
	client_secret: "...",
	redirect_uri: "https://app.example.com/auth/callback",
	post_logout_redirect_uri: "https://app.example.com/",
	cookie_secret: "...",
)
```

Pass `secure: false` as the final argument only for local HTTP development.
Applications that need nonstandard endpoints, claims, scope, audience, or
cookie names can construct `OidcSessionOptions` directly; nullable endpoint
fields opt back into discovery when omitted.

File-based login, callback, and logout routes call the session package:

```trb
import { Context, Response } from trb/web
import { SESSION_AUTH } from auth_config
import trb/web/auth/session

def get(context: Context): Response
	return session.start_login(context, SESSION_AUTH, "/")
end
```

Use `session.complete_login(context, SESSION_AUTH)` in the callback route and
`session.end_session(context, SESSION_AUTH)` in the logout route. Protect a
route subtree with `_middleware.trb`:

```trb
import { Context, Next, Response } from trb/web
import { SESSION_AUTH } from auth_config
import trb/web/auth/session

def call(context: Context, next_handler: Next): Response
	return session.authenticate(context, next_handler, SESSION_AUTH)
end
```

The React application does not handle an access token. Navigate to the login
route and use the session transport for typed JSON calls:

```trb
import { FetchError } from trb/platform/typescript/browser
import trb/platform/typescript/browser/session

def load_todos(): Array<Todo> fails FetchError
	return session.get_json<Array<Todo>>("/api/todos")
end
```

The transport sends same-origin credentials and copies the session-bound CSRF
cookie into `X-CSRF-Token` for unsafe methods.

## Browser-bearer profile

Protect the portable API with `trb/web/auth/bearer`. The concise configuration
discovers the provider JWKS URI from the issuer:

```trb
import { bearer_options } from trb/auth/oidc
import { Context, Next, Response } from trb/web
import trb/web/auth/bearer

BEARER_AUTH := bearer_options(
	issuer: "https://identity.example.com/",
	audience: "type-rb-api",
)

def call(context: Context, next_handler: Next): Response
	return bearer.authenticate(context, next_handler, BEARER_AUTH)
end
```

Construct `OidcBearerOptions` directly to override `jwks_uri` or the roles
claim for a nonstandard provider.

React uses the existing OIDC ecosystem through `react-oidc-context`:

```trb
import { OidcBrowserOptions } from trb/auth/oidc
import { ReactNode } from trb/platform/typescript/react
import { provider, use_oidc } from trb/platform/typescript/react/oidc

BROWSER_AUTH := OidcBrowserOptions.new(
	authority: "https://identity.example.com/",
	client_id: "type-rb-browser",
	redirect_uri: "https://app.example.com/callback",
	post_logout_redirect_uri: "https://app.example.com/",
	scope: "openid profile email",
	audience: "type-rb-api",
	roles_claim: "roles",
)

def Account(): ReactNode
	auth := use_oidc()
	if auth.loading
		return <p>Loading</p>
	end
	if !auth.authenticated
		return <button onClick={auth.sign_in}>Sign in</button>
	end
	return <button onClick={auth.sign_out}>Sign out</button>
end

def App(): ReactNode
	return provider(BROWSER_AUTH, <Account />)
end
```

Pass `auth.access_token` after handling its nullable state to the typed
functions in `trb/platform/typescript/browser/bearer`.

Both server profiles verify RS256 signatures from JWKS, issuer, audience,
expiry, not-before, subject, and OIDC nonce where applicable. Provider metadata
and JWKS are cached for five minutes. An unknown signing key ID triggers one
rate-limited JWKS refresh, allowing normal provider key rotation without
turning arbitrary tokens into an unbounded fetch path. The session profile also
enforces authorization-code PKCE, state, encrypted cookies, and constant-time
CSRF comparison. Cookie-key rotation, downstream BFF token forwarding, and
configurable server-side session stores remain future work.
