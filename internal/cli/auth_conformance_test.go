package cli

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/type-rb/type-rb/internal/project"
)

func TestOfficialOidcAuthenticationAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			requireWebServerRuntime(t, mode)
			identity := newOIDCTestProvider(t)
			defer identity.Close()

			port := availableTCPPort(t)
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/auth-conformance"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			writeOidcTestApplication(t, config.SourcePath(), port, identity.URL)

			var buildStdout, buildStderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &buildStdout, Stderr: &buildStderr}
			if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
				t.Fatalf("build status=%d stdout=%s stderr=%s", status, buildStdout.String(), buildStderr.String())
			}

			server := webServerCommand(t, mode, filepath.Join(root, "build"))
			var serverOutput bytes.Buffer
			server.Stdout = &serverOutput
			server.Stderr = &serverOutput
			if err := server.Start(); err != nil {
				t.Fatal(err)
			}
			wait := make(chan error, 1)
			go func() { wait <- server.Wait() }()
			running := true
			t.Cleanup(func() {
				if running && server.Process.Kill() == nil {
					<-wait
				}
			})

			jar, err := cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			client := &http.Client{
				Timeout: 3 * time.Second,
				Jar:     jar,
				CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
			baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
			waitForWebServer(t, client, baseURL+"/health", wait, &serverOutput)

			assertOidcResponse(t, client, http.MethodGet, baseURL+"/bearer", "", nil, 401, "unauthorized")
			validBearer := identity.Token(t, identity.URL, "api", "bearer-user", "")
			assertOidcResponse(t, client, http.MethodGet, baseURL+"/bearer", "Bearer "+validBearer, nil, 200, "bearer ok")
			bearerSegments := strings.Split(validBearer, ".")
			tamperedSignature := "A" + bearerSegments[2][1:]
			if strings.HasPrefix(bearerSegments[2], "A") {
				tamperedSignature = "B" + bearerSegments[2][1:]
			}
			tamperedBearer := bearerSegments[0] + "." + bearerSegments[1] + "." + tamperedSignature
			assertOidcResponse(t, client, http.MethodGet, baseURL+"/bearer", "Bearer "+tamperedBearer, nil, 401, "unauthorized")
			wrongAudience := identity.Token(t, identity.URL, "other-api", "bearer-user", "")
			assertOidcResponse(t, client, http.MethodGet, baseURL+"/bearer", "Bearer "+wrongAudience, nil, 401, "unauthorized")

			loginResponse := assertOidcResponse(t, client, http.MethodGet, baseURL+"/auth/login", "", nil, 302, "")
			authorizationURL, err := url.Parse(loginResponse.Header.Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			if authorizationURL.Query().Get("code_challenge_method") != "S256" {
				t.Fatalf("login did not request PKCE S256: %s", authorizationURL)
			}
			identity.ExpectLogin(authorizationURL.Query().Get("nonce"), authorizationURL.Query().Get("code_challenge"))
			state := authorizationURL.Query().Get("state")
			invalidCallbackURL := baseURL + "/auth/callback?code=valid-code&state=" + url.QueryEscape(state+"-tampered")
			assertOidcResponse(t, client, http.MethodGet, invalidCallbackURL, "", nil, 400, "invalid_oidc_state")
			callbackURL := baseURL + "/auth/callback?code=valid-code&state=" + url.QueryEscape(state)
			callbackResponse := assertOidcResponse(t, client, http.MethodGet, callbackURL, "", nil, 302, "")
			if callbackResponse.Header.Get("Location") != "/" {
				t.Fatalf("unexpected callback location %q", callbackResponse.Header.Get("Location"))
			}

			assertOidcResponse(t, client, http.MethodGet, baseURL+"/session", "", nil, 200, "session ok")
			assertOidcResponse(t, client, http.MethodPost, baseURL+"/session", "", nil, 403, "invalid_csrf_token")
			csrf := cookieValue(client.Jar, baseURL, "trb_csrf")
			if csrf == "" {
				t.Fatal("session login did not set the CSRF cookie")
			}
			assertOidcResponse(t, client, http.MethodPost, baseURL+"/session", "", map[string]string{"X-CSRF-Token": csrf}, 200, "session ok")

			assertOidcResponse(t, client, http.MethodGet, baseURL+"/auth/logout", "", nil, 302, "")
			assertOidcResponse(t, client, http.MethodGet, baseURL+"/session", "", nil, 401, "unauthorized")

			if err := server.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			<-wait
			running = false
		})
	}
}

func writeOidcTestApplication(t *testing.T, sourceDirectory string, port int, issuer string) {
	t.Helper()
	redirectURI := "http://127.0.0.1:" + strconv.Itoa(port) + "/auth/callback"
	files := map[string]string{
		"main.trb": fmt.Sprintf(`import { configure_server, serve } from trb/web

def main()
	serve(configure_server(host: "127.0.0.1", port: %d))
	return
end
`, port),
		"routes/health.trb": `import { Context, Response, text } from trb/web

def get(_context: Context): Response
	return text("ok")
end
`,
		"routes/bearer/index.trb": `import { Context, Response, text } from trb/web

def get(_context: Context): Response
	return text("bearer ok")
end
`,
		"routes/bearer/_middleware.trb": fmt.Sprintf(`import { OidcBearerOptions } from trb/auth/oidc
import { Context, Next, Response } from trb/web
import trb/web/auth/bearer

BEARER_AUTH := OidcBearerOptions.new(
	issuer: %q,
	audience: "api",
	jwks_uri: %q,
	roles_claim: "roles",
)

def call(context: Context, next_handler: Next): Response
	return bearer.authenticate(context, next_handler, BEARER_AUTH)
end
`, issuer, issuer+"/jwks"),
		"routes/session/index.trb": `import { Context, Response, text } from trb/web

def get(_context: Context): Response
	return text("session ok")
end

def post(_context: Context): Response
	return text("session ok")
end
`,
		"routes/session/_middleware.trb": fmt.Sprintf(`import { OidcSessionOptions } from trb/auth/oidc
import { Context, Next, Response } from trb/web
import trb/web/auth/session

SESSION_AUTH := %s

def call(context: Context, next_handler: Next): Response
	return session.authenticate(context, next_handler, SESSION_AUTH)
end
`, oidcSessionOptionsSource(issuer, redirectURI)),
		"routes/auth/login.trb": fmt.Sprintf(`import { OidcSessionOptions } from trb/auth/oidc
import { Context, Response } from trb/web
import trb/web/auth/session

LOGIN_AUTH := %s

def get(context: Context): Response
	return session.start_login(context, LOGIN_AUTH, "/")
end
`, oidcSessionOptionsSource(issuer, redirectURI)),
		"routes/auth/callback.trb": fmt.Sprintf(`import { OidcSessionOptions } from trb/auth/oidc
import { Context, Response } from trb/web
import trb/web/auth/session

CALLBACK_AUTH := %s

def get(context: Context): Response
	return session.complete_login(context, CALLBACK_AUTH)
end
`, oidcSessionOptionsSource(issuer, redirectURI)),
		"routes/auth/logout.trb": fmt.Sprintf(`import { OidcSessionOptions } from trb/auth/oidc
import { Context, Response } from trb/web
import trb/web/auth/session

LOGOUT_AUTH := %s

def get(context: Context): Response
	return session.end_session(context, LOGOUT_AUTH)
end
`, oidcSessionOptionsSource(issuer, redirectURI)),
	}
	for name, source := range files {
		path := filepath.Join(sourceDirectory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func oidcSessionOptionsSource(issuer, redirectURI string) string {
	return fmt.Sprintf(`OidcSessionOptions.new(
	issuer: %q,
	client_id: "server-client",
	client_secret: "server-secret",
	authorization_endpoint: %q,
	token_endpoint: %q,
	jwks_uri: %q,
	redirect_uri: %q,
	post_logout_redirect_uri: "http://127.0.0.1/",
	end_session_endpoint: nil,
	scope: "openid profile email",
	audience: nil,
	roles_claim: "roles",
	cookie_name: "trb_session",
	cookie_secret: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	secure: false,
)`, issuer, issuer+"/authorize", issuer+"/token", issuer+"/jwks", redirectURI)
}

func assertOidcResponse(t *testing.T, client *http.Client, method, target, authorization string, headers map[string]string, status int, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	content, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != status || (body != "" && !strings.Contains(string(content), body)) {
		t.Fatalf("unexpected %s %s response: status=%d body=%q", method, target, response.StatusCode, content)
	}
	return response
}

func cookieValue(jar http.CookieJar, rawURL, name string) string {
	target, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	for _, cookie := range jar.Cookies(target) {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

type oidcTestProvider struct {
	*httptest.Server
	key       *rsa.PrivateKey
	mu        sync.Mutex
	nonce     string
	challenge string
}

func newOIDCTestProvider(t *testing.T) *oidcTestProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := &oidcTestProvider{key: key}
	provider.Server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/jwks":
			modulus := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
			exponent := base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1})
			_ = json.NewEncoder(response).Encode(map[string]any{"keys": []map[string]string{{"kid": "test-key", "kty": "RSA", "use": "sig", "alg": "RS256", "n": modulus, "e": exponent}}})
		case "/token":
			if username, password, ok := request.BasicAuth(); !ok || username != "server-client" || password != "server-secret" {
				http.Error(response, "invalid client", http.StatusUnauthorized)
				return
			}
			if err := request.ParseForm(); err != nil || request.Form.Get("code") != "valid-code" || request.Form.Get("grant_type") != "authorization_code" {
				http.Error(response, "invalid request", http.StatusBadRequest)
				return
			}
			provider.mu.Lock()
			nonce, challenge := provider.nonce, provider.challenge
			provider.mu.Unlock()
			digest := sha256.Sum256([]byte(request.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(digest[:]) != challenge {
				http.Error(response, "invalid verifier", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(response).Encode(map[string]string{
				"id_token":     provider.Token(t, provider.URL, "server-client", "session-user", nonce),
				"access_token": "opaque-access-token",
			})
		default:
			http.NotFound(response, request)
		}
	}))
	return provider
}

func (provider *oidcTestProvider) ExpectLogin(nonce, challenge string) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.nonce = nonce
	provider.challenge = challenge
}

func (provider *oidcTestProvider) Token(t *testing.T, issuer, audience, subject, nonce string) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test-key", "typ": "JWT"})
	claims := map[string]any{
		"iss":   issuer,
		"aud":   audience,
		"sub":   subject,
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"name":  "TypeRB User",
		"email": "user@example.com",
		"roles": []string{"user"},
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	payload, _ := json.Marshal(claims)
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	message := encodedHeader + "." + encodedPayload
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, provider.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return message + "." + base64.RawURLEncoding.EncodeToString(signature)
}
