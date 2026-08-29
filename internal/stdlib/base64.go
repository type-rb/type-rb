package stdlib

func base64Source() string {
	return `import trb/std/result
import trb/internal/encoding/base64 as native_base64

enum Base64DecodeErrorKind
	InvalidLength
	InvalidCharacter
	InvalidPadding
	NonCanonical
end

record Base64DecodeError
	kind: Base64DecodeErrorKind
	input: String
	index: Integer
	message: String
end

def encode(value: Bytes): String
	return native_base64.encode(value)
end

def decode(value: String): Result<Bytes, Base64DecodeError>
	return native_base64.decode(value)
end

def url_encode(value: Bytes): String
	return native_base64.url_encode(value)
end

def url_decode(value: String): Result<Bytes, Base64DecodeError>
	return native_base64.url_decode(value)
end
`
}
