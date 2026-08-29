package stdlib

func jsonSource() string {
	return `import trb/std/result
import trb/internal/json as native_json

module JSON
	enum ErrorKind
		Syntax
		Decode
		Encode
	end

	record Error
		kind: JSON::ErrorKind
		message: String
		path: String
		line: Integer?
		column: Integer?
	end

	enum Value
		Null
		Boolean(value: Boolean)
		Integer(value: Integer)
		Float(value: Float)
		String(value: String)
		Array(value: Array<JSON::Value>)
		Object(value: Hash<String, JSON::Value>)
	end
end

def parse(source: String): Result<JSON::Value, JSON::Error>
	return native_json.parse(source)
end

def stringify(value: JSON::Value): Result<String, JSON::Error>
	return native_json.stringify(value)
end

def as_boolean(value: JSON::Value): Result<Boolean, JSON::Error>
	case value
	when JSON::Value::Boolean(result)
		return Result<Boolean, JSON::Error>::Ok(result)
	else
		return Result<Boolean, JSON::Error>::Err(_decode_error("", "JSON value is not Boolean"))
	end
end

def as_integer(value: JSON::Value): Result<Integer, JSON::Error>
	case value
	when JSON::Value::Integer(result)
		return Result<Integer, JSON::Error>::Ok(result)
	else
		return Result<Integer, JSON::Error>::Err(_decode_error("", "JSON value is not Integer"))
	end
end

def as_float(value: JSON::Value): Result<Float, JSON::Error>
	case value
	when JSON::Value::Float(result)
		return Result<Float, JSON::Error>::Ok(result)
	else
		return Result<Float, JSON::Error>::Err(_decode_error("", "JSON value is not Float"))
	end
end

def as_string(value: JSON::Value): Result<String, JSON::Error>
	case value
	when JSON::Value::String(result)
		return Result<String, JSON::Error>::Ok(result)
	else
		return Result<String, JSON::Error>::Err(_decode_error("", "JSON value is not String"))
	end
end

def as_array(value: JSON::Value): Result<Array<JSON::Value>, JSON::Error>
	case value
	when JSON::Value::Array(result)
		return Result<Array<JSON::Value>, JSON::Error>::Ok(result)
	else
		return Result<Array<JSON::Value>, JSON::Error>::Err(_decode_error("", "JSON value is not Array"))
	end
end

def as_object(value: JSON::Value): Result<Hash<String, JSON::Value>, JSON::Error>
	case value
	when JSON::Value::Object(result)
		return Result<Hash<String, JSON::Value>, JSON::Error>::Ok(result)
	else
		return Result<Hash<String, JSON::Value>, JSON::Error>::Err(_decode_error("", "JSON value is not Object"))
	end
end

def field(value: JSON::Value, name: String): Result<JSON::Value, JSON::Error>
	case value
	when JSON::Value::Object(fields)
		if fields.key?(name)
			return Result<JSON::Value, JSON::Error>::Ok(fields.fetch(name))
		end
		return Result<JSON::Value, JSON::Error>::Err(_decode_error("/" + name, "JSON object field is missing"))
	else
		return Result<JSON::Value, JSON::Error>::Err(_decode_error("", "JSON value is not Object"))
	end
end

def _decode_error(path: String, message: String): JSON::Error
	return JSON::Error.new(
		kind: JSON::ErrorKind::Decode,
		message: message,
		path: path,
		line: nil,
		column: nil
	)
end
`
}
