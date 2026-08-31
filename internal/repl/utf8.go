package repl

import "unicode/utf8"

// utf8WithReplacement follows the Unicode maximal-subpart replacement rule
// used by Ruby's UTF-8 transcoder and the WHATWG UTF-8 decoder. Go's
// strings.ToValidUTF8 is not suitable here because it replaces an entire run
// of invalid bytes with one replacement character.
func utf8WithReplacement(input []byte) string {
	if utf8.Valid(input) {
		return string(input)
	}

	output := make([]rune, 0, len(input))
	continuation := func(value byte) bool {
		return value >= 0x80 && value <= 0xbf
	}
	for len(input) > 0 {
		character, size := utf8.DecodeRune(input)
		if character != utf8.RuneError || size > 1 {
			output = append(output, character)
			input = input[size:]
			continue
		}

		maximalSubpart := 1
		if len(input) > 1 {
			first, second := input[0], input[1]
			validSecond := false
			switch {
			case first == 0xe0:
				validSecond = second >= 0xa0 && second <= 0xbf
			case first >= 0xe1 && first <= 0xec:
				validSecond = continuation(second)
			case first == 0xed:
				validSecond = second >= 0x80 && second <= 0x9f
			case first >= 0xee && first <= 0xef:
				validSecond = continuation(second)
			case first == 0xf0:
				validSecond = second >= 0x90 && second <= 0xbf
			case first >= 0xf1 && first <= 0xf3:
				validSecond = continuation(second)
			case first == 0xf4:
				validSecond = second >= 0x80 && second <= 0x8f
			}
			if validSecond {
				maximalSubpart = 2
				if first >= 0xf0 && len(input) > 2 && continuation(input[2]) {
					maximalSubpart = 3
				}
			}
		}
		output = append(output, utf8.RuneError)
		input = input[maximalSubpart:]
	}
	return string(output)
}
