package stdlib

func urlSource() string {
	return `import { Result } from trb/std/result
import trb/internal/url as native_url

module URL
	enum DecodeErrorKind
		InvalidEscape
		InvalidUtf8
	end

	record DecodeError
		kind: URL::DecodeErrorKind
		input: String
		message: String
	end

	record QueryParameter
		name: String
		value: String
	end
end

def encode_component(value: String): String
	return native_url.encode_component(value)
end

def decode_component(value: String): Result<String, URL::DecodeError>
	return native_url.decode_component(value)
end

def _decode_query_component(value: String): Result<String, URL::DecodeError>
	case decode_component(value.split("+").join(" "))
	when Result::Ok(decoded)
		return Result<String, URL::DecodeError>::Ok(decoded)
	when Result::Err(error)
		case error.kind
		when URL::DecodeErrorKind::InvalidEscape
			query_error := URL::DecodeError.new(
				kind: error.kind,
				input: value,
				message: "invalid percent escape in URL query component",
			)
			return Result<String, URL::DecodeError>::Err(query_error)
		when URL::DecodeErrorKind::InvalidUtf8
			query_error := URL::DecodeError.new(
				kind: error.kind,
				input: value,
				message: "decoded URL query component is not valid UTF-8",
			)
			return Result<String, URL::DecodeError>::Err(query_error)
		end
	end
end

def _encode_query_component(value: String): String
	encoded := encode_component(value)
	return encoded.split("%20").join("+").split("%2A").join("*").split("~").join("%7E")
end

def _parse_query_parameter(value: String): Result<URL::QueryParameter, URL::DecodeError>
	parts := value.split("=")
	name_part := parts.first()
	mut value_parts := parts.dup()
	value_parts.shift()
	value_part := value_parts.join("=")
	case _decode_query_component(name_part)
	when Result::Err(error)
		return Result<URL::QueryParameter, URL::DecodeError>::Err(error)
	when Result::Ok(name)
		case _decode_query_component(value_part)
		when Result::Err(error)
			return Result<URL::QueryParameter, URL::DecodeError>::Err(error)
		when Result::Ok(parameter_value)
			parameter := URL::QueryParameter.new(name: name, value: parameter_value)
			return Result<URL::QueryParameter, URL::DecodeError>::Ok(parameter)
		end
	end
end

def parse_query(value: String): Result<Array<URL::QueryParameter>, URL::DecodeError>
	mut parameters: Array<URL::QueryParameter> := []
	value.split("&").each do |part|
		if part == ""
			next
		end
		case _parse_query_parameter(part)
		when Result::Ok(parameter)
			parameters.push(parameter)
		when Result::Err(error)
			return Result<Array<URL::QueryParameter>, URL::DecodeError>::Err(error)
		end
	end
	return Result<Array<URL::QueryParameter>, URL::DecodeError>::Ok(parameters)
end

def build_query(parameters: Array<URL::QueryParameter>): String
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
