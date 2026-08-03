package diagnostic

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/token"
)

type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

type Diagnostic struct {
	Severity Severity
	Message  string
	Span     token.Span
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s:%s: %s", d.Span.Start, d.Severity, d.Message)
}
