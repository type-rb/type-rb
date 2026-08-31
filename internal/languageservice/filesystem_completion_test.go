package languageservice_test

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/languageservice"
)

func TestFilesystemCompletionUsesDeclarationRootsAndExactPeerImports(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			candidates := languageservice.StandardImportCandidates(mode)
			for _, expected := range []struct {
				name   string
				path   string
				symbol string
			}{
				{name: "File", path: "trb/std/file"},
				{name: "FileMode", path: "trb/std/file", symbol: "FileMode"},
				{name: "Dir", path: "trb/std/dir"},
				{name: "DirEntry", path: "trb/std/dir", symbol: "DirEntry"},
				{name: "DirEntryKind", path: "trb/std/dir", symbol: "DirEntryKind"},
			} {
				symbol, ok := filesystemCompletionSymbol(candidates.Symbols, expected.name)
				if !ok || symbol.Import == nil || symbol.Import.Path != expected.path || symbol.Import.Symbol != expected.symbol {
					t.Errorf("%s candidate=%#v, want import path=%q symbol=%q", expected.name, symbol, expected.path, expected.symbol)
				}
			}
			for _, removed := range []string{"FileSystem", "Path"} {
				if symbol, ok := filesystemCompletionSymbol(candidates.Symbols, removed); ok {
					t.Errorf("removed %s root remains in standard completion: %#v", removed, symbol)
				}
			}

			file, _ := filesystemCompletionSymbol(candidates.Symbols, "File")
			open, ok := filesystemCompletionSymbol(file.Members, "open")
			if !ok || open.Call == nil || len(open.Call.Parameters) != 2 {
				t.Fatalf("File.open completion metadata=%#v", open)
			}
			modeParameter := open.Call.Parameters[1]
			if modeParameter.Name != "mode" || !modeParameter.NamedOnly || !modeParameter.Keyword || !modeParameter.Optional {
				t.Errorf("File.open mode metadata=%#v", modeParameter)
			}
			if len(open.Call.BlockParameters) != 1 || open.Call.BlockParameters[0].String() != "File" {
				t.Errorf("File.open block metadata=%#v", open.Call.BlockParameters)
			}
			for _, receiverOnly := range []string{"read", "read_text", "write", "write_text"} {
				if member, exists := filesystemCompletionSymbol(file.Members, receiverOnly); exists {
					t.Errorf("File.%s was advertised as a static member: %#v", receiverOnly, member)
				}
			}
			directory, _ := filesystemCompletionSymbol(candidates.Symbols, "Dir")
			for _, root := range []languageservice.Symbol{file, directory} {
				if constructor, exists := filesystemCompletionSymbol(root.Members, "new"); exists {
					t.Errorf("opaque %s root advertised new: %#v", root.Name, constructor)
				}
			}

			service := languageservice.New(mode)
			service.SetCandidates(candidates)
			openItem, ok := findCompletion(service.Complete("File.op", len("File.op")), "open")
			if !ok || !strings.Contains(openItem.Detail, "mode: FileMode") || len(openItem.AdditionalEdits) != 1 || openItem.AdditionalEdits[0].NewText != "import trb/std/file\n" {
				t.Errorf("File.open completion=%#v, ok=%v", openItem, ok)
			}
			childrenItem, ok := findCompletion(service.Complete("Dir.ch", len("Dir.ch")), "children")
			if !ok || len(childrenItem.AdditionalEdits) != 1 || childrenItem.AdditionalEdits[0].NewText != "import trb/std/dir\n" {
				t.Errorf("Dir.children completion=%#v, ok=%v", childrenItem, ok)
			}
			for _, source := range []string{"File.ne", "Dir.ne"} {
				if constructor, exists := findCompletion(service.Complete(source, len(source)), "new"); exists {
					t.Errorf("opaque root completion %q advertised new: %#v", source, constructor)
				}
			}
			for _, expected := range []struct {
				prefix string
				name   string
				edit   string
			}{
				{prefix: "FileM", name: "FileMode", edit: "import { FileMode } from trb/std/file\n"},
				{prefix: "DirE", name: "DirEntry", edit: "import { DirEntry } from trb/std/dir\n"},
				{prefix: "DirEntryK", name: "DirEntryKind", edit: "import { DirEntryKind } from trb/std/dir\n"},
			} {
				item, ok := findCompletion(service.Complete(expected.prefix, len(expected.prefix)), expected.name)
				if !ok || len(item.AdditionalEdits) != 1 || item.AdditionalEdits[0].NewText != expected.edit {
					t.Errorf("%s completion=%#v, ok=%v", expected.name, item, ok)
				}
			}
		})
	}
}

func TestScopedFileBlockBindingCompletesReceiverMethodsAcrossModes(t *testing.T) {
	const checkedSource = `import trb/std/file
import trb/std/dir

def main()
	return
end
`
	const completionSource = `import trb/std/file

def inspect(path: String)
	File.open(path) do |file|
		file.
`
	const signatureSource = `import trb/std/file

def inspect(path: String)
	File.open(path) do |file|
		file.read(max_bytes: 1)
	end
end
`

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact := compile(t, mode, checkedSource)
			service := languageservice.New(mode)
			service.Update([]*ir.Program{artifact.IR}, "repl")

			context := languageservice.BuildContext([]*ir.Program{artifact.IR}, "repl")
			for _, rootName := range []string{"File", "Dir"} {
				root, ok := filesystemCompletionSymbol(context.Symbols, rootName)
				if !ok {
					t.Fatalf("checked %s root is missing", rootName)
				}
				if constructor, exists := filesystemCompletionSymbol(root.Members, "new"); exists {
					t.Errorf("checked opaque %s root advertised new: %#v", rootName, constructor)
				}
			}

			completionCursor := strings.LastIndex(completionSource, "file.") + len("file.")
			items := service.Complete(completionSource, completionCursor)
			for _, method := range []string{"read", "read_text", "write", "write_text"} {
				if item, ok := findCompletion(items, method); !ok {
					t.Errorf("File block receiver completion omitted %s: %v", method, labels(items))
				} else if method == "read" && !strings.Contains(item.Detail, "max_bytes: Integer") {
					t.Errorf("File#read completion lost its bounded argument: %#v", item)
				}
			}

			cursor := strings.LastIndex(signatureSource, "1") + 1
			help, ok := service.Signatures(signatureSource, cursor)
			if !ok || len(help.Signatures) != 1 || !strings.Contains(help.Signatures[0].Label, "read(*, max_bytes: Integer)") {
				t.Fatalf("File#read signature help=(%#v, %v)", help, ok)
			}
			if parameters := help.Signatures[0].Parameters; len(parameters) != 1 || parameters[0].Label != "max_bytes: Integer" {
				t.Errorf("File#read signature parameters=%#v", parameters)
			}
		})
	}
}

func filesystemCompletionSymbol(symbols []languageservice.Symbol, name string) (languageservice.Symbol, bool) {
	for _, symbol := range symbols {
		if symbol.Name == name {
			return symbol, true
		}
	}
	return languageservice.Symbol{}, false
}
