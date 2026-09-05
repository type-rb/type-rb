package cli

import (
	"testing"

	"github.com/type-rb/type-rb/internal/testsupport/resourceborrow"
)

func TestRunResourceBorrowAcrossBackends(t *testing.T) {
	for _, backend := range dirBoundaryBackends {
		t.Run(backend.name, func(t *testing.T) {
			requireDirBoundaryRuntime(t, backend)
			declarations, expression, expected := resourceborrow.Scenario(t)
			source := declarations + "\ndef main()\n\tputs(" + expression + ")\nend\n"
			if got := runDirBoundaryProject(t, backend, source); got != expected+"\n" {
				t.Fatalf("got %q, want %q", got, expected)
			}
		})
	}
}
