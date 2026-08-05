package stdlib

func jsonSource() string {
	return `import { Result } from trb/std/result
import trb/internal/json as native_json

enum JsonErrorKind
	Syntax
	Decode
	Encode
end

record JsonError
	kind: JsonErrorKind
	message: String
	path: String
	line: Integer?
	column: Integer?
end

enum JsonValue
	Null
	Boolean(value: Boolean)
	Integer(value: Integer)
	Float(value: Float)
	String(value: String)
	Array(value: Array<JsonValue>)
	Object(value: Hash<String, JsonValue>)
end

def parse(source: String): Result<JsonValue, JsonError>
	return native_json.parse(source)
end

def stringify(value: JsonValue): Result<String, JsonError>
	return native_json.stringify(value)
end

def as_boolean(value: JsonValue): Result<Boolean, JsonError>
	case value
	when JsonValue::Boolean(result)
		return Result<Boolean, JsonError>::Ok(result)
	else
		return Result<Boolean, JsonError>::Err(_decode_error("", "JSON value is not Boolean"))
	end
end

def as_integer(value: JsonValue): Result<Integer, JsonError>
	case value
	when JsonValue::Integer(result)
		return Result<Integer, JsonError>::Ok(result)
	else
		return Result<Integer, JsonError>::Err(_decode_error("", "JSON value is not Integer"))
	end
end

def as_float(value: JsonValue): Result<Float, JsonError>
	case value
	when JsonValue::Float(result)
		return Result<Float, JsonError>::Ok(result)
	else
		return Result<Float, JsonError>::Err(_decode_error("", "JSON value is not Float"))
	end
end

def as_string(value: JsonValue): Result<String, JsonError>
	case value
	when JsonValue::String(result)
		return Result<String, JsonError>::Ok(result)
	else
		return Result<String, JsonError>::Err(_decode_error("", "JSON value is not String"))
	end
end

def as_array(value: JsonValue): Result<Array<JsonValue>, JsonError>
	case value
	when JsonValue::Array(result)
		return Result<Array<JsonValue>, JsonError>::Ok(result)
	else
		return Result<Array<JsonValue>, JsonError>::Err(_decode_error("", "JSON value is not Array"))
	end
end

def as_object(value: JsonValue): Result<Hash<String, JsonValue>, JsonError>
	case value
	when JsonValue::Object(result)
		return Result<Hash<String, JsonValue>, JsonError>::Ok(result)
	else
		return Result<Hash<String, JsonValue>, JsonError>::Err(_decode_error("", "JSON value is not Object"))
	end
end

def field(value: JsonValue, name: String): Result<JsonValue, JsonError>
	case value
	when JsonValue::Object(fields)
		if fields.key?(name)
			return Result<JsonValue, JsonError>::Ok(fields.fetch(name))
		end
		return Result<JsonValue, JsonError>::Err(_decode_error("/" + name, "JSON object field is missing"))
	else
		return Result<JsonValue, JsonError>::Err(_decode_error("", "JSON value is not Object"))
	end
end

def _decode_error(path: String, message: String): JsonError
	return JsonError.new(
		kind: JsonErrorKind::Decode,
		message: message,
		path: path,
		line: nil,
		column: nil
	)
end
`
}
