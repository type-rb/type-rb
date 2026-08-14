package stdlib

func errorsSource() string {
	return `enum NumberParseErrorKind
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
