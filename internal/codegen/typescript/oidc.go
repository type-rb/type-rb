package typescript

import "github.com/type-rb/type-rb/internal/ir"

const oidcVerifyBearerIntrinsic = "trb.internal.auth.oidc.verify_bearer"

func (g *generator) oidcIntrinsic(name string, call *ir.Call, arguments []string) (string, bool) {
	if name != oidcVerifyBearerIntrinsic {
		return "", false
	}
	g.oidcRuntime = true
	if len(arguments) != 2 || len(call.ExprType().Args) != 2 {
		return "undefined", true
	}
	resultType := g.tsType(call.ExprType())
	principalType := g.tsType(call.ExprType().Args[0])
	errorType := g.tsType(call.ExprType().Args[1])
	result := g.runtimeName("Result")
	errorKind := g.runtimeName("OidcAuthErrorKind")
	request := arguments[0]
	options := arguments[1]
	return "(async (): Promise<" + resultType + "> => { const requestValue = " + request + "; const optionsValue = " + options + "; const authorization = requestValue.__trb_headers.entries().filter((header) => header.name.toLowerCase() === \"authorization\").map((header) => header.value); const verified = await trb_oidc_verify_bearer(authorization, optionsValue.issuer, optionsValue.audience, optionsValue.jwks_uri, optionsValue.roles_claim, optionsValue.clock_skew_seconds); if (verified.failure !== undefined) { const kind = verified.failure.kind === \"missing\" ? " + errorKind + ".MissingCredentials : verified.failure.kind === \"provider\" ? " + errorKind + ".Provider : verified.failure.kind === \"configuration\" ? " + errorKind + ".Configuration : " + errorKind + ".InvalidCredentials; return " + result + ".Err<" + principalType + ", " + errorType + ">(({ kind, message: verified.failure.message }) satisfies " + errorType + "); } const data = verified.data!; return " + result + ".Ok<" + principalType + ", " + errorType + ">(({ subject: data.subject, name: data.name, email: data.email, roles: data.roles }) satisfies " + principalType + "); })()", true
}

func (g *generator) oidcBearerRuntimeSupport() {
	g.b.WriteString(tsOidcBearerRuntime)
	g.b.WriteByte('\n')
}

const tsOidcBearerRuntime = `
type TrbOidcPrincipalData = Readonly<{ subject: string; name: string | null; email: string | null; roles: string[] }>;
type TrbOidcFailure = Readonly<{ kind: "missing" | "invalid" | "provider" | "configuration"; message: string }>;
type TrbOidcJwk = Readonly<{ kid: string; kty: string; use?: string; alg?: string; n: string; e: string }>;
type TrbOidcProviderMetadata = Readonly<{ issuer: string; jwks_uri: string }>;
type TrbOidcResult<T> = Readonly<{ data?: T; failure?: TrbOidcFailure }>;
const trb_oidc_provider_cache = new Map<string, Readonly<{ value: TrbOidcProviderMetadata; expires: number }>>();
const trb_oidc_jwks_cache = new Map<string, Readonly<{ keys: TrbOidcJwk[]; expires: number; refresh_after: number }>>();

function trb_oidc_invalid(message: string): TrbOidcResult<never> {
	return { failure: { kind: "invalid", message } };
}

function trb_oidc_base64url(value: string): Uint8Array {
	const padded = value.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(value.length / 4) * 4, "=");
	return Uint8Array.from(atob(padded), (character) => character.charCodeAt(0));
}

async function trb_oidc_get_json(uri: string, label: string): Promise<TrbOidcResult<unknown>> {
	try {
		const response = await fetch(uri, { signal: AbortSignal.timeout(5000) });
		if (!response.ok) return { failure: { kind: "provider", message: label + ": HTTP " + String(response.status) } };
		const body = await response.text();
		if (new TextEncoder().encode(body).byteLength > 1_048_576) return { failure: { kind: "provider", message: label + " response is too large" } };
		return { data: JSON.parse(body) as unknown };
	} catch (error) {
		return { failure: { kind: "provider", message: label + ": " + (error instanceof Error ? error.message : String(error)) } };
	}
}

async function trb_oidc_load_provider(issuer: string): Promise<TrbOidcResult<TrbOidcProviderMetadata>> {
	if (issuer === "") return { failure: { kind: "configuration", message: "OIDC issuer is empty" } };
	const cached = trb_oidc_provider_cache.get(issuer);
	if (cached !== undefined && cached.expires > Date.now()) return { data: cached.value };
	const loaded = await trb_oidc_get_json(issuer.replace(/\/+$/, "") + "/.well-known/openid-configuration", "load OIDC discovery");
	if (loaded.failure !== undefined) return { failure: loaded.failure };
	const metadata = loaded.data as Partial<TrbOidcProviderMetadata>;
	if (metadata.issuer !== issuer || typeof metadata.jwks_uri !== "string" || metadata.jwks_uri === "") return { failure: { kind: "provider", message: "OIDC discovery response is invalid" } };
	const value = { issuer: metadata.issuer, jwks_uri: metadata.jwks_uri } satisfies TrbOidcProviderMetadata;
	trb_oidc_provider_cache.set(issuer, { value, expires: Date.now() + 300_000 });
	return { data: value };
}

async function trb_oidc_load_jwks(uri: string, force: boolean): Promise<TrbOidcResult<TrbOidcJwk[]>> {
	if (uri === "") return { failure: { kind: "configuration", message: "OIDC JWKS URI is empty" } };
	const now = Date.now();
	const cached = trb_oidc_jwks_cache.get(uri);
	if (force && cached !== undefined && cached.refresh_after > now) return { data: cached.keys };
	if (!force && cached !== undefined && cached.expires > now) return { data: cached.keys };
	if (force && cached !== undefined) trb_oidc_jwks_cache.set(uri, { ...cached, refresh_after: now + 30_000 });
	const loaded = await trb_oidc_get_json(uri, "load OIDC JWKS");
	if (loaded.failure !== undefined) return { failure: loaded.failure };
	const keys = (loaded.data as { keys?: unknown }).keys;
	if (!Array.isArray(keys) || keys.length === 0) return { failure: { kind: "provider", message: "OIDC JWKS response is invalid" } };
	const typed = keys as TrbOidcJwk[];
	trb_oidc_jwks_cache.set(uri, { keys: typed, expires: now + 300_000, refresh_after: force ? now + 30_000 : (cached?.refresh_after ?? 0) });
	return { data: typed };
}

function trb_oidc_select_jwk(keys: TrbOidcJwk[], kid: string): TrbOidcJwk | undefined {
	return keys.find((key) => key.kid === kid && key.kty === "RSA" && (key.use === undefined || key.use === "" || key.use === "sig") && (key.alg === undefined || key.alg === "" || key.alg === "RS256"));
}

async function trb_oidc_verify_bearer(values: string[], issuer: string, audience: string, jwks_uri: string | null, roles_claim: string, clock_skew_seconds: number): Promise<TrbOidcResult<TrbOidcPrincipalData>> {
	if (issuer === "" || audience === "" || roles_claim === "" || clock_skew_seconds < 0) return { failure: { kind: "configuration", message: "OIDC bearer configuration is invalid" } };
	if (values.length === 0) return { failure: { kind: "missing", message: "Authorization header is required" } };
	if (values.length !== 1) return { failure: { kind: "invalid", message: "exactly one Authorization header is required" } };
	const parts = values[0]!.trim().split(/\s+/);
	if (parts.length !== 2 || parts[0]!.toLowerCase() !== "bearer") return { failure: { kind: "invalid", message: "Authorization header must use Bearer" } };
	return await trb_oidc_verify_jwt(parts[1]!, issuer, audience, jwks_uri, roles_claim, clock_skew_seconds);
}

async function trb_oidc_verify_jwt(token: string, issuer: string, audience: string, jwks_uri: string | null, roles_claim: string, clock_skew_seconds: number): Promise<TrbOidcResult<TrbOidcPrincipalData>> {
	try {
		if (jwks_uri === null) {
			const provider = await trb_oidc_load_provider(issuer);
			if (provider.failure !== undefined) return { failure: provider.failure };
			jwks_uri = provider.data!.jwks_uri;
		}
		const segments = token.split(".");
		if (segments.length !== 3) return trb_oidc_invalid("JWT must contain three segments");
		const header = JSON.parse(new TextDecoder().decode(trb_oidc_base64url(segments[0]!))) as Record<string, unknown>;
		const claims = JSON.parse(new TextDecoder().decode(trb_oidc_base64url(segments[1]!))) as Record<string, unknown>;
		if (header.alg !== "RS256" || typeof header.kid !== "string" || header.kid === "") return trb_oidc_invalid("JWT must use RS256 with a key id");
		let loaded = await trb_oidc_load_jwks(jwks_uri, false);
		if (loaded.failure !== undefined) return { failure: loaded.failure };
		let jwk = trb_oidc_select_jwk(loaded.data!, header.kid);
		if (jwk === undefined) {
			loaded = await trb_oidc_load_jwks(jwks_uri, true);
			if (loaded.failure !== undefined) return { failure: loaded.failure };
			jwk = trb_oidc_select_jwk(loaded.data!, header.kid);
		}
		if (jwk === undefined) return trb_oidc_invalid("JWT signing key was not found");
		if (trb_oidc_base64url(jwk.n).byteLength < 256) return trb_oidc_invalid("OIDC signing key is invalid");
		const key = await crypto.subtle.importKey("jwk", { ...jwk, ext: true }, { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" }, false, ["verify"]);
		const valid = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", key, trb_oidc_base64url(segments[2]!) as BufferSource, new TextEncoder().encode(segments[0]! + "." + segments[1]!) as BufferSource);
		if (!valid) return trb_oidc_invalid("JWT signature is invalid");
		if (claims.iss !== issuer) return trb_oidc_invalid("JWT issuer does not match");
		const audiences = typeof claims.aud === "string" ? [claims.aud] : Array.isArray(claims.aud) ? claims.aud : [];
		if (!audiences.includes(audience)) return trb_oidc_invalid("JWT audience does not match");
		const now = Math.floor(Date.now() / 1000);
		if (typeof claims.exp !== "number" || claims.exp <= now - clock_skew_seconds) return trb_oidc_invalid("JWT is expired");
		if (typeof claims.nbf === "number" && claims.nbf > now + clock_skew_seconds) return trb_oidc_invalid("JWT is not active");
		if (typeof claims.sub !== "string" || claims.sub === "") return trb_oidc_invalid("JWT subject is missing");
		const raw_roles = claims[roles_claim] ?? [];
		if (!Array.isArray(raw_roles) || !raw_roles.every((role) => typeof role === "string")) return trb_oidc_invalid("JWT roles claim is invalid");
		return { data: { subject: claims.sub, name: typeof claims.name === "string" ? claims.name : null, email: typeof claims.email === "string" ? claims.email : null, roles: raw_roles as string[] } };
	} catch (error) {
		return trb_oidc_invalid(error instanceof Error ? error.message : String(error));
	}
}
`
