// Package nativesnapshot implements the temporary bootstrap bridge for
// type-rb-native. It serializes deliberately small, data-only subsets of the
// checked TypeRB IR and exposes no compiler object across the process boundary.
package nativesnapshot

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

const (
	Format       = "type-rb-bootstrap-snapshot"
	Version      = 2
	Gate2Version = 3
)

type Snapshot struct {
	Format        string     `json:"format"`
	Version       int        `json:"version"`
	Module        string     `json:"module"`
	EntryFunction string     `json:"entryFunction"`
	Sources       []Source   `json:"sources"`
	Functions     []Function `json:"functions"`
}

type Gate2Snapshot struct {
	Format        string           `json:"format"`
	Version       int              `json:"version"`
	Module        string           `json:"module"`
	EntryFunction string           `json:"entryFunction"`
	Sources       []Source         `json:"sources"`
	Types         []TypeDefinition `json:"types"`
	Functions     []Function       `json:"functions"`
}

type TypeDefinition struct {
	Kind     string     `json:"kind"`
	ID       string     `json:"id"`
	Fields   *[]Field   `json:"fields,omitempty"`
	Variants *[]Variant `json:"variants,omitempty"`
}

type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Variant struct {
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
}

type Source struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type Origin struct {
	Source      string `json:"source"`
	StartLine   int    `json:"startLine"`
	StartColumn int    `json:"startColumn"`
	EndLine     int    `json:"endLine"`
	EndColumn   int    `json:"endColumn"`
}

type Parameter struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Origin Origin `json:"origin"`
}

type Function struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Parameters []Parameter `json:"parameters"`
	Result     string      `json:"result"`
	Entry      string      `json:"entry"`
	Origin     Origin      `json:"origin"`
	Blocks     []Block     `json:"blocks"`
}

type Block struct {
	ID           string      `json:"id"`
	Parameters   []Parameter `json:"parameters"`
	Origin       Origin      `json:"origin"`
	Instructions []any       `json:"instructions"`
	Terminator   any         `json:"terminator"`
}

type BooleanLiteral struct {
	Op     string `json:"op"`
	Result string `json:"result"`
	Value  bool   `json:"value"`
	Origin Origin `json:"origin"`
}

type IntegerLiteral struct {
	Op     string `json:"op"`
	Result string `json:"result"`
	Value  int64  `json:"value"`
	Origin Origin `json:"origin"`
}

type FloatLiteral struct {
	Op     string  `json:"op"`
	Result string  `json:"result"`
	Value  float64 `json:"value"`
	Origin Origin  `json:"origin"`
}

type BinaryInstruction struct {
	Op       string `json:"op"`
	Result   string `json:"result"`
	Operator string `json:"operator"`
	Left     string `json:"left"`
	Right    string `json:"right"`
	Origin   Origin `json:"origin"`
}

type BooleanNot struct {
	Op     string `json:"op"`
	Result string `json:"result"`
	Value  string `json:"value"`
	Origin Origin `json:"origin"`
}

type RecordConstruct struct {
	Op        string   `json:"op"`
	Result    string   `json:"result"`
	Type      string   `json:"type"`
	Arguments []string `json:"arguments"`
	Origin    Origin   `json:"origin"`
}

type RecordProject struct {
	Op     string `json:"op"`
	Result string `json:"result"`
	Type   string `json:"type"`
	Record string `json:"record"`
	Field  string `json:"field"`
	Origin Origin `json:"origin"`
}

type VariantConstruct struct {
	Op        string   `json:"op"`
	Result    string   `json:"result"`
	Type      string   `json:"type"`
	Variant   string   `json:"variant"`
	Arguments []string `json:"arguments"`
	Origin    Origin   `json:"origin"`
}

type VariantTest struct {
	Op      string `json:"op"`
	Result  string `json:"result"`
	Type    string `json:"type"`
	Value   string `json:"value"`
	Variant string `json:"variant"`
	Origin  Origin `json:"origin"`
}

type VariantProject struct {
	Op      string `json:"op"`
	Result  string `json:"result"`
	Type    string `json:"type"`
	Value   string `json:"value"`
	Variant string `json:"variant"`
	Field   string `json:"field"`
	Origin  Origin `json:"origin"`
}

type Call struct {
	Op        string   `json:"op"`
	Result    *string  `json:"result"`
	Function  string   `json:"function"`
	Arguments []string `json:"arguments"`
	Origin    Origin   `json:"origin"`
}

type WriteStatic struct {
	Op     string `json:"op"`
	Value  string `json:"value"`
	Origin Origin `json:"origin"`
}

type Jump struct {
	Op        string   `json:"op"`
	Target    string   `json:"target"`
	Arguments []string `json:"arguments"`
	Origin    Origin   `json:"origin"`
}

type Branch struct {
	Op             string   `json:"op"`
	Condition      string   `json:"condition"`
	WhenTrue       string   `json:"whenTrue"`
	TrueArguments  []string `json:"trueArguments"`
	WhenFalse      string   `json:"whenFalse"`
	FalseArguments []string `json:"falseArguments"`
	Origin         Origin   `json:"origin"`
}

type Return struct {
	Op     string  `json:"op"`
	Value  *string `json:"value"`
	Origin Origin  `json:"origin"`
}

type UnsupportedError struct {
	Path    string
	Span    token.Span
	Feature string
	Version int
}

func (e *UnsupportedError) Error() string {
	version := e.Version
	if version == 0 {
		version = Version
	}
	return fmt.Sprintf("%s:%d:%d: native snapshot v%d does not support %s", e.Path, e.Span.Start.Line, e.Span.Start.Column, version, e.Feature)
}

type methodInput struct {
	program *ir.Program
	method  *ir.Method
}

// Build is the intentionally narrow Gate 1 bridge used by type-rb-native. This
// package can be removed once the native frontend produces the same MIR itself.
func Build(artifacts []*compiler.Artifact, sourceRoot string) (Snapshot, error) {
	inputs := projectMethods(artifacts)
	if len(inputs) == 0 {
		return Snapshot{}, fmt.Errorf("native snapshot v2 found no project functions")
	}
	methodIDs := make(map[string]string, len(inputs))
	for _, input := range inputs {
		id := functionID(input.program, input.method)
		if key := input.method.Declaration.Key(); key != "" {
			methodIDs[key] = id
		}
		methodIDs["name:"+input.program.ModulePath+"#"+input.method.Name] = id
	}
	entry := ""
	module := ""
	for _, input := range inputs {
		if input.method.Name != compiler.MainFunction {
			continue
		}
		if entry != "" {
			return Snapshot{}, fmt.Errorf("native snapshot v2 requires exactly one top-level main function")
		}
		entry = functionID(input.program, input.method)
		module = input.program.ModulePath
	}
	if entry == "" {
		return Snapshot{}, fmt.Errorf("native snapshot v2 requires one top-level def main()")
	}

	sources, sourceIDs := projectSources(inputs, sourceRoot)
	functions := make([]Function, 0, len(inputs))
	for _, input := range inputs {
		lowered, err := lowerFunction(input.program, input.method, sourceIDs[input.program.SourcePath], methodIDs)
		if err != nil {
			return Snapshot{}, err
		}
		functions = append(functions, lowered)
	}
	sort.Slice(functions, func(i, j int) bool { return functions[i].ID < functions[j].ID })
	return Snapshot{
		Format: Format, Version: Version, Module: module, EntryFunction: entry,
		Sources: sources, Functions: functions,
	}, nil
}

func projectMethods(artifacts []*compiler.Artifact) []methodInput {
	var result []methodInput
	for _, artifact := range artifacts {
		if artifact == nil || artifact.IR == nil || artifact.CompilerOwned || artifact.ExternalPackage {
			continue
		}
		for _, statement := range artifact.IR.Statements {
			method, ok := statement.(*ir.Method)
			if !ok || method.External || method.Class {
				continue
			}
			result = append(result, methodInput{program: artifact.IR, method: method})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return functionID(result[i].program, result[i].method) < functionID(result[j].program, result[j].method)
	})
	return result
}

func projectSources(inputs []methodInput, sourceRoot string) ([]Source, map[string]string) {
	paths := make([]string, 0, len(inputs))
	seen := map[string]bool{}
	for _, input := range inputs {
		if !seen[input.program.SourcePath] {
			seen[input.program.SourcePath] = true
			paths = append(paths, input.program.SourcePath)
		}
	}
	sort.Strings(paths)
	sources := make([]Source, len(paths))
	ids := make(map[string]string, len(paths))
	for index, path := range paths {
		id := fmt.Sprintf("source-%d", index)
		ids[path] = id
		display := filepath.ToSlash(path)
		if sourceRoot != "" {
			if relative, err := filepath.Rel(sourceRoot, path); err == nil && relative != "." && relative != ".." && !filepath.IsAbs(relative) {
				display = filepath.ToSlash(relative)
			}
		}
		sources[index] = Source{ID: id, Path: display}
	}
	return sources, ids
}

func functionID(program *ir.Program, method *ir.Method) string {
	module := program.ModulePath
	name := method.Name
	if !method.Declaration.Empty() {
		if method.Declaration.Module != "" {
			module = method.Declaration.Module
		}
		if method.Declaration.Name != "" {
			name = method.Declaration.Name
		}
	}
	return module + "#" + name
}

func scalarTypeName(typ types.Type) (string, bool) {
	if base, ok := types.LiteralBase(typ); ok {
		typ = base
	}
	if base, ok := types.LiteralUnionBase(typ); ok {
		typ = base
	}
	if typ.Nullable {
		return "", false
	}
	switch typ.Kind {
	case types.Void:
		return "Void", true
	case types.Bool:
		return "Boolean", true
	case types.Int:
		return "Integer", true
	case types.Float:
		return "Float", true
	default:
		return "", false
	}
}

func origin(sourceID string, span token.Span) Origin {
	return Origin{
		Source: sourceID, StartLine: span.Start.Line, StartColumn: span.Start.Column,
		EndLine: span.End.Line, EndColumn: span.End.Column,
	}
}
