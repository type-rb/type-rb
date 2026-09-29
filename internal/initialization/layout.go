package initialization

import (
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

// Records have inline value storage. Nullable values, classes, collections and
// recursive enum payloads introduce indirection and therefore break a cycle.
func validateLayouts(programs []*ir.Program) []diagnostic.Diagnostic {
	declarations := map[identity.Declaration]ir.InlineDeclaration{}
	type recordSource struct {
		record  *ir.Record
		program *ir.Program
	}
	var records []recordSource
	for _, program := range programs {
		var collect func([]ir.Statement)
		collect = func(statements []ir.Statement) {
			for _, statement := range statements {
				switch node := statement.(type) {
				case *ir.Module:
					collect(node.Body)
				case *ir.Class:
					collect(node.Body)
				case *ir.Record:
					entry := ir.InlineDeclaration{Parameters: node.TypeParameters}
					for _, item := range node.Body {
						if field, ok := item.(*ir.RecordField); ok {
							entry.Fields = append(entry.Fields, field.Type)
						}
					}
					declarations[node.Declaration] = entry
					if program.CyclicInitialization {
						records = append(records, recordSource{node, program})
					}
				case *ir.TypeAlias:
					declarations[node.Declaration] = ir.InlineDeclaration{Parameters: node.TypeParameters, Fields: []types.Type{node.Target}}
				case *ir.Newtype:
					declarations[node.Declaration] = ir.InlineDeclaration{Fields: []types.Type{node.Target}}
				}
			}
		}
		collect(program.Statements)
	}
	var diagnostics []diagnostic.Diagnostic
	for _, source := range records {
		for _, field := range declarations[source.record.Declaration].Fields {
			if ir.InlineReaches(field, source.record.Declaration, declarations, map[identity.Declaration]bool{}) {
				diagnostics = append(diagnostics, diagnostic.Diagnostic{Code: diagnostic.TypeError, Severity: diagnostic.Error, Path: source.program.SourcePath, Span: source.record.SourceSpan(), Message: "infinite value layout involving record " + source.record.Name + "; use a nullable value or collection to introduce indirection"})
				break
			}
		}
	}
	return diagnostics
}
