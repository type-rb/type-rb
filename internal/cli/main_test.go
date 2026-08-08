package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// Generated project roots remain isolated; only Go's content-addressed caches are shared.
	cacheRoot, err := os.MkdirTemp("", "trb-cli-test-go-cache-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create shared CLI test cache: %v\n", err)
		os.Exit(1)
	}

	for name, path := range map[string]string{
		"GOCACHE":    filepath.Join(cacheRoot, "build"),
		"GOMODCACHE": filepath.Join(cacheRoot, "modules"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "create %s for CLI tests: %v\n", name, err)
			_ = os.RemoveAll(cacheRoot)
			os.Exit(1)
		}
		if err := os.Setenv(name, path); err != nil {
			fmt.Fprintf(os.Stderr, "set %s for CLI tests: %v\n", name, err)
			_ = os.RemoveAll(cacheRoot)
			os.Exit(1)
		}
	}

	status := m.Run()
	if err := os.RemoveAll(cacheRoot); err != nil && status == 0 {
		fmt.Fprintf(os.Stderr, "remove shared CLI test cache: %v\n", err)
		status = 1
	}
	os.Exit(status)
}
