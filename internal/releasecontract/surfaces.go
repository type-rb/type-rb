package releasecontract

import (
	"bytes"
	"fmt"
	goast "go/ast"
	"go/format"
	"go/token"
	"path/filepath"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/stdlib"
)

func DiscoverBuiltins(root string) ([]Surface, error) {
	const source = "internal/stdlib/registry.go"
	file, err := goFile(root, source)
	if err != nil {
		return nil, err
	}
	literal, err := mapLiteral(file, "receiverMethods")
	if err != nil {
		return nil, err
	}
	result := []Surface{}
	for _, element := range literal.Elts {
		entry, ok := element.(*goast.KeyValueExpr)
		if !ok {
			return nil, fmt.Errorf("non-keyed receiver kind")
		}
		kind, ok := entry.Key.(*goast.SelectorExpr)
		if !ok {
			return nil, fmt.Errorf("non-canonical receiver kind")
		}
		name := kind.Sel.Name
		if name == "Int" {
			name = "Integer"
		} else if name == "Bool" {
			name = "Boolean"
		}
		methods, ok := entry.Value.(*goast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("non-literal receiver catalog for %s", name)
		}
		definition := &stdlib.Package{Kind: stdlib.Portable, Symbols: map[string]stdlib.Symbol{}}
		for _, element := range methods.Elts {
			method, ok := element.(*goast.KeyValueExpr)
			if !ok {
				return nil, fmt.Errorf("non-keyed receiver method")
			}
			spelling, ok := quoted(method.Key)
			if !ok {
				return nil, fmt.Errorf("non-literal receiver spelling")
			}
			target, ok := method.Value.(*goast.CompositeLit)
			if !ok {
				return nil, fmt.Errorf("non-literal receiver owner")
			}
			fields := map[string]string{}
			for _, element := range target.Elts {
				field, ok := element.(*goast.KeyValueExpr)
				if !ok {
					return nil, fmt.Errorf("non-keyed receiver owner")
				}
				key, ok := field.Key.(*goast.Ident)
				if !ok {
					return nil, fmt.Errorf("non-identifier receiver owner field")
				}
				value, ok := quoted(field.Value)
				if !ok {
					return nil, fmt.Errorf("non-literal receiver owner field")
				}
				fields[key.Name] = value
			}
			owner, ok := stdlib.Lookup(fields["PackagePath"])
			if !ok {
				return nil, fmt.Errorf("missing receiver owner %s", fields["PackagePath"])
			}
			symbol, ok := owner.Symbols[fields["Symbol"]]
			if !ok || len(symbol.Parameters) == 0 {
				return nil, fmt.Errorf("missing receiver operation %s", spelling)
			}
			_, public, ok := stdlib.LookupReceiverMethod(symbol.Parameters[0].Type, spelling)
			if !ok {
				return nil, fmt.Errorf("cannot project receiver %s#%s", name, spelling)
			}
			definition.Symbols[spelling] = public
		}
		surface, err := packageSurface("builtin:"+name, source, definition)
		if err != nil {
			return nil, err
		}
		result = append(result, surface)
	}
	return result, nil
}

func DiscoverSyntax(root string) ([]Surface, error) {
	const source = "internal/ast/ast.go"
	file, err := goFile(root, source)
	if err != nil {
		return nil, err
	}
	byID := map[string]Surface{}
	for _, declaration := range file.Decls {
		method, ok := declaration.(*goast.FuncDecl)
		if !ok || method.Recv == nil || len(method.Recv.List) != 1 ||
			(method.Name.Name != "statementNode" && method.Name.Name != "expressionNode") {
			continue
		}
		pointer, ok := method.Recv.List[0].Type.(*goast.StarExpr)
		if !ok {
			return nil, fmt.Errorf("non-pointer syntax node")
		}
		name, ok := pointer.X.(*goast.Ident)
		if !ok {
			return nil, fmt.Errorf("non-identifier syntax node")
		}
		id := "syntax:" + name.Name
		surface := byID[id]
		surface.ID, surface.Kind, surface.Source = id, "language", source
		surface.Members = append(surface.Members, strings.TrimSuffix(method.Name.Name, "Node"))
		byID[id] = surface
	}
	result := []Surface{}
	for _, surface := range byID {
		surface.Members = sortedUnique(surface.Members)
		result = append(result, surface)
	}
	return result, nil
}

// DiscoverCommands inventories dispatch spellings from the actual handlers.
// Flags remain attached to their owning function, including shared flag groups;
// they cannot be hidden by an abbreviated help-text sample.
func DiscoverCommands(root string) ([]Surface, error) {
	files, err := filepath.Glob(filepath.Join(root, "internal/cli/*.go"))
	if err != nil {
		return nil, err
	}
	owners := map[string]string{"Run": "", "runWeb": "web", "runJobs": "jobs", "runCompiler": "compiler", "runAdapter": "adapter", "runDatabase": "db"}
	type ownedFunction struct {
		function *goast.FuncDecl
		source   string
	}
	functions := []ownedFunction{}
	factories := map[string]bool{}
	result := []Surface{}
	for _, filename := range files {
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		source, err := filepath.Rel(root, filename)
		if err != nil {
			return nil, err
		}
		source = filepath.ToSlash(source)
		file, err := goFile(root, source)
		if err != nil {
			return nil, err
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*goast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			functions = append(functions, ownedFunction{function, source})
			if function.Type.Results != nil {
				for _, field := range function.Type.Results.List {
					if isFlagSet(field.Type) {
						factories[function.Name.Name] = true
					}
				}
			}
		}
	}
	for _, owned := range functions {
		function, source := owned.function, owned.source
		flagSets := flagSetNames(function, factories)
		flags := []string{}
		var discoveryError error
		goast.Inspect(function.Body, func(node goast.Node) bool {
			if selection, ok := node.(*goast.SwitchStmt); ok {
				index, ok := selection.Tag.(*goast.IndexExpr)
				if !ok {
					return true
				}
				argument, ok := index.X.(*goast.Ident)
				if !ok || argument.Name != "args" {
					return true
				}
				zero, ok := index.Index.(*goast.BasicLit)
				if !ok || zero.Value != "0" {
					return true
				}
				prefix, known := owners[function.Name.Name]
				if !known {
					discoveryError = fmt.Errorf("new command dispatcher %s needs an explicit root", function.Name.Name)
					return false
				}
				for _, item := range selection.Body.List {
					branch := item.(*goast.CaseClause)
					for _, expression := range branch.List {
						spelling, ok := quoted(expression)
						if !ok {
							discoveryError = fmt.Errorf("non-literal command spelling")
							return false
						}
						result = append(result, Surface{ID: "command:" + strings.TrimSpace(prefix+" "+spelling), Kind: "cli", Source: source})
					}
				}
			}
			if call, ok := node.(*goast.CallExpr); ok {
				member, ok := call.Fun.(*goast.SelectorExpr)
				if !ok {
					return true
				}
				receiver, ok := member.X.(*goast.Ident)
				if !ok || !flagSets[receiver.Name] {
					return true
				}
				position := 0
				switch member.Sel.Name {
				case "String", "Bool", "Int", "Int64", "Uint", "Uint64", "Float64", "Duration", "Func", "BoolFunc":
				case "StringVar", "BoolVar", "IntVar", "Int64Var", "UintVar", "Uint64Var", "Float64Var", "DurationVar", "TextVar", "Var":
					position = 1
				default:
					return true
				}
				if len(call.Args) > position {
					if spelling, ok := quoted(call.Args[position]); ok {
						description := spelling + " (" + member.Sel.Name
						if member.Sel.Name != "Var" && member.Sel.Name != "Func" && member.Sel.Name != "BoolFunc" && len(call.Args) > position+1 {
							var expression bytes.Buffer
							if err := format.Node(&expression, token.NewFileSet(), call.Args[position+1]); err != nil {
								discoveryError = err
								return false
							}
							description += ", default " + expression.String()
						}
						flags = append(flags, description+")")
					} else {
						discoveryError = fmt.Errorf("non-literal flag in %s", function.Name.Name)
					}
				}
			}
			return true
		})
		if discoveryError != nil {
			return nil, discoveryError
		}
		if len(flags) > 0 {
			result = append(result, Surface{ID: "flags:" + function.Name.Name, Kind: "cli", Source: source, Members: sortedUnique(flags)})
		}
	}
	return result, nil
}

func isFlagSet(expression goast.Expr) bool {
	pointer, ok := expression.(*goast.StarExpr)
	if !ok {
		return false
	}
	member, ok := pointer.X.(*goast.SelectorExpr)
	if !ok || member.Sel.Name != "FlagSet" {
		return false
	}
	owner, ok := member.X.(*goast.Ident)
	return ok && owner.Name == "flag"
}

func flagSetNames(function *goast.FuncDecl, factories map[string]bool) map[string]bool {
	names := map[string]bool{}
	if function.Type.Params != nil {
		for _, field := range function.Type.Params.List {
			if isFlagSet(field.Type) {
				for _, name := range field.Names {
					names[name.Name] = true
				}
			}
		}
	}
	for changed := true; changed; {
		changed = false
		goast.Inspect(function.Body, func(node goast.Node) bool {
			assignment, ok := node.(*goast.AssignStmt)
			if !ok || len(assignment.Lhs) != len(assignment.Rhs) {
				return true
			}
			for i, expression := range assignment.Rhs {
				name, ok := assignment.Lhs[i].(*goast.Ident)
				if !ok || names[name.Name] {
					continue
				}
				known := false
				switch value := expression.(type) {
				case *goast.Ident:
					known = names[value.Name]
				case *goast.CallExpr:
					switch function := value.Fun.(type) {
					case *goast.Ident:
						known = factories[function.Name]
					case *goast.SelectorExpr:
						owner, ok := function.X.(*goast.Ident)
						known = factories[function.Sel.Name] || (ok && owner.Name == "flag" && function.Sel.Name == "NewFlagSet")
					}
				}
				if known {
					names[name.Name], changed = true, true
				}
			}
			return true
		})
	}
	return names
}

func Discover(root string) ([]Surface, error) {
	result := []Surface{}
	for _, discover := range []func(string) ([]Surface, error){DiscoverPackages, DiscoverBuiltins, DiscoverSyntax, DiscoverCommands, DiscoverDocumentation, DiscoverProtocols, DiscoverLSP, DiscoverDelivery, DiscoverTypeKinds, DiscoverTestOwners} {
		part, err := discover(root)
		if err != nil {
			return nil, err
		}
		result = append(result, part...)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	for i, surface := range result {
		if i > 0 && surface.ID == result[i-1].ID {
			return nil, fmt.Errorf("duplicate surface %s", surface.ID)
		}
	}
	return result, nil
}
