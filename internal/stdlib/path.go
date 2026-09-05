package stdlib

import (
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/types"
)

const pathModulePath = "trb/std/path/index"

var pathDeclaration = identity.Declaration{Module: pathModulePath, Name: "Path", Kind: identity.Newtype}
var relativePathDeclaration = identity.Declaration{Module: pathModulePath, Name: "RelativePath", Kind: identity.Newtype}

func pathJoinSymbol() Symbol {
	return Symbol{
		Name: "join", Intrinsic: "trb.std.path.join", RuntimeIndependent: true,
		Receiver:   declaredType(pathDeclaration),
		Parameters: []Parameter{{Name: "path", Type: declaredType(relativePathDeclaration)}},
		Return:     declaredType(pathDeclaration),
	}
}

// PathType returns the host path value's exact nominal declaration.
func PathType() types.Type { return declaredType(pathDeclaration) }

// RelativePathType returns the validated descendant path's exact declaration.
func RelativePathType() types.Type { return declaredType(relativePathDeclaration) }

// The value contract is ordinary TypeRB source. Only Path#join needs a host
// intrinsic: portable source has no host-separator query, and exposing native
// imports in this implementation would prevent the same package from checking
// in every mode. Validation never consults host filename rules or the filesystem.
func pathSource() string {
	return `import trb/std/result

enum RelativePathError
	Empty
	EmptyComponent
	DotComponent
	InvalidCharacter
	MultipleComponents

	def to_s(): String
		case self
		when RelativePathError::Empty
			return "relative path must not be empty"
		when RelativePathError::EmptyComponent
			return "relative path components must not be empty"
		when RelativePathError::DotComponent
			return "dot and parent components are not allowed"
		when RelativePathError::InvalidCharacter
			return "relative path contains a prohibited character"
		when RelativePathError::MultipleComponents
			return "child requires exactly one component"
		end
	end
end

newtype Path = String do
	def to_s(): String
		return value()
	end
end

newtype RelativePath = String do
	private new

	def self.parse(source: String): Result<RelativePath, RelativePathError>
		if source.empty?()
			return Result<RelativePath, RelativePathError>::Err(RelativePathError::Empty)
		end
		if source.start_with?("/") || source.end_with?("/") || source.include?("//")
			return Result<RelativePath, RelativePathError>::Err(RelativePathError::EmptyComponent)
		end
		source.split("/").each do |component|
			failure := self._component_error(component)
			if failure != nil
				return Result<RelativePath, RelativePathError>::Err(failure)
			end
		end
		return Result<RelativePath, RelativePathError>::Ok(self.new(source))
	end

	def to_s(): String
		return value()
	end

	def join(path: RelativePath): RelativePath
		return RelativePath.new(value() + "/" + path.value())
	end

	def child(name: String): Result<RelativePath, RelativePathError>
		if name.include?("/")
			return Result<RelativePath, RelativePathError>::Err(RelativePathError::MultipleComponents)
		end
		path := try RelativePath.parse(name)
		return Result<RelativePath, RelativePathError>::Ok(join(path))
	end

	def parent(): RelativePath?
		source := value()
		separator := source.rindex("/")
		if separator == nil
			return nil
		end
		return RelativePath.new(source.slice(0...separator))
	end

	def self._component_error(name: String): RelativePathError?
		if name == "." || name == ".."
			return RelativePathError::DotComponent
		end
		points := name.codepoints()
		points.each do |point|
			if point == 0 || point == 58 || point == 92
				return RelativePathError::InvalidCharacter
			end
		end
		return nil
	end
end
`
}
