package typescript

import (
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

func (g *generator) authIntrinsic(name string, call *ir.Call, arguments []string) (string, bool) {
	if !strings.HasPrefix(name, "trb.web.auth.bearer.") && !strings.HasPrefix(name, "trb.web.auth.session.") {
		return "", false
	}
	g.oidcRuntime = true
	context := arguments[0]
	options := arguments[len(arguments)-1]
	if strings.HasPrefix(name, "trb.web.auth.session.") {
		if name == "trb.web.auth.session.start_login" {
			options = arguments[1]
		}
		return g.sessionAuthIntrinsic(name, call, arguments, context, options), true
	}
	verification := "trbOidcVerifyBearer(" + context + ".request.headers, " + options + ".issuer, " + options + ".audience, " + options + ".jwks_uri, " + options + ".roles_claim)"
	if name == "trb.web.auth.bearer.authenticate" {
		next := arguments[1]
		return "(async (): Promise<TrbWebResponse> => { const verified = await " + verification + "; if (verified.failure !== undefined) return { status: 401, headers: { \"content-type\": [\"application/json; charset=utf-8\"], \"www-authenticate\": [\"Bearer\"] }, body: new TextEncoder().encode(\"{\\\"error\\\":\\\"unauthorized\\\"}\") }; return await " + next + ".call(" + context + "); })()", true
	}
	resultType := g.tsType(call.ExprType())
	principalType := g.runtimeName("OidcPrincipal")
	errorType := g.runtimeName("OidcAuthError")
	result := g.runtimeName("Result")
	errorValue := "verified.failure.kind === \"missing\" ? " + errorType + ".MissingCredentials : verified.failure.kind === \"provider\" ? " + errorType + ".Provider(verified.failure.message) : " + errorType + ".InvalidCredentials(verified.failure.message)"
	return "(async (): Promise<" + resultType + "> => { const verified = await " + verification + "; if (verified.failure !== undefined) return " + result + ".Err(" + errorValue + "); return " + result + ".Ok({ subject: verified.data!.subject, name: verified.data!.name, email: verified.data!.email, roles: verified.data!.roles } satisfies " + principalType + "); })()", true
}

func (g *generator) sessionAuthIntrinsic(name string, call *ir.Call, arguments []string, context, options string) string {
	switch name {
	case "trb.web.auth.session.start_login":
		return "trbOidcStartLogin(" + context + ".request, " + options + ", " + arguments[2] + ")"
	case "trb.web.auth.session.complete_login":
		return "trbOidcCompleteLogin(" + context + ".request, " + options + ")"
	case "trb.web.auth.session.end_session":
		return "trbOidcEndSession(" + context + ".request, " + options + ")"
	}
	verification := "trbOidcSessionPrincipal(" + context + ".request.headers, " + options + ".cookie_name, " + options + ".cookie_secret)"
	if name == "trb.web.auth.session.authenticate" {
		return "(async (): Promise<TrbWebResponse> => { const verified = await " + verification + "; if (verified.failure !== undefined) return trbOidcAuthResponse(401, \"unauthorized\"); if (!trbOidcCsrfValid(" + context + ".request, verified.session!.csrf)) return trbOidcAuthResponse(403, \"invalid_csrf_token\"); return await " + arguments[1] + ".call(" + context + "); })()"
	}
	resultType := g.tsType(call.ExprType())
	result := g.runtimeName("Result")
	errorType := g.runtimeName("OidcAuthError")
	principalType := g.runtimeName("OidcPrincipal")
	return "(async (): Promise<" + resultType + "> => { const verified = await " + verification + "; if (verified.failure !== undefined) return " + result + ".Err(verified.failure.kind === \"missing\" ? " + errorType + ".MissingCredentials : " + errorType + ".InvalidCredentials(verified.failure.message)); return " + result + ".Ok({ subject: verified.data!.subject, name: verified.data!.name, email: verified.data!.email, roles: verified.data!.roles } satisfies " + principalType + "); })()"
}

func (g *generator) oidcRuntimeSupport() {
	g.line(`type TrbOidcPrincipalData = Readonly<{ subject: string; name: string | null; email: string | null; roles: string[] }>;`)
	g.line(`type TrbOidcFailure = Readonly<{ kind: "missing" | "invalid" | "provider"; message: string }>;`)
	g.line(`type TrbOidcJwk = Readonly<{ kid: string; kty: string; use?: string; alg?: string; n: string; e: string }>;`)
	g.line(`const trbOidcJwksCache = new Map<string, Readonly<{ keys: TrbOidcJwk[]; expires: number }>>();`)
	g.line(`function trbOidcBase64Url(value: string): Uint8Array {`)
	g.indent++
	g.line(`const padded = value.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(value.length / 4) * 4, "=");`)
	g.line(`return Uint8Array.from(atob(padded), (character) => character.charCodeAt(0));`)
	g.indent--
	g.line(`}`)
	g.line(`async function trbOidcVerifyBearer(headers: Record<string, string[]>, issuer: string, audience: string, jwksUri: string, rolesClaim: string): Promise<Readonly<{ data?: TrbOidcPrincipalData; failure?: TrbOidcFailure }>> {`)
	g.indent++
	g.line(`const values = Object.entries(headers).filter(([name]) => name.toLowerCase() === "authorization").flatMap(([, values]) => values);`)
	g.line(`if (values.length !== 1) return { failure: { kind: "missing", message: "exactly one Authorization header is required" } };`)
	g.line(`const parts = values[0]!.trim().split(/\s+/);`)
	g.line(`if (parts.length !== 2 || parts[0]!.toLowerCase() !== "bearer") return { failure: { kind: "invalid", message: "Authorization header must use Bearer" } };`)
	g.line(`return await trbOidcVerifyJwt(parts[1]!, issuer, audience, jwksUri, rolesClaim, "");`)
	g.indent--
	g.line(`}`)
	g.line(`async function trbOidcVerifyJwt(token: string, issuer: string, audience: string, jwksUri: string, rolesClaim: string, nonce: string): Promise<Readonly<{ data?: TrbOidcPrincipalData; failure?: TrbOidcFailure }>> {`)
	g.indent++
	g.line(`try {`)
	g.indent++
	g.line(`const segments = token.split("."); if (segments.length !== 3) return { failure: { kind: "invalid", message: "JWT must contain three segments" } };`)
	g.line(`const header = JSON.parse(new TextDecoder().decode(trbOidcBase64Url(segments[0]!))) as Record<string, unknown>;`)
	g.line(`const claims = JSON.parse(new TextDecoder().decode(trbOidcBase64Url(segments[1]!))) as Record<string, unknown>;`)
	g.line(`if (header.alg !== "RS256" || typeof header.kid !== "string" || header.kid === "") return { failure: { kind: "invalid", message: "JWT must use RS256 with a key id" } };`)
	g.line(`const loaded = await trbOidcLoadJwks(jwksUri); if (loaded.failure !== undefined) return { failure: loaded.failure };`)
	g.line(`const jwk = loaded.keys!.find((candidate) => candidate.kid === header.kid && candidate.kty === "RSA" && (candidate.use === undefined || candidate.use === "sig") && (candidate.alg === undefined || candidate.alg === "RS256"));`)
	g.line(`if (jwk === undefined) return { failure: { kind: "invalid", message: "JWT signing key was not found" } };`)
	g.line(`const key = await crypto.subtle.importKey("jwk", { ...jwk, ext: true }, { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" }, false, ["verify"]);`)
	g.line(`const valid = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", key, trbOidcBase64Url(segments[2]!) as BufferSource, new TextEncoder().encode(segments[0]! + "." + segments[1]!) as BufferSource);`)
	g.line(`if (!valid) return { failure: { kind: "invalid", message: "JWT signature is invalid" } };`)
	g.line(`if (claims.iss !== issuer) return { failure: { kind: "invalid", message: "JWT issuer does not match" } };`)
	g.line(`const audiences = typeof claims.aud === "string" ? [claims.aud] : Array.isArray(claims.aud) ? claims.aud : []; if (!audiences.includes(audience)) return { failure: { kind: "invalid", message: "JWT audience does not match" } };`)
	g.line(`if (nonce !== "" && audiences.length > 1 && claims.azp !== audience) return { failure: { kind: "invalid", message: "OIDC authorized party does not match" } };`)
	g.line(`const now = Math.floor(Date.now() / 1000); if (typeof claims.exp !== "number" || claims.exp <= now - 60) return { failure: { kind: "invalid", message: "JWT is expired" } }; if (typeof claims.nbf === "number" && claims.nbf > now + 60) return { failure: { kind: "invalid", message: "JWT is not active" } };`)
	g.line(`if (nonce !== "" && claims.nonce !== nonce) return { failure: { kind: "invalid", message: "OIDC nonce does not match" } };`)
	g.line(`if (typeof claims.sub !== "string" || claims.sub === "") return { failure: { kind: "invalid", message: "JWT subject is missing" } };`)
	g.line(`const rawRoles = claims[rolesClaim]; const roles = Array.isArray(rawRoles) ? rawRoles.filter((role): role is string => typeof role === "string") : [];`)
	g.line(`return { data: { subject: claims.sub, name: typeof claims.name === "string" ? claims.name : null, email: typeof claims.email === "string" ? claims.email : null, roles } };`)
	g.indent--
	g.line(`} catch (error) {`)
	g.indent++
	g.line(`return { failure: { kind: "invalid", message: error instanceof Error ? error.message : String(error) } };`)
	g.indent--
	g.line(`}`)
	g.indent--
	g.line(`}`)
	g.line(`async function trbOidcLoadJwks(uri: string): Promise<Readonly<{ keys?: TrbOidcJwk[]; failure?: TrbOidcFailure }>> {`)
	g.indent++
	g.line(`if (uri === "") return { failure: { kind: "provider", message: "JWKS URI is empty" } }; const cached = trbOidcJwksCache.get(uri); if (cached !== undefined && cached.expires > Date.now()) return { keys: cached.keys };`)
	g.line(`try { const response = await fetch(uri, { signal: AbortSignal.timeout(5000) }); if (!response.ok) return { failure: { kind: "provider", message: ` + "`load JWKS: HTTP ${response.status}`" + ` } }; const text = await response.text(); if (text.length > 1_048_576) return { failure: { kind: "provider", message: "JWKS response is too large" } }; const document = JSON.parse(text) as { keys?: TrbOidcJwk[] }; if (!Array.isArray(document.keys) || document.keys.length === 0) return { failure: { kind: "provider", message: "JWKS response is invalid" } }; trbOidcJwksCache.set(uri, { keys: document.keys, expires: Date.now() + 300_000 }); return { keys: document.keys }; } catch (error) { return { failure: { kind: "provider", message: error instanceof Error ? error.message : String(error) } }; }`)
	g.indent--
	g.line(`}`)
	g.b.WriteString(tsOidcSessionRuntime)
	g.b.WriteByte('\n')
}

const tsOidcSessionRuntime = `
type TrbOidcSessionOptions = Readonly<{ issuer: string; client_id: string; client_secret: string; authorization_endpoint: string; token_endpoint: string; jwks_uri: string; redirect_uri: string; post_logout_redirect_uri: string; end_session_endpoint: string | null; scope: string; audience: string | null; roles_claim: string; cookie_name: string; cookie_secret: string; secure: boolean }>;
type TrbOidcLoginState = Readonly<{ state: string; nonce: string; verifier: string; return_to: string; expires: number }>;
type TrbOidcSessionData = Readonly<{ subject: string; name: string; email: string; roles: string[]; csrf: string; id_token: string; expires: number }>;
function trbOidcEncodeBase64Url(value: Uint8Array): string { let binary = ""; for (const byte of value) binary += String.fromCharCode(byte); return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, ""); }
function trbOidcRandom(size: number): string { const value = new Uint8Array(size); crypto.getRandomValues(value); return trbOidcEncodeBase64Url(value); }
function trbOidcCookieSecret(value: string): Uint8Array { const decoded = trbOidcBase64Url(value); if (decoded.byteLength !== 32) throw new Error("OIDC cookie secret must encode exactly 32 bytes"); return decoded; }
async function trbOidcEncrypt(value: unknown, secretText: string): Promise<string> { const secret = trbOidcCookieSecret(secretText); const key = await crypto.subtle.importKey("raw", secret as BufferSource, "AES-GCM", false, ["encrypt"]); const nonce = new Uint8Array(12); crypto.getRandomValues(nonce); const encrypted = new Uint8Array(await crypto.subtle.encrypt({ name: "AES-GCM", iv: nonce as BufferSource }, key, new TextEncoder().encode(JSON.stringify(value)) as BufferSource)); const output = new Uint8Array(nonce.length + encrypted.length); output.set(nonce); output.set(encrypted, nonce.length); return trbOidcEncodeBase64Url(output); }
async function trbOidcDecrypt<T>(value: string, secretText: string): Promise<T> { const secret = trbOidcCookieSecret(secretText); const encoded = trbOidcBase64Url(value); if (encoded.byteLength < 13) throw new Error("encrypted OIDC cookie is truncated"); const key = await crypto.subtle.importKey("raw", secret as BufferSource, "AES-GCM", false, ["decrypt"]); const plain = await crypto.subtle.decrypt({ name: "AES-GCM", iv: encoded.slice(0, 12) as BufferSource }, key, encoded.slice(12) as BufferSource); return JSON.parse(new TextDecoder().decode(plain)) as T; }
function trbOidcCookies(headers: Record<string, string[]>): Map<string, string[]> { const result = new Map<string, string[]>(); const values = Object.entries(headers).filter(([name]) => name.toLowerCase() === "cookie").flatMap(([, entries]) => entries); for (const header of values) { for (const part of header.split(";")) { const index = part.indexOf("="); if (index < 0) continue; const name = part.slice(0, index).trim(); const value = part.slice(index + 1); result.set(name, [...(result.get(name) ?? []), value]); } } return result; }
function trbOidcCookie(name: string, value: string, maxAge: number, secure: boolean, httpOnly: boolean): string { const parts = [name + "=" + value, "Path=/", "Max-Age=" + String(maxAge), "SameSite=Lax"]; if (secure) parts.push("Secure"); if (httpOnly) parts.push("HttpOnly"); return parts.join("; "); }
function trbOidcAuthResponse(status: number, code: string): TrbWebResponse { return { status, headers: { "content-type": ["application/json; charset=utf-8"] }, body: new TextEncoder().encode(JSON.stringify({ error: code })) }; }
async function trbOidcStartLogin(_request: TrbWebRequest, options: TrbOidcSessionOptions, authoredReturnTo: string): Promise<TrbWebResponse> { try { if (options.authorization_endpoint === "" || options.client_id === "" || options.redirect_uri === "" || options.scope === "" || options.cookie_name === "") return trbOidcAuthResponse(500, "invalid_auth_configuration"); const returnTo = !authoredReturnTo.startsWith("/") || authoredReturnTo.startsWith("//") || /[\\\r\n]/.test(authoredReturnTo) ? "/" : authoredReturnTo; const state = trbOidcRandom(32); const nonce = trbOidcRandom(32); const verifier = trbOidcRandom(48); const encrypted = await trbOidcEncrypt({ state, nonce, verifier, return_to: returnTo, expires: Date.now() + 600_000 } satisfies TrbOidcLoginState, options.cookie_secret); const challenge = trbOidcEncodeBase64Url(new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier) as BufferSource))); const target = new URL(options.authorization_endpoint); target.searchParams.set("response_type", "code"); target.searchParams.set("client_id", options.client_id); target.searchParams.set("redirect_uri", options.redirect_uri); target.searchParams.set("scope", options.scope); target.searchParams.set("state", state); target.searchParams.set("nonce", nonce); target.searchParams.set("code_challenge", challenge); target.searchParams.set("code_challenge_method", "S256"); if (options.audience !== null) target.searchParams.set("audience", options.audience); return { status: 302, headers: { location: [target.toString()], "set-cookie": [trbOidcCookie(options.cookie_name + "_state", encrypted, 600, options.secure, true)] }, body: new Uint8Array() }; } catch { return trbOidcAuthResponse(500, "invalid_auth_configuration"); } }
function trbOidcQueryValue(raw: string, name: string): string | undefined { const values = new URLSearchParams(raw).getAll(name); return values.length === 1 && values[0] !== "" ? values[0] : undefined; }
function trbOidcBasic(clientId: string, clientSecret: string): string { const bytes = new TextEncoder().encode(clientId + ":" + clientSecret); let binary = ""; for (const byte of bytes) binary += String.fromCharCode(byte); return btoa(binary); }
async function trbOidcCompleteLogin(request: TrbWebRequest, options: TrbOidcSessionOptions): Promise<TrbWebResponse> { try { const code = trbOidcQueryValue(request.query_string, "code"); const state = trbOidcQueryValue(request.query_string, "state"); const stateCookies = trbOidcCookies(request.headers).get(options.cookie_name + "_state") ?? []; if (code === undefined || state === undefined || stateCookies.length !== 1) return trbOidcAuthResponse(400, "invalid_oidc_callback"); const login = await trbOidcDecrypt<TrbOidcLoginState>(stateCookies[0]!, options.cookie_secret); if (login.expires < Date.now() || !trbOidcConstantTime(login.state, state)) return trbOidcAuthResponse(400, "invalid_oidc_state"); const body = new URLSearchParams({ grant_type: "authorization_code", code, redirect_uri: options.redirect_uri, code_verifier: login.verifier }); const tokenResponse = await fetch(options.token_endpoint, { method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded", "Authorization": "Basic " + trbOidcBasic(options.client_id, options.client_secret) }, body }); if (!tokenResponse.ok) return trbOidcAuthResponse(502, "identity_provider_error"); const text = await tokenResponse.text(); if (text.length > 1_048_576) return trbOidcAuthResponse(502, "identity_provider_error"); const tokens = JSON.parse(text) as { id_token?: string }; if (typeof tokens.id_token !== "string") return trbOidcAuthResponse(502, "identity_provider_error"); const verified = await trbOidcVerifyJwt(tokens.id_token, options.issuer, options.client_id, options.jwks_uri, options.roles_claim, login.nonce); if (verified.failure !== undefined) return trbOidcAuthResponse(400, "invalid_identity_token"); const csrf = trbOidcRandom(32); const session: TrbOidcSessionData = { subject: verified.data!.subject, name: verified.data!.name ?? "", email: verified.data!.email ?? "", roles: verified.data!.roles, csrf, id_token: tokens.id_token, expires: Date.now() + 28_800_000 }; const encrypted = await trbOidcEncrypt(session, options.cookie_secret); return { status: 302, headers: { location: [login.return_to], "set-cookie": [trbOidcCookie(options.cookie_name, encrypted, 28800, options.secure, true), trbOidcCookie("trb_csrf", encodeURIComponent(csrf), 28800, options.secure, false), trbOidcCookie(options.cookie_name + "_state", "", 0, options.secure, true)] }, body: new Uint8Array() }; } catch { return trbOidcAuthResponse(502, "identity_provider_unavailable"); } }
async function trbOidcSessionPrincipal(headers: Record<string, string[]>, cookieName: string, secret: string): Promise<Readonly<{ data?: TrbOidcPrincipalData; session?: TrbOidcSessionData; failure?: TrbOidcFailure }>> { const values = trbOidcCookies(headers).get(cookieName) ?? []; if (values.length !== 1) return { failure: { kind: "missing", message: "session cookie is missing" } }; try { const session = await trbOidcDecrypt<TrbOidcSessionData>(values[0]!, secret); if (session.expires <= Date.now() || session.subject === "") return { failure: { kind: "invalid", message: "session cookie is invalid" } }; return { data: { subject: session.subject, name: session.name === "" ? null : session.name, email: session.email === "" ? null : session.email, roles: session.roles }, session }; } catch { return { failure: { kind: "invalid", message: "session cookie is invalid" } }; } }
function trbOidcConstantTime(left: string, right: string): boolean { const size = Math.max(left.length, right.length); let difference = left.length ^ right.length; for (let index = 0; index < size; index += 1) difference |= (left.charCodeAt(index) || 0) ^ (right.charCodeAt(index) || 0); return difference === 0; }
function trbOidcCsrfValid(request: TrbWebRequest, expected: string): boolean { const method = request.method.toUpperCase(); if (method === "GET" || method === "HEAD" || method === "OPTIONS") return true; const cookieValues = trbOidcCookies(request.headers).get("trb_csrf") ?? []; const headerValues = Object.entries(request.headers).filter(([name]) => name.toLowerCase() === "x-csrf-token").flatMap(([, entries]) => entries); if (cookieValues.length !== 1 || headerValues.length !== 1) return false; try { return trbOidcConstantTime(expected, decodeURIComponent(cookieValues[0]!)) && trbOidcConstantTime(expected, headerValues[0]!); } catch { return false; } }
async function trbOidcEndSession(request: TrbWebRequest, options: TrbOidcSessionOptions): Promise<TrbWebResponse> { let location = options.post_logout_redirect_uri; if (options.end_session_endpoint !== null) { const target = new URL(options.end_session_endpoint); target.searchParams.set("post_logout_redirect_uri", options.post_logout_redirect_uri); target.searchParams.set("client_id", options.client_id); const verified = await trbOidcSessionPrincipal(request.headers, options.cookie_name, options.cookie_secret); if (verified.session?.id_token) target.searchParams.set("id_token_hint", verified.session.id_token); location = target.toString(); } return { status: 302, headers: { location: [location], "set-cookie": [trbOidcCookie(options.cookie_name, "", 0, options.secure, true), trbOidcCookie("trb_csrf", "", 0, options.secure, false)] }, body: new Uint8Array() }; }
`
