package diagnostic

import "github.com/type-rb/type-rb/internal/token"

const JSONSchemaVersion = 1

type JSONReport struct {
	SchemaVersion int              `json:"schemaVersion"`
	ToolVersion   string           `json:"toolVersion,omitempty"`
	Diagnostics   []JSONDiagnostic `json:"diagnostics"`
	Summary       JSONSummary      `json:"summary"`
}

type JSONSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

type JSONDiagnostic struct {
	Code               Code                     `json:"code"`
	Severity           Severity                 `json:"severity"`
	Message            string                   `json:"message"`
	Location           *JSONLocation            `json:"location,omitempty"`
	RelatedInformation []JSONRelatedInformation `json:"relatedInformation,omitempty"`
	Fixes              []JSONFix                `json:"fixes,omitempty"`
}

type JSONRelatedInformation struct {
	Message  string       `json:"message"`
	Location JSONLocation `json:"location"`
}

type JSONFix struct {
	Message string         `json:"message"`
	Edits   []JSONTextEdit `json:"edits"`
}

type JSONTextEdit struct {
	Location    JSONLocation `json:"location"`
	Replacement string       `json:"replacement"`
}

type JSONLocation struct {
	Path string   `json:"path"`
	Span JSONSpan `json:"span"`
}

type JSONSpan struct {
	Start JSONPosition `json:"start"`
	End   JSONPosition `json:"end"`
}

type JSONPosition struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

func NewJSONReport(items []Diagnostic) JSONReport {
	report := JSONReport{SchemaVersion: JSONSchemaVersion, Diagnostics: make([]JSONDiagnostic, 0, len(items))}
	for _, item := range items {
		converted := JSONDiagnostic{Code: item.Code, Severity: item.Severity, Message: item.Message}
		if item.Path != "" {
			location := jsonLocation(Location{Path: item.Path, Span: item.Span})
			converted.Location = &location
		}
		for _, related := range item.Related {
			converted.RelatedInformation = append(converted.RelatedInformation, JSONRelatedInformation{
				Message: related.Message, Location: jsonLocation(related.Location),
			})
		}
		for _, fix := range item.Fixes {
			convertedFix := JSONFix{Message: fix.Message, Edits: make([]JSONTextEdit, 0, len(fix.Edits))}
			for _, edit := range fix.Edits {
				convertedFix.Edits = append(convertedFix.Edits, JSONTextEdit{
					Location: jsonLocation(edit.Location), Replacement: edit.Replacement,
				})
			}
			converted.Fixes = append(converted.Fixes, convertedFix)
		}
		report.Diagnostics = append(report.Diagnostics, converted)
		switch item.Severity {
		case Error:
			report.Summary.Errors++
		case Warning:
			report.Summary.Warnings++
		}
	}
	return report
}

func jsonLocation(location Location) JSONLocation {
	return JSONLocation{Path: location.Path, Span: JSONSpan{
		Start: jsonPosition(location.Span.Start),
		End:   jsonPosition(location.Span.End),
	}}
}

func jsonPosition(position token.Position) JSONPosition {
	return JSONPosition{Offset: position.Offset, Line: position.Line, Column: position.Column}
}
