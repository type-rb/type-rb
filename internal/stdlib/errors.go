package stdlib

func errorsSource() string {
	return `import { Path, RelativePath } from trb/std/path

enum FileSystemErrorKind
	Other
	AlreadyExists
	NotFound
	PermissionDenied
	InvalidPath
	InvalidLimit
	TooLarge
	InvalidEncoding
	UnsupportedName
end

enum FileSystemTarget
	Host(path: Path)
	Relative(path: RelativePath)
	Root
end

record FileSystemError
	operation: String
	target: FileSystemTarget
	message: String
	kind: FileSystemErrorKind = FileSystemErrorKind::Other
end

enum NumberParseErrorKind
	InvalidFormat
	OutOfRange
end

record NumberParseError
	kind: NumberParseErrorKind
	input: String
	message: String
end

record IndexLookupError
	index: Integer
	size: Integer
	message: String
end

record SliceRangeError
	start: Integer
	finish: Integer
	exclusive: Boolean
	size: Integer
	message: String
end

record KeyLookupError
	key: String | Integer
	message: String
end

record EnumValueError
	value: String | Integer
	message: String
end
`
}
