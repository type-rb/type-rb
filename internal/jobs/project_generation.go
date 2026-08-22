package jobs

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/types"
)

const (
	projectDispatchSourceID = "worker-dispatch"
	projectEnqueueSourceID  = "enqueue:"
)

// GenerateProject returns portable TypeRB source for project-wide Jobs
// behavior. Runtime adapters remain responsible for persistence and worker
// lifecycle; this source owns payload encoding, scheduling normalization,
// payload decoding, and typed Job dispatch.
func GenerateProject(input packageextension.ProjectDeclarationInput, entrypointModule, configurationModule string, origin packageextension.SourceSpan) (packageextension.ProjectGenerationResponse, error) {
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
	if len(jobs) == 0 {
		return response, nil
	}
	if strings.TrimSpace(configurationModule) == "" {
		return response, fmt.Errorf("trb/jobs project generation requires a configuration module")
	}
	if strings.TrimSpace(entrypointModule) != "" {
		source, imports := jobsDispatchSource(jobs, entrypointModule)
		response.Sources = append(response.Sources, packageextension.ProjectGeneratedSource{
			ID: projectDispatchSourceID, ModulePath: entrypointModule, Source: source, RequiredImports: imports, Origin: origin,
		})
	}
	enqueueSources, err := jobsEnqueueSources(input, jobs, configurationModule)
	if err != nil {
		return response, err
	}
	response.Sources = append(response.Sources, enqueueSources...)
	if err := packageextension.ValidateProjectGenerationResponse(response); err != nil {
		return packageextension.ProjectGenerationResponse{}, err
	}
	return response, nil
}

// EnqueueHelperName returns the compiler-reserved portable helper that owns a
// derived Job enqueue method's serialization and scheduling behavior.
func EnqueueHelperName(jobName, method string) string {
	return "__trb_jobs_" + jobName + "_" + method
}

func jobsEnqueueSources(input packageextension.ProjectDeclarationInput, jobs []Job, configurationModule string) ([]packageextension.ProjectGeneratedSource, error) {
	origins := map[string]packageextension.SourceSpan{}
	for _, module := range input.Modules {
		for _, class := range module.Classes {
			origins[module.ModulePath+"\x00"+class.Name] = class.Span
		}
	}

	result := make([]packageextension.ProjectGeneratedSource, 0, len(jobs))
	for _, job := range jobs {
		source, err := jobsEnqueueSource(job)
		if err != nil {
			return nil, err
		}
		required := map[string]map[string]bool{
			"trb/jobs": {
				"EnqueueError": true, "EnqueueErrorKind": true, "EnqueueRequest": true, "JobReference": true,
			},
			"trb/std/json":   {"JsonValue": true, "stringify": true},
			"trb/std/result": {"Result": true},
			"trb/std/time":   {"Duration": true, "Instant": true},
		}
		if configurationModule != job.ModulePath {
			required[configurationModule] = map[string]bool{"JOBS_ADAPTER": true}
		}
		result = append(result, packageextension.ProjectGeneratedSource{
			ID:              projectEnqueueSourceID + job.ModulePath + ":" + job.Name,
			ModulePath:      job.ModulePath,
			Source:          source,
			RequiredImports: sortedProjectGenerationImports(required),
			Origin:          origins[job.ModulePath+"\x00"+job.Name],
		})
	}
	return result, nil
}

func jobsEnqueueSource(job Job) (string, error) {
	parameters := make([]string, len(job.Parameters))
	arguments := make([]string, len(job.Parameters))
	values := make([]string, len(job.Parameters))
	for index, parameter := range job.Parameters {
		parameters[index] = parameter.Name + ": " + parameter.Type.String()
		arguments[index] = parameter.Name
		wireType := parameter.WireType
		if wireType.Kind == "" {
			wireType = parameter.Type
		}
		value, err := jobsJSONValue(wireType, parameter.Name)
		if err != nil {
			return "", fmt.Errorf("trb/jobs Job %s parameter %s: %w", job.Name, parameter.Name, err)
		}
		values[index] = value
	}
	requestHelper := EnqueueHelperName(job.Name, "request")
	laterHelper := EnqueueHelperName(job.Name, "perform_later")
	inHelper := EnqueueHelperName(job.Name, "perform_in")
	atHelper := EnqueueHelperName(job.Name, "perform_at")
	requestResult := "Result<EnqueueRequest, EnqueueError>"
	enqueueResult := "Result<JobReference, EnqueueError>"
	maximumAttempts := "nil"
	if job.MaximumAttempts > 0 {
		maximumAttempts = strconv.Itoa(job.MaximumAttempts)
	}

	var source strings.Builder
	source.WriteString("def " + requestHelper + "(" + strings.Join(parameters, ", ") + "): " + requestResult + "\n")
	source.WriteString("\tpayload_values: Array<JsonValue> := [" + strings.Join(values, ", ") + "]\n")
	source.WriteString("\tpayload := stringify(JsonValue::Array(payload_values)) catch |error|\n")
	source.WriteString("\t\treturn " + requestResult + "::Err(EnqueueError.new(kind: EnqueueErrorKind::Serialization, message: error.message))\n")
	source.WriteString("\tend\n")
	source.WriteString("\treturn " + requestResult + "::Ok(EnqueueRequest.new(\n")
	source.WriteString("\t\tjob_name: " + strconv.Quote(job.Name) + ",\n")
	source.WriteString("\t\tpayload: payload,\n")
	source.WriteString("\t\tpayload_version: 1,\n")
	source.WriteString("\t\tqueue_name: " + strconv.Quote(job.Queue) + ",\n")
	source.WriteString("\t\tpriority: " + strconv.Itoa(job.Priority) + ",\n")
	source.WriteString("\t\tmaximum_attempts: " + maximumAttempts + ",\n")
	source.WriteString("\t))\n")
	source.WriteString("end\n\n")

	source.WriteString("def " + laterHelper + "(" + strings.Join(parameters, ", ") + "): " + enqueueResult + "\n")
	source.WriteString("\trequest := try " + requestHelper + "(" + strings.Join(arguments, ", ") + ")\n")
	source.WriteString("\treturn JOBS_ADAPTER.enqueue(request)\n")
	source.WriteString("end\n\n")

	delayedParameters := append([]string{"delay: Duration"}, parameters...)
	source.WriteString("def " + inHelper + "(" + strings.Join(delayedParameters, ", ") + "): " + enqueueResult + "\n")
	source.WriteString("\tif delay.before?(Duration.seconds(0))\n")
	source.WriteString("\t\treturn " + enqueueResult + "::Err(EnqueueError.new(kind: EnqueueErrorKind::InvalidArgument, message: \"job delay must not be negative\"))\n")
	source.WriteString("\tend\n")
	source.WriteString("\trequest := try " + requestHelper + "(" + strings.Join(arguments, ", ") + ")\n")
	source.WriteString("\treturn JOBS_ADAPTER.enqueue_at(request, Instant.now().add(delay))\n")
	source.WriteString("end\n\n")

	scheduledParameters := append([]string{"scheduled_at: Instant"}, parameters...)
	source.WriteString("def " + atHelper + "(" + strings.Join(scheduledParameters, ", ") + "): " + enqueueResult + "\n")
	source.WriteString("\trequest := try " + requestHelper + "(" + strings.Join(arguments, ", ") + ")\n")
	source.WriteString("\treturn JOBS_ADAPTER.enqueue_at(request, scheduled_at)\n")
	source.WriteString("end\n")
	return source.String(), nil
}

func jobsJSONValue(typ types.Type, value string) (string, error) {
	switch typ.Kind {
	case types.Bool:
		return "JsonValue::Boolean(" + value + ")", nil
	case types.Int:
		return "JsonValue::Integer(" + value + ")", nil
	case types.Float:
		return "JsonValue::Float(" + value + ")", nil
	case types.String:
		return "JsonValue::String(" + value + ")", nil
	default:
		return "", fmt.Errorf("unsupported payload wire type %s", typ.String())
	}
}

func sortedProjectGenerationImports(required map[string]map[string]bool) []packageextension.RequiredImport {
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
	return imports
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
