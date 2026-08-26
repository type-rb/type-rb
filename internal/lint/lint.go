// Package lint implements configurable, source-oriented TypeRB style checks.
// Language correctness remains owned by the parser, resolver, and checker.
package lint

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/rivo/uniseg"
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/token"
)

type Level string

const (
	Off     Level = "off"
	Warning Level = "warning"
	Error   Level = "error"
)

const (
	RecommendedPreset               = "recommended"
	NoRulesPreset                   = "none"
	PreferConditionalTransferRuleID = "trb/prefer-conditional-transfer"
	maxConditionalTransferColumns   = 120
)

type Metadata struct {
	ID            string
	Summary       string
	DefaultLevel  Level
	Recommended   bool
	Since         string
	Fixable       bool
	Documentation string
}

type Options struct {
	Preset string
	Rules  map[string]string
}

type ResolvedOptions struct {
	levels map[string]Level
}

var registry = []Metadata{{
	ID:            PreferConditionalTransferRuleID,
	Summary:       "Prefer a one-line conditional transfer for a simple guard clause.",
	DefaultLevel:  Warning,
	Recommended:   true,
	Since:         "0.3.25",
	Fixable:       true,
	Documentation: "prefer-conditional-transfer",
}}

func Registry() []Metadata {
	return append([]Metadata(nil), registry...)
}

func Resolve(options Options) (ResolvedOptions, error) {
	preset := strings.TrimSpace(options.Preset)
	if preset == "" {
		preset = RecommendedPreset
	}
	if preset != RecommendedPreset && preset != NoRulesPreset {
		return ResolvedOptions{}, fmt.Errorf("lint.preset must be recommended or none; got %q", options.Preset)
	}
	levels := map[string]Level{}
	for _, rule := range registry {
		level := Off
		if preset == RecommendedPreset && rule.Recommended {
			level = rule.DefaultLevel
		}
		levels[rule.ID] = level
	}
	for id, configured := range options.Rules {
		if _, known := levels[id]; !known {
			return ResolvedOptions{}, fmt.Errorf("unknown lint rule %q", id)
		}
		level := Level(strings.TrimSpace(configured))
		if level != Off && level != Warning && level != Error {
			return ResolvedOptions{}, fmt.Errorf("lint rule %s level must be off, warning, or error; got %q", id, configured)
		}
		levels[id] = level
	}
	return ResolvedOptions{levels: levels}, nil
}

func Analyze(program *ast.Program, source []byte, path string, options ResolvedOptions) []diagnostic.Diagnostic {
	level := options.levels[PreferConditionalTransferRuleID]
	if level == Off || program == nil {
		return nil
	}
	severity := diagnostic.Warning
	if level == Error {
		severity = diagnostic.Error
	}
	items := []diagnostic.Diagnostic{}
	walkStatements(program.Statements, func(node *ast.IfStatement) {
		item, ok := preferConditionalTransfer(node, source, path, severity)
		if ok {
			items = append(items, item)
		}
	})
	SortDiagnostics(items)
	return items
}

func SortDiagnostics(items []diagnostic.Diagnostic) {
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].Path != items[right].Path {
			return items[left].Path < items[right].Path
		}
		if items[left].Span.Start.Offset != items[right].Span.Start.Offset {
			return items[left].Span.Start.Offset < items[right].Span.Start.Offset
		}
		return items[left].Code < items[right].Code
	})
}

func ApplyFixes(source []byte, path string, items []diagnostic.Diagnostic) ([]byte, int, error) {
	type edit struct {
		start       int
		end         int
		replacement string
	}
	edits := []edit{}
	fixes := 0
	for _, item := range items {
		if len(item.Fixes) == 0 {
			continue
		}
		selected := item.Fixes[0]
		if len(selected.Edits) == 0 {
			continue
		}
		applicable := true
		editStart := len(edits)
		for _, candidate := range selected.Edits {
			if candidate.Location.Path != "" && candidate.Location.Path != path {
				applicable = false
				break
			}
			span := candidate.Location.Span
			if span.Start.Offset < 0 || span.End.Offset < span.Start.Offset || span.End.Offset > len(source) {
				return nil, 0, fmt.Errorf("lint fix for %s has an invalid source span", item.Code)
			}
			edits = append(edits, edit{start: span.Start.Offset, end: span.End.Offset, replacement: candidate.Replacement})
		}
		if applicable {
			fixes++
		} else {
			edits = edits[:editStart]
		}
	}
	sort.SliceStable(edits, func(left, right int) bool { return edits[left].start > edits[right].start })
	previousStart := len(source)
	result := append([]byte(nil), source...)
	for _, candidate := range edits {
		if candidate.end > previousStart {
			return nil, 0, errors.New("lint fixes overlap")
		}
		result = append(result[:candidate.start], append([]byte(candidate.replacement), result[candidate.end:]...)...)
		previousStart = candidate.start
	}
	return result, fixes, nil
}

func preferConditionalTransfer(node *ast.IfStatement, source []byte, path string, severity diagnostic.Severity) (diagnostic.Diagnostic, bool) {
	if node == nil || node.Ternary || node.ConditionalTransfer || node.HasElse || len(node.ElseIf) != 0 || len(node.Then) != 1 || node.TrailingComment != "" {
		return diagnostic.Diagnostic{}, false
	}
	transfer := node.Then[0]
	keyword := ""
	switch value := transfer.(type) {
	case *ast.ReturnStatement:
		keyword = "return"
		if value.TrailingComment != "" {
			return diagnostic.Diagnostic{}, false
		}
	case *ast.BreakStatement:
		keyword = "break"
		if value.TrailingComment != "" {
			return diagnostic.Diagnostic{}, false
		}
	case *ast.NextStatement:
		keyword = "next"
		if value.TrailingComment != "" {
			return diagnostic.Diagnostic{}, false
		}
	default:
		return diagnostic.Diagnostic{}, false
	}
	span := node.Span()
	transferSpan := transfer.Span()
	conditionSpan := node.Condition.Span()
	if !validSourceSpan(span, source) || !validSourceSpan(transferSpan, source) || !validSourceSpan(conditionSpan, source) ||
		transferSpan.Start.Line != transferSpan.End.Line || conditionSpan.Start.Line != conditionSpan.End.Line {
		return diagnostic.Diagnostic{}, false
	}
	transferText := strings.TrimSpace(string(source[transferSpan.Start.Offset:transferSpan.End.Offset]))
	conditionText := strings.TrimSpace(string(source[conditionSpan.Start.Offset:conditionSpan.End.Offset]))
	if transferText == "" || conditionText == "" {
		return diagnostic.Diagnostic{}, false
	}
	replacement := transferText + " if " + conditionText
	lineStart := strings.LastIndex(string(source[:span.Start.Offset]), "\n") + 1
	indent := string(source[lineStart:span.Start.Offset])
	if sourceLineWidth(indent+replacement) > maxConditionalTransferColumns {
		return diagnostic.Diagnostic{}, false
	}
	return diagnostic.Diagnostic{
		Code:     diagnostic.Code(PreferConditionalTransferRuleID),
		Severity: severity,
		Message:  fmt.Sprintf("Prefer `%s ... if ...` for this simple guard clause.", keyword),
		Path:     path,
		Span:     span,
		Fixes: []diagnostic.Fix{{
			Message: "Rewrite as a conditional transfer.",
			Edits: []diagnostic.TextEdit{{
				Location:    diagnostic.Location{Path: path, Span: span},
				Replacement: replacement,
			}},
		}},
	}, true
}

func sourceLineWidth(value string) int {
	columns := 0
	for {
		tab := strings.IndexByte(value, '\t')
		if tab < 0 {
			return columns + uniseg.StringWidth(value)
		}
		columns += uniseg.StringWidth(value[:tab])
		columns += 4 - columns%4
		value = value[tab+1:]
	}
}

func validSourceSpan(span token.Span, source []byte) bool {
	return span.Start.Offset >= 0 && span.End.Offset >= span.Start.Offset && span.End.Offset <= len(source)
}

func walkStatements(statements []ast.Statement, visit func(*ast.IfStatement)) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.ClassStatement:
			walkExpression(node.Superclass, visit)
			walkStatements(node.Body, visit)
		case *ast.RecordStatement:
			walkStatements(node.Body, visit)
		case *ast.RecordFieldStatement:
			for _, attribute := range node.Attributes {
				for _, argument := range attribute.Arguments {
					walkExpression(argument.Value, visit)
				}
			}
		case *ast.EnumStatement:
			walkStatements(node.Body, visit)
		case *ast.EnumMemberStatement:
			walkExpression(node.RawValue, visit)
			for _, parameter := range node.Parameters {
				walkExpression(parameter.Default, visit)
			}
		case *ast.ModuleStatement:
			walkStatements(node.Body, visit)
		case *ast.InterfaceStatement:
			for _, method := range node.Methods {
				walkStatements([]ast.Statement{method}, visit)
			}
		case *ast.FieldStatement:
			walkExpression(node.Value, visit)
		case *ast.MethodStatement:
			for _, parameter := range node.Parameters {
				walkExpression(parameter.Default, visit)
			}
			walkStatements(node.Body, visit)
		case *ast.VariableStatement:
			walkExpression(node.Value, visit)
		case *ast.AssignmentStatement:
			walkExpression(node.Target, visit)
			walkExpression(node.Value, visit)
		case *ast.ReturnStatement:
			walkExpression(node.Value, visit)
		case *ast.ExpressionStatement:
			walkExpression(node.Expression, visit)
		case *ast.IfStatement:
			visit(node)
			walkExpression(node.Condition, visit)
			walkStatements(node.Then, visit)
			for _, branch := range node.ElseIf {
				walkExpression(branch.Condition, visit)
				walkStatements(branch.Body, visit)
			}
			walkStatements(node.Else, visit)
		case *ast.CaseStatement:
			walkExpression(node.Value, visit)
			walkStatements(node.Leading, visit)
			for _, branch := range node.Branches {
				walkExpression(branch.Value, visit)
				for _, alternative := range branch.Alternatives {
					walkExpression(alternative, visit)
				}
				walkStatements(branch.Body, visit)
			}
			walkStatements(node.Else, visit)
		case *ast.WhileStatement:
			walkExpression(node.Condition, visit)
			walkStatements(node.Body, visit)
		case *ast.NativeBlock:
			walkStatements(node.Body, visit)
		}
	}
}

func walkExpression(expression ast.Expression, visit func(*ast.IfStatement)) {
	switch node := expression.(type) {
	case nil:
		return
	case *ast.IfStatement:
		visit(node)
		walkExpression(node.Condition, visit)
		walkStatements(node.Then, visit)
		for _, branch := range node.ElseIf {
			walkExpression(branch.Condition, visit)
			walkStatements(branch.Body, visit)
		}
		walkStatements(node.Else, visit)
	case *ast.CaseStatement:
		walkExpression(node.Value, visit)
		walkStatements(node.Leading, visit)
		for _, branch := range node.Branches {
			walkExpression(branch.Value, visit)
			for _, alternative := range branch.Alternatives {
				walkExpression(alternative, visit)
			}
			walkStatements(branch.Body, visit)
		}
		walkStatements(node.Else, visit)
	case *ast.InterpolatedString:
		for _, part := range node.Parts {
			walkExpression(part.Expression, visit)
		}
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			walkExpression(element, visit)
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			walkExpression(entry.Key, visit)
			walkExpression(entry.Value, visit)
		}
	case *ast.JSXElement:
		walkExpression(node.Component, visit)
		for _, attribute := range node.Attributes {
			walkExpression(attribute.Value, visit)
		}
		for _, child := range node.Children {
			if expression, ok := child.(*ast.JSXExpression); ok {
				walkExpression(expression.Value, visit)
			} else if element, ok := child.(*ast.JSXElement); ok {
				walkExpression(element, visit)
			}
		}
	case *ast.UnaryExpression:
		walkExpression(node.Operand, visit)
	case *ast.BinaryExpression:
		walkExpression(node.Left, visit)
		walkExpression(node.Right, visit)
	case *ast.RangeExpression:
		walkExpression(node.Start, visit)
		walkExpression(node.End, visit)
	case *ast.AttemptExpression:
		walkExpression(node.Value, visit)
		walkStatements(node.Body, visit)
	case *ast.TryExpression:
		walkExpression(node.Value, visit)
	case *ast.CatchExpression:
		walkExpression(node.Value, visit)
		walkStatements(node.Body, visit)
	case *ast.LambdaExpression:
		for _, parameter := range node.Parameters {
			walkExpression(parameter.Default, visit)
		}
		walkStatements(node.Body, visit)
	case *ast.IterationExpression:
		walkExpression(node.Source, visit)
		walkExpression(node.SliceSize, visit)
		walkExpression(node.Initial, visit)
		walkExpression(node.Limit, visit)
		if node.Block != nil {
			walkStatements(node.Block.Body, visit)
		}
	case *ast.CallExpression:
		walkExpression(node.Callee, visit)
		for _, argument := range node.Arguments {
			walkExpression(argument.Value, visit)
		}
		if node.Block != nil {
			walkStatements(node.Block.Body, visit)
		}
	case *ast.GenericExpression:
		walkExpression(node.Receiver, visit)
	case *ast.MemberExpression:
		walkExpression(node.Receiver, visit)
	case *ast.IndexExpression:
		walkExpression(node.Receiver, visit)
		walkExpression(node.Index, visit)
	case *ast.BlockExpression:
		walkStatements(node.Body, visit)
	}
}
