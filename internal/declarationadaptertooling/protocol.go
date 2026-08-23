// Package declarationadaptertooling defines the versioned JSON report emitted
// by explicit declaration-adapter authoring commands. It reports validated
// semantic data only and does not expose compiler AST, typed IR, or backend
// hooks.
package declarationadaptertooling

import "github.com/type-rb/type-rb/internal/diagnostic"

const ProtocolVersion = 1

type Report struct {
	ProtocolVersion int                         `json:"protocolVersion"`
	CompilerVersion string                      `json:"compilerVersion"`
	Package         *Package                    `json:"package,omitempty"`
	Adapters        []Adapter                   `json:"adapters"`
	Diagnostics     []diagnostic.JSONDiagnostic `json:"diagnostics"`
	Summary         Summary                     `json:"summary"`
}

type Package struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	ManifestPath string `json:"manifestPath"`
}

type Adapter struct {
	Mode                       string `json:"mode"`
	Path                       string `json:"path"`
	Valid                      bool   `json:"valid"`
	DeclarationProtocolVersion int    `json:"declarationProtocolVersion,omitempty"`
	Modules                    int    `json:"modules"`
	Exports                    int    `json:"exports"`
	SupportingRecords          int    `json:"supportingRecords"`
}

type Summary struct {
	Adapters          int `json:"adapters"`
	ValidAdapters     int `json:"validAdapters"`
	Modules           int `json:"modules"`
	Exports           int `json:"exports"`
	SupportingRecords int `json:"supportingRecords"`
	Errors            int `json:"errors"`
	Warnings          int `json:"warnings"`
}

type BuildOptions struct {
	CompilerVersion string
	Package         *Package
	Adapters        []Adapter
	Diagnostics     []diagnostic.Diagnostic
}

func Build(options BuildOptions) Report {
	diagnostics := diagnostic.NewJSONReport(options.Diagnostics)
	report := Report{
		ProtocolVersion: ProtocolVersion,
		CompilerVersion: options.CompilerVersion,
		Package:         options.Package,
		Adapters:        append([]Adapter(nil), options.Adapters...),
		Diagnostics:     diagnostics.Diagnostics,
		Summary: Summary{
			Adapters: len(options.Adapters), Errors: diagnostics.Summary.Errors, Warnings: diagnostics.Summary.Warnings,
		},
	}
	if report.Adapters == nil {
		report.Adapters = []Adapter{}
	}
	for _, adapter := range report.Adapters {
		if adapter.Valid {
			report.Summary.ValidAdapters++
		}
		report.Summary.Modules += adapter.Modules
		report.Summary.Exports += adapter.Exports
		report.Summary.SupportingRecords += adapter.SupportingRecords
	}
	return report
}
