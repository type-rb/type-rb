// Package declarationproviderhost loads fixed semantic declarations supplied
// by external TypeRB packages. It accepts a deliberately smaller subset than
// compiler-owned declaration providers and never executes package code.
package declarationproviderhost

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
)

type Source struct {
	Package string
	Mode    string
	Module  string
	Path    string
}

func Read(source Source) (*declaration.Catalog, error) {
	if strings.TrimSpace(source.Package) == "" || source.Mode != "ruby" || strings.TrimSpace(source.Module) == "" || strings.TrimSpace(source.Path) == "" {
		return nil, fmt.Errorf("declaration provider source requires a package, ruby mode, root module, and path")
	}
	if source.Module != source.Package {
		return nil, fmt.Errorf("declaration provider root module %s must match package %s", source.Module, source.Package)
	}
	data, err := os.ReadFile(source.Path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var provided packageextension.DeclarationCatalog
	if err := decoder.Decode(&provided); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("trailing JSON content")
	}
	if err := packageextension.ValidateDeclarationCatalog(provided); err != nil {
		return nil, err
	}
	if provided.Provider != source.Package {
		return nil, fmt.Errorf("declaration provider identifies %s, expected package %s", provided.Provider, source.Package)
	}
	if err := validateFixedCatalog(provided); err != nil {
		return nil, err
	}
	return packageextensionhost.ImportDeclarationCatalog(provided)
}

func validateFixedCatalog(catalog packageextension.DeclarationCatalog) error {
	if len(catalog.FunctionBlockRules) > 0 || len(catalog.FunctionArgumentReferenceRules) > 0 || len(catalog.RuntimeTypes) > 0 {
		return fmt.Errorf("fixed declaration provider %s cannot declare project rules or compiler runtime types", catalog.Provider)
	}
	if len(catalog.Types) == 0 && len(catalog.Modules) == 0 {
		return fmt.Errorf("fixed declaration provider %s contains no type or module declarations", catalog.Provider)
	}
	typeNames := map[string]bool{}
	for _, declared := range catalog.Types {
		if declared.SourceModule != "" {
			return fmt.Errorf("fixed declaration provider %s type %s cannot claim a project source module", catalog.Provider, declared.Name)
		}
		typeNames[declared.Name] = true
		if err := validateFixedMembers(catalog.Provider, declared.Name, declared.InstanceMembers); err != nil {
			return err
		}
		if err := validateFixedMembers(catalog.Provider, declared.Name, declared.ClassMembers); err != nil {
			return err
		}
	}
	for _, declared := range catalog.Modules {
		if typeNames[declared.Name] {
			return fmt.Errorf("fixed declaration provider %s declares %s as both a type and a module", catalog.Provider, declared.Name)
		}
		if err := validateFixedMembers(catalog.Provider, declared.Name, declared.InstanceMembers); err != nil {
			return err
		}
	}
	return nil
}

func validateFixedMembers(provider, owner string, members []packageextension.DeclaredMember) error {
	for _, member := range members {
		if member.RuntimeOperation != "" || member.CallSpecializer != "" {
			return fmt.Errorf("fixed declaration provider %s member %s.%s cannot select compiler execution hooks", provider, owner, member.Name)
		}
		if member.Block != nil {
			return fmt.Errorf("fixed declaration provider %s member %s.%s cannot declare compiler-controlled block behavior", provider, owner, member.Name)
		}
		if err := validateFixedType(member.Return); err != nil {
			return fmt.Errorf("fixed declaration provider %s member %s.%s return: %w", provider, owner, member.Name, err)
		}
		for _, parameter := range member.Parameters {
			if parameter.RepresentationBoundary {
				return fmt.Errorf("fixed declaration provider %s member %s.%s cannot weaken nominal representation boundaries", provider, owner, member.Name)
			}
			if err := validateFixedType(parameter.Type); err != nil {
				return fmt.Errorf("fixed declaration provider %s member %s.%s parameter %s: %w", provider, owner, member.Name, parameter.Name, err)
			}
		}
		for _, signature := range member.Alternatives {
			if err := validateFixedType(signature.Return); err != nil {
				return fmt.Errorf("fixed declaration provider %s member %s.%s alternative return: %w", provider, owner, member.Name, err)
			}
			for _, parameter := range signature.Parameters {
				if parameter.RepresentationBoundary {
					return fmt.Errorf("fixed declaration provider %s member %s.%s cannot weaken nominal representation boundaries", provider, owner, member.Name)
				}
				if err := validateFixedType(parameter.Type); err != nil {
					return fmt.Errorf("fixed declaration provider %s member %s.%s alternative parameter %s: %w", provider, owner, member.Name, parameter.Name, err)
				}
			}
		}
	}
	return nil
}

func validateFixedType(typ packageextension.Type) error {
	if typ.Kind == "any" || typ.Kind == "invalid" {
		return fmt.Errorf("cannot use unsafe type kind %s", typ.Kind)
	}
	if typ.Representation != nil {
		return fmt.Errorf("cannot supply compiler-derived nominal representation metadata")
	}
	for _, argument := range typ.Arguments {
		if err := validateFixedType(argument); err != nil {
			return err
		}
	}
	return nil
}
