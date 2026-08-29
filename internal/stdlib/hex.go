package stdlib

func hexSource() string {
	return `import trb/std/result
import trb/internal/encoding/hex as native_hex

enum HexDecodeErrorKind
	OddLength
	InvalidCharacter
end

record HexDecodeError
	kind: HexDecodeErrorKind
	input: String
	index: Integer
	message: String
end

def encode(value: Bytes): String
	return native_hex.encode(value)
end

def decode(value: String): Result<Bytes, HexDecodeError>
	return native_hex.decode(value)
end
`
}
