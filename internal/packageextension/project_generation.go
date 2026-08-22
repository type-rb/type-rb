package packageextension

import (
	"fmt"
	"strings"
)

const ProjectGenerationProtocolVersion = 1

// ProjectGenerationResponse is a versioned, data-only response from a
// project package provider. Sources are ordinary TypeRB fragments attached to
// existing project modules by the compiler host.
type ProjectGenerationResponse struct {
	ProtocolVersion int                      `json:"protocolVersion"`
	Provider        string                   `json:"provider"`
	Sources         []ProjectGeneratedSource `json:"sources,omitempty"`
	Issues          []ProjectGenerationIssue `json:"issues,omitempty"`
}

type ProjectGeneratedSource struct {
	ID              string           `json:"id"`
	ModulePath      string           `json:"modulePath"`
	Source          string           `json:"source"`
	RequiredImports []RequiredImport `json:"requiredImports,omitempty"`
	Origin          SourceSpan       `json:"origin"`
}

type ProjectGenerationIssue struct {
	ModulePath string     `json:"modulePath"`
	Message    string     `json:"message"`
	Span       SourceSpan `json:"span"`
}

func ValidateProjectGenerationResponse(response ProjectGenerationResponse) error {
	if response.ProtocolVersion != ProjectGenerationProtocolVersion {
		return fmt.Errorf("unsupported project generation protocol version %d", response.ProtocolVersion)
	}
	if strings.TrimSpace(response.Provider) == "" {
		return fmt.Errorf("project generation response provider is missing")
	}
	seen := map[string]bool{}
	for _, source := range response.Sources {
		if strings.TrimSpace(source.ID) == "" {
			return fmt.Errorf("project generation response contains a source without an id")
		}
		if strings.TrimSpace(source.ModulePath) == "" {
			return fmt.Errorf("project generated source %s has no module path", source.ID)
		}
		if strings.TrimSpace(source.Source) == "" {
			return fmt.Errorf("project generated source %s in module %s is empty", source.ID, source.ModulePath)
		}
		key := source.ModulePath + "\x00" + source.ID
		if seen[key] {
			return fmt.Errorf("project generation response contains duplicate source %s in module %s", source.ID, source.ModulePath)
		}
		seen[key] = true
		for _, imported := range source.RequiredImports {
			if strings.TrimSpace(imported.Path) == "" || len(imported.Symbols) == 0 {
				return fmt.Errorf("project generated source %s in module %s contains an invalid required import", source.ID, source.ModulePath)
			}
			for _, symbol := range imported.Symbols {
				if strings.TrimSpace(symbol) == "" {
					return fmt.Errorf("project generated source %s in module %s contains an invalid required import", source.ID, source.ModulePath)
				}
			}
		}
		if source.Origin == (SourceSpan{}) {
			return fmt.Errorf("project generated source %s in module %s has no authored origin", source.ID, source.ModulePath)
		}
		if err := validateSourceSpan(source.Origin); err != nil {
			return fmt.Errorf("project generated source %s in module %s origin: %w", source.ID, source.ModulePath, err)
		}
	}
	for _, issue := range response.Issues {
		if strings.TrimSpace(issue.ModulePath) == "" {
			return fmt.Errorf("project generation response contains an issue without a module path")
		}
		if strings.TrimSpace(issue.Message) == "" {
			return fmt.Errorf("project generation response contains an empty issue in module %s", issue.ModulePath)
		}
		if issue.Span == (SourceSpan{}) {
			return fmt.Errorf("project generation response contains an unlocated issue in module %s", issue.ModulePath)
		}
		if err := validateSourceSpan(issue.Span); err != nil {
			return fmt.Errorf("project generation issue in module %s: %w", issue.ModulePath, err)
		}
	}
	return nil
}
