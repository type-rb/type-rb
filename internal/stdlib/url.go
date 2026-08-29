package stdlib

func urlSource() string {
	return `import trb/std/result
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

record QueryParameter
	name: String
	value: String
end

def encode_component(value: String): String
	return native_url.encode_component(value)
end

def decode_component(value: String): Result<String, PercentDecodeError>
	return native_url.decode_component(value)
end

def _decode_query_component(value: String): Result<String, PercentDecodeError>
	case decode_component(value.split("+").join(" "))
	when Result::Ok(decoded)
		return Result<String, PercentDecodeError>::Ok(decoded)
	when Result::Err(error)
		case error.kind
		when PercentDecodeErrorKind::InvalidEscape
			query_error := PercentDecodeError.new(
				kind: error.kind,
				input: value,
				message: "invalid percent escape in URL query component",
			)
			return Result<String, PercentDecodeError>::Err(query_error)
		when PercentDecodeErrorKind::InvalidUtf8
			query_error := PercentDecodeError.new(
				kind: error.kind,
				input: value,
				message: "decoded URL query component is not valid UTF-8",
			)
			return Result<String, PercentDecodeError>::Err(query_error)
		end
	end
end

def _encode_query_component(value: String): String
	encoded := encode_component(value)
	return encoded.split("%20").join("+").split("%2A").join("*").split("~").join("%7E")
end

def _parse_query_parameter(value: String): Result<QueryParameter, PercentDecodeError>
	parts := value.split("=")
	name_part := parts.first()
	mut value_parts := parts.dup()
	value_parts.shift()
	value_part := value_parts.join("=")
	case _decode_query_component(name_part)
	when Result::Err(error)
		return Result<QueryParameter, PercentDecodeError>::Err(error)
	when Result::Ok(name)
		case _decode_query_component(value_part)
		when Result::Err(error)
			return Result<QueryParameter, PercentDecodeError>::Err(error)
		when Result::Ok(parameter_value)
			parameter := QueryParameter.new(name: name, value: parameter_value)
			return Result<QueryParameter, PercentDecodeError>::Ok(parameter)
		end
	end
end

def parse_query(value: String): Result<Array<QueryParameter>, PercentDecodeError>
	mut parameters: Array<QueryParameter> := []
	value.split("&").each do |part|
		if part == ""
			next
		end
		case _parse_query_parameter(part)
		when Result::Ok(parameter)
			parameters.push(parameter)
		when Result::Err(error)
			return Result<Array<QueryParameter>, PercentDecodeError>::Err(error)
		end
	end
	return Result<Array<QueryParameter>, PercentDecodeError>::Ok(parameters)
end

def build_query(parameters: Array<QueryParameter>): String
	mut parts: Array<String> := []
	parameters.each do |parameter|
		name := _encode_query_component(parameter.name)
		value := _encode_query_component(parameter.value)
		parts.push(name + "=" + value)
	end
	return parts.join("&")
end
`
}
