package stdlib

func urlSource() string {
	return `import { Result } from trb/std/result
import trb/internal/url as native_url

enum PercentDecodeErrorKind
	InvalidEscape
	InvalidUtf8
end

record PercentDecodeError
	kind: PercentDecodeErrorKind
	input: String
	message: String
end

def encode_component(value: String): String
	return native_url.encode_component(value)
end

def decode_component(value: String): Result<String, PercentDecodeError>
	return native_url.decode_component(value)
end
`
}
