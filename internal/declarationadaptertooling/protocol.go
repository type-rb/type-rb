// Package declarationadaptertooling defines the versioned JSON report emitted
// by explicit declaration-adapter authoring commands. It reports validated
// semantic data only and does not expose compiler AST, typed IR, or backend
// hooks.
package declarationadaptertooling

import "github.com/type-rb/type-rb/internal/diagnostic"

const ProtocolVersion = 1

const (
	TestPhaseAdapterCheck = "adapter_check"
	TestPhaseBuild        = "build"
	TestPhaseNativeCheck  = "native_check"

	TestStatusPassed = "passed"
	TestStatusFailed = "failed"
	TestStatusNotRun = "not_run"
)

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

// TestReport is the versioned machine-readable result of an explicitly
// invoked adapter conformance run. It contains stable phase states while
// native tool output remains on the command's diagnostic stream.
type TestReport struct {
	ProtocolVersion int                         `json:"protocolVersion"`
	CompilerVersion string                      `json:"compilerVersion"`
	Package         *Package                    `json:"package,omitempty"`
	Tests           []Test                      `json:"tests"`
	Diagnostics     []diagnostic.JSONDiagnostic `json:"diagnostics"`
	Summary         TestSummary                 `json:"summary"`
}

type Test struct {
	Mode       string      `json:"mode"`
	ConfigPath string      `json:"configPath"`
	Command    []string    `json:"command"`
	Passed     bool        `json:"passed"`
	Phases     []TestPhase `json:"phases"`
}

type TestPhase struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type TestSummary struct {
	Tests       int `json:"tests"`
	PassedTests int `json:"passedTests"`
	FailedTests int `json:"failedTests"`
	Errors      int `json:"errors"`
	Warnings    int `json:"warnings"`
}

type TestBuildOptions struct {
	CompilerVersion string
	Package         *Package
	Tests           []Test
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

func BuildTest(options TestBuildOptions) TestReport {
	diagnostics := diagnostic.NewJSONReport(options.Diagnostics)
	report := TestReport{
		ProtocolVersion: ProtocolVersion,
		CompilerVersion: options.CompilerVersion,
		Package:         options.Package,
		Tests:           append([]Test(nil), options.Tests...),
		Diagnostics:     diagnostics.Diagnostics,
		Summary: TestSummary{
			Tests: len(options.Tests), Errors: diagnostics.Summary.Errors, Warnings: diagnostics.Summary.Warnings,
		},
	}
	if report.Tests == nil {
		report.Tests = []Test{}
	}
	for index := range report.Tests {
		report.Tests[index].Command = append([]string(nil), report.Tests[index].Command...)
		report.Tests[index].Phases = append([]TestPhase(nil), report.Tests[index].Phases...)
		if report.Tests[index].Command == nil {
			report.Tests[index].Command = []string{}
		}
		if report.Tests[index].Phases == nil {
			report.Tests[index].Phases = []TestPhase{}
		}
		if report.Tests[index].Passed {
			report.Summary.PassedTests++
		} else {
			report.Summary.FailedTests++
		}
	}
	return report
}
