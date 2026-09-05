package codegen

import (
	"sort"

	"github.com/type-rb/type-rb/internal/codegen/effectplan"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/nativefs"
)

// SupportFile is a compiler-owned target source, not a portable source module
// and not a provider artifact. Path is relative to the generated project root.
type SupportFile struct {
	Path   string
	Output []byte
}

func nativeSupport(programs []*ir.Program) ([]SupportFile, map[string]string) {
	if len(programs) == 0 || programs[0].Mode != "go" {
		return nil, nil
	}
	needed := false
	plan := effectplan.Analyze(programs, effectplan.Options{Intrinsic: func(name string) bool { return name == "trb.std.dir.try_lock" }})
	for call := range plan.Calls {
		if anchoredIntrinsic(call.Callee) == "trb.std.dir.try_lock" {
			needed = true
			break
		}
	}
	if !needed {
		return nil, nil
	}
	sources := nativefs.Sources()
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	files := make([]SupportFile, 0, len(names))
	for _, name := range names {
		files = append(files, SupportFile{Path: "trb/runtime/nativefs/" + name, Output: sources[name]})
	}
	return files, map[string]string{nativefs.ModulePath: nativefs.ModuleVersion}
}
