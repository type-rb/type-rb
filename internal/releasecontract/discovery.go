// Package releasecontract inventories the public surface used by a paired
// implementation release. Discovery describes contracts, not passing evidence.
package releasecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	goast "go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/official"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/stdlib"
)

// Surface is one discovered owner and its complete current declaration list.
// A release row must explicitly own every surface; a growing registry cannot
// silently increase a row's scope through a wildcard selector.
type Surface struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Source  string   `json:"source"`
	Modes   []string `json:"modes,omitempty"`
	Members []string `json:"members,omitempty"`
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func read(root, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
}

func goFile(root, name string) (*goast.File, error) {
	data, err := read(root, name)
	if err != nil {
		return nil, err
	}
	return goparser.ParseFile(token.NewFileSet(), name, data, 0)
}

func mapLiteral(file *goast.File, name string) (*goast.CompositeLit, error) {
	for _, declaration := range file.Decls {
		group, ok := declaration.(*goast.GenDecl)
		if !ok {
			continue
		}
		for _, entry := range group.Specs {
			value, ok := entry.(*goast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != name || len(value.Values) != 1 {
				continue
			}
			if literal, ok := value.Values[0].(*goast.CompositeLit); ok {
				return literal, nil
			}
		}
	}
	return nil, fmt.Errorf("cannot discover map %s", name)
}

func quoted(expression goast.Expr) (string, bool) {
	literal, ok := expression.(*goast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func packageSurface(id, source string, definition *stdlib.Package) (Surface, error) {
	result := Surface{ID: id, Kind: string(definition.Kind), Source: source}
	if definition.Root != "" {
		result.Members = append(result.Members, "declaration root "+definition.Root)
	}
	if definition.NativeSyntax {
		result.Members = append(result.Members, "explicit native syntax")
	}
	for declaration := range definition.OpaqueTypes {
		result.Members = append(result.Members, "opaque "+declaration.Module+"::"+declaration.Name)
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if definition.Supports(mode) {
			result.Modes = append(result.Modes, mode)
		}
	}
	for _, exported := range definition.RuntimeExports {
		result.Members = append(result.Members, exported.Kind+" "+exported.Name)
	}
	for name, symbol := range definition.Symbols {
		if symbol.CompilerOnly || strings.HasPrefix(name, "_") {
			continue
		}
		parameters := []string{}
		for _, parameter := range symbol.Parameters {
			flags := ""
			if parameter.Optional {
				flags += " optional"
			}
			if parameter.Keyword {
				flags += " keyword"
			}
			if parameter.Mutable {
				flags += " mutable"
			}
			if parameter.Exact {
				flags += " exact"
			}
			parameters = append(parameters, parameter.Name+": "+parameter.Type.String()+flags)
		}
		owner := ""
		if symbol.HasReceiver() {
			owner = symbol.Receiver.String() + "#"
		} else if symbol.HasStaticOwner() {
			owner = symbol.StaticOwner + "."
		}
		signature := owner + name + "(" + strings.Join(parameters, ", ") + "): " + symbol.Return.String()
		if len(symbol.TypeParameters) > 0 {
			signature += " type parameters " + strings.Join(symbol.TypeParameters, ",")
		}
		if symbol.Variadic {
			signature += " variadic"
		}
		if symbol.ReceiverMutable {
			signature += " mutable receiver"
		}
		for _, constraint := range symbol.EqualityTypes {
			signature += " equality " + constraint.String()
		}
		for _, constraint := range symbol.OrderingTypes {
			signature += " ordering " + constraint.String()
		}
		if symbol.Block != nil {
			block, err := json.Marshal(symbol.Block)
			if err != nil {
				return result, err
			}
			signature += " block " + string(block)
		}
		result.Members = append(result.Members, signature)
	}
	if definition.Source != "" {
		program, diagnostics := parser.Parse([]byte(definition.Source))
		if len(diagnostics) != 0 {
			return result, fmt.Errorf("cannot discover %s source: %v", id, diagnostics)
		}
		members, err := sourceMembers(program.Statements, "", definition.Source)
		if err != nil {
			return result, err
		}
		result.Members = append(result.Members, members...)
	}
	if definition.TypeProvider != "" {
		result.Members = append(result.Members, "type provider "+definition.TypeProvider)
	}
	if definition.Capability {
		result.Members = append(result.Members, "explicit capability activation")
	}
	if definition.JSX != nil {
		result.Members = append(result.Members, "JSX node "+definition.JSX.Node.String())
		for name, typ := range definition.JSX.IntrinsicAttributes {
			result.Members = append(result.Members, "JSX intrinsic "+name+": "+typ.String())
		}
	}
	result.Members = sortedUnique(result.Members)
	return result, nil
}

func sourceMembers(statements []ast.Statement, owner, source string) ([]string, error) {
	result := []string{}
	for _, statement := range statements {
		name, kind := "", ""
		var err error
		var body []ast.Statement
		switch node := statement.(type) {
		case *ast.ClassStatement:
			name, kind, body = node.Name+typeParameters(node.TypeParameters), "class", node.Body
			if node.Superclass != nil {
				var parent string
				parent, err = nodeSource(node.Superclass, source)
				kind += " extends " + parent
			}
			for _, implemented := range node.Implements {
				kind += " implements " + implemented.String()
			}
		case *ast.RecordStatement:
			name, kind, body = node.Name+typeParameters(node.TypeParameters), "record", node.Body
		case *ast.EnumStatement:
			name, kind, body = node.Name+typeParameters(node.TypeParameters), "enum", node.Body
		case *ast.ModuleStatement:
			name, kind, body = node.Name, "module", node.Body
		case *ast.InterfaceStatement:
			name, kind = node.Name+typeParameters(node.TypeParameters), "interface"
			for _, method := range node.Methods {
				body = append(body, method)
			}
		case *ast.NewtypeStatement:
			name, kind, body = node.Name, "newtype "+node.Target.String(), node.Body
			if node.PrivateNew {
				kind += " private new"
			}
		case *ast.TypeAliasStatement:
			name, kind = node.Name+typeParameters(node.TypeParameters), "alias "+node.Target.String()
		case *ast.RecordFieldStatement:
			if strings.HasPrefix(node.Name, "_") {
				continue
			}
			name, err = nodeSource(node, source)
			kind = "field"
		case *ast.EnumMemberStatement:
			if strings.HasPrefix(node.Name, "_") {
				continue
			}
			name, err = nodeSource(node, source)
			kind = "variant"
		case *ast.MethodStatement:
			if strings.HasPrefix(node.Name, "_") {
				continue
			}
			// Retain the authored signature, including defaults and named-only
			// parameters, without copying an implementation body into the census.
			start, end := node.Span().Start.Offset, node.Span().End.Offset
			if len(node.Body) > 0 {
				end = node.Body[0].Span().Start.Offset
			}
			if start < 0 || end < start || end > len(source) {
				return nil, fmt.Errorf("invalid source signature for %s%s", owner, node.Name)
			}
			result = append(result, owner+strings.TrimSpace(source[start:end]))
			continue
		case *ast.VariableStatement:
			name, kind = node.Name, "binding "+node.Type.String()
			if node.Value != nil {
				var value string
				value, err = nodeSource(node.Value, source)
				if len(value) > 200 {
					// Preserve exact initializer identity without duplicating large
					// public data tables in the API catalog.
					kind += " initializer sha256 " + hashBytes([]byte(value))
				} else {
					kind += " = " + value
				}
			}
		}
		if err != nil {
			return nil, err
		}
		if name == "" || strings.HasPrefix(name, "_") {
			continue
		}
		result = append(result, kind+" "+owner+name)
		if body != nil {
			members, err := sourceMembers(body, owner+name+"::", source)
			if err != nil {
				return nil, err
			}
			result = append(result, members...)
		}
	}
	return result, nil
}

func nodeSource(node ast.Node, source string) (string, error) {
	span := node.Span()
	if span.Start.Offset < 0 || span.End.Offset < span.Start.Offset || span.End.Offset > len(source) {
		return "", fmt.Errorf("invalid API source span %v", span)
	}
	return strings.TrimSpace(source[span.Start.Offset:span.End.Offset]), nil
}

func typeParameters(parameters []ast.TypeParameter) string {
	if len(parameters) == 0 {
		return ""
	}
	names := make([]string, len(parameters))
	for i, parameter := range parameters {
		names[i] = parameter.Name
	}
	return "<" + strings.Join(names, ", ") + ">"
}

func sortedUnique(values []string) []string {
	sort.Strings(values)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

// DiscoverPackages reads every registered public standard and official package,
// including platform-only packages. RuntimeExportPackages alone omits those
// platform contracts and packages without an exported runtime type.
func DiscoverPackages(root string) ([]Surface, error) {
	const registry = "internal/stdlib/registry.go"
	file, err := goFile(root, registry)
	if err != nil {
		return nil, err
	}
	literal, err := mapLiteral(file, "registry")
	if err != nil {
		return nil, err
	}
	result := []Surface{}
	for _, element := range literal.Elts {
		entry, ok := element.(*goast.KeyValueExpr)
		if !ok {
			return nil, fmt.Errorf("non-keyed standard package entry")
		}
		name, ok := quoted(entry.Key)
		if !ok {
			return nil, fmt.Errorf("non-literal standard package identity")
		}
		definition, ok := stdlib.Lookup(name)
		if !ok {
			return nil, fmt.Errorf("discovery source and compiled standard registry disagree for %s", name)
		}
		if definition.Internal {
			continue
		}
		surface, err := packageSurface("package:"+name, registry, definition)
		if err != nil {
			return nil, err
		}
		result = append(result, surface)
	}
	for _, name := range official.Names() {
		definition, ok := official.Lookup(name)
		if !ok {
			return nil, fmt.Errorf("missing official package %s", name)
		}
		surface, err := packageSurface("package:"+name, "internal/official/"+definition.ManifestPath, definition.Definition)
		if err != nil {
			return nil, err
		}
		if definition.ProjectProvider != "" {
			surface.Members = append(surface.Members, "project provider "+definition.ProjectProvider)
		}
		data, err := read(root, surface.Source)
		if err != nil {
			return nil, err
		}
		var manifest map[string]json.RawMessage
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, err
		}
		for _, field := range []string{"aliases", "nativeDependencies", "nativeDependenciesByOption", "nativeDependencyOptionDefaults"} {
			if raw, ok := manifest[field]; ok {
				var value any
				if err := json.Unmarshal(raw, &value); err != nil {
					return nil, err
				}
				canonical, err := json.Marshal(value)
				if err != nil {
					return nil, err
				}
				surface.Members = append(surface.Members, field+" "+string(canonical))
			}
		}
		surface.Members = sortedUnique(surface.Members)
		result = append(result, surface)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}
