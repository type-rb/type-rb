package target

import "testing"

func TestDeclaredModesSeparateAnalysisFromBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript", "trb"} {
		if !IsDeclared(mode) {
			t.Fatalf("mode %s is not declared", mode)
		}
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if !IsBuilt(mode) {
			t.Fatalf("mode %s has no backend", mode)
		}
	}
	if IsBuilt("trb") {
		t.Fatal("mode trb unexpectedly has a backend")
	}
	if IsDeclared("") || IsDeclared("native") || IsBuilt("") {
		t.Fatal("unknown modes must not be declared or built")
	}
	if got := Unavailable("build", "trb").Error(); got != "build is not available for mode trb in this implementation" {
		t.Fatalf("unexpected message %q", got)
	}
	modes := Declared()
	modes[0] = "changed"
	if Declared()[0] != "go" {
		t.Fatal("Declared returned shared storage")
	}
}
