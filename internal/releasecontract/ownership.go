package releasecontract

import (
	"fmt"
	goast "go/ast"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
)

// Owning tests are discovery inputs, not evidence that a release candidate
// passed. Subtests and their applicability still require reviewed scenarios.
func DiscoverTestOwners(root string) ([]Surface, error) {
	owners := map[string][]string{}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == "testdata" || entry.Name() == "releasecontract") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		source := filepath.ToSlash(relative)
		file, err := goFile(root, source)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*goast.FuncDecl)
			if !ok || function.Recv != nil || !strings.HasPrefix(function.Name.Name, "Test") {
				continue
			}
			owner := filepath.ToSlash(filepath.Dir(source))
			owners[owner] = append(owners[owner], function.Name.Name+" ("+source+")")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := []Surface{}
	for owner, members := range owners {
		result = append(result, Surface{ID: "tests:" + owner, Kind: "existing test owner", Source: owner, Members: sortedUnique(members)})
	}
	return result, nil
}

func DiscoverTypeKinds(root string) ([]Surface, error) {
	result := []Surface{}
	for _, source := range []string{"internal/types/types.go", "internal/token/token.go"} {
		file, err := goFile(root, source)
		if err != nil {
			return nil, err
		}
		members := []string{}
		for _, declaration := range file.Decls {
			group, ok := declaration.(*goast.GenDecl)
			if !ok || group.Tok != token.CONST {
				continue
			}
			for _, specification := range group.Specs {
				value, ok := specification.(*goast.ValueSpec)
				if !ok {
					continue
				}
				kind, ok := value.Type.(*goast.Ident)
				if !ok || kind.Name != "Kind" {
					continue
				}
				if len(value.Names) != 1 || len(value.Values) != 1 {
					return nil, fmt.Errorf("nonliteral type/token kind in %s", source)
				}
				spelling, ok := quoted(value.Values[0])
				if !ok {
					return nil, fmt.Errorf("nonliteral type/token spelling in %s", source)
				}
				members = append(members, value.Names[0].Name+" = "+spelling)
			}
		}
		if len(members) == 0 {
			return nil, fmt.Errorf("missing type/token kinds in %s", source)
		}
		result = append(result, Surface{ID: "kinds:" + source, Kind: "semantic and lexical ownership", Source: source, Members: sortedUnique(members)})
	}
	return result, nil
}
