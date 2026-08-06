package repl

import (
	"strings"

	"github.com/type-rb/type-rb/internal/languageservice"
)

const (
	colorReset    = "\x1b[0m"
	colorTitle    = "\x1b[1;38;2;130;170;255m"
	colorInput    = "\x1b[1;38;2;214;222;235m"
	colorMuted    = "\x1b[38;2;98;114;164m"
	colorName     = "\x1b[38;2;197;228;120m"
	colorValue    = "\x1b[38;2;197;228;120m"
	colorType     = "\x1b[38;2;127;219;202m"
	colorSuccess  = "\x1b[38;2;34;218;110m"
	colorError    = "\x1b[38;2;255;88;116m"
	colorKeyword  = "\x1b[1;38;2;198;120;221m"
	colorString   = "\x1b[38;2;197;228;120m"
	colorNumber   = "\x1b[38;2;255;184;108m"
	colorComment  = "\x1b[3;38;2;98;114;164m"
	colorFunction = "\x1b[38;2;130;170;255m"
	colorConstant = "\x1b[38;2;255;214;102m"
	colorInvalid  = "\x1b[4;38;2;255;88;116m"
)

func colorize(enabled bool, color, value string) string {
	if !enabled {
		return value
	}
	return color + value + colorReset
}

func highlightInput(source string, spans []languageservice.HighlightSpan) string {
	var output strings.Builder
	output.WriteString(colorInput)
	cursor := 0
	for _, span := range spans {
		if span.Range.Start < cursor || span.Range.Start < 0 || span.Range.End > len(source) || span.Range.End <= span.Range.Start {
			continue
		}
		output.WriteString(source[cursor:span.Range.Start])
		output.WriteString(highlightColor(span.Kind))
		output.WriteString(source[span.Range.Start:span.Range.End])
		output.WriteString(colorInput)
		cursor = span.Range.End
	}
	output.WriteString(source[cursor:])
	output.WriteString(colorReset)
	return output.String()
}

func highlightColor(kind languageservice.HighlightKind) string {
	switch kind {
	case languageservice.HighlightKeyword:
		return colorKeyword
	case languageservice.HighlightType:
		return colorType
	case languageservice.HighlightConstant:
		return colorConstant
	case languageservice.HighlightString:
		return colorString
	case languageservice.HighlightNumber, languageservice.HighlightBoolean:
		return colorNumber
	case languageservice.HighlightComment:
		return colorComment
	case languageservice.HighlightFunction, languageservice.HighlightMethod:
		return colorFunction
	case languageservice.HighlightInvalid:
		return colorInvalid
	default:
		return colorInput
	}
}
