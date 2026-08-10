package ruby

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) intrinsic(name string, call *ir.Call, arguments []string) string {
	if name == "trb.internal.runtime.fail" {
		return "raise " + arguments[0]
	}
	if strings.HasPrefix(name, "trb.orm.") {
		return g.ormIntrinsic(name, call, arguments)
	}
	unicodeCall := func(symbol string) string {
		if _, named := call.Callee.(*ir.Identifier); named {
			return symbol
		}
		return "Unicode." + symbol
	}
	pathCall := func(symbol string) string {
		if _, named := call.Callee.(*ir.Identifier); named {
			return symbol
		}
		return "Path." + symbol
	}
	filesystemOK := func(value string) string {
		return "Result::Ok.new(" + value + ")"
	}
	filesystemError := func(operation, path, message string) string {
		value := "FileError.new(operation: " + strconv.Quote(operation) + ", path: " + path + ", message: " + message + ")"
		return "Result::Err.new(" + value + ")"
	}
	processError := func(operation, command, message string) string {
		value := "ProcessError.new(operation: " + strconv.Quote(operation) + ", command: " + command + ", message: " + message + ")"
		return "Result::Err.new(" + value + ")"
	}
	numberParseError := func(kind, input, message string) string {
		value := "NumberParseError.new(kind: NumberParseErrorKind::" + kind + ", input: " + input + ", message: " + strconv.Quote(message) + ")"
		return "Result::Err.new(" + value + ")"
	}
	hexDecodeError := func(kind, input, index, message string) string {
		value := "HexDecodeError.new(kind: HexDecodeErrorKind::" + kind + ", input: " + input + ", index: " + index + ", message: " + strconv.Quote(message) + ")"
		return "Result::Err.new(" + value + ")"
	}
	base64DecodeError := func(kind, input, index, message string) string {
		value := "Base64DecodeError.new(kind: Base64DecodeErrorKind::" + kind + ", input: " + input + ", index: " + index + ", message: " + strconv.Quote(message) + ")"
		return "Result::Err.new(" + value + ")"
	}
	percentDecodeError := func(kind, input, message string) string {
		value := "PercentDecodeError.new(kind: PercentDecodeErrorKind::" + kind + ", input: " + input + ", message: " + strconv.Quote(message) + ")"
		return "Result::Err.new(" + value + ")"
	}
	indexLookupError := func(index, size, message string) string {
		value := "IndexLookupError.new(index: " + index + ", size: " + size + ", message: " + strconv.Quote(message) + ")"
		return "Result::Err.new(" + value + ")"
	}
	keyLookupError := func(key, message string) string {
		value := "KeyLookupError.new(key: " + key + ", message: " + strconv.Quote(message) + ")"
		return "Result::Err.new(" + value + ")"
	}
	switch name {
	case "trb.std.io.puts":
		if len(call.Arguments) == 1 && call.Arguments[0].Value.ExprType().Kind == types.Float {
			return "$stdout.puts(" + portableFloatString(arguments[0]) + ")"
		}
		return "$stdout.puts(" + strings.Join(arguments, ", ") + ")"
	case "trb.std.path.separator":
		return pathCall("separator") + "()"
	case "trb.std.path.clean":
		return pathCall("clean") + "(" + arguments[0] + ")"
	case "trb.std.path.join":
		return pathCall("join") + "(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.path.absolute":
		return pathCall("absolute") + "(" + arguments[0] + ")"
	case "trb.std.path.components":
		return pathCall("components") + "(" + arguments[0] + ")"
	case "trb.std.path.base":
		return pathCall("base") + "(" + arguments[0] + ")"
	case "trb.std.path.directory":
		return pathCall("directory") + "(" + arguments[0] + ")"
	case "trb.std.url.encode_component":
		return "->(value) { value.encode(Encoding::UTF_8).bytes.map { |byte| unreserved = (byte >= 65 && byte <= 90) || (byte >= 97 && byte <= 122) || (byte >= 48 && byte <= 57) || byte == 45 || byte == 46 || byte == 95 || byte == 126; unreserved ? byte.chr : format(\"%%%02X\", byte) }.join }.call(" + arguments[0] + ")"
	case "trb.std.url.decode_component":
		invalidEscape := percentDecodeError("InvalidEscape", "input", "invalid percent escape in URL component")
		invalidUtf8 := percentDecodeError("InvalidUtf8", "input", "decoded URL component is not valid UTF-8")
		return "->(input) { characters = input.each_char.to_a; bytes = []; failure = nil; index = 0; while index < characters.length; character = characters[index]; if character != \"%\"; bytes.concat(character.encode(Encoding::UTF_8).bytes); index += 1; next; end; if index + 2 >= characters.length || !characters[index + 1].match?(/\\A[0-9A-Fa-f]\\z/) || !characters[index + 2].match?(/\\A[0-9A-Fa-f]\\z/); failure = " + invalidEscape + "; break; end; bytes << (characters[index + 1] + characters[index + 2]).to_i(16); index += 3; end; if failure; failure; else; value = bytes.pack(\"C*\").force_encoding(Encoding::UTF_8); if value.valid_encoding?; Result::Ok.new(value); else; " + invalidUtf8 + "; end; end }.call(" + arguments[0] + ")"
	case "trb.internal.filesystem.exists":
		return "->(path) { begin; File.stat(path); " + filesystemOK("true") + "; rescue Errno::ENOENT; " + filesystemOK("false") + "; rescue StandardError => error; " + filesystemError("exists", "path", "error.message") + "; end }.call(" + arguments[0] + ")"
	case "trb.internal.filesystem.read_text":
		value := "File.binread(path).force_encoding(Encoding::UTF_8).encode(Encoding::UTF_8, invalid: :replace, undef: :replace)"
		return "->(path) { begin; " + filesystemOK(value) + "; rescue StandardError => error; " + filesystemError("read_text", "path", "error.message") + "; end }.call(" + arguments[0] + ")"
	case "trb.internal.filesystem.read_bytes":
		return "->(path) { begin; " + filesystemOK("File.binread(path).b") + "; rescue StandardError => error; " + filesystemError("read_bytes", "path", "error.message") + "; end }.call(" + arguments[0] + ")"
	case "trb.internal.filesystem.write_text":
		return "->(path, value) { begin; File.binwrite(path, value.encode(Encoding::UTF_8)); " + filesystemOK("Unit.new") + "; rescue StandardError => error; " + filesystemError("write_text", "path", "error.message") + "; end }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.internal.filesystem.write_bytes":
		return "->(path, value) { begin; File.binwrite(path, value); " + filesystemOK("Unit.new") + "; rescue StandardError => error; " + filesystemError("write_bytes", "path", "error.message") + "; end }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.internal.filesystem.create_directory":
		return "->(path) { begin; require \"fileutils\"; FileUtils.mkdir_p(path); " + filesystemOK("Unit.new") + "; rescue StandardError => error; " + filesystemError("create_directory", "path", "error.message") + "; end }.call(" + arguments[0] + ")"
	case "trb.internal.filesystem.list":
		return "->(path) { begin; " + filesystemOK("Dir.children(path).sort") + "; rescue StandardError => error; " + filesystemError("list", "path", "error.message") + "; end }.call(" + arguments[0] + ")"
	case "trb.internal.process.arguments":
		return "ARGV.dup"
	case "trb.internal.process.environment":
		return "ENV[" + arguments[0] + "]"
	case "trb.internal.process.working_directory":
		return "-> { begin; " + filesystemOK("Dir.pwd") + "; rescue StandardError => error; " + processError("working_directory", strconv.Quote(""), "error.message") + "; end }.call"
	case "trb.internal.process.run":
		text := "->(value) { value.force_encoding(Encoding::UTF_8).encode(Encoding::UTF_8, invalid: :replace, undef: :replace) }"
		value := "ProcessResult.new(status: status.exitstatus || -1, stdout: text.call(stdout), stderr: text.call(stderr), success: status.success?)"
		return "->(command, arguments) { begin; require \"open3\"; stdout, stderr, status = Open3.capture3(command, *arguments); text = " + text + "; " + filesystemOK(value) + "; rescue StandardError => error; " + processError("run", "command", "error.message") + "; end }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.internal.json.parse":
		return rubyJSONParse(arguments[0], false)
	case "trb.internal.json.parse_jsonc":
		return rubyJSONParse(arguments[0], true)
	case "trb.internal.json.stringify":
		return rubyJSONStringify(arguments[0])
	case "trb.internal.json.decode":
		return rubyJSONDecode(call, arguments[0])
	case "trb.internal.json.encode":
		return rubyJSONEncode(call, arguments[0])
	case "trb.web.request_json":
		return rubyWebRequestJSON(call, arguments[0])
	case "trb.web.json":
		return rubyWebJSON(call, arguments)
	case "trb.web.configure_server":
		values := map[string]string{
			"host":                          `"0.0.0.0"`,
			"port":                          "3000",
			"body_limit_bytes":              "1048576",
			"shutdown_timeout_milliseconds": "10000",
		}
		for index, argument := range call.Arguments {
			values[argument.Name] = g.expr(call.Arguments[index].Value)
		}
		return "ServerConfig.new(host: " + values["host"] + ", port: " + values["port"] + ", body_limit_bytes: " + values["body_limit_bytes"] + ", shutdown_timeout_milliseconds: " + values["shutdown_timeout_milliseconds"] + ")"
	case "trb.web.serve":
		config := `ServerConfig.new(host: "0.0.0.0", port: 3000, body_limit_bytes: 1048576, shutdown_timeout_milliseconds: 10000)`
		if len(arguments) > 0 {
			config = arguments[0]
		}
		return "trb_web_serve(" + config + ")"
	case "trb.web.testing.dispatch":
		return "trb_web_dispatch(" + arguments[0] + ")"
	case "trb.web.middleware.logger.call":
		return rubyWebLogger(arguments)
	case "trb.std.strings.length":
		return arguments[0] + ".each_codepoint.count"
	case "trb.std.strings.empty":
		return arguments[0] + ".empty?"
	case "trb.std.strings.strip", "trb.std.strings.lstrip", "trb.std.strings.rstrip":
		whitespace := `[\u{0009}-\u{000D}\u{0020}\u{0085}\u{00A0}\u{1680}\u{2000}-\u{200A}\u{2028}-\u{2029}\u{202F}\u{205F}\u{3000}]`
		value := "(" + arguments[0] + ")"
		if name != "trb.std.strings.rstrip" {
			value += `.sub(/\A` + whitespace + `+/u, "")`
		}
		if name != "trb.std.strings.lstrip" {
			value += `.sub(/` + whitespace + `+\z/u, "")`
		}
		return value
	case "trb.std.strings.uppercase":
		return arguments[0] + ".upcase"
	case "trb.std.strings.lowercase":
		return arguments[0] + ".downcase"
	case "trb.std.strings.starts_with":
		return arguments[0] + ".start_with?(" + arguments[1] + ")"
	case "trb.std.strings.ends_with":
		return arguments[0] + ".end_with?(" + arguments[1] + ")"
	case "trb.std.strings.split":
		return "->(value, separator) { raise ArgumentError, \"String split separator is empty\" if separator.empty?; value.split(separator, -1) }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.strings.contains":
		return arguments[0] + ".include?(" + arguments[1] + ")"
	case "trb.std.strings.replace_all":
		return "->(value, pattern, replacement) { raise ArgumentError, \"String replacement pattern is empty\" if pattern.empty?; value.gsub(pattern) { replacement } }.call(" + arguments[0] + ", " + arguments[1] + ", " + arguments[2] + ")"
	case "trb.std.strings.codepoints":
		return arguments[0] + ".codepoints"
	case "trb.std.strings.characters":
		return arguments[0] + ".each_char.to_a"
	case "trb.std.strings.reverse":
		return arguments[0] + ".each_char.to_a.reverse.join"
	case "trb.std.strings.fetch":
		return "->(value, index) { characters = value.each_char.to_a; raise IndexError, \"String index is out of bounds\" if index < 0 || index >= characters.length; characters.fetch(index) }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.strings.try_fetch":
		return "->(value, index) { characters = value.each_char.to_a; index < 0 || index >= characters.length ? " + indexLookupError("index", "characters.length", "String index is out of bounds") + " : Result::Ok.new(characters.fetch(index)) }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.unicode.version":
		return unicodeCall("version") + "()"
	case "trb.std.unicode.valid_scalar":
		return unicodeCall("valid_scalar") + "(" + arguments[0] + ")"
	case "trb.std.unicode.letter":
		return unicodeCall("letter") + "(" + arguments[0] + ")"
	case "trb.std.unicode.digit":
		return unicodeCall("digit") + "(" + arguments[0] + ")"
	case "trb.std.unicode.uppercase":
		return unicodeCall("uppercase") + "(" + arguments[0] + ")"
	case "trb.std.unicode.lowercase":
		return unicodeCall("lowercase") + "(" + arguments[0] + ")"
	case "trb.std.unicode.whitespace":
		return unicodeCall("whitespace") + "(" + arguments[0] + ")"
	case "trb.std.unicode.identifier_start":
		return unicodeCall("identifier_start") + "(" + arguments[0] + ")"
	case "trb.std.unicode.identifier_part":
		return unicodeCall("identifier_part") + "(" + arguments[0] + ")"
	case "trb.std.unicode.from_codepoint":
		return unicodeCall("from_codepoint") + "(" + arguments[0] + ")"
	case "trb.std.bytes.from_string":
		return "(" + arguments[0] + ").encode(Encoding::UTF_8).b"
	case "trb.std.bytes.to_string":
		return "(" + arguments[0] + ").dup.force_encoding(Encoding::UTF_8).encode(Encoding::UTF_8, invalid: :replace, undef: :replace)"
	case "trb.std.bytes.length":
		return arguments[0] + ".bytesize"
	case "trb.std.bytes.at":
		return arguments[0] + ".bytes.fetch(" + arguments[1] + ")"
	case "trb.std.bytes.concat":
		return arguments[0] + " + " + arguments[1]
	case "trb.std.bytes.valid_utf8":
		return "(" + arguments[0] + ").dup.force_encoding(Encoding::UTF_8).valid_encoding?"
	case "trb.std.encoding.hex.encode":
		return "(" + arguments[0] + ").unpack1(\"H*\")"
	case "trb.std.encoding.hex.decode":
		invalid := hexDecodeError("InvalidCharacter", "input", "invalid_index", "invalid hexadecimal character")
		odd := hexDecodeError("OddLength", "input", "characters.length", "hex input has odd length")
		return "->(input) { characters = input.each_char.to_a; invalid_index = characters.find_index { |character| !character.match?(/\\A[0-9A-Fa-f]\\z/) }; if invalid_index; " + invalid + "; elsif characters.length.odd?; " + odd + "; else; Result::Ok.new([input].pack(\"H*\").b); end }.call(" + arguments[0] + ")"
	case "trb.std.encoding.base64.encode":
		return "[" + arguments[0] + "].pack(\"m0\")"
	case "trb.std.encoding.base64.url_encode":
		return "[" + arguments[0] + "].pack(\"m0\").tr(\"+/\", \"-_\").delete(\"=\")"
	case "trb.std.encoding.base64.decode":
		lengthError := base64DecodeError("InvalidLength", "input", "characters.length", "base64 input length must be a multiple of 4")
		paddingError := base64DecodeError("InvalidPadding", "input", "index", "invalid base64 padding")
		characterError := base64DecodeError("InvalidCharacter", "input", "index", "invalid base64 character")
		nonCanonical := base64DecodeError("NonCanonical", "input", "characters.length - padding - 1", "non-canonical base64 encoding")
		return "->(input) { characters = input.each_char.to_a; if characters.length % 4 != 0; " + lengthError + "; else; padding = 0; failure = nil; characters.each_with_index do |character, index|; if character == \"=\"; padding += 1; if index < characters.length - 2 || padding > 2; failure = " + paddingError + "; break; end; elsif padding > 0; failure = " + paddingError + "; break; elsif !character.match?(/\\A[A-Za-z0-9+\\/]\\z/); failure = " + characterError + "; break; end; end; if failure; failure; else; begin; value = input.unpack1(\"m0\").b; if [value].pack(\"m0\") != input; " + nonCanonical + "; else; Result::Ok.new(value); end; rescue ArgumentError; " + nonCanonical + "; end; end; end }.call(" + arguments[0] + ")"
	case "trb.std.encoding.base64.url_decode":
		lengthError := base64DecodeError("InvalidLength", "input", "characters.length", "base64url input has invalid length")
		paddingError := base64DecodeError("InvalidPadding", "input", "index", "base64url input must not contain padding")
		characterError := base64DecodeError("InvalidCharacter", "input", "index", "invalid base64url character")
		nonCanonical := base64DecodeError("NonCanonical", "input", "characters.length - 1", "non-canonical base64url encoding")
		return "->(input) { characters = input.each_char.to_a; if characters.length % 4 == 1; " + lengthError + "; else; failure = nil; characters.each_with_index do |character, index|; if character == \"=\"; failure = " + paddingError + "; break; elsif !character.match?(/\\A[A-Za-z0-9_-]\\z/); failure = " + characterError + "; break; end; end; if failure; failure; else; padded = input.tr(\"-_\", \"+/\") + \"=\" * ((4 - input.length % 4) % 4); begin; value = padded.unpack1(\"m0\").b; canonical = [value].pack(\"m0\").tr(\"+/\", \"-_\").delete(\"=\"); if canonical != input; " + nonCanonical + "; else; Result::Ok.new(value); end; rescue ArgumentError; " + nonCanonical + "; end; end; end }.call(" + arguments[0] + ")"
	case "trb.std.hash.md5":
		return "->(value) { require \"digest\"; Digest::MD5.digest(value).b }.call(" + arguments[0] + ")"
	case "trb.std.hash.sha1":
		return "->(value) { require \"digest\"; Digest::SHA1.digest(value).b }.call(" + arguments[0] + ")"
	case "trb.std.hash.sha256":
		return "->(value) { require \"digest\"; Digest::SHA256.digest(value).b }.call(" + arguments[0] + ")"
	case "trb.std.hash.sha512":
		return "->(value) { require \"digest\"; Digest::SHA512.digest(value).b }.call(" + arguments[0] + ")"
	case "trb.std.hmac.sha256":
		return "->(key, message) { require \"openssl\"; OpenSSL::HMAC.digest(\"SHA256\", key, message).b }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.hmac.sha512":
		return "->(key, message) { require \"openssl\"; OpenSSL::HMAC.digest(\"SHA512\", key, message).b }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.hmac.equal", "trb.std.secure_compare.equal":
		return "->(left, right) { if left.bytesize != right.bytesize; false; else; difference = 0; left.bytes.zip(right.bytes) { |left_byte, right_byte| difference |= left_byte ^ right_byte }; difference == 0; end }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.random.float":
		return "Random.rand()"
	case "trb.std.random.integer":
		return "->(upper) { raise ArgumentError, \"random.integer upper bound must be greater than zero\" if upper <= 0; Random.rand(upper) }.call(" + arguments[0] + ")"
	case "trb.std.secure_random.bytes":
		return "->(length) { raise ArgumentError, \"secure_random.bytes length must be between 0 and 65536\" if length < 0 || length > 65536; Random.urandom(length).b }.call(" + arguments[0] + ")"
	case "trb.std.string_builder.new":
		return "String.new(encoding: Encoding::UTF_8)"
	case "trb.std.string_builder.from_string":
		return "(" + arguments[0] + ").dup.force_encoding(Encoding::UTF_8)"
	case "trb.std.string_builder.append":
		return arguments[0] + " << " + arguments[1]
	case "trb.std.string_builder.append_codepoint":
		return arguments[0] + " << (" + arguments[1] + ").chr(Encoding::UTF_8)"
	case "trb.std.string_builder.length":
		return arguments[0] + ".each_codepoint.count"
	case "trb.std.string_builder.empty":
		return arguments[0] + ".empty?"
	case "trb.std.string_builder.to_string":
		return arguments[0] + ".dup"
	case "trb.std.string_builder.clear":
		return arguments[0] + ".clear"
	case "trb.std.arrays.length":
		return arguments[0] + ".length"
	case "trb.std.arrays.empty":
		return arguments[0] + ".empty?"
	case "trb.std.arrays.fetch":
		return "->(values, index) { raise IndexError, \"Array index is out of bounds\" if index < 0 || index >= values.length; values.fetch(index) }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.arrays.try_fetch":
		return "->(values, index) { index < 0 || index >= values.length ? " + indexLookupError("index", "values.length", "Array index is out of bounds") + " : Result::Ok.new(values.fetch(index)) }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.arrays.first":
		return "->(values) { raise IndexError, \"Array is empty\" if values.empty?; values.fetch(0) }.call(" + arguments[0] + ")"
	case "trb.std.arrays.last":
		return "->(values) { raise IndexError, \"Array is empty\" if values.empty?; values.fetch(values.length - 1) }.call(" + arguments[0] + ")"
	case "trb.std.arrays.copy":
		return arguments[0] + ".dup"
	case "trb.std.arrays.contains":
		return arguments[0] + ".include?(" + arguments[1] + ")"
	case "trb.std.arrays.count":
		return arguments[0] + ".count(" + arguments[1] + ")"
	case "trb.std.arrays.join":
		return arguments[0] + ".join(" + arguments[1] + ")"
	case "trb.std.arrays.pop":
		return "->(values) { raise IndexError, \"Array is empty\" if values.empty?; values.pop }.call(" + arguments[0] + ")"
	case "trb.std.arrays.shift":
		return "->(values) { raise IndexError, \"Array is empty\" if values.empty?; values.shift }.call(" + arguments[0] + ")"
	case "trb.std.arrays.push":
		return arguments[0] + " << " + arguments[1]
	case "trb.std.arrays.unshift":
		return arguments[0] + ".unshift(" + arguments[1] + ")"
	case "trb.std.arrays.reverse":
		return arguments[0] + ".reverse"
	case "trb.std.hashes.length":
		return arguments[0] + ".length"
	case "trb.std.hashes.empty":
		return arguments[0] + ".empty?"
	case "trb.std.hashes.fetch":
		return arguments[0] + ".fetch(" + arguments[1] + ")"
	case "trb.std.hashes.try_fetch":
		return "->(values, key) { values.key?(key) ? Result::Ok.new(values[key]) : " + keyLookupError("key", "Hash key is missing") + " }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.hashes.contains_key":
		return arguments[0] + ".key?(" + arguments[1] + ")"
	case "trb.std.hashes.keys":
		return arguments[0] + ".keys"
	case "trb.std.hashes.values":
		return arguments[0] + ".values"
	case "trb.std.hashes.copy":
		return arguments[0] + ".dup"
	case "trb.std.hashes.delete":
		return "->(values, key) { raise KeyError, \"Hash key is missing\" unless values.key?(key); values.delete(key) }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.hashes.merge":
		return arguments[0] + ".merge(" + arguments[1] + ")"
	case "trb.std.hashes.update":
		return arguments[0] + ".update(" + arguments[1] + ")"
	case "trb.std.numbers.to_string":
		return arguments[0] + ".to_s"
	case "trb.std.numbers.integer_to_float":
		return "(" + arguments[0] + ").to_f"
	case "trb.std.numbers.integer_absolute":
		return "(" + arguments[0] + ").abs"
	case "trb.std.numbers.integer_min":
		return "[(" + arguments[0] + "), (" + arguments[1] + ")].min"
	case "trb.std.numbers.integer_max":
		return "[(" + arguments[0] + "), (" + arguments[1] + ")].max"
	case "trb.std.numbers.integer_clamp":
		return "->(value, minimum, maximum) { raise ArgumentError, \"clamp minimum exceeds maximum\" if minimum > maximum; value.clamp(minimum, maximum) }.call(" + strings.Join(arguments, ", ") + ")"
	case "trb.std.numbers.integer_zero":
		return "(" + arguments[0] + ").zero?"
	case "trb.std.numbers.integer_positive":
		return "(" + arguments[0] + ").positive?"
	case "trb.std.numbers.integer_negative":
		return "(" + arguments[0] + ").negative?"
	case "trb.std.numbers.integer_even":
		return "(" + arguments[0] + ").even?"
	case "trb.std.numbers.integer_odd":
		return "(" + arguments[0] + ").odd?"
	case "trb.std.numbers.float_to_string":
		return portableFloatString(arguments[0])
	case "trb.std.numbers.float_to_integer":
		return portableFloatInteger(arguments[0], "truncate")
	case "trb.std.numbers.float_floor":
		return portableFloatInteger(arguments[0], "floor")
	case "trb.std.numbers.float_ceil":
		return portableFloatInteger(arguments[0], "ceil")
	case "trb.std.numbers.float_round":
		return portableFloatInteger(arguments[0], "round")
	case "trb.std.numbers.float_absolute":
		return "(" + arguments[0] + ").abs"
	case "trb.std.numbers.float_finite":
		return "(" + arguments[0] + ").finite?"
	case "trb.std.numbers.float_infinite":
		return "!((" + arguments[0] + ").infinite?).nil?"
	case "trb.std.numbers.float_nan":
		return "(" + arguments[0] + ").nan?"
	case "trb.std.numbers.parse_integer":
		return "->(input) { raise ArgumentError, \"invalid Integer\" unless /\\A[+-]?[0-9]+\\z/.match?(input); value = Integer(input, 10); raise RangeError, \"Integer is outside the portable range\" if value < -9007199254740991 || value > 9007199254740991; value }.call(" + arguments[0] + ")"
	case "trb.std.numbers.try_parse_integer":
		return "->(input) { if !/\\A[+-]?[0-9]+\\z/.match?(input); " + numberParseError("InvalidFormat", "input", "invalid Integer") + "; else; value = Integer(input, 10); if value < -9007199254740991 || value > 9007199254740991; " + numberParseError("OutOfRange", "input", "Integer is outside the portable range") + "; else; Result::Ok.new(value); end; end }.call(" + arguments[0] + ")"
	case "trb.std.numbers.parse_float":
		return "->(input) { raise ArgumentError, \"invalid Float\" unless /\\A[+-]?(?:[0-9]+(?:\\.[0-9]*)?|\\.[0-9]+)(?:[eE][+-]?[0-9]+)?\\z/.match?(input); value = Float(input); raise RangeError, \"Float is outside the portable range\" unless value.finite?; value }.call(" + arguments[0] + ")"
	case "trb.std.numbers.try_parse_float":
		return "->(input) { if !/\\A[+-]?(?:[0-9]+(?:\\.[0-9]*)?|\\.[0-9]+)(?:[eE][+-]?[0-9]+)?\\z/.match?(input); " + numberParseError("InvalidFormat", "input", "invalid Float") + "; else; value = Float(input); if !value.finite?; " + numberParseError("OutOfRange", "input", "Float is outside the portable range") + "; else; Result::Ok.new(value); end; end }.call(" + arguments[0] + ")"
	case "trb.std.math.sqrt":
		return "->(value) { value < 0 ? Float::NAN : Math.sqrt(value) }.call(" + arguments[0] + ")"
	case "trb.std.math.exp":
		return "Math.exp(" + arguments[0] + ")"
	case "trb.std.math.log":
		return portableLog(arguments[0], "log")
	case "trb.std.math.log2":
		return portableLog(arguments[0], "log2")
	case "trb.std.math.log10":
		return portableLog(arguments[0], "log10")
	case "trb.std.booleans.to_string":
		return "(" + arguments[0] + ").to_s"
	default:
		return "nil"
	}
}

func rubyWebLogger(arguments []string) string {
	options := "logger_options = nil; "
	if len(arguments) > 2 {
		options = "logger_options = " + arguments[2] + "; "
	}
	return "-> { require \"json\"; logger_context = " + arguments[0] + "; logger_next_handler = " + arguments[1] + "; " + options + "excluded = logger_options && logger_options.exclude_paths.include?(logger_context.request.path); if excluded; logger_next_handler.call(logger_context); else; started = Process.clock_gettime(Process::CLOCK_MONOTONIC); status = 500; begin; response = logger_next_handler.call(logger_context); status = response.status; response; ensure; level = status >= 500 ? \"error\" : \"info\"; entry = { timestamp: Time.now.utc.strftime(\"%Y-%m-%dT%H:%M:%S.%9NZ\"), level: level, event: \"http_request\", method: logger_context.request.method, path: logger_context.request.path, status: status, duration_ms: (Process.clock_gettime(Process::CLOCK_MONOTONIC) - started) * 1000.0 }; output = logger_options && logger_options.stderr ? $stderr : $stdout; output.puts(JSON.generate(entry)); end; end }.call"
}

func rubyWebRequestJSON(call *ir.Call, request string) string {
	decoded := rubyJSONDecode(call, "source")
	return "-> { request_value = " + request + "; content_types = request_value.headers.each_with_object([]) { |(name, values), result| result.concat(values) if name.downcase == \"content-type\" }; if content_types.empty?; Result::Err.new(RequestError::MissingContentType); elsif content_types.length != 1; Result::Err.new(RequestError::DuplicateContentType); else; media_type = content_types.first.split(\";\", 2).first.strip.downcase; if media_type != \"application/json\" && !(media_type.start_with?(\"application/\") && media_type.end_with?(\"+json\")); Result::Err.new(RequestError::UnsupportedContentType.new(content_types.first)); else; source = request_value.body.dup.force_encoding(Encoding::UTF_8); if !source.valid_encoding?; Result::Err.new(RequestError::InvalidUtf8); else; decoded = " + decoded + "; if decoded.is_a?(Result::Err); Result::Err.new(RequestError::InvalidJson.new(decoded.error)); else; decoded; end; end; end; end }.call"
}

func rubyWebJSON(call *ir.Call, arguments []string) string {
	if call.Codec == nil || len(arguments) == 0 {
		return "nil"
	}
	status := "200"
	if len(arguments) > 1 {
		status = arguments[1]
	}
	encoded := rubyJSONEncode(call, arguments[0])
	headers := `{ "content-type" => ["application/json; charset=utf-8"] }`
	return "-> { encoded = " + encoded + "; if encoded.is_a?(Result::Err); Response.new(status: 500, headers: " + headers + ", body: \"{\\\"error\\\":\\\"internal_server_error\\\"}\".b); else; Response.new(status: " + status + ", headers: " + headers + ", body: encoded.value.b); end }.call"
}
