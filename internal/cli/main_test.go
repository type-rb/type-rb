package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("TRB_TEST_SQLDEF_HELPER") == "1" {
		runTestSqldefHelper()
		return
	}
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
	if err := removeTestCache(cacheRoot); err != nil && status == 0 {
		fmt.Fprintf(os.Stderr, "remove shared CLI test cache: %v\n", err)
		status = 1
	}
	os.Exit(status)
}

func removeTestCache(root string) error {
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		return nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(root)
}
