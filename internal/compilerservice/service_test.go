package compilerservice

import (
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/languageservice"
)

func TestServiceKeepsLastGoodContextAcrossInvalidOverlay(t *testing.T) {
	filename := "models/user.trb"
	unit := compiler.SourceUnit{
		Filename: filename, ModulePath: "models/user", Package: "models",
		Source: []byte("record User\n\tname: String\nend\n"),
	}
	service := New([]compiler.SourceUnit{unit}, compiler.Options{Mode: "go", GoModule: "example.com/service"})

	initial := service.Analyze()
	if initial.Version != 1 || initial.HasErrors() || initial.Stale {
		t.Fatalf("unexpected initial snapshot: %#v", initial)
	}
	context, ok := initial.Context("models/user")
	if !ok || !hasCompletion(context, "User") {
		t.Fatalf("initial context does not contain User: %#v", context)
	}
	if cached := service.Analyze(); cached.Version != initial.Version {
		t.Fatalf("cached snapshot version=%d, want %d", cached.Version, initial.Version)
	}

	unit.Source = []byte("record User\n\tname: String\nend\n\ndef broken(): String\n\treturn missing()\nend\n")
	service.SetDocument(unit)
	invalid := service.Analyze()
	if invalid.Version != 2 || !invalid.HasErrors() || !invalid.Stale {
		t.Fatalf("unexpected invalid snapshot: %#v", invalid)
	}
	if len(invalid.Diagnostics) != 1 || invalid.Diagnostics[0].Code != diagnostic.TypeError || invalid.Diagnostics[0].Path == "" {
		t.Fatalf("unexpected invalid diagnostics: %#v", invalid.Diagnostics)
	}
	context, ok = invalid.Context("models/user")
	if !ok || !hasCompletion(context, "User") {
		t.Fatal("invalid overlay discarded the last good language context")
	}

	service.CloseDocument(filename)
	restored := service.Analyze()
	if restored.Version != 3 || restored.HasErrors() || restored.Stale {
		t.Fatalf("unexpected restored snapshot: %#v", restored)
	}
}

func TestServiceAddsAndRemovesNewDocumentOverlays(t *testing.T) {
	service := New(nil, compiler.Options{Mode: "typescript"})
	unit := compiler.SourceUnit{Filename: "new.trb", ModulePath: "new", Source: []byte("def greeting(): String\n\treturn \"hello\"\nend\n")}
	service.SetDocument(unit)
	added := service.Analyze()
	context, ok := added.Context("new")
	if added.HasErrors() || !ok || !hasCompletion(context, "greeting") {
		t.Fatalf("new overlay was not analyzed: %#v", added)
	}

	service.CloseDocument(unit.Filename)
	removed := service.Analyze()
	if removed.HasErrors() || len(removed.Artifacts) != 0 {
		t.Fatalf("closed new overlay remains in snapshot: %#v", removed)
	}
}

func hasCompletion(context languageservice.Context, name string) bool {
	items := languageservice.Complete(languageservice.CompletionRequest{Source: name[:1], Cursor: 1, Mode: "go", Context: context})
	for _, item := range items {
		if item.Label == name {
			return true
		}
	}
	return false
}
