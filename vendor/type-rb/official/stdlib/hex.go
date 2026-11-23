package stdlib

func hexSource() string {
	return `import trb/std/result
import trb/internal/encoding/hex as native_hex

module Hex
	enum DecodeErrorKind
		OddLength
		InvalidCharacter
	end

	record DecodeError
		kind: Hex::DecodeErrorKind
		input: String
		index: Integer
		message: String
	end
end

def encode(value: Bytes): String
	return native_hex.encode(value)
end

def decode(value: String): Result<Bytes, Hex::DecodeError>
	return native_hex.decode(value)
end
`
}
