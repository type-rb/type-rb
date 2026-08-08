package cli

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/type-rb/type-rb/internal/project"
)

func TestOfficialWebServerLifecycleAcrossAvailableBackends(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("portable signal lifecycle test is not supported on Windows")
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireWebServerRuntime(t, mode)
			port := availableTCPPort(t)
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if mode == "go" {
				config.Go.Module = "example.com/type-rb/web-server-lifecycle"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			routeDirectory := filepath.Join(root, "src", "routes")
			if err := os.MkdirAll(routeDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
			mainSource := fmt.Sprintf(`import { configure_server, serve } from trb/web

def main()
	serve(configure_server(host: "127.0.0.1", port: %d, body_limit_bytes: 8, shutdown_timeout_milliseconds: 500))
	return
end
`, port)
			if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
				t.Fatal(err)
			}
			routeSource := `import { Context, Response, text } from trb/web

def get(_context: Context): Response
	return text("ok")
end
`
			if err := os.WriteFile(filepath.Join(routeDirectory, "health.trb"), []byte(routeSource), 0o644); err != nil {
				t.Fatal(err)
			}

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
				if running {
					if err := server.Process.Kill(); err == nil {
						<-wait
					}
				}
			})

			client := &http.Client{
				Timeout: 2 * time.Second,
				Transport: &http.Transport{
					DisableKeepAlives: true,
				},
			}
			url := "http://127.0.0.1:" + strconv.Itoa(port) + "/health"
			waitForWebServer(t, client, url, wait, &serverOutput)

			response, err := client.Get(url)
			if err != nil {
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr != nil || response.StatusCode != 200 || string(body) != "ok" {
				t.Fatalf("unexpected health response: status=%d body=%q err=%v", response.StatusCode, body, readErr)
			}

			response, err = client.Post(url, "text/plain", strings.NewReader("123456789"))
			if err != nil {
				t.Fatal(err)
			}
			body, readErr = io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr != nil || response.StatusCode != 413 || !bytes.Contains(body, []byte("payload_too_large")) {
				t.Fatalf("unexpected body-limit response: status=%d body=%q err=%v", response.StatusCode, body, readErr)
			}

			if err := server.Process.Signal(os.Interrupt); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-wait:
				running = false
				if err != nil {
					t.Fatalf("server did not stop cleanly: %v\n%s", err, serverOutput.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("server did not stop before the lifecycle deadline\n%s", serverOutput.String())
			}
		})
	}
}

func TestOfficialWebServerRejectsInvalidConfigAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireWebServerRuntime(t, mode)
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if mode == "go" {
				config.Go.Module = "example.com/type-rb/web-server-invalid-config"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
				t.Fatal(err)
			}
			source := `import { configure_server, serve } from trb/web

def main()
	serve(configure_server(port: 0))
	return
end
`
			if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status == 0 {
				t.Fatalf("invalid server config succeeded: stdout=%s stderr=%s", stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), "trb/web ServerConfig.port must be between 1 and 65535") {
				t.Fatalf("unexpected invalid-config diagnostic: %s", stderr.String())
			}
		})
	}
}

func requireWebServerRuntime(t *testing.T, mode string) {
	t.Helper()
	name := map[string]string{"go": "go", "ruby": "ruby", "typescript": "node"}[mode]
	if _, err := exec.LookPath(name); err != nil {
		t.Skip(name + " is not installed")
	}
}

func availableTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func webServerCommand(t *testing.T, mode, buildDirectory string) *exec.Cmd {
	t.Helper()
	switch mode {
	case "go":
		output := filepath.Join(t.TempDir(), "server")
		build := exec.Command("go", "build", "-o", output, ".")
		build.Dir = buildDirectory
		if result, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build failed: %v\n%s", err, result)
		}
		return exec.Command(output)
	case "ruby":
		return exec.Command("ruby", filepath.Join(buildDirectory, "main.rb"))
	case "typescript":
		return exec.Command("node", "--experimental-strip-types", filepath.Join(buildDirectory, "main.ts"))
	default:
		t.Fatalf("unknown mode %q", mode)
		return nil
	}
}

func waitForWebServer(t *testing.T, client *http.Client, url string, wait <-chan error, output *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-wait:
			t.Fatalf("server exited before accepting requests: %v\n%s", err, output.String())
		default:
		}
		response, err := client.Get(url)
		if err == nil {
			_ = response.Body.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("server did not start before the lifecycle deadline\n%s", output.String())
}
