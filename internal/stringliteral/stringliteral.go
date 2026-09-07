// Package stringliteral decodes TypeRB source String literals independently of
// the selected target language.
package stringliteral

import (
	"strconv"
	"strings"
)

// Unquote accepts the existing quoted-literal escapes plus \# in double-quoted
// Strings. Pairwise scanning preserves backslash parity and never interprets
// decoded text as interpolation source.
func Unquote(raw string) (string, error) {
	if len(raw) < 2 || raw[0] != '"' {
		return strconv.Unquote(raw)
	}
	var normalized strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			if raw[i+1] == '#' {
				normalized.WriteByte('#')
			} else {
				normalized.WriteByte(raw[i])
				normalized.WriteByte(raw[i+1])
			}
			i++
		} else {
			normalized.WriteByte(raw[i])
		}
	}
	return strconv.Unquote(normalized.String())
}

// Text decodes one literal segment between interpolation expressions.
func Text(raw string) (string, error) {
	return Unquote(`"` + raw + `"`)
}
