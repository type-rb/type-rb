package stdlib

import "github.com/type-rb/type-rb/internal/types"

func optionalRelativePathType() types.Type {
	typ := RelativePathType()
	typ.Nullable = true
	return typ
}

func rootedDirEntryType() types.Type {
	entry := DirEntryType()
	entry.Args = []types.Type{RelativePathType()}
	return entry
}

func dirOpenSymbol() Symbol {
	return Symbol{Name: "open", Intrinsic: "trb.std.dir.open", StaticOwner: "Dir", TypeParameters: []string{"T"},
		trustedResourceOrigin: true,
		Parameters:            []Parameter{{Name: "path", Type: PathType()}}, Return: filesystemResult(typeT),
		Block: &Block{Parameters: []types.Type{DirResourceType()}, Return: typeT, ResultBoundary: fileSystemErrorType, Structured: true, ScopedParameters: []bool{true}},
	}
}

func dirOpenFileSymbol() Symbol {
	symbol := dirOpenSymbol()
	symbol.Name = "open_file"
	symbol.Intrinsic = "trb.std.dir.open_file"
	symbol.StaticOwner = ""
	symbol.Receiver = DirResourceType()
	symbol.Parameters = []Parameter{{Name: "path", Type: RelativePathType()}, {Name: "mode", Type: fileModeType, Optional: true, Keyword: true}}
	symbol.RuntimeDependencies = []types.Type{fileModeType}
	symbol.Block.Parameters = []types.Type{FileResourceType()}
	return symbol
}

func dirTryLockSymbol() Symbol {
	symbol := dirOpenFileSymbol()
	symbol.Name = "try_lock"
	symbol.Intrinsic = "trb.std.dir.try_lock"
	symbol.Parameters = []Parameter{{Name: "path", Type: RelativePathType()}}
	symbol.RuntimeDependencies = nil
	symbol.Block.Parameters = nil
	symbol.Block.ScopedParameters = nil
	return symbol
}

func dirSource() string {
	return `class Dir
end

enum DirEntryKind
	File
	Directory
	Other
end

record DirEntry<P>
	name: String
	path: P
	kind: DirEntryKind
end
`
}
