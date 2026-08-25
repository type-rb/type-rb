package ruby

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

func TestPrivateTopLevelFunctionsUseModuleScopedNames(t *testing.T) {
	generate := func(modulePath string) string {
		return Generate(&ir.Program{
			Mode:       "ruby",
			ModulePath: modulePath,
			Statements: []ir.Statement{
				&ir.Method{Name: "_invalid"},
				&ir.Method{Name: "__trb_specialize_bind_42"},
			},
		})
	}

	firstTarget := rubyPrivateFunctionName("alpha", "_invalid")
	secondTarget := rubyPrivateFunctionName("beta", "_invalid")
	if firstTarget == secondTarget {
		t.Fatalf("private Ruby function targets collided: %q", firstTarget)
	}
	for modulePath, target := range map[string]string{"alpha": firstTarget, "beta": secondTarget} {
		output := generate(modulePath)
		if !strings.Contains(output, "def "+target+"(") || strings.Contains(output, "def _invalid(") {
			t.Fatalf("generated %s private function is not module scoped:\n%s", modulePath, output)
		}
		if !strings.Contains(output, "def __trb_specialize_bind_42(") {
			t.Fatalf("generated %s compiler-owned function was renamed:\n%s", modulePath, output)
		}
	}
}
