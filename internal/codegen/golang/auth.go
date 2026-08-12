package golang

import (
	pathpkg "path"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) authIntrinsic(name string, call *ir.Call, arguments []string) (string, bool) {
	if !strings.HasPrefix(name, "trb.web.auth.bearer.") && !strings.HasPrefix(name, "trb.web.auth.session.") {
		return "", false
	}
	g.oidcRuntime = true
	webPath := pathpkg.Join(g.goModule, "trb/web")
	g.requireImport(webPath, "web")
	context := arguments[0]
	options := arguments[len(arguments)-1]
	if strings.HasPrefix(name, "trb.web.auth.session.") {
		if name == "trb.web.auth.session.start_login" {
			options = arguments[1]
		}
		return g.sessionAuthIntrinsic(name, call, arguments, context, options), true
	}
	verification := "trbOidcVerifyBearer(" + context + ".Request.Headers, " + options + ".Issuer, " + options + ".Audience, " + options + ".JwksUri, " + options + ".RolesClaim)"
	if name == "trb.web.auth.bearer.authenticate" {
		next := arguments[1]
		return "func() web.Response { _, failure := " + verification + "; if failure != nil { return web.Response{Status: 401, Headers: map[string][]string{\"content-type\": {\"application/json; charset=utf-8\"}, \"www-authenticate\": {\"Bearer\"}}, Body: []byte(\"{\\\"error\\\":\\\"unauthorized\\\"}\")} }; return " + next + ".Call(" + context + ") }()", true
	}
	resultType := g.goType(call.ExprType())
	principalType := g.goType(types.FromName("OidcPrincipal"))
	errorType := g.goType(types.FromName("OidcAuthError"))
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	errorAlias := g.typeAliases["OidcAuthError"]
	if errorAlias == "" {
		errorAlias = strings.TrimSuffix(errorType, ".OidcAuthError")
	}
	missing := errorAlias + "." + goConstantIdentifier("OidcAuthError", "MissingCredentials")
	invalid := errorAlias + ".NewOidcAuthErrorInvalidCredentials(failure.message)"
	return "func() " + resultType + " { data, failure := " + verification + "; if failure != nil { authError := " + invalid + "; if failure.kind == \"missing\" { authError = " + missing + " }; return " + resultAlias + ".NewResultErr[" + principalType + ", " + errorType + "](authError) }; var name *string; if data.name != \"\" { value := data.name; name = &value }; var email *string; if data.email != \"\" { value := data.email; email = &value }; principal := " + principalType + "{Subject: data.subject, Name: name, Email: email, Roles: data.roles}; return " + resultAlias + ".NewResultOk[" + principalType + ", " + errorType + "](principal) }()", true
}

func (g *generator) sessionAuthIntrinsic(name string, call *ir.Call, arguments []string, context, options string) string {
	base := context + ".Request, " + options
	switch name {
	case "trb.web.auth.session.start_login":
		return "trbOidcStartLogin(" + context + ".Request, " + options + ".AuthorizationEndpoint, " + options + ".ClientId, " + options + ".RedirectUri, " + options + ".Scope, " + nullableGoString(options+".Audience") + ", " + options + ".CookieName, " + options + ".CookieSecret, " + boolString(options+".Secure") + ", " + arguments[2] + ")"
	case "trb.web.auth.session.complete_login":
		return "trbOidcCompleteLogin(" + base + ".Issuer, " + options + ".ClientId, " + options + ".ClientSecret, " + options + ".RedirectUri, " + options + ".TokenEndpoint, " + options + ".JwksUri, " + options + ".RolesClaim, " + options + ".CookieName, " + options + ".CookieSecret, " + boolString(options+".Secure") + ")"
	case "trb.web.auth.session.end_session":
		return "trbOidcEndSession(" + context + ".Request, " + options + ".CookieName, " + options + ".CookieSecret, " + boolString(options+".Secure") + ", " + nullableGoString(options+".EndSessionEndpoint") + ", " + options + ".PostLogoutRedirectUri, " + options + ".ClientId)"
	}
	verification := "trbOidcSessionPrincipal(" + context + ".Request.Headers, " + options + ".CookieName, " + options + ".CookieSecret)"
	if name == "trb.web.auth.session.authenticate" {
		next := arguments[1]
		return "func() web.Response { _, session, failure := " + verification + "; if failure != nil { return trbOidcAuthResponse(401, \"unauthorized\") }; if !trbOidcCsrfValid(" + context + ".Request, session.Csrf) { return trbOidcAuthResponse(403, \"invalid_csrf_token\") }; return " + next + ".Call(" + context + ") }()"
	}
	resultType := g.goType(call.ExprType())
	principalType := g.goType(types.FromName("OidcPrincipal"))
	errorType := g.goType(types.FromName("OidcAuthError"))
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	errorAlias := g.typeAliases["OidcAuthError"]
	if errorAlias == "" {
		errorAlias = strings.TrimSuffix(errorType, ".OidcAuthError")
	}
	return "func() " + resultType + " { data, _, failure := " + verification + "; if failure != nil { authError := " + errorAlias + ".NewOidcAuthErrorInvalidCredentials(failure.message); if failure.kind == \"missing\" { authError = " + errorAlias + "." + goConstantIdentifier("OidcAuthError", "MissingCredentials") + " }; return " + resultAlias + ".NewResultErr[" + principalType + ", " + errorType + "](authError) }; var name *string; if data.name != \"\" { value := data.name; name = &value }; var email *string; if data.email != \"\" { value := data.email; email = &value }; principal := " + principalType + "{Subject: data.subject, Name: name, Email: email, Roles: data.roles}; return " + resultAlias + ".NewResultOk[" + principalType + ", " + errorType + "](principal) }()"
}

func nullableGoString(value string) string {
	return "func(value *string) string { if value == nil { return \"\" }; return *value }(" + value + ")"
}

func boolString(value string) string { return value }

func (g *generator) oidcRuntimeSupport() {
	g.requireImport("crypto", "")
	g.requireImport("crypto/aes", "")
	g.requireImport("crypto/cipher", "")
	g.requireImport("crypto/rand", "")
	g.requireImport("crypto/rsa", "")
	g.requireImport("crypto/sha256", "")
	g.requireImport("crypto/subtle", "")
	g.requireImport("encoding/base64", "")
	g.requireImport("encoding/json", "")
	g.requireImport("fmt", "")
	g.requireImport("io", "")
	g.requireImport("math/big", "")
	g.requireImport("net/http", "")
	g.requireImport("net/url", "neturl")
	g.requireImport("strconv", "")
	g.requireImport("strings", "")
	g.requireImport("sync", "")
	g.requireImport("time", "")
	g.line(`type trbOidcPrincipalData struct { subject string; name string; email string; roles []string }`)
	g.line(`type trbOidcFailure struct { kind string; message string }`)
	g.line(`type trbOidcJwk struct { Kid string ` + "`json:\"kid\"`" + `; Kty string ` + "`json:\"kty\"`" + `; Use string ` + "`json:\"use\"`" + `; Alg string ` + "`json:\"alg\"`" + `; N string ` + "`json:\"n\"`" + `; E string ` + "`json:\"e\"`" + ` }`)
	g.line(`type trbOidcJwksCacheEntry struct { keys []trbOidcJwk; expires time.Time }`)
	g.line(`var trbOidcJwksCache = struct { sync.Mutex; entries map[string]trbOidcJwksCacheEntry }{entries: map[string]trbOidcJwksCacheEntry{}}`)
	g.line(`func trbOidcVerifyBearer(headers map[string][]string, issuer string, audience string, jwksURI string, rolesClaim string) (trbOidcPrincipalData, *trbOidcFailure) {`)
	g.indent++
	g.line(`values := headers["authorization"]`)
	g.line(`if len(values) == 0 { for name, candidates := range headers { if strings.EqualFold(name, "authorization") { values = append(values, candidates...) } } }`)
	g.line(`if len(values) != 1 { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "missing", message: "exactly one Authorization header is required"} }`)
	g.line(`parts := strings.Fields(values[0]); if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "Authorization header must use Bearer"} }`)
	g.line(`return trbOidcVerifyJWT(parts[1], issuer, audience, jwksURI, rolesClaim, "")`)
	g.indent--
	g.line(`}`)
	g.line(`func trbOidcVerifyJWT(token string, issuer string, audience string, jwksURI string, rolesClaim string, nonce string) (trbOidcPrincipalData, *trbOidcFailure) {`)
	g.indent++
	g.line(`segments := strings.Split(token, "."); if len(segments) != 3 { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWT must contain three segments"} }`)
	g.line(`headerBytes, err := base64.RawURLEncoding.DecodeString(segments[0]); if err != nil { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWT header is not base64url"} }`)
	g.line(`payloadBytes, err := base64.RawURLEncoding.DecodeString(segments[1]); if err != nil { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWT payload is not base64url"} }`)
	g.line(`signature, err := base64.RawURLEncoding.DecodeString(segments[2]); if err != nil { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWT signature is not base64url"} }`)
	g.line(`var header struct { Alg string ` + "`json:\"alg\"`" + `; Kid string ` + "`json:\"kid\"`" + ` }; if json.Unmarshal(headerBytes, &header) != nil || header.Alg != "RS256" || header.Kid == "" { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWT must use RS256 with a key id"} }`)
	g.line(`keys, failure := trbOidcLoadJwks(jwksURI); if failure != nil { return trbOidcPrincipalData{}, failure }`)
	g.line(`var selected *trbOidcJwk; for index := range keys { if keys[index].Kid == header.Kid && keys[index].Kty == "RSA" && (keys[index].Use == "" || keys[index].Use == "sig") && (keys[index].Alg == "" || keys[index].Alg == "RS256") { selected = &keys[index]; break } }; if selected == nil { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWT signing key was not found"} }`)
	g.line(`modulus, err := base64.RawURLEncoding.DecodeString(selected.N); if err != nil { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWKS modulus is invalid"} }; exponentBytes, err := base64.RawURLEncoding.DecodeString(selected.E); if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWKS exponent is invalid"} }; exponent := 0; for _, value := range exponentBytes { exponent = exponent<<8 | int(value) }`)
	g.line(`digest := sha256.Sum256([]byte(segments[0] + "." + segments[1])); publicKey := &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}; if rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature) != nil { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWT signature is invalid"} }`)
	g.line(`var claims map[string]any; if json.Unmarshal(payloadBytes, &claims) != nil { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWT claims are invalid"} }`)
	g.line(`if value, ok := claims["iss"].(string); !ok || value != issuer { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWT issuer does not match"} }`)
	g.line(`if !trbOidcAudienceMatches(claims["aud"], audience) { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWT audience does not match"} }`)
	g.line(`if values, ok := claims["aud"].([]any); nonce != "" && ok && len(values) > 1 { if authorizedParty, ok := claims["azp"].(string); !ok || authorizedParty != audience { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "OIDC authorized party does not match"} } }`)
	g.line(`now := float64(time.Now().Unix()); exp, ok := claims["exp"].(float64); if !ok || exp <= now-60 { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWT is expired"} }; if nbf, ok := claims["nbf"].(float64); ok && nbf > now+60 { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWT is not active"} }; if nonce != "" { if value, ok := claims["nonce"].(string); !ok || value != nonce { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "OIDC nonce does not match"} } }`)
	g.line(`subject, ok := claims["sub"].(string); if !ok || subject == "" { return trbOidcPrincipalData{}, &trbOidcFailure{kind: "invalid", message: "JWT subject is missing"} }`)
	g.line(`data := trbOidcPrincipalData{subject: subject, roles: []string{}}; data.name, _ = claims["name"].(string); data.email, _ = claims["email"].(string); if values, ok := claims[rolesClaim].([]any); ok { for _, value := range values { if role, ok := value.(string); ok { data.roles = append(data.roles, role) } } }`)
	g.line(`return data, nil`)
	g.indent--
	g.line(`}`)
	g.line(`func trbOidcAudienceMatches(value any, audience string) bool { if text, ok := value.(string); ok { return text == audience }; if values, ok := value.([]any); ok { for _, value := range values { if text, ok := value.(string); ok && text == audience { return true } } }; return false }`)
	g.line(`func trbOidcLoadJwks(uri string) ([]trbOidcJwk, *trbOidcFailure) {`)
	g.indent++
	g.line(`if uri == "" { return nil, &trbOidcFailure{kind: "invalid", message: "JWKS URI is empty"} }; trbOidcJwksCache.Lock(); cached, ok := trbOidcJwksCache.entries[uri]; trbOidcJwksCache.Unlock(); if ok && time.Now().Before(cached.expires) { return cached.keys, nil }`)
	g.line(`client := &http.Client{Timeout: 5 * time.Second}; response, err := client.Get(uri); if err != nil { return nil, &trbOidcFailure{kind: "provider", message: fmt.Sprintf("load JWKS: %v", err)} }; defer response.Body.Close(); if response.StatusCode != http.StatusOK { return nil, &trbOidcFailure{kind: "provider", message: fmt.Sprintf("load JWKS: HTTP %d", response.StatusCode)} }; body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20)); if err != nil { return nil, &trbOidcFailure{kind: "provider", message: fmt.Sprintf("read JWKS: %v", err)} }`)
	g.line(`var document struct { Keys []trbOidcJwk ` + "`json:\"keys\"`" + ` }; if json.Unmarshal(body, &document) != nil || len(document.Keys) == 0 { return nil, &trbOidcFailure{kind: "provider", message: "JWKS response is invalid"} }; trbOidcJwksCache.Lock(); trbOidcJwksCache.entries[uri] = trbOidcJwksCacheEntry{keys: document.Keys, expires: time.Now().Add(5 * time.Minute)}; trbOidcJwksCache.Unlock(); return document.Keys, nil`)
	g.indent--
	g.line(`}`)
	g.b.WriteString(goOidcSessionRuntime)
	g.b.WriteByte('\n')
}

const goOidcSessionRuntime = `
type trbOidcLoginState struct {
	State string ` + "`json:\"state\"`" + `
	Nonce string ` + "`json:\"nonce\"`" + `
	Verifier string ` + "`json:\"verifier\"`" + `
	ReturnTo string ` + "`json:\"return_to\"`" + `
	Expires int64 ` + "`json:\"expires\"`" + `
}
type trbOidcSessionData struct {
	Subject string ` + "`json:\"subject\"`" + `
	Name string ` + "`json:\"name\"`" + `
	Email string ` + "`json:\"email\"`" + `
	Roles []string ` + "`json:\"roles\"`" + `
	Csrf string ` + "`json:\"csrf\"`" + `
	IdToken string ` + "`json:\"id_token\"`" + `
	Expires int64 ` + "`json:\"expires\"`" + `
}
func trbOidcRandom(size int) (string, error) { value := make([]byte, size); if _, err := rand.Read(value); err != nil { return "", err }; return base64.RawURLEncoding.EncodeToString(value), nil }
func trbOidcCookieSecret(value string) ([]byte, error) { decoded, err := base64.RawURLEncoding.DecodeString(value); if err != nil { decoded, err = base64.StdEncoding.DecodeString(value) }; if err != nil || len(decoded) != 32 { return nil, fmt.Errorf("OIDC cookie secret must encode exactly 32 bytes") }; return decoded, nil }
func trbOidcEncrypt(value any, secretText string) (string, error) { secret, err := trbOidcCookieSecret(secretText); if err != nil { return "", err }; block, err := aes.NewCipher(secret); if err != nil { return "", err }; var aead cipher.AEAD; aead, err = cipher.NewGCM(block); if err != nil { return "", err }; nonce := make([]byte, aead.NonceSize()); if _, err = rand.Read(nonce); err != nil { return "", err }; plain, err := json.Marshal(value); if err != nil { return "", err }; sealed := aead.Seal(nil, nonce, plain, nil); return base64.RawURLEncoding.EncodeToString(append(nonce, sealed...)), nil }
func trbOidcDecrypt(value string, secretText string, destination any) error { secret, err := trbOidcCookieSecret(secretText); if err != nil { return err }; encoded, err := base64.RawURLEncoding.DecodeString(value); if err != nil { return err }; block, err := aes.NewCipher(secret); if err != nil { return err }; aead, err := cipher.NewGCM(block); if err != nil { return err }; if len(encoded) < aead.NonceSize() { return fmt.Errorf("encrypted OIDC cookie is truncated") }; plain, err := aead.Open(nil, encoded[:aead.NonceSize()], encoded[aead.NonceSize():], nil); if err != nil { return err }; return json.Unmarshal(plain, destination) }
func trbOidcCookies(headers map[string][]string) map[string][]string { result := map[string][]string{}; values := []string{}; for name, entries := range headers { if strings.EqualFold(name, "cookie") { values = append(values, entries...) } }; for _, header := range values { for _, part := range strings.Split(header, ";") { pair := strings.SplitN(strings.TrimSpace(part), "=", 2); if len(pair) == 2 { result[pair[0]] = append(result[pair[0]], pair[1]) } } }; return result }
func trbOidcCookie(name string, value string, maxAge int, secure bool, httpOnly bool) string { parts := []string{name + "=" + value, "Path=/", fmt.Sprintf("Max-Age=%d", maxAge), "SameSite=Lax"}; if secure { parts = append(parts, "Secure") }; if httpOnly { parts = append(parts, "HttpOnly") }; return strings.Join(parts, "; ") }
func trbOidcAuthResponse(status int, code string) web.Response { return web.Response{Status: status, Headers: map[string][]string{"content-type": {"application/json; charset=utf-8"}}, Body: []byte("{\"error\":" + strconv.Quote(code) + "}")} }
func trbOidcStartLogin(request web.Request, endpoint string, clientId string, redirectURI string, scope string, audience string, cookieName string, secret string, secure bool, returnTo string) web.Response { if endpoint == "" || clientId == "" || redirectURI == "" || scope == "" || cookieName == "" { return trbOidcAuthResponse(500, "invalid_auth_configuration") }; if !strings.HasPrefix(returnTo, "/") || strings.HasPrefix(returnTo, "//") || strings.ContainsAny(returnTo, "\\\r\n") { returnTo = "/" }; state, err := trbOidcRandom(32); if err != nil { return trbOidcAuthResponse(500, "auth_unavailable") }; nonce, err := trbOidcRandom(32); if err != nil { return trbOidcAuthResponse(500, "auth_unavailable") }; verifier, err := trbOidcRandom(48); if err != nil { return trbOidcAuthResponse(500, "auth_unavailable") }; encrypted, err := trbOidcEncrypt(trbOidcLoginState{State: state, Nonce: nonce, Verifier: verifier, ReturnTo: returnTo, Expires: time.Now().Add(10 * time.Minute).Unix()}, secret); if err != nil { return trbOidcAuthResponse(500, "invalid_auth_configuration") }; challenge := sha256.Sum256([]byte(verifier)); query := neturl.Values{"response_type": {"code"}, "client_id": {clientId}, "redirect_uri": {redirectURI}, "scope": {scope}, "state": {state}, "nonce": {nonce}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}}; if audience != "" { query.Set("audience", audience) }; separator := "?"; if strings.Contains(endpoint, "?") { separator = "&" }; return web.Response{Status: 302, Headers: map[string][]string{"location": {endpoint + separator + query.Encode()}, "set-cookie": {trbOidcCookie(cookieName+"_state", encrypted, 600, secure, true)}}, Body: []byte{}} }
func trbOidcQueryValue(raw string, name string) (string, bool) { values, err := neturl.ParseQuery(raw); if err != nil { return "", false }; entries := values[name]; return func() (string, bool) { if len(entries) != 1 || entries[0] == "" { return "", false }; return entries[0], true }() }
func trbOidcCompleteLogin(request web.Request, issuer string, clientId string, clientSecret string, redirectURI string, tokenEndpoint string, jwksURI string, rolesClaim string, cookieName string, secret string, secure bool) web.Response { code, codeOK := trbOidcQueryValue(request.QueryString, "code"); state, stateOK := trbOidcQueryValue(request.QueryString, "state"); cookies := trbOidcCookies(request.Headers); stateCookies := cookies[cookieName+"_state"]; if !codeOK || !stateOK || len(stateCookies) != 1 { return trbOidcAuthResponse(400, "invalid_oidc_callback") }; var login trbOidcLoginState; if trbOidcDecrypt(stateCookies[0], secret, &login) != nil || login.Expires < time.Now().Unix() || subtle.ConstantTimeCompare([]byte(login.State), []byte(state)) != 1 { return trbOidcAuthResponse(400, "invalid_oidc_state") }; return trbOidcCompleteLoginExchange(issuer, clientId, clientSecret, redirectURI, tokenEndpoint, jwksURI, rolesClaim, cookieName, secret, secure, code, login) }
func trbOidcCompleteLoginExchange(issuer string, clientId string, clientSecret string, redirectURI string, tokenEndpoint string, jwksURI string, rolesClaim string, cookieName string, secret string, secure bool, code string, login trbOidcLoginState) web.Response { form := neturl.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI}, "code_verifier": {login.Verifier}}; tokenRequest, err := http.NewRequest(http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode())); if err != nil { return trbOidcAuthResponse(500, "invalid_auth_configuration") }; tokenRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded"); tokenRequest.SetBasicAuth(clientId, clientSecret); response, err := (&http.Client{Timeout: 10 * time.Second}).Do(tokenRequest); if err != nil { return trbOidcAuthResponse(502, "identity_provider_unavailable") }; defer response.Body.Close(); body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20)); if err != nil || response.StatusCode != http.StatusOK { return trbOidcAuthResponse(502, "identity_provider_error") }; var tokens struct { IdToken string ` + "`json:\"id_token\"`" + ` }; if json.Unmarshal(body, &tokens) != nil || tokens.IdToken == "" { return trbOidcAuthResponse(502, "identity_provider_error") }; principal, failure := trbOidcVerifyJWT(tokens.IdToken, issuer, clientId, jwksURI, rolesClaim, login.Nonce); if failure != nil { return trbOidcAuthResponse(400, "invalid_identity_token") }; csrf, err := trbOidcRandom(32); if err != nil { return trbOidcAuthResponse(500, "auth_unavailable") }; session := trbOidcSessionData{Subject: principal.subject, Name: principal.name, Email: principal.email, Roles: principal.roles, Csrf: csrf, IdToken: tokens.IdToken, Expires: time.Now().Add(8 * time.Hour).Unix()}; encrypted, err := trbOidcEncrypt(session, secret); if err != nil { return trbOidcAuthResponse(500, "invalid_auth_configuration") }; return web.Response{Status: 302, Headers: map[string][]string{"location": {login.ReturnTo}, "set-cookie": {trbOidcCookie(cookieName, encrypted, 28800, secure, true), trbOidcCookie("trb_csrf", neturl.QueryEscape(csrf), 28800, secure, false), trbOidcCookie(cookieName+"_state", "", 0, secure, true)}}, Body: []byte{}} }
func trbOidcSessionPrincipal(headers map[string][]string, cookieName string, secret string) (trbOidcPrincipalData, trbOidcSessionData, *trbOidcFailure) { cookies := trbOidcCookies(headers); values := cookies[cookieName]; if len(values) != 1 { return trbOidcPrincipalData{}, trbOidcSessionData{}, &trbOidcFailure{kind: "missing", message: "session cookie is missing"} }; var session trbOidcSessionData; if trbOidcDecrypt(values[0], secret, &session) != nil || session.Expires <= time.Now().Unix() || session.Subject == "" { return trbOidcPrincipalData{}, trbOidcSessionData{}, &trbOidcFailure{kind: "invalid", message: "session cookie is invalid"} }; return trbOidcPrincipalData{subject: session.Subject, name: session.Name, email: session.Email, roles: session.Roles}, session, nil }
func trbOidcCsrfValid(request web.Request, expected string) bool { method := strings.ToUpper(request.Method); if method == "GET" || method == "HEAD" || method == "OPTIONS" { return true }; cookies := trbOidcCookies(request.Headers); cookieValues := cookies["trb_csrf"]; headerValues := []string{}; for name, values := range request.Headers { if strings.EqualFold(name, "x-csrf-token") { headerValues = append(headerValues, values...) } }; if len(cookieValues) != 1 || len(headerValues) != 1 { return false }; cookieValue, err := neturl.QueryUnescape(cookieValues[0]); if err != nil { return false }; return subtle.ConstantTimeCompare([]byte(expected), []byte(cookieValue)) == 1 && subtle.ConstantTimeCompare([]byte(expected), []byte(headerValues[0])) == 1 }
func trbOidcEndSession(request web.Request, cookieName string, secret string, secure bool, endpoint string, redirectURI string, clientId string) web.Response { location := redirectURI; if endpoint != "" { values := neturl.Values{"post_logout_redirect_uri": {redirectURI}, "client_id": {clientId}}; if _, session, failure := trbOidcSessionPrincipal(request.Headers, cookieName, secret); failure == nil && session.IdToken != "" { values.Set("id_token_hint", session.IdToken) }; separator := "?"; if strings.Contains(endpoint, "?") { separator = "&" }; location = endpoint + separator + values.Encode() }; return web.Response{Status: 302, Headers: map[string][]string{"location": {location}, "set-cookie": {trbOidcCookie(cookieName, "", 0, secure, true), trbOidcCookie("trb_csrf", "", 0, secure, false)}}, Body: []byte{}} }
`
