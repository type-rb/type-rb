package declarationadapterhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/packageextension"
)

type Source struct {
	Package      string
	Mode         string
	Path         string
	Dependencies map[string]string
}

func Read(path string) (packageextension.DeclarationAdapterCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return packageextension.DeclarationAdapterCatalog{}, err
	}
	var header struct {
		ProtocolVersion int `json:"protocolVersion"`
		FormatVersion   int `json:"formatVersion"`
	}
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&header); err != nil {
		return packageextension.DeclarationAdapterCatalog{}, err
	}
	if header.ProtocolVersion == 0 && header.FormatVersion != 0 {
		return packageextension.DeclarationAdapterCatalog{}, fmt.Errorf("legacy native type provider formatVersion %d is not supported; rename nativeTypeProviders to declarationAdapters, replace formatVersion with protocolVersion %d, use arguments instead of args in semantic types, and run trb install", header.FormatVersion, packageextension.DeclarationAdapterProtocolVersion)
	}
	if header.ProtocolVersion == 1 && packageextension.DeclarationAdapterProtocolVersion == 2 {
		return packageextension.DeclarationAdapterCatalog{}, fmt.Errorf("declaration adapter protocolVersion 1 is no longer supported; set protocolVersion to 2, replace class members with instanceMembers or classMembers, and run trb install")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var catalog packageextension.DeclarationAdapterCatalog
	if err := decoder.Decode(&catalog); err != nil {
		return packageextension.DeclarationAdapterCatalog{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return packageextension.DeclarationAdapterCatalog{}, fmt.Errorf("trailing JSON content")
	}
	if err := packageextension.ValidateDeclarationAdapterCatalog(catalog); err != nil {
		return packageextension.DeclarationAdapterCatalog{}, err
	}
	return catalog, nil
}

func Checksums(sources []Source) (map[string]string, error) {
	sorted := append([]Source(nil), sources...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Package != sorted[j].Package {
			return sorted[i].Package < sorted[j].Package
		}
		if sorted[i].Mode != sorted[j].Mode {
			return sorted[i].Mode < sorted[j].Mode
		}
		return sorted[i].Path < sorted[j].Path
	})
	result := make(map[string]string, len(sorted))
	for _, source := range sorted {
		if strings.TrimSpace(source.Package) == "" || strings.TrimSpace(source.Mode) == "" || strings.TrimSpace(source.Path) == "" {
			return nil, fmt.Errorf("declaration adapter source requires a package, mode, and path")
		}
		key := source.Package + "#" + source.Mode
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("declaration adapter source %s is duplicated", key)
		}
		data, err := os.ReadFile(source.Path)
		if err != nil {
			return nil, fmt.Errorf("read declaration adapter %s (%s): %w", source.Package, source.Path, err)
		}
		sum := sha256.Sum256(data)
		result[key] = "sha256:" + hex.EncodeToString(sum[:])
	}
	return result, nil
}
