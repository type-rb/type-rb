package stdlib

func jsoncSource() string {
	return `import { Result } from trb/std/result
import trb/std/json
import trb/internal/json as native_json

def parse(source: String): Result<JSON::Value, JSON::Error>
	return native_json.parse_jsonc(source)
end
`
}
