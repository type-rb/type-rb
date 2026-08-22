package jobs

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/types"
)

const projectDispatchSourceID = "worker-dispatch"

// GenerateProject returns portable TypeRB source for project-wide Jobs
// behavior. Runtime adapters remain responsible for persistence and worker
// lifecycle; this source owns payload decoding and typed Job dispatch.
func GenerateProject(input packageextension.ProjectDeclarationInput, entrypointModule string, origin packageextension.SourceSpan) (packageextension.ProjectGenerationResponse, error) {
	response := packageextension.ProjectGenerationResponse{
		ProtocolVersion: packageextension.ProjectGenerationProtocolVersion,
		Provider:        ProjectProvider,
	}
	if err := packageextension.ValidateProjectDeclarationInput(input); err != nil {
		return response, err
	}
	if input.Provider != PackageName {
		return response, fmt.Errorf("trb/jobs received project declaration input for provider %s", input.Provider)
	}
	jobs, err := discoverDeclarationJobs(input)
	if err != nil {
		return response, err
	}
	if len(jobs) == 0 || strings.TrimSpace(entrypointModule) == "" {
		return response, nil
	}
	source, imports := jobsDispatchSource(jobs, entrypointModule)
	response.Sources = []packageextension.ProjectGeneratedSource{{
		ID: projectDispatchSourceID, ModulePath: entrypointModule, Source: source, RequiredImports: imports, Origin: origin,
	}}
	if err := packageextension.ValidateProjectGenerationResponse(response); err != nil {
		return packageextension.ProjectGenerationResponse{}, err
	}
	return response, nil
}

func jobsDispatchSource(jobs []Job, entrypointModule string) (string, []packageextension.RequiredImport) {
	required := map[string]map[string]bool{
		"trb/jobs":     {"JobError": true, "JobResult": true},
		"trb/std/json": {"as_array": true, "parse": true},
	}
	floatArguments := false
	for _, job := range jobs {
		if job.PerformKind != PerformJobResult {
			if required["trb/std/unit"] == nil {
				required["trb/std/unit"] = map[string]bool{}
			}
			required["trb/std/unit"]["Unit"] = true
		}
		if job.ModulePath != entrypointModule {
			if required[job.ModulePath] == nil {
				required[job.ModulePath] = map[string]bool{}
			}
			required[job.ModulePath][job.Name] = true
		}
		for _, parameter := range job.Parameters {
			wireType := parameter.WireType
			if wireType.Kind == "" {
				wireType = parameter.Type
			}
			switch wireType.Kind {
			case types.Bool:
				required["trb/std/json"]["as_boolean"] = true
			case types.Int:
				required["trb/std/json"]["as_integer"] = true
			case types.Float:
				floatArguments = true
				required["trb/std/json"]["as_float"] = true
			case types.String:
				required["trb/std/json"]["as_string"] = true
			}
		}
	}

	var source strings.Builder
	if floatArguments {
		required["trb/std/json"]["JsonError"] = true
		required["trb/std/json"]["JsonValue"] = true
		required["trb/std/result"] = map[string]bool{"Result": true}
		source.WriteString("def __trb_jobs_as_float(value: JsonValue): Result<Float, JsonError>\n")
		source.WriteString("\tcase value\n")
		source.WriteString("\twhen JsonValue::Integer(integer)\n")
		source.WriteString("\t\treturn Result<Float, JsonError>::Ok(integer.to_f())\n")
		source.WriteString("\telse\n")
		source.WriteString("\t\treturn as_float(value)\n")
		source.WriteString("\tend\n")
		source.WriteString("end\n\n")
	}
	source.WriteString("def __trb_jobs_dispatch(job_name: String, payload: String, payload_version: Integer): JobResult\n")
	source.WriteString("\tif payload_version != 1\n")
	source.WriteString("\t\treturn JobResult::Err(JobError.new(message: \"unsupported job payload version \" + payload_version.to_s()))\n")
	source.WriteString("\tend\n")
	source.WriteString("\tparsed := parse(payload) catch |error|\n")
	source.WriteString("\t\treturn JobResult::Err(JobError.new(message: \"decode job payload: \" + error.message))\n")
	source.WriteString("\tend\n")
	source.WriteString("\tpayload_values := as_array(parsed) catch |error|\n")
	source.WriteString("\t\treturn JobResult::Err(JobError.new(message: \"decode job payload: \" + error.message))\n")
	source.WriteString("\tend\n")
	source.WriteString("\tcase job_name\n")
	for _, job := range jobs {
		source.WriteString("\twhen " + strconv.Quote(job.Name) + "\n")
		source.WriteString("\t\tif payload_values.size() != " + strconv.Itoa(len(job.Parameters)) + "\n")
		source.WriteString("\t\t\treturn JobResult::Err(JobError.new(message: \"job " + job.Name + " expects " + strconv.Itoa(len(job.Parameters)) + " arguments, got \" + payload_values.size().to_s()))\n")
		source.WriteString("\t\tend\n")
		argumentNames := make([]string, len(job.Parameters))
		for index, parameter := range job.Parameters {
			name := "argument" + strconv.Itoa(index)
			argumentNames[index] = name
			wireType := parameter.WireType
			if wireType.Kind == "" {
				wireType = parameter.Type
			}
			converter := jobsArgumentConverter(wireType)
			source.WriteString("\t\t" + name + " := " + converter + "(payload_values[" + strconv.Itoa(index) + "]) catch |error|\n")
			source.WriteString("\t\t\treturn JobResult::Err(JobError.new(message: \"decode " + job.Name + "." + parameter.Name + ": \" + error.message))\n")
			source.WriteString("\t\tend\n")
		}
		call := job.Name + ".new().perform(" + strings.Join(argumentNames, ", ") + ")"
		if job.PerformKind == PerformJobResult {
			source.WriteString("\t\treturn " + call + "\n")
		} else {
			source.WriteString("\t\t" + call + "\n")
			source.WriteString("\t\treturn JobResult::Ok(Unit.new())\n")
		}
	}
	source.WriteString("\telse\n")
	source.WriteString("\t\treturn JobResult::Err(JobError.new(message: \"unknown job \" + job_name))\n")
	source.WriteString("\tend\n")
	source.WriteString("end\n")

	paths := make([]string, 0, len(required))
	for path := range required {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	imports := make([]packageextension.RequiredImport, 0, len(paths))
	for _, path := range paths {
		symbols := make([]string, 0, len(required[path]))
		for symbol := range required[path] {
			symbols = append(symbols, symbol)
		}
		sort.Strings(symbols)
		imports = append(imports, packageextension.RequiredImport{Path: path, Symbols: symbols})
	}
	return source.String(), imports
}

func jobsArgumentConverter(typ types.Type) string {
	switch typ.Kind {
	case types.Bool:
		return "as_boolean"
	case types.Int:
		return "as_integer"
	case types.Float:
		return "__trb_jobs_as_float"
	case types.String:
		return "as_string"
	default:
		panic("unsupported Jobs wire type " + typ.String())
	}
}
