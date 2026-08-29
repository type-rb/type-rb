package stdlib

import (
	"strconv"
	"strings"
	stdunicode "unicode"
)

// unicodeSource builds one compiler-owned TypeRB implementation from the Go
// toolchain's pinned Unicode tables. The generated Ruby, Go, and TypeScript
// modules consequently classify the same scalar values instead of consulting
// three independently versioned runtime databases.
func unicodeSource() string {
	var source strings.Builder
	source.WriteString("import trb/std/string_builder\n\n")
	writeRangeTable(&source, "LETTER_RANGES", stdunicode.Letter)
	writeRangeTable(&source, "DIGIT_RANGES", stdunicode.Digit)
	writeRangeTable(&source, "UPPERCASE_RANGES", stdunicode.Upper)
	writeRangeTable(&source, "LOWERCASE_RANGES", stdunicode.Lower)
	writeRangeTable(&source, "WHITESPACE_RANGES", stdunicode.White_Space)
	source.WriteString(`UNICODE_DATA_VERSION := "` + stdunicode.Version + `"


def _in_ranges(value: Integer, ranges: Array<Array<Integer>>): Boolean
	mut low := 0
	mut high := ranges.size() - 1
	while low <= high
		middle := (low + high) / 2
		entry := ranges[middle]
		if value < entry[0]
			high = middle - 1
		elsif value > entry[1]
			low = middle + 1
		else
			return (value - entry[0]) % entry[2] == 0
		end
	end
	return false
end

def _valid_scalar(value: Integer): Boolean
	return value >= 0 && value <= 1114111 && !(value >= 55296 && value <= 57343)
end

def _letter(value: Integer): Boolean
	return _valid_scalar(value) && _in_ranges(value, LETTER_RANGES)
end

def _digit(value: Integer): Boolean
	return _valid_scalar(value) && _in_ranges(value, DIGIT_RANGES)
end

def _identifier_start(value: Integer): Boolean
	return value == 95 || value == 64 || _letter(value)
end

class Unicode
	def self.version(): String
		return UNICODE_DATA_VERSION
	end

	def self.valid_scalar(value: Integer): Boolean
		return _valid_scalar(value)
	end

	def self.letter(value: Integer): Boolean
		return _letter(value)
	end

	def self.digit(value: Integer): Boolean
		return _digit(value)
	end

	def self.uppercase(value: Integer): Boolean
		return _valid_scalar(value) && _in_ranges(value, UPPERCASE_RANGES)
	end

	def self.lowercase(value: Integer): Boolean
		return _valid_scalar(value) && _in_ranges(value, LOWERCASE_RANGES)
	end

	def self.whitespace(value: Integer): Boolean
		return _valid_scalar(value) && _in_ranges(value, WHITESPACE_RANGES)
	end

	def self.identifier_start(value: Integer): Boolean
		return _identifier_start(value)
	end

	def self.identifier_part(value: Integer): Boolean
		return _identifier_start(value) || _digit(value)
	end

	def self.from_codepoint(value: Integer): String
		mut builder := string_builder.new()
		builder.append_codepoint(value)
		return builder.to_s()
	end
end

def version(): String
	return Unicode.version()
end

def valid_scalar(value: Integer): Boolean
	return Unicode.valid_scalar(value)
end

def letter(value: Integer): Boolean
	return Unicode.letter(value)
end

def digit(value: Integer): Boolean
	return Unicode.digit(value)
end

def uppercase(value: Integer): Boolean
	return Unicode.uppercase(value)
end

def lowercase(value: Integer): Boolean
	return Unicode.lowercase(value)
end

def whitespace(value: Integer): Boolean
	return Unicode.whitespace(value)
end

def identifier_start(value: Integer): Boolean
	return Unicode.identifier_start(value)
end

def identifier_part(value: Integer): Boolean
	return Unicode.identifier_part(value)
end

def from_codepoint(value: Integer): String
	return Unicode.from_codepoint(value)
end
`)
	return source.String()
}

func writeRangeTable(source *strings.Builder, name string, table *stdunicode.RangeTable) {
	source.WriteString(name + " := [\n")
	for _, value := range table.R16 {
		writeRange(source, uint64(value.Lo), uint64(value.Hi), uint64(value.Stride))
	}
	for _, value := range table.R32 {
		writeRange(source, uint64(value.Lo), uint64(value.Hi), uint64(value.Stride))
	}
	source.WriteString("]\n\n")
}

func writeRange(source *strings.Builder, low, high, stride uint64) {
	source.WriteString("\t[")
	source.WriteString(strconv.FormatUint(low, 10))
	source.WriteString(", ")
	source.WriteString(strconv.FormatUint(high, 10))
	source.WriteString(", ")
	source.WriteString(strconv.FormatUint(stride, 10))
	source.WriteString("],\n")
}
