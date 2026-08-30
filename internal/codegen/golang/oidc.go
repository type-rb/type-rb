package golang

import "github.com/type-rb/type-rb/internal/ir"

const oidcVerifyBearerIntrinsic = "trb.internal.auth.oidc.verify_bearer"

func (g *generator) oidcIntrinsic(name string, call *ir.Call, arguments []string) (string, bool) {
	if name != oidcVerifyBearerIntrinsic {
		return "", false
	}
	g.oidcRuntime = true
	if len(arguments) != 2 || len(call.ExprType().Args) != 2 {
		return "nil", true
	}
	resultType := g.goType(call.ExprType())
	principalType := g.goType(call.ExprType().Args[0])
	errorType := g.goType(call.ExprType().Args[1])
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	kindAlias := g.typeAliases["OidcAuthErrorKind"]
	kind := func(name string) string {
		value := goConstantIdentifier("OidcAuthErrorKind", name)
		if kindAlias != "" {
			value = kindAlias + "." + value
		}
		return value
	}
	request := arguments[0]
	options := arguments[1]
	return "func() " + resultType + " { requestValue := " + request + "; optionsValue := " + options + "; authorization := []string{}; for _, header := range " + g.arrayValues("requestValue.TrbFieldHeaders.Entries()") + " { if strings.EqualFold(header.Name, \"authorization\") { authorization = append(authorization, header.Value) } }; jwksURI := \"\"; if optionsValue.JwksUri != nil { jwksURI = *optionsValue.JwksUri }; data, failure := trbOidcVerifyBearer(authorization, optionsValue.Issuer, optionsValue.Audience, jwksURI, optionsValue.RolesClaim, optionsValue.ClockSkewSeconds); if failure != nil { errorKind := " + kind("InvalidCredentials") + "; if failure.kind == \"missing\" { errorKind = " + kind("MissingCredentials") + " } else if failure.kind == \"provider\" { errorKind = " + kind("Provider") + " } else if failure.kind == \"configuration\" { errorKind = " + kind("Configuration") + " }; return " + resultAlias + ".NewResultErr[" + principalType + ", " + errorType + "](" + errorType + "{Kind: errorKind, Message: failure.message}) }; var displayName *string; if data.name != \"\" { value := data.name; displayName = &value }; var email *string; if data.email != \"\" { value := data.email; email = &value }; principal := " + principalType + "{Subject: data.subject, Name: displayName, Email: email, Roles: " + g.arrayReference("data.roles") + "}; return " + resultAlias + ".NewResultOk[" + principalType + ", " + errorType + "](principal) }()", true
}

func (g *generator) oidcBearerRuntimeSupport() {
	for path, alias := range map[string]string{
		"crypto":          "",
		"crypto/rsa":      "",
		"crypto/sha256":   "",
		"encoding/base64": "",
		"encoding/json":   "stdjson",
		"fmt":             "",
		"io":              "",
		"math/big":        "",
		"net/http":        "nethttp",
		"strings":         "",
		"sync":            "",
		"time":            "stdtime",
	} {
		g.requireImport(path, alias)
	}
	g.line("type trbOidcPrincipalData struct { subject string; name string; email string; roles []string }")
	g.line("type trbOidcFailure struct { kind string; message string }")
	g.line("type trbOidcJwk struct { Kid string `json:\"kid\"`; Kty string `json:\"kty\"`; Use string `json:\"use\"`; Alg string `json:\"alg\"`; N string `json:\"n\"`; E string `json:\"e\"` }")
	g.line("type trbOidcProviderMetadata struct { Issuer string `json:\"issuer\"`; JwksURI string `json:\"jwks_uri\"` }")
	g.line("type trbOidcProviderCacheEntry struct { metadata trbOidcProviderMetadata; expires stdtime.Time }")
	g.line("type trbOidcJwksCacheEntry struct { keys []trbOidcJwk; expires stdtime.Time; refreshAfter stdtime.Time }")
	g.line("var trbOidcHTTPClient = &nethttp.Client{Timeout: 5 * stdtime.Second}")
	g.line("var trbOidcProviderCache = struct { sync.Mutex; entries map[string]trbOidcProviderCacheEntry }{entries: map[string]trbOidcProviderCacheEntry{}}")
	g.line("var trbOidcJwksCache = struct { sync.Mutex; entries map[string]trbOidcJwksCacheEntry }{entries: map[string]trbOidcJwksCacheEntry{}}")
	g.line("func trbOidcGetJSON(uri string, target any, label string) *trbOidcFailure {")
	g.indent++
	g.line("response, err := trbOidcHTTPClient.Get(uri)")
	g.line("if err != nil { return &trbOidcFailure{kind: \"provider\", message: fmt.Sprintf(\"%s: %v\", label, err)} }")
	g.line("defer response.Body.Close()")
	g.line("if response.StatusCode != nethttp.StatusOK { return &trbOidcFailure{kind: \"provider\", message: fmt.Sprintf(\"%s: HTTP %d\", label, response.StatusCode)} }")
	g.line("body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))")
	g.line("if err != nil { return &trbOidcFailure{kind: \"provider\", message: fmt.Sprintf(\"%s: %v\", label, err)} }")
	g.line("if len(body) > 1<<20 { return &trbOidcFailure{kind: \"provider\", message: label + \" response is too large\"} }")
	g.line("if err := stdjson.Unmarshal(body, target); err != nil { return &trbOidcFailure{kind: \"provider\", message: label + \" response is invalid\"} }")
	g.line("return nil")
	g.indent--
	g.line("}")
	g.line("func trbOidcLoadProvider(issuer string) (trbOidcProviderMetadata, *trbOidcFailure) {")
	g.indent++
	g.line("if issuer == \"\" { return trbOidcProviderMetadata{}, &trbOidcFailure{kind: \"configuration\", message: \"OIDC issuer is empty\"} }")
	g.line("now := stdtime.Now(); trbOidcProviderCache.Lock(); cached, ok := trbOidcProviderCache.entries[issuer]; trbOidcProviderCache.Unlock()")
	g.line("if ok && now.Before(cached.expires) { return cached.metadata, nil }")
	g.line("var metadata trbOidcProviderMetadata")
	g.line("uri := strings.TrimRight(issuer, \"/\") + \"/.well-known/openid-configuration\"")
	g.line("if failure := trbOidcGetJSON(uri, &metadata, \"load OIDC discovery\"); failure != nil { return trbOidcProviderMetadata{}, failure }")
	g.line("if metadata.Issuer != issuer || metadata.JwksURI == \"\" { return trbOidcProviderMetadata{}, &trbOidcFailure{kind: \"provider\", message: \"OIDC discovery response is invalid\"} }")
	g.line("trbOidcProviderCache.Lock(); trbOidcProviderCache.entries[issuer] = trbOidcProviderCacheEntry{metadata: metadata, expires: now.Add(5 * stdtime.Minute)}; trbOidcProviderCache.Unlock()")
	g.line("return metadata, nil")
	g.indent--
	g.line("}")
	g.line("func trbOidcLoadJwks(uri string, force bool) ([]trbOidcJwk, *trbOidcFailure) {")
	g.indent++
	g.line("if uri == \"\" { return nil, &trbOidcFailure{kind: \"configuration\", message: \"OIDC JWKS URI is empty\"} }")
	g.line("now := stdtime.Now(); trbOidcJwksCache.Lock(); cached, ok := trbOidcJwksCache.entries[uri]; if force && ok && now.Before(cached.refreshAfter) { trbOidcJwksCache.Unlock(); return cached.keys, nil }; if force && ok { cached.refreshAfter = now.Add(30 * stdtime.Second); trbOidcJwksCache.entries[uri] = cached }; trbOidcJwksCache.Unlock()")
	g.line("if !force && ok && now.Before(cached.expires) { return cached.keys, nil }")
	g.line("var document struct { Keys []trbOidcJwk `json:\"keys\"` }")
	g.line("if failure := trbOidcGetJSON(uri, &document, \"load OIDC JWKS\"); failure != nil { return nil, failure }")
	g.line("if len(document.Keys) == 0 { return nil, &trbOidcFailure{kind: \"provider\", message: \"OIDC JWKS response is invalid\"} }")
	g.line("refreshAfter := stdtime.Time{}; if force { refreshAfter = now.Add(30 * stdtime.Second) } else if ok { refreshAfter = cached.refreshAfter }")
	g.line("trbOidcJwksCache.Lock(); trbOidcJwksCache.entries[uri] = trbOidcJwksCacheEntry{keys: document.Keys, expires: now.Add(5 * stdtime.Minute), refreshAfter: refreshAfter}; trbOidcJwksCache.Unlock()")
	g.line("return document.Keys, nil")
	g.indent--
	g.line("}")
	g.line("func trbOidcSelectJwk(keys []trbOidcJwk, kid string) *trbOidcJwk { for index := range keys { key := &keys[index]; if key.Kid == kid && key.Kty == \"RSA\" && (key.Use == \"\" || key.Use == \"sig\") && (key.Alg == \"\" || key.Alg == \"RS256\") { return key } }; return nil }")
	g.line("func trbOidcVerifyBearer(values []string, issuer string, audience string, jwksURI string, rolesClaim string, clockSkewSeconds int) (trbOidcPrincipalData, *trbOidcFailure) {")
	g.indent++
	g.line("if issuer == \"\" || audience == \"\" || rolesClaim == \"\" || clockSkewSeconds < 0 { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"configuration\", message: \"OIDC bearer configuration is invalid\"} }")
	g.line("if len(values) == 0 { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"missing\", message: \"Authorization header is required\"} }")
	g.line("if len(values) != 1 { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"exactly one Authorization header is required\"} }")
	g.line("parts := strings.Fields(values[0]); if len(parts) != 2 || !strings.EqualFold(parts[0], \"Bearer\") { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"Authorization header must use Bearer\"} }")
	g.line("return trbOidcVerifyJWT(parts[1], issuer, audience, jwksURI, rolesClaim, clockSkewSeconds)")
	g.indent--
	g.line("}")
	g.line("func trbOidcVerifyJWT(token string, issuer string, audience string, jwksURI string, rolesClaim string, clockSkewSeconds int) (trbOidcPrincipalData, *trbOidcFailure) {")
	g.indent++
	g.line("if jwksURI == \"\" { metadata, failure := trbOidcLoadProvider(issuer); if failure != nil { return trbOidcPrincipalData{}, failure }; jwksURI = metadata.JwksURI }")
	g.line("segments := strings.Split(token, \".\"); if len(segments) != 3 { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT must contain three segments\"} }")
	g.line("headerBytes, err := base64.RawURLEncoding.DecodeString(segments[0]); if err != nil { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT header is not base64url\"} }")
	g.line("payloadBytes, err := base64.RawURLEncoding.DecodeString(segments[1]); if err != nil { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT payload is not base64url\"} }")
	g.line("signature, err := base64.RawURLEncoding.DecodeString(segments[2]); if err != nil { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT signature is not base64url\"} }")
	g.line("var header struct { Alg string `json:\"alg\"`; Kid string `json:\"kid\"` }; if stdjson.Unmarshal(headerBytes, &header) != nil || header.Alg != \"RS256\" || header.Kid == \"\" { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT must use RS256 with a key id\"} }")
	g.line("keys, failure := trbOidcLoadJwks(jwksURI, false); if failure != nil { return trbOidcPrincipalData{}, failure }; selected := trbOidcSelectJwk(keys, header.Kid); if selected == nil { keys, failure = trbOidcLoadJwks(jwksURI, true); if failure != nil { return trbOidcPrincipalData{}, failure }; selected = trbOidcSelectJwk(keys, header.Kid) }; if selected == nil { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT signing key was not found\"} }")
	g.line("modulus, err := base64.RawURLEncoding.DecodeString(selected.N); if err != nil || len(modulus) == 0 { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"OIDC signing key is invalid\"} }; exponentBytes, err := base64.RawURLEncoding.DecodeString(selected.E); if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"OIDC signing key is invalid\"} }; exponent := 0; for _, value := range exponentBytes { exponent = exponent<<8 | int(value) }; if exponent < 3 { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"OIDC signing key is invalid\"} }")
	g.line("key := &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}; if key.N.BitLen() < 2048 || exponent%2 == 0 { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"OIDC signing key is invalid\"} }; digest := sha256.Sum256([]byte(segments[0] + \".\" + segments[1])); if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT signature is invalid\"} }")
	g.line("var claims map[string]any; if stdjson.Unmarshal(payloadBytes, &claims) != nil { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT claims are invalid\"} }")
	g.line("if value, ok := claims[\"iss\"].(string); !ok || value != issuer { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT issuer does not match\"} }")
	g.line("audienceMatches := false; switch value := claims[\"aud\"].(type) { case string: audienceMatches = value == audience; case []any: for _, item := range value { if text, ok := item.(string); ok && text == audience { audienceMatches = true } } }; if !audienceMatches { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT audience does not match\"} }")
	g.line("now := stdtime.Now().Unix(); skew := int64(clockSkewSeconds); expiration, ok := claims[\"exp\"].(float64); if !ok || int64(expiration) <= now-skew { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT is expired\"} }; if notBefore, ok := claims[\"nbf\"].(float64); ok && int64(notBefore) > now+skew { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT is not active\"} }")
	g.line("subject, ok := claims[\"sub\"].(string); if !ok || subject == \"\" { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT subject is missing\"} }")
	g.line("name, _ := claims[\"name\"].(string); email, _ := claims[\"email\"].(string); roles := []string{}; if raw, exists := claims[rolesClaim]; exists { values, ok := raw.([]any); if !ok { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT roles claim is invalid\"} }; for _, value := range values { role, ok := value.(string); if !ok { return trbOidcPrincipalData{}, &trbOidcFailure{kind: \"invalid\", message: \"JWT roles claim is invalid\"} }; roles = append(roles, role) } }")
	g.line("return trbOidcPrincipalData{subject: subject, name: name, email: email, roles: roles}, nil")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}
