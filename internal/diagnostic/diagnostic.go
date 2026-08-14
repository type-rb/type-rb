package diagnostic

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/token"
)

// Code is a stable, machine-readable diagnostic identifier. Messages may be
// improved without changing integrations that use the code.
type Code string

const (
	SyntaxError        Code = "TRB1000"
	ResolutionError    Code = "TRB2000"
	TypeError          Code = "TRB3000"
	DuplicateBinding   Code = "TRB3001"
	UnusedBinding      Code = "TRB3002"
	ProjectError       Code = "TRB4000"
	ProjectIntegration Code = "TRB4001"
	BackendError       Code = "TRB5000"
)

type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

type Diagnostic struct {
	Code     Code
	Severity Severity
	Message  string
	Path     string
	Span     token.Span
	Related  []RelatedInformation
	Fixes    []Fix
}

type Location struct {
	Path string
	Span token.Span
}

type RelatedInformation struct {
	Message  string
	Location Location
}

// Fix describes one coherent source change. A fix may contain multiple edits
// so refactors such as adding an import and replacing a name remain atomic.
type Fix struct {
	Message string
	Edits   []TextEdit
}

type TextEdit struct {
	Location    Location
	Replacement string
}

// Normalize fills phase and source defaults without replacing more specific
// information recorded at the diagnostic's origin.
func Normalize(items []Diagnostic, path string, fallback Code) []Diagnostic {
	for index := range items {
		if items[index].Code == "" {
			items[index].Code = fallback
		}
		if items[index].Path == "" {
			items[index].Path = path
		}
		for relatedIndex := range items[index].Related {
			if items[index].Related[relatedIndex].Location.Path == "" {
				items[index].Related[relatedIndex].Location.Path = items[index].Path
			}
		}
		for fixIndex := range items[index].Fixes {
			for editIndex := range items[index].Fixes[fixIndex].Edits {
				if items[index].Fixes[fixIndex].Edits[editIndex].Location.Path == "" {
					items[index].Fixes[fixIndex].Edits[editIndex].Location.Path = items[index].Path
				}
			}
		}
	}
	return items
}

func (d Diagnostic) String() string {
	prefix := ""
	if d.Code != "" {
		prefix = string(d.Code) + ": "
	}
	return fmt.Sprintf("%s:%s: %s%s", d.Span.Start, d.Severity, prefix, d.Message)
}
