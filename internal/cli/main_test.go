package cli

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("TRB_TEST_SQLDEF_HELPER") == "1" {
		runTestSqldefHelper()
		return
	}
	if os.Getenv("TRB_TEST_SIGNAL_HELPER") == "1" {
		runTestSignalHelper()
		return
	}
	if os.Getenv("TRB_TEST_JOBS_CLI_HELPER") == "1" {
		os.Exit(New().Run(os.Args[1:]))
	}
	// Go's content-addressed caches are shared; generated project workspaces
	// remain isolated. Honor caller configuration and otherwise use Go defaults.
	os.Exit(m.Run())
}
