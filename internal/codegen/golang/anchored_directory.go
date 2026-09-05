package golang

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/stdlib"
)

func (g *generator) anchoredCreateAll(call *ir.Call, arguments []string) string {
	g.requireImport("os", "")
	success, failure := call.ExprType().Args[0], call.ExprType().Args[1]
	failed := g.filesystemResultErr(success, failure, g.filesystemResourceNativeError("create_all", "path", "cause", "true"))
	return "func(root *os.Root, path string) " + g.goType(call.ExprType()) + " { if cause := root.MkdirAll(path, 0o777); cause != nil { return " + failed + " }; return " + g.filesystemResultOK(success, failure, g.goType(success)+"{}") + " }(" + strings.Join(arguments, ", ") + ")"
}

func (g *generator) anchoredChildren(call *ir.Call, arguments []string) string {
	for _, name := range []string{"os", "io", "strings", "slices", "unicode/utf8"} {
		g.requireImport(name, "")
	}
	success, failure := call.ExprType().Args[0], call.ExprType().Args[1]
	entry := g.goType(success.Args[0])
	err := func(value string) string { return g.filesystemResultErr(success, failure, value) }
	inputError := func(kind, message string) string {
		return err(g.filesystemResourceError(failure, "children", "path", strconv.Quote(message), kind, "true"))
	}
	// Reuse the public value's factory, rather than maintaining a second grammar.
	parse := g.declarationAlias(stdlib.RelativePathType().Declaration) + "." + newtypeMethodName(identity.Dispatch{Owner: stdlib.RelativePathType().Declaration, Name: "parse"})
	path, maximum := "nil", arguments[len(arguments)-1]
	if len(arguments) == 3 {
		path = arguments[1]
		if !call.Arguments[0].Value.ExprType().Nullable && call.Arguments[0].Value.ExprType().Kind != "nil" {
			path = "func(value string) *string { return &value }(" + path + ")"
		}
	}
	return strings.NewReplacer(
		"$result", g.goType(call.ExprType()), "$entry", entry,
		"$invalidLimit", inputError("InvalidLimit", "max_entries must be non-negative"),
		"$tooLarge", inputError("TooLarge", "directory exceeds max_entries"),
		"$invalidName", inputError("UnsupportedName", "directory entry name is not a portable relative component"),
		"$failure", err(g.filesystemResourceNativeError("children", "path", "cause", "true")),
		"$metadataFailure", err(g.filesystemResourceNativeError("children", "childPath", "infoErr", "true")),
		"$closeFailure", err(g.filesystemResourceNativeError("children", "path", "closeError", "true")),
		"$other", g.filesystemDirectoryEntryKind("Other"), "$file", g.filesystemDirectoryEntryKind("File"), "$directory", g.filesystemDirectoryEntryKind("Directory"),
		"$parse", parse, "$errTag", g.filesystemResultAlias()+".ResultErrTag",
		"$ok", g.filesystemResultOK(success, failure, g.arrayReference("entries")),
	).Replace(`func(root *os.Root, relative *string, maximum int) (result $result) {
		path := ""; lookup := "."; if relative != nil { path = *relative; lookup = path }
		if maximum < 0 { return $invalidLimit }
		// Open the listing directory as its own anchor. Metadata lookup must
		// observe that same directory even if its name is concurrently replaced.
		listing, cause := root.OpenRoot(lookup); if cause != nil { return $failure }
		completed := false
		defer func() { if closeError := listing.Close(); completed && closeError != nil { result = $closeFailure } }()
		directoryHandle, cause := listing.Open("."); if cause != nil { return $failure }
		defer func() { if closeError := directoryHandle.Close(); completed && closeError != nil { result = $closeFailure; completed = false } }()
		entries := make([]$entry, 0)
		for {
			count := min(128, maximum - len(entries) + 1)
			source, cause := directoryHandle.Readdirnames(count)
			if cause != nil && cause != io.EOF { return $failure }
			if len(source) > maximum - len(entries) { return $tooLarge }
			for _, name := range source {
				if !utf8.ValidString(name) || $parse(name).Kind == $errTag { return $invalidName }
				childPath := name; if path != "" { childPath = path + "/" + name }
				info, infoErr := listing.Lstat(name); if infoErr != nil { return $metadataFailure }
				kind := $other; if info.Mode().IsRegular() { kind = $file } else if info.IsDir() { kind = $directory }
				entries = append(entries, $entry{Name: name, Path: childPath, Kind: kind})
			}
			if cause == io.EOF { break }
		}
		slices.SortStableFunc(entries, func(left, right $entry) int { return strings.Compare(left.Name, right.Name) })
		completed = true
		return $ok
	}(` + arguments[0] + ", " + path + ", " + maximum + ")")
}
