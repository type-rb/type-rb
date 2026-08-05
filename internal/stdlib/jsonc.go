package stdlib

func jsoncSource() string {
	return `import { Result } from trb/std/result
import { JsonError, JsonErrorKind, JsonValue } from trb/std/json
import trb/internal/json as native_json

def parse(source: String): Result<JsonValue, JsonError>
	return native_json.parse_jsonc(source)
end
`
}
