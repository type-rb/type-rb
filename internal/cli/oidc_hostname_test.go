package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/type-rb/type-rb/internal/project"
)

// Requests use a loopback proxy, so no test hostname reaches public DNS.
func TestOidcEndpointHostnamesAcrossRuntimes(t *testing.T) {
	const source = `import { OidcAuthErrorKind, OidcBearerOptions } from trb/auth/oidc
import { Body, Header, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import trb/web/auth/bearer
import trb/std/process
import trb/std/result

def main()
 args := Process.argv()
 mut uri: String? := args[1]
 if args[1] == ""
  uri = nil
 end
 options := OidcBearerOptions.new(issuer: args[0], audience: "api", jwks_uri: uri, roles_claim: "roles", clock_skew_seconds: 0)
 request := Request.new(method: HttpMethod.get(), path: "/", query_string: "",
  headers: Headers.new([Header.new(name: "Authorization", value: "Bearer " + args[2])]), body: Body.new("".to_bytes()))
 case Bearer.verify(request, options)
 when Result::Ok(value)
  puts("ok:" + value.subject)
 when Result::Err(error)
  case error.kind
  when OidcAuthErrorKind::Provider
   puts("provider:" + error.message)
  else
   puts("invalid:" + error.message)
  end
 end
end
`
	provider := newOidcBearerProvider(t)
	defer provider.Close()
	const issuer = "https://ＢＵＣＨＥＲ.example.invalid"
	token := provider.Token(t, issuer, "api", "subject", provider.kid)
	keys := map[string]any{"keys": []map[string]string{{"kid": provider.kid, "kty": "RSA", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(provider.key.PublicKey.N.Bytes()), "e": "AQAB"}}}
	var mu sync.Mutex
	var requests []string
	certificate, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{"bucher.example.invalid", "xn--bcher-kva.example.invalid"},
		IsCA:     true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true}, &provider.key.PublicKey, provider.key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})
	pair, err := tls.X509KeyPair(certPEM, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(provider.key)}))
	if err != nil {
		t.Fatal(err)
	}
	tlsProvider := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Host+" "+r.URL.EscapedPath()+"?"+r.URL.RawQuery)
		mu.Unlock()
		if r.URL.Path == "/.well-known/openid-configuration" {
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "jwks_uri": "https://bücher.example.invalid:8443/keys"})
		} else {
			_ = json.NewEncoder(w).Encode(keys)
		}
	}))
	tlsProvider.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		mu.Lock()
		requests = append(requests, "SNI "+hello.ServerName)
		mu.Unlock()
		return nil, nil
	}}
	tlsProvider.StartTLS()
	defer tlsProvider.Close()
	certificatePath := filepath.Join(t.TempDir(), "root.pem")
	if err := os.WriteFile(certificatePath, certPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	var proxy *httptest.Server
	proxy = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			upstreamAddress := tlsProvider.Listener.Addr().String()
			label := "CONNECT "
			if strings.HasSuffix(r.Host, ":80") || strings.HasSuffix(r.Host, ":8080") {
				upstreamAddress = proxy.Listener.Addr().String()
				label = "CONNECT-HTTP "
			}
			mu.Lock()
			requests = append(requests, label+r.Host)
			mu.Unlock()
			upstream, err := net.DialTimeout("tcp", upstreamAddress, time.Second)
			if err != nil {
				http.Error(w, "upstream", 502)
				return
			}
			client, buffer, err := w.(http.Hijacker).Hijack()
			if err != nil {
				upstream.Close()
				return
			}
			defer client.Close()
			defer upstream.Close()
			if _, err := buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
				return
			}
			if err := buffer.Flush(); err != nil {
				return
			}
			done := make(chan struct{})
			go func() { defer close(done); io.Copy(upstream, buffer); upstream.Close() }()
			io.Copy(client, upstream)
			client.Close()
			<-done
			return
		}
		mu.Lock()
		requests = append(requests, r.Host+" "+r.URL.EscapedPath()+"?"+r.URL.RawQuery)
		mu.Unlock()
		switch r.URL.Path {
		case "/redirect":
			w.Header().Set("Location", "http://%EF%BC%A2%EF%BC%B5%EF%BC%A3%EF%BC%A8%EF%BC%A5%EF%BC%B2.example.invalid/keys?after=%2f")
			w.WriteHeader(302)
		case "/invalid-redirect":
			w.Header().Set("Location", "http://bad_name.example.invalid/keys")
			w.WriteHeader(302)
		case "/loop":
			w.Header().Set("Location", "/loop")
			w.WriteHeader(302)
		case "/oversized":
			_, _ = io.WriteString(w, strings.Repeat(" ", (1<<20)+1))
		case "/slow-redirect", "/slow-keys":
			select {
			case <-r.Context().Done():
				return
			case <-time.After(3 * time.Second):
			}
			if r.URL.Path == "/slow-redirect" {
				w.Header().Set("Location", "/slow-keys")
				w.WriteHeader(302)
			} else {
				_ = json.NewEncoder(w).Encode(keys)
			}
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "jwks_uri": "http://bücher.example.invalid/keys"})
		default:
			_ = json.NewEncoder(w).Encode(keys)
		}
	}))
	defer proxy.Close()
	var ipv6URL string
	if listener, err := net.Listen("tcp6", "[::1]:0"); err == nil {
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(keys) }))
		server.Listener.Close()
		server.Listener = listener
		server.Start()
		defer server.Close()
		ipv6URL = server.URL
	} else {
		t.Logf("IPv6 loopback unavailable: %v", err)
	}
	valid := []struct{ input, ascii string }{
		{"ＢＵＣＨＥＲ.example.invalid", "bucher.example.invalid"},
		{"BÜCHER.example.invalid", "xn--bcher-kva.example.invalid"},
		{"bu\u0308cher.example.invalid", "xn--bcher-kva.example.invalid"},
		{"XN--BCHER-KVA.example.invalid", "xn--bcher-kva.example.invalid"},
		{"faß.example.invalid", "xn--fa-hia.example.invalid"},
		{"ς.example.invalid", "xn--3xa.example.invalid"},
		{"例え。example.invalid", "xn--r8jz45g.example.invalid"},
		{"Ａ.example.invalid.", "a.example.invalid."},
		{"bücher.example.invalid:8080", "xn--bcher-kva.example.invalid:8080"},
	}
	invalid := []string{"bad_name.example.invalid", "-bad.example.invalid", "ab--cd.example.invalid", "bad..example.invalid", "xn--.example.invalid", "a\u200cb.example.invalid", "אa.example.invalid", "\u0301a.example.invalid", strings.Repeat("a", 64) + ".example.invalid", "127.1", "0x7f000001", "0177.0.0.1", "example.123", "[127.0.0.1]", "[fe80::1%25lo0]", "%62ucher.example.invalid", "%FF.example.invalid"}
	for _, backend := range filesystemRuntimeBackends {
		t.Run(backend.name, func(t *testing.T) {
			if _, err := exec.LookPath(backend.executable); err != nil {
				t.Skipf("%s unavailable", backend.executable)
			}
			root := t.TempDir()
			config := project.New(root, backend.mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/oidc-hostnames"
			}
			if backend.bun {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				config.TypeScript.PackageManager = "bun"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			cli := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			args := []string{"build", "--config", config.Path}
			executable := filepath.Join(root, "application")
			if backend.mode == "go" {
				args = append(args, "--compile", "--outfile", executable)
			}
			if code := cli.Run(args); code != 0 {
				t.Fatalf("build %d: %s\n%s", code, stdout.String(), stderr.String())
			}
			if backend.mode == "typescript" {
				if code := cli.Run([]string{"install", "--config", config.Path}); code != 0 {
					t.Fatalf("install %d: %s", code, stderr.String())
				}
			}
			command := []string{backend.executable}
			if backend.mode == "go" {
				command = []string{executable}
			} else {
				if backend.executable == "node" {
					command = append(command, "--use-env-proxy", "--experimental-strip-types")
				}
				command = append(command, generatedEntrypoint(t, config.OutputPath(), backend.mode))
			}
			environment := []string{}
			for _, value := range os.Environ() {
				key := strings.ToLower(strings.SplitN(value, "=", 2)[0])
				if strings.HasSuffix(key, "_proxy") || key == "request_method" || key == "node_options" {
					continue
				}
				environment = append(environment, value)
			}
			environment = append(environment, "HTTP_PROXY="+proxy.URL, "HTTPS_PROXY="+proxy.URL, "NO_PROXY=127.0.0.1,::1,[::1]", "SSL_CERT_FILE="+certificatePath, "NODE_EXTRA_CA_CERTS="+certificatePath)
			run := func(t *testing.T, uri, configuredIssuer, bearer string, allowIdleTunnel ...bool) (string, []string) {
				t.Helper()
				mu.Lock()
				requests = nil
				mu.Unlock()
				ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, command[0], append(append([]string{}, command[1:]...), configuredIssuer, uri, bearer)...)
				cmd.Dir = root
				cmd.Env = environment
				var output, errors bytes.Buffer
				cmd.Stdout = &output
				cmd.Stderr = &errors
				if err := cmd.Run(); err != nil {
					t.Fatalf("run %q: %v\n%s\n%s", uri, err, output.String(), errors.String())
				}
				mu.Lock()
				observed := append([]string{}, requests...)
				mu.Unlock()
				filtered := []string{}
				for i, entry := range observed {
					if strings.HasPrefix(entry, "CONNECT-HTTP ") {
						if i+1 >= len(observed) {
							// Node may replenish its proxy pool as a request is
							// aborted. Permit only this known target in the deadline
							// control; an idle tunnel is not another HTTP request.
							if len(allowIdleTunnel) == 1 && allowIdleTunnel[0] && entry == "CONNECT-HTTP example.invalid:80" {
								continue
							}
							t.Fatalf("HTTP tunnel has no request: %q", observed)
						}
						host := strings.SplitN(observed[i+1], " ", 2)[0]
						if !strings.Contains(host, ":") {
							host += ":80"
						}
						if entry != "CONNECT-HTTP "+host {
							t.Fatalf("CONNECT and HTTP target differ: %q", observed)
						}
					} else {
						filtered = append(filtered, entry)
					}
				}
				return strings.TrimSpace(output.String()), filtered
			}
			for _, fixture := range valid {
				t.Run(fixture.input, func(t *testing.T) {
					output, observed := run(t, "http://"+fixture.input+"/keys%2fpart?q=%2f", issuer, token)
					want := fixture.ascii + " /keys%2fpart?q=%2f"
					if output != "ok:subject" || len(observed) != 1 || observed[0] != want {
						t.Fatalf("output=%s requests=%q want=%q", output, observed, want)
					}
				})
			}
			for _, host := range invalid {
				t.Run(host, func(t *testing.T) {
					output, observed := run(t, "http://"+host+"/keys", issuer, token)
					if !strings.HasPrefix(output, "provider:") || len(observed) != 0 {
						t.Fatalf("invalid host output=%s requests=%q", output, observed)
					}
				})
			}
			output, observed := run(t, "http://example.invalid/redirect", issuer, token)
			if output != "ok:subject" || len(observed) != 2 || observed[1] != "bucher.example.invalid /keys?after=%2f" {
				t.Fatalf("redirect output=%s requests=%q", output, observed)
			}
			output, observed = run(t, "http://example.invalid/invalid-redirect", issuer, token)
			if !strings.HasPrefix(output, "provider:") || len(observed) != 1 {
				t.Fatalf("invalid redirect output=%s requests=%q", output, observed)
			}
			output, observed = run(t, "http://example.invalid/loop", issuer, token)
			if !strings.HasPrefix(output, "provider:") || len(observed) != 10 {
				t.Fatalf("redirect limit output=%s requests=%q", output, observed)
			}
			output, _ = run(t, "http://bücher.example.invalid/keys", issuer, provider.Token(t, "https://bucher.example.invalid", "api", "subject", provider.kid))
			if output != "invalid:JWT issuer does not match" {
				t.Fatalf("issuer was normalized: %s", output)
			}
			// A fullwidth IPv4 endpoint is mapped before the direct loopback connection.
			direct := strings.ReplaceAll(provider.URL, "127.0.0.1", "１２７。０。０。１") + "/jwks"
			beforeDirect := provider.JWKSRequests()
			output, observed = run(t, direct, issuer, token)
			if output != "ok:subject" || len(observed) != 0 || provider.JWKSRequests() != beforeDirect+1 {
				t.Fatalf("direct mapped IPv4: %s requests=%q", output, observed)
			}
			output, observed = run(t, "https://ＢＵＣＨＥＲ.example.invalid:8443/keys", issuer, token)
			wantTLS := []string{"CONNECT bucher.example.invalid:8443", "SNI bucher.example.invalid", "bucher.example.invalid:8443 /keys?"}
			if output != "ok:subject" || strings.Join(observed, "\n") != strings.Join(wantTLS, "\n") {
				t.Fatalf("TLS output=%s observations=%q", output, observed)
			}
			output, observed = run(t, "https://wrong.example.invalid:8443/keys", issuer, token)
			if !strings.HasPrefix(output, "provider:") || len(observed) != 2 {
				t.Fatalf("certificate mismatch output=%s observations=%q", output, observed)
			}
			output, observed = run(t, "", issuer, token)
			if output != "ok:subject" || len(observed) != 6 {
				t.Fatalf("discovery identity output=%s observations=%q", output, observed)
			}
			output, _ = run(t, "", "https://bucher.example.invalid", token)
			if output != "provider:OIDC discovery response is invalid" {
				t.Fatalf("discovery issuer was normalized: %s", output)
			}
			if ipv6URL != "" {
				output, observed = run(t, ipv6URL+"/keys", issuer, token)
				if output != "ok:subject" || len(observed) != 0 {
					t.Fatalf("IPv6 direct: %s requests=%q", output, observed)
				}
			}
			t.Run("response-size-limit", func(t *testing.T) {
				output, observed := run(t, "http://example.invalid/oversized", issuer, token)
				if output != "provider:load OIDC JWKS response is too large" || len(observed) != 1 {
					t.Fatalf("size limit output=%s requests=%q", output, observed)
				}
			})
			t.Run("deadline-includes-redirects", func(t *testing.T) {
				// Each response fits the budget separately; their combined six
				// seconds must fail under the single five-second request budget.
				output, observed := run(t, "http://example.invalid/slow-redirect", issuer, token, true)
				if !strings.HasPrefix(output, "provider:") || len(observed) != 2 {
					t.Fatalf("redirect deadline output=%s requests=%q", output, observed)
				}
			})
			t.Logf("qualified %d valid/invalid hosts, redirects, issuer identity, IPv4/IPv6, TLS, size and deadline limits", len(valid)+len(invalid))
		})
	}
}
