package compilerservice

import (
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/ir"
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

func TestServiceDoesNotInvalidateWhenOpeningAnUnchangedWorkspaceDocument(t *testing.T) {
	unit := compiler.SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte("def main()\n\treturn\nend\n"),
	}
	service := New([]compiler.SourceUnit{unit}, compiler.Options{Mode: "go", GoModule: "example.com/service"})
	initial := service.Analyze()
	service.SetDocument(unit)
	unchanged := service.Analyze()
	if unchanged.Version != initial.Version {
		t.Fatalf("opening unchanged document advanced snapshot version from %d to %d", initial.Version, unchanged.Version)
	}
}

func TestServiceTracksWorkspaceDocumentsBeneathOpenOverlays(t *testing.T) {
	filename := "models/user.trb"
	base := compiler.SourceUnit{
		Filename: filename, ModulePath: "models/user", Package: "models",
		Source: []byte("record User\n\tname: String\nend\n"),
	}
	service := New([]compiler.SourceUnit{base}, compiler.Options{Mode: "go", GoModule: "example.com/service"})
	overlay := base
	overlay.Source = []byte("record EditingUser\n\tname: String\nend\n")
	service.SetDocument(overlay)

	saved := base
	saved.Source = []byte("record SavedUser\n\tname: String\nend\n")
	service.SetWorkspaceDocument(saved)
	withOverlay := service.Analyze()
	context, ok := withOverlay.Context("models/user")
	if !ok || !hasCompletion(context, "EditingUser") || hasCompletion(context, "SavedUser") {
		t.Fatalf("workspace update replaced the open overlay: %#v", context)
	}

	service.CloseDocument(filename)
	restored := service.Analyze()
	context, ok = restored.Context("models/user")
	if !ok || !hasCompletion(context, "SavedUser") || hasCompletion(context, "EditingUser") {
		t.Fatalf("closing overlay did not restore the saved workspace unit: %#v", context)
	}

	service.RemoveWorkspaceDocument(filename)
	removed := service.Analyze()
	if removed.HasErrors() || len(removed.Artifacts) != 0 {
		t.Fatalf("removed workspace unit remains in snapshot: %#v", removed)
	}
}

func TestServiceAtomicallyReplacesWorkspaceDocuments(t *testing.T) {
	first := compiler.SourceUnit{
		Filename: "first.trb", ModulePath: "first", Package: "main",
		Source: []byte("def first(): Integer\n\treturn 1\nend\n"),
	}
	service := New([]compiler.SourceUnit{first}, compiler.Options{Mode: "go", GoModule: "example.com/service"})
	initial := service.Analyze()

	second := compiler.SourceUnit{
		Filename: "second.trb", ModulePath: "second", Package: "main",
		Source: []byte("def second(): Integer\n\treturn 2\nend\n"),
	}
	service.ReplaceWorkspaceDocuments([]compiler.SourceUnit{second})
	replaced := service.Analyze()
	if replaced.Version != initial.Version+1 || replaced.HasErrors() || len(replaced.Artifacts) != 1 || replaced.Artifacts[0].Filename != cleanPath(second.Filename) {
		t.Fatalf("unexpected replaced snapshot: %#v", replaced)
	}

	service.ReplaceWorkspaceDocuments([]compiler.SourceUnit{second})
	unchanged := service.Analyze()
	if unchanged.Version != replaced.Version {
		t.Fatalf("equal replacement advanced snapshot version from %d to %d", replaced.Version, unchanged.Version)
	}
}

func TestServiceInvalidatesForEveryCompilerSignificantUnitChange(t *testing.T) {
	base := compiler.SourceUnit{
		Filename: "main.trb", Source: []byte("def main()\n\treturn\nend\n"), ModulePath: "main", Package: "main",
		PackageAliases: map[string]string{"shared": "example.com/shared"},
	}
	changes := map[string]func(*compiler.SourceUnit){
		"source":              func(unit *compiler.SourceUnit) { unit.Source = []byte("def changed()\n\treturn\nend\n") },
		"module path":         func(unit *compiler.SourceUnit) { unit.ModulePath = "changed" },
		"package":             func(unit *compiler.SourceUnit) { unit.Package = "changed" },
		"package alias":       func(unit *compiler.SourceUnit) { unit.PackageAliases["shared"] = "example.com/changed" },
		"nil package aliases": func(unit *compiler.SourceUnit) { unit.PackageAliases = nil },
		"compiler owned":      func(unit *compiler.SourceUnit) { unit.CompilerOwned = true },
		"official":            func(unit *compiler.SourceUnit) { unit.Official = true },
		"external package":    func(unit *compiler.SourceUnit) { unit.ExternalPackage = true },
		"test registration":   func(unit *compiler.SourceUnit) { unit.TestRegistration = "register_tests" },
		"main replacement":    func(unit *compiler.SourceUnit) { unit.MainReplacement = "test_main" },
	}
	operations := map[string]func(*Service, compiler.SourceUnit){
		"document overlay":   func(service *Service, unit compiler.SourceUnit) { service.SetDocument(unit) },
		"workspace document": func(service *Service, unit compiler.SourceUnit) { service.SetWorkspaceDocument(unit) },
		"workspace replacement": func(service *Service, unit compiler.SourceUnit) {
			service.ReplaceWorkspaceDocuments([]compiler.SourceUnit{unit})
		},
	}
	for changeName, change := range changes {
		for operationName, operation := range operations {
			t.Run(changeName+"/"+operationName, func(t *testing.T) {
				service := New([]compiler.SourceUnit{base}, compiler.Options{Mode: "go", GoModule: "example.com/service"})
				changed := cloneUnit(base)
				change(&changed)
				operation(service, changed)
				if generation := service.Generation(); generation != 2 {
					t.Fatalf("generation=%d, want 2 after compiler-significant change", generation)
				}
			})
		}
	}
}

func TestServiceDoesNotInvalidateForEquivalentPackageAliasMaps(t *testing.T) {
	base := compiler.SourceUnit{
		Filename: "main.trb", Source: []byte("def main()\n\treturn\nend\n"), ModulePath: "main", Package: "main",
		PackageAliases: map[string]string{"first": "example.com/first", "second": "example.com/second"},
	}
	equivalent := cloneUnit(base)
	equivalent.PackageAliases = map[string]string{"second": "example.com/second", "first": "example.com/first"}
	for name, operation := range map[string]func(*Service){
		"document overlay":      func(service *Service) { service.SetDocument(equivalent) },
		"workspace document":    func(service *Service) { service.SetWorkspaceDocument(equivalent) },
		"workspace replacement": func(service *Service) { service.ReplaceWorkspaceDocuments([]compiler.SourceUnit{equivalent}) },
	} {
		t.Run(name, func(t *testing.T) {
			service := New([]compiler.SourceUnit{base}, compiler.Options{Mode: "go", GoModule: "example.com/service"})
			operation(service)
			if generation := service.Generation(); generation != 1 {
				t.Fatalf("generation=%d, want 1 for equivalent aliases", generation)
			}
		})
	}
}

func TestServiceDistinguishesDefaultAndExplicitlyEmptyPackageAliases(t *testing.T) {
	base := compiler.SourceUnit{
		Filename: "main.trb", Source: []byte("def main()\n\treturn\nend\n"), ModulePath: "main", Package: "main",
	}
	service := New([]compiler.SourceUnit{base}, compiler.Options{Mode: "go", GoModule: "example.com/service"})
	explicitlyEmpty := cloneUnit(base)
	explicitlyEmpty.PackageAliases = map[string]string{}
	service.SetWorkspaceDocument(explicitlyEmpty)
	if generation := service.Generation(); generation != 2 {
		t.Fatalf("generation=%d, want 2 when explicit aliases replace inherited defaults", generation)
	}
}

func TestEquivalentPackageAliasesRequireTheSameKeys(t *testing.T) {
	if equalStrings(map[string]string{"left": ""}, map[string]string{"right": ""}) {
		t.Fatal("package alias maps with different blank-valued keys were equal")
	}
}

func TestServiceAnalyzeOnceRejectsObsoleteGeneration(t *testing.T) {
	filename := "main.trb"
	unit := compiler.SourceUnit{
		Filename: filename, ModulePath: "main", Package: "main",
		Source: []byte("def value(): Integer\n\treturn 1\nend\n"),
	}
	service := New([]compiler.SourceUnit{unit}, compiler.Options{Mode: "go", GoModule: "example.com/service"})
	originalCompile := service.compile
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	service.compile = func(units []compiler.SourceUnit, options compiler.Options) ([]*compiler.Artifact, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		return originalCompile(units, options)
	}

	type analysisResult struct {
		snapshot Snapshot
		current  bool
	}
	result := make(chan analysisResult, 1)
	go func() {
		snapshot, current := service.AnalyzeOnce()
		result <- analysisResult{snapshot: snapshot, current: current}
	}()
	<-started
	unit.Source = []byte("def value(): Integer\n\treturn 2\nend\n")
	service.SetDocument(unit)
	generation := service.Generation()
	close(release)
	if generation != 2 {
		t.Fatalf("generation=%d, want 2", generation)
	}
	obsolete := <-result
	if obsolete.current {
		t.Fatalf("obsolete analysis was committed: %#v", obsolete.snapshot)
	}

	current, committed := service.AnalyzeOnce()
	if !committed || current.Version != 2 {
		t.Fatalf("current analysis=(%#v, %t), want generation 2", current, committed)
	}
	source, ok := current.Source(filename)
	if !ok || string(source) != string(unit.Source) {
		t.Fatalf("current source=(%q, %t), want %q", source, ok, unit.Source)
	}
}

func TestSnapshotSourceIsNormalizedImmutableAndCached(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "nested", "..", "main.trb")
	original := []byte("def value(): Integer\n\treturn 1\nend\n")
	unit := compiler.SourceUnit{
		Filename: filename, ModulePath: "main", Package: "main", Source: original,
	}
	service := New([]compiler.SourceUnit{unit}, compiler.Options{Mode: "go", GoModule: "example.com/service"})
	original[0] = 'x'

	snapshot, current := service.AnalyzeOnce()
	if !current {
		t.Fatal("initial analysis was unexpectedly obsolete")
	}
	normalized := filepath.Join(root, "main.trb")
	source, ok := snapshot.Source(normalized)
	if !ok || string(source) != "def value(): Integer\n\treturn 1\nend\n" {
		t.Fatalf("snapshot source=(%q, %t)", source, ok)
	}
	source[0] = 'x'
	again, ok := snapshot.Source(filename)
	if !ok || string(again) != "def value(): Integer\n\treturn 1\nend\n" {
		t.Fatalf("mutating returned source changed snapshot: (%q, %t)", again, ok)
	}

	cached, current := service.AnalyzeOnce()
	if !current || cached.Version != snapshot.Version {
		t.Fatalf("cached analysis=(%#v, %t), want version %d", cached, current, snapshot.Version)
	}
	cachedSource, ok := cached.Source(normalized)
	if !ok || string(cachedSource) != string(again) {
		t.Fatalf("cached source=(%q, %t), want %q", cachedSource, ok, again)
	}
	if missing, found := cached.Source(filepath.Join(root, "missing.trb")); found || missing != nil {
		t.Fatalf("missing source=(%q, %t), want (nil, false)", missing, found)
	}

	unit.Source = []byte("def value(): Integer\n\treturn 2\nend\n")
	service.SetDocument(unit)
	oldSource, ok := snapshot.Source(normalized)
	if !ok || string(oldSource) != string(again) {
		t.Fatalf("later input mutation changed old snapshot: (%q, %t)", oldSource, ok)
	}
}

func TestSnapshotImportCandidatesStayLastGoodAndCached(t *testing.T) {
	units := []compiler.SourceUnit{
		{
			Filename: "app/main.trb", ModulePath: "app/main",
			Source: []byte("def value(): Integer\n\treturn 1\nend\n"),
		},
		{
			Filename: "models/user.trb", ModulePath: "models/user",
			Source: []byte("record User\n\tname: String\nend\n\nrecord Unique\n\tid: Integer\nend\n"),
		},
	}
	service := New(units, compiler.Options{Mode: "typescript"})
	initial := service.Analyze()
	if initial.HasErrors() || initial.Stale {
		t.Fatalf("unexpected initial snapshot: %#v", initial)
	}
	if !hasImportCandidate(initial.ImportCandidates("app/main"), "Unique", "models/user") {
		t.Fatal("initial snapshot is missing the unambiguous Unique import")
	}
	if hasImportCandidate(initial.ImportCandidates("models/user"), "Unique", "") {
		t.Fatal("initial snapshot contains a same-module Unique import")
	}

	cached, current := service.AnalyzeOnce()
	if !current || cached.Version != initial.Version || !hasImportCandidate(cached.ImportCandidates("app/main"), "Unique", "models/user") {
		t.Fatalf("cached import candidates were not retained: current=%t snapshot=%#v", current, cached)
	}

	invalid := units[0]
	invalid.Source = []byte("def broken(): Missing\n\treturn missing\nend\n")
	service.SetDocument(invalid)
	stale := service.Analyze()
	if !stale.HasErrors() || !stale.Stale {
		t.Fatalf("unexpected invalid snapshot: %#v", stale)
	}
	if !hasImportCandidate(stale.ImportCandidates("app/main"), "Unique", "models/user") {
		t.Fatal("stale snapshot discarded last-good import candidates")
	}
}

func TestProjectImportCandidatesExcludeNonProjectArtifacts(t *testing.T) {
	artifact := func(modulePath, name string, flags ...string) *compiler.Artifact {
		result := &compiler.Artifact{IR: &ir.Program{ModulePath: modulePath, Statements: []ir.Statement{&ir.Record{Name: name}}}}
		for _, flag := range flags {
			switch flag {
			case "compiler-owned":
				result.CompilerOwned = true
			case "official":
				result.Official = true
			case "external":
				result.ExternalPackage = true
			}
		}
		return result
	}
	candidates := buildProjectImportCandidates([]*compiler.Artifact{
		artifact("models/visible", "Visible"),
		artifact("runtime/internal", "CompilerInternal", "compiler-owned"),
		artifact("official/package", "OfficialType", "official"),
		artifact("vendor/package", "ExternalType", "external"),
	}).ForModule("app/main")
	if !hasImportCandidate(candidates, "Visible", "models/visible") {
		t.Fatal("project import candidates omitted Visible")
	}
	for _, name := range []string{"CompilerInternal", "OfficialType", "ExternalType"} {
		if hasImportCandidate(candidates, name, "") {
			t.Fatalf("project import candidates contain excluded %s", name)
		}
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

func hasImportCandidate(context languageservice.Context, name, path string) bool {
	for _, symbol := range context.Symbols {
		if symbol.Name == name && symbol.Import != nil && (path == "" || symbol.Import.Path == path) {
			return true
		}
	}
	return false
}
