package packageextension

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectGenerationResponseIsVersionedAndSerializable(t *testing.T) {
	span := SourceSpan{
		Start: SourcePosition{Offset: 10, Line: 2, Column: 1},
		End:   SourcePosition{Offset: 20, Line: 4, Column: 4},
	}
	response := ProjectGenerationResponse{
		ProtocolVersion: ProjectGenerationProtocolVersion,
		Provider:        "trb.jobs.manifest",
		Sources: []ProjectGeneratedSource{{
			ID: "worker-dispatch", ModulePath: "main", Source: "def __trb_jobs_dispatch()\n\treturn\nend\n",
			RequiredImports: []RequiredImport{{Path: "trb/jobs", Symbols: []string{"JobResult"}}}, Origin: span,
		}},
		Issues: []ProjectGenerationIssue{{ModulePath: "main", Message: "example issue", Span: span}},
	}
	if err := ValidateProjectGenerationResponse(response); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ProjectGenerationResponse
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProjectGenerationResponse(decoded); err != nil {
		t.Fatal(err)
	}
}

func TestProjectGenerationResponseRejectsInvalidBoundaryData(t *testing.T) {
	valid := func() ProjectGenerationResponse {
		span := SourceSpan{
			Start: SourcePosition{Offset: 0, Line: 1, Column: 1},
			End:   SourcePosition{Offset: 1, Line: 1, Column: 2},
		}
		return ProjectGenerationResponse{
			ProtocolVersion: ProjectGenerationProtocolVersion,
			Provider:        "trb.jobs.manifest",
			Sources:         []ProjectGeneratedSource{{ID: "worker-dispatch", ModulePath: "main", Source: "def generated()\n\treturn\nend\n", Origin: span}},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*ProjectGenerationResponse)
		message string
	}{
		{name: "version", mutate: func(response *ProjectGenerationResponse) { response.ProtocolVersion++ }, message: "unsupported project generation protocol version"},
		{name: "provider", mutate: func(response *ProjectGenerationResponse) { response.Provider = "" }, message: "provider is missing"},
		{name: "source id", mutate: func(response *ProjectGenerationResponse) { response.Sources[0].ID = "" }, message: "without an id"},
		{name: "source module", mutate: func(response *ProjectGenerationResponse) { response.Sources[0].ModulePath = "" }, message: "has no module path"},
		{name: "source body", mutate: func(response *ProjectGenerationResponse) { response.Sources[0].Source = " \n" }, message: "is empty"},
		{name: "duplicate source", mutate: func(response *ProjectGenerationResponse) {
			response.Sources = append(response.Sources, response.Sources[0])
		}, message: "duplicate source"},
		{name: "required import", mutate: func(response *ProjectGenerationResponse) {
			response.Sources[0].RequiredImports = []RequiredImport{{Path: "trb/jobs"}}
		}, message: "invalid required import"},
		{name: "source origin", mutate: func(response *ProjectGenerationResponse) {
			response.Sources[0].Origin = SourceSpan{}
		}, message: "has no authored origin"},
		{name: "source span", mutate: func(response *ProjectGenerationResponse) {
			response.Sources[0].Origin = SourceSpan{Start: SourcePosition{Offset: 1}, End: SourcePosition{Offset: 2}}
		}, message: "invalid source span"},
		{name: "issue module", mutate: func(response *ProjectGenerationResponse) {
			response.Issues = []ProjectGenerationIssue{{Message: "invalid"}}
		}, message: "without a module path"},
		{name: "issue message", mutate: func(response *ProjectGenerationResponse) {
			response.Issues = []ProjectGenerationIssue{{ModulePath: "main", Span: response.Sources[0].Origin}}
		}, message: "empty issue"},
		{name: "issue span", mutate: func(response *ProjectGenerationResponse) {
			response.Issues = []ProjectGenerationIssue{{ModulePath: "main", Message: "invalid"}}
		}, message: "unlocated issue"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := valid()
			test.mutate(&response)
			err := ValidateProjectGenerationResponse(response)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q, got %v", test.message, err)
			}
		})
	}
}
