package stdlib

import (
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/types"
)

const (
	fileModulePath   = "trb/std/file/index"
	dirModulePath    = "trb/std/dir/index"
	errorsModulePath = "trb/std/errors/index"
	resultModulePath = "trb/std/result/index"
)

var (
	fileDeclaration = identity.Declaration{Module: fileModulePath, Name: "File", Kind: identity.Class}
	dirDeclaration  = identity.Declaration{Module: dirModulePath, Name: "Dir", Kind: identity.Class}

	fileModeDeclaration            = identity.Declaration{Module: fileModulePath, Name: "FileMode", Kind: identity.Enum}
	dirEntryDeclaration            = identity.Declaration{Module: dirModulePath, Name: "DirEntry", Kind: identity.Record}
	dirEntryKindDeclaration        = identity.Declaration{Module: dirModulePath, Name: "DirEntryKind", Kind: identity.Enum}
	fileSystemErrorDeclaration     = identity.Declaration{Module: errorsModulePath, Name: "FileSystemError", Kind: identity.Record}
	fileSystemErrorKindDeclaration = identity.Declaration{Module: errorsModulePath, Name: "FileSystemErrorKind", Kind: identity.Enum}
	resultDeclaration              = identity.Declaration{Module: resultModulePath, Name: "Result", Kind: identity.Enum}
)

func declaredType(declaration identity.Declaration) types.Type {
	return types.Type{Kind: types.Named, Name: declaration.LeafName(), Declaration: declaration}
}

// FileResourceType returns the exact compiler-owned scoped File declaration.
// Its identity distinguishes host filesystem handles from unrelated public
// declarations that happen to use the name File.
func FileResourceType() types.Type {
	return declaredType(fileDeclaration)
}

// FileModeType returns the exact standard FileMode declaration.
func FileModeType() types.Type {
	return declaredType(fileModeDeclaration)
}

// DirEntryType returns the exact standard DirEntry declaration.
func DirEntryType() types.Type {
	return declaredType(dirEntryDeclaration)
}

// DirEntryKindType returns the exact standard DirEntryKind declaration.
func DirEntryKindType() types.Type {
	return declaredType(dirEntryKindDeclaration)
}

// FileSystemErrorType returns the exact standard FileSystemError declaration.
func FileSystemErrorType() types.Type {
	return declaredType(fileSystemErrorDeclaration)
}

// FileSystemErrorKindType returns the exact standard FileSystemErrorKind declaration.
func FileSystemErrorKindType() types.Type {
	return declaredType(fileSystemErrorKindDeclaration)
}

// ResultType returns the exact standard Result declaration with its arguments.
func ResultType(value, failure types.Type) types.Type {
	result := declaredType(resultDeclaration)
	result.Args = []types.Type{value, failure}
	return result
}

// IsFilesystemContractType reports whether typ participates in the scoped
// filesystem contract whose generated runtime references require exact import
// ownership. Result is included because every filesystem operation crosses
// that boundary.
func IsFilesystemContractType(typ types.Type) bool {
	switch typ.Declaration {
	case fileDeclaration,
		fileModeDeclaration,
		dirDeclaration,
		dirEntryDeclaration,
		dirEntryKindDeclaration,
		fileSystemErrorDeclaration,
		fileSystemErrorKindDeclaration,
		resultDeclaration:
		return true
	default:
		return false
	}
}

// IsFileResourceType reports whether typ is the standard scoped host File.
func IsFileResourceType(typ types.Type) bool {
	return typ.Kind == types.Named && typ.Declaration == fileDeclaration
}

// IsTrustedFileOpenContract reports whether a library block is the bundled
// declaration that owns standard File values. Extension and native declaration
// providers cannot opt into this contract merely by returning the same type or
// marking one of their own block parameters as scoped.
func IsTrustedFileOpenContract(definition *Package, symbol *Symbol) bool {
	return definition != nil && definition == registry["trb/std/file"] &&
		symbol != nil && symbol.trustedScopedFileOrigin && symbol.Intrinsic == "trb.std.file.open"
}
