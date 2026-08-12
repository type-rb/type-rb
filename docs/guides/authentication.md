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

Define an `OidcSessionOptions` value in application code and load its secrets
from the deployment environment. `cookie_secret` must be a base64url-encoded
32-byte key; use `secure: true` outside local HTTP development.

```trb
import { OidcSessionOptions } from trb/auth/oidc

SESSION_AUTH := OidcSessionOptions.new(
	issuer: "https://identity.example.com/",
	client_id: "type-rb-web",
	client_secret: "...",
	authorization_endpoint: "https://identity.example.com/authorize",
	token_endpoint: "https://identity.example.com/token",
	jwks_uri: "https://identity.example.com/.well-known/jwks.json",
	redirect_uri: "https://app.example.com/auth/callback",
	post_logout_redirect_uri: "https://app.example.com/",
	end_session_endpoint: "https://identity.example.com/logout",
	scope: "openid profile email",
	audience: nil,
	roles_claim: "roles",
	cookie_name: "trb_session",
	cookie_secret: "...",
	secure: true,
)
```

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

Protect the portable API with `trb/web/auth/bearer` and an
`OidcBearerOptions` value:

```trb
import { OidcBearerOptions } from trb/auth/oidc
import { Context, Next, Response } from trb/web
import trb/web/auth/bearer

BEARER_AUTH := OidcBearerOptions.new(
	issuer: "https://identity.example.com/",
	audience: "type-rb-api",
	jwks_uri: "https://identity.example.com/.well-known/jwks.json",
	roles_claim: "roles",
)

def call(context: Context, next_handler: Next): Response
	return bearer.authenticate(context, next_handler, BEARER_AUTH)
end
```

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
expiry, not-before, subject, and OIDC nonce where applicable. The session
profile also enforces authorization-code PKCE, state, encrypted cookies, and
constant-time CSRF comparison. The initial alpha API uses explicit provider
endpoints and one active cookie key. Discovery, key rotation, downstream BFF
token forwarding, and configurable server-side session stores remain future
work.
