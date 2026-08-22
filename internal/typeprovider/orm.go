package typeprovider

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
)

func init() {
	register(ormintegration.TypeProvider, loadORM, ormProviderInputs)
}

func loadORM(programs []*ast.Program, context Context) (*declaration.Catalog, error) {
	provided, err := loadORMDeclarations(programs, context)
	if err != nil {
		return nil, err
	}
	return packageextensionhost.ImportDeclarationCatalog(provided)
}

func loadORMDeclarations(programs []*ast.Program, context Context) (packageextension.DeclarationCatalog, error) {
	input, err := ormDeclarationInput(programs, context)
	if err != nil {
		return packageextension.DeclarationCatalog{}, err
	}
	catalog, err := ormintegration.Declarations(input)
	if err != nil {
		return packageextension.DeclarationCatalog{}, err
	}
	return packageextensionhost.ExportDeclarationCatalog(ormintegration.PackageName, catalog)
}

func ormDeclarationInput(programs []*ast.Program, context Context) (ormintegration.DeclarationInput, error) {
	relevant := providerPrograms(programs, ormProviderProgram)
	project, err := packageextensionhost.ExportProjectDeclarationInput(ormintegration.PackageName, relevant, packageextensionhost.ProjectDeclarationInputOptions{
		PackageAliasesByModule: context.PackageAliasesByModule,
	})
	if err != nil {
		return ormintegration.DeclarationInput{}, err
	}
	schema, err := ormintegration.LoadSchema(context.ProjectRoot, context.PackageOptions)
	if err != nil {
		return ormintegration.DeclarationInput{}, err
	}
	return ormintegration.ExportDeclarationInput(project, schema)
}

func ormProviderInputs(programs []*ast.Program, context Context) providerInputSnapshot {
	result := providerInputSnapshot{programs: providerPrograms(programs, ormProviderProgram), reusable: true}
	var configured struct {
		SchemaLock string `json:"schemaLock"`
	}
	if err := json.Unmarshal(context.PackageOptions[ormintegration.PackageName], &configured); err != nil {
		result.reusable = false
		return result
	}
	configured.SchemaLock = strings.TrimSpace(configured.SchemaLock)
	path := ""
	if configured.SchemaLock == "" {
		path = filepath.Join(context.ProjectRoot, "db", "schema.lock.json")
	} else {
		if filepath.IsAbs(configured.SchemaLock) {
			result.reusable = false
			return result
		}
		clean := filepath.Clean(configured.SchemaLock)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			result.reusable = false
			return result
		}
		path = filepath.Join(context.ProjectRoot, clean)
	}
	file, ok := captureProviderFile(path, false)
	if !ok {
		result.reusable = false
		return result
	}
	result.files = []providerFileSnapshot{file}
	return result
}

func ormProviderProgram(program *ast.Program) bool {
	for _, statement := range program.Statements {
		switch node := statement.(type) {
		case *ast.EnumStatement:
			return true
		case *ast.ClassStatement:
			if identifier, ok := node.Superclass.(*ast.Identifier); ok && identifier.Name == "Model" {
				return true
			}
		}
	}
	return false
}
