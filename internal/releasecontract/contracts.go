package releasecontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	goast "go/ast"
	"go/format"
	"go/token"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var headingCharacters = regexp.MustCompile(`[^a-z0-9 -]`)

func DiscoverDocumentation(root string) ([]Surface, error) {
	// The 1.0 roadmap/capability proposal is intentionally outside this list.
	// These sources describe existing contracts or the accepted v0.5 transition.
	paths := []string{"docs/specification.md", "docs/standard-library.md", "docs/cli.md", "docs/configuration.md", "docs/editor-support.md", "docs/containers.md", "docs/releasing.md", "docs/native-transition.md"}
	for _, pattern := range []string{"docs/guides/*.md", "docs/tutorials/*.md", "docs/lint-rules/*.md"} {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return nil, err
		}
		for _, name := range matches {
			relative, err := filepath.Rel(root, name)
			if err != nil {
				return nil, err
			}
			paths = append(paths, filepath.ToSlash(relative))
		}
	}
	result := []Surface{}
	for _, source := range sortedUnique(paths) {
		data, err := read(root, source)
		if err != nil {
			return nil, err
		}
		fence := ""
		anchors := map[string]int{}
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				marker := trimmed[:3]
				if fence == "" {
					fence = marker
				} else if marker == fence {
					fence = ""
				}
				continue
			}
			if fence != "" || !strings.HasPrefix(line, "#") {
				continue
			}
			heading := strings.TrimLeft(line, "#")
			if heading == line || !strings.HasPrefix(heading, " ") {
				continue
			}
			heading = strings.TrimSpace(heading)
			anchor := strings.ReplaceAll(headingCharacters.ReplaceAllString(strings.ToLower(heading), ""), " ", "-")
			count := anchors[anchor]
			anchors[anchor]++
			if count > 0 {
				anchor += "-" + strconv.Itoa(count)
			}
			result = append(result, Surface{ID: "document:" + source + "#" + anchor, Kind: "documented contract", Source: source, Members: []string{heading}})
		}
	}
	return result, nil
}

func DiscoverProtocols(root string) ([]Surface, error) {
	patterns := []string{"internal/packageextension/*.go", "internal/toolingprotocol/*.go", "internal/packages/*.go", "internal/project/*.go", "internal/nativepackage/*.go", "internal/diagnostic/*.go", "internal/cli/*.go", "internal/lsp/*.go", "internal/schemalock/*.go", "internal/declarationadaptertooling/*.go", "internal/declaration/*.go", "internal/compilerservice/*.go", "internal/web/protocol.go"}
	result := []Surface{}
	for _, pattern := range patterns {
		paths, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return nil, err
		}
		for _, filename := range paths {
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
				group, ok := declaration.(*goast.GenDecl)
				if !ok {
					continue
				}
				for _, specification := range group.Specs {
					switch item := specification.(type) {
					case *goast.TypeSpec:
						structure, ok := item.Type.(*goast.StructType)
						if !ok {
							continue
						}
						members := []string{}
						for _, field := range structure.Fields.List {
							if field.Tag == nil {
								continue
							}
							tag, err := strconv.Unquote(field.Tag.Value)
							if err != nil {
								return nil, err
							}
							name, ok := reflect.StructTag(tag).Lookup("json")
							if !ok || name == "-" {
								continue
							}
							var shape bytes.Buffer
							if err := format.Node(&shape, token.NewFileSet(), field.Type); err != nil {
								return nil, err
							}
							members = append(members, name+": "+shape.String())
						}
						if len(members) != 0 {
							result = append(result, Surface{ID: "protocol:" + source + ":" + item.Name.Name, Kind: "structured contract", Source: source, Members: sortedUnique(members)})
						}
					case *goast.ValueSpec:
						if group.Tok != token.CONST || len(item.Names) != len(item.Values) {
							continue
						}
						for i, name := range item.Names {
							value, literal := item.Values[i].(*goast.BasicLit)
							if literal && value.Kind == token.INT && strings.HasSuffix(strings.ToLower(name.Name), "version") {
								result = append(result, Surface{ID: "protocol:" + source + ":" + name.Name, Kind: "protocol version", Source: source, Members: []string{value.Value}})
							}
							if value, ok := quoted(item.Values[i]); ok && strings.HasPrefix(value, "TRB") {
								result = append(result, Surface{ID: "diagnostic:" + value, Kind: "diagnostic", Source: source, Members: []string{name.Name}})
							}
						}
					}
				}
			}
		}
	}
	return result, nil
}

func DiscoverLSP(root string) ([]Surface, error) {
	const source = "internal/lsp/server.go"
	file, err := goFile(root, source)
	if err != nil {
		return nil, err
	}
	methods := map[string]bool{}
	notifications := map[string]bool{}
	goast.Inspect(file, func(node goast.Node) bool {
		if selection, ok := node.(*goast.SwitchStmt); ok {
			member, ok := selection.Tag.(*goast.SelectorExpr)
			if ok && member.Sel.Name == "Method" {
				for _, item := range selection.Body.List {
					for _, expression := range item.(*goast.CaseClause).List {
						if name, ok := quoted(expression); ok {
							methods[name] = true
						}
					}
				}
			}
		}
		if field, ok := node.(*goast.KeyValueExpr); ok {
			if key, ok := field.Key.(*goast.Ident); ok && key.Name == "Method" {
				if name, ok := quoted(field.Value); ok {
					notifications[name] = true
				}
			}
		}
		return true
	})
	if len(methods) == 0 {
		return nil, fmt.Errorf("LSP dispatch is not discoverable")
	}
	result := []Surface{}
	for name := range methods {
		result = append(result, Surface{ID: "lsp:" + name, Kind: "LSP input", Source: source})
	}
	for name := range notifications {
		result = append(result, Surface{ID: "lsp-output:" + name, Kind: "LSP notification", Source: source})
	}
	return result, nil
}

func DiscoverDelivery(root string) ([]Surface, error) {
	patterns := []string{".github/workflows/*.yml", "scripts/*release*.sh", "scripts/*homebrew*.sh", "scripts/*container*.sh", "Dockerfile*", "packaging/*/Dockerfile*", "editors/*/README.md", "editors/*/package.json"}
	result := []Surface{}
	for _, pattern := range patterns {
		paths, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return nil, err
		}
		for _, filename := range paths {
			if strings.HasSuffix(filename, "_test.sh") {
				continue
			}
			source, err := filepath.Rel(root, filename)
			if err != nil {
				return nil, err
			}
			source = filepath.ToSlash(source)
			surface := Surface{ID: "delivery:" + source, Kind: "delivery", Source: source}
			if strings.HasSuffix(source, "/package.json") {
				data, err := read(root, source)
				if err != nil {
					return nil, err
				}
				var manifest map[string]json.RawMessage
				if err := json.Unmarshal(data, &manifest); err != nil {
					return nil, err
				}
				for _, key := range []string{"name", "contributes", "activationEvents", "engines", "scripts"} {
					if raw, ok := manifest[key]; ok {
						var value any
						if err := json.Unmarshal(raw, &value); err != nil {
							return nil, err
						}
						canonical, err := json.Marshal(value)
						if err != nil {
							return nil, err
						}
						surface.Members = append(surface.Members, key+" "+string(canonical))
					}
				}
			}
			result = append(result, surface)
		}
	}
	return result, nil
}
