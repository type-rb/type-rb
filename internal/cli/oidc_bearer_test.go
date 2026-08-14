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
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/type-rb/type-rb/internal/project"
)

func TestOidcBearerAuthenticationAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			if mode == "typescript" {
				if _, err := exec.LookPath("bun"); err != nil {
					t.Skip("bun is not installed")
				}
			} else {
				requireWebServerRuntime(t, mode)
			}
			identity := newOidcBearerProvider(t)
			defer identity.Close()

			port := availableTCPPort(t)
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if mode == "go" {
				config.Go.Module = "example.com/type-rb/oidc-bearer"
			}
			if mode == "typescript" {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				config.TypeScript.PackageManager = "bun"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			writeOidcBearerApplication(t, config.SourcePath(), port, identity.URL)

			var buildStdout, buildStderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &buildStdout, Stderr: &buildStderr}
			if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
				t.Fatalf("build status=%d stdout=%s stderr=%s", status, buildStdout.String(), buildStderr.String())
			}

			server := oidcBearerServerCommand(t, mode, filepath.Join(root, "build"))
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

			client := &http.Client{Timeout: 3 * time.Second}
			baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
			waitForWebServer(t, client, baseURL+"/health", wait, &serverOutput)

			response := oidcBearerRequest(t, client, baseURL+"/protected", "")
			if response.status != 401 || response.body != `{"error":"unauthorized"}` || response.authenticate != "Bearer" {
				t.Fatalf("missing bearer response=%#v", response)
			}

			valid := identity.Token(t, identity.URL, "api", "user-1", identity.kid)
			response = oidcBearerRequest(t, client, baseURL+"/protected", "Bearer "+valid)
			if response.status != 200 || response.body != "user-1:user,editor" {
				t.Fatalf("valid bearer response=%#v\n%s", response, serverOutput.String())
			}

			wrongAudience := identity.Token(t, identity.URL, "other", "user-1", identity.kid)
			if response = oidcBearerRequest(t, client, baseURL+"/protected", "Bearer "+wrongAudience); response.status != 401 {
				t.Fatalf("wrong audience response=%#v", response)
			}

			rotated := identity.RotateAndToken(t, identity.URL, "api", "user-2")
			if response = oidcBearerRequest(t, client, baseURL+"/protected", "Bearer "+rotated); response.status != 200 || response.body != "user-2:user,editor" {
				t.Fatalf("rotated key response=%#v", response)
			}
			requests := identity.JWKSRequests()
			unknown := identity.Token(t, identity.URL, "api", "unknown", "unknown-key")
			if response = oidcBearerRequest(t, client, baseURL+"/protected", "Bearer "+unknown); response.status != 401 {
				t.Fatalf("unknown key response=%#v", response)
			}
			if identity.JWKSRequests() != requests {
				t.Fatal("unknown signing keys bypassed the JWKS refresh rate limit")
			}

			if err := server.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			<-wait
			running = false
		})
	}
}

func oidcBearerServerCommand(t *testing.T, mode, buildDirectory string) *exec.Cmd {
	t.Helper()
	if mode == "typescript" {
		command := exec.Command("bun", "main.ts")
		command.Dir = buildDirectory
		return command
	}
	return webServerCommand(t, mode, buildDirectory)
}

func writeOidcBearerApplication(t *testing.T, sourceDirectory string, port int, issuer string) {
	t.Helper()
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
		"routes/protected/_middleware.trb": fmt.Sprintf(`import { OidcAuthError, OidcBearerOptions, OidcPrincipal, bearer_options } from trb/auth/oidc
import { Result } from trb/std/result
import { Context, Next, Request, Response } from trb/web
import trb/web/auth/bearer

AUTH := bearer_options(
	issuer: %q,
	audience: "api",
)

def verify_all(requests: Array<Request>, options: OidcBearerOptions): Array<Result<OidcPrincipal, OidcAuthError>>
	return requests.map do |request|
		verified := bearer.verify(request, options)
		verified
	end
end

def call(context: Context, next_handler: Next): Response
	return bearer.authenticate(context, next_handler, AUTH)
end
`, issuer),
		"routes/protected/index.trb": `import { Result } from trb/std/result
import { Context, Response, text } from trb/web
import trb/web/auth/bearer

def get(context: Context): Response
	case bearer.principal(context)
	when Result::Ok(value)
		return text(value.subject + ":" + value.roles.join(","))
	when Result::Err(_error)
		return text("missing principal", 500)
	end
end
`,
	}
	for name, source := range files {
		filename := filepath.Join(sourceDirectory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

type oidcBearerResponse struct {
	status       int
	body         string
	authenticate string
}

func oidcBearerRequest(t *testing.T, client *http.Client, target, authorization string) oidcBearerResponse {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return oidcBearerResponse{status: response.StatusCode, body: string(body), authenticate: response.Header.Get("WWW-Authenticate")}
}

type oidcBearerProvider struct {
	*httptest.Server
	mu           sync.Mutex
	key          *rsa.PrivateKey
	kid          string
	jwksRequests int
}

func newOidcBearerProvider(t *testing.T) *oidcBearerProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := &oidcBearerProvider{key: key, kid: "initial-key"}
	provider.Server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(response).Encode(map[string]string{"issuer": provider.URL, "jwks_uri": provider.URL + "/jwks"})
		case "/jwks":
			provider.mu.Lock()
			provider.jwksRequests++
			key, kid := provider.key, provider.kid
			provider.mu.Unlock()
			_ = json.NewEncoder(response).Encode(map[string]any{"keys": []map[string]string{{
				"kid": kid,
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1}),
			}}})
		default:
			http.NotFound(response, request)
		}
	}))
	return provider
}

func (provider *oidcBearerProvider) JWKSRequests() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.jwksRequests
}

func (provider *oidcBearerProvider) RotateAndToken(t *testing.T, issuer, audience, subject string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	provider.key = key
	provider.kid = "rotated-key"
	provider.mu.Unlock()
	return provider.Token(t, issuer, audience, subject, "rotated-key")
}

func (provider *oidcBearerProvider) Token(t *testing.T, issuer, audience, subject, kid string) string {
	t.Helper()
	provider.mu.Lock()
	key := provider.key
	provider.mu.Unlock()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iss":   issuer,
		"aud":   audience,
		"sub":   subject,
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"name":  "TypeRB User",
		"email": "user@example.com",
		"roles": []string{"user", "editor"},
	})
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claims)
	message := encodedHeader + "." + encodedClaims
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return message + "." + base64.RawURLEncoding.EncodeToString(signature)
}
