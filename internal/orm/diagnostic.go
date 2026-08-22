package orm

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/token"
)

type providerDiagnosticError struct {
	diagnostics []diagnostic.Diagnostic
}

func (e *providerDiagnosticError) Error() string {
	if e == nil || len(e.diagnostics) == 0 {
		return "ORM declaration analysis failed"
	}
	return e.diagnostics[0].Message
}

// Diagnostics exposes provider failures to the compiler without coupling the
// provider interface to one diagnostic implementation.
func (e *providerDiagnosticError) Diagnostics() []diagnostic.Diagnostic {
	if e == nil {
		return nil
	}
	return append([]diagnostic.Diagnostic(nil), e.diagnostics...)
}

func associationModelGroupError(source, target Model, targetSpan *token.Span, association string, span token.Span) error {
	sourceGroup := displayModelGroup(modelGroup(source.ModulePath))
	targetGroup := displayModelGroup(modelGroup(target.ModulePath))
	item := diagnostic.Diagnostic{
		Code:     diagnostic.ProjectIntegration,
		Severity: diagnostic.Error,
		Path:     source.ModulePath,
		Span:     span,
		Message: fmt.Sprintf(
			"ORM association %s.%s targets %s in model group %q, but %s is in %q; associated models must be declared in the same source directory; move the models into one directory, or keep the foreign key and query through an application repository",
			source.Name, association, target.Name, targetGroup, source.Name, sourceGroup,
		),
	}
	if targetSpan != nil {
		item.Related = []diagnostic.RelatedInformation{{
			Message:  "target model " + target.Name + " is declared here",
			Location: diagnostic.Location{Path: target.ModulePath, Span: *targetSpan},
		}}
	}
	return &providerDiagnosticError{diagnostics: []diagnostic.Diagnostic{item}}
}

func displayModelGroup(group string) string {
	if group == "" {
		return "."
	}
	return group
}
