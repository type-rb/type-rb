package compiler

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnumSelfCallUsesExactEffectOwnerAcrossBackends(t *testing.T) {
	source := []byte(`enum Status
	Ready

	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end

	def forwarded(input: Array<Integer>): Array<Integer>
		return self.values(input)
	end
end

def main()
	status := Status::Ready
	puts(status.forwarded([7])[0].to_s())
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "enum_effect_owner.trb", source, "7")
		})
	}
}

func TestNestedEnumCallKeepsQualifiedEffectOwnerAcrossBackends(t *testing.T) {
	source := []byte(`module Services
	enum Status
		Ready

		def values(input: Array<Integer>): Array<Integer>
			return input.concurrent_map do |value|
				value
			end
		end

		def forwarded(input: Array<Integer>): Array<Integer>
			return self.values(input)
		end
	end
end

def main()
	status := Services::Status::Ready
	puts(status.forwarded([8])[0].to_s())
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "nested_enum_effect_owner.trb", source, "8")
		})
	}
}

func TestNestedRawEnumUsesQualifiedOwnerAcrossBackends(t *testing.T) {
	source := []byte(`import { Result } from trb/std/result

module Services
	enum Status
		Ready = "READY"
	end
end

def main()
	parsed := Services::Status.from_raw("READY")
	case parsed
	when Result::Ok(value)
		puts(value.raw_value())
	when Result::Err(error)
		puts(error.message)
	end
	return
end
`)
	wants := map[string]string{
		"go":         "StatusReady",
		"ruby":       "Result::Ok.new(Services::Status::Ready)",
		"typescript": "Result.Ok<Services.Status, EnumValueError>(Services.Status.Ready)",
	}
	typeScript, err := Compile("nested_raw_enum.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	if output := string(typeScript.Output); !strings.Contains(output, "const parsed: Result<Services.Status, EnumValueError>") {
		t.Fatalf("generated TypeScript raw enum binding is not qualified:\n%s", output)
	}
	typeScriptProject, err := CompileProject([]SourceUnit{{
		Filename: "main.trb", ModulePath: "main", Package: "main", Source: source,
	}}, Options{Mode: "typescript", TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	checkTypeScriptArtifacts(t, typeScriptProject, "nested_raw_enum")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := Compile("nested_raw_enum.trb", source, mode)
			if err != nil {
				t.Fatal(err)
			}
			if output := string(artifact.Output); !strings.Contains(output, wants[mode]) {
				t.Fatalf("generated %s raw enum conversion is missing qualified owner %q:\n%s", mode, wants[mode], output)
			}
		})
	}
}

func TestQualifiedNestedRecordDefaultsUseHelpersAcrossBackends(t *testing.T) {
	source := []byte(`def map_values(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		value
	end
end

module Services
	record Config
		values: Array<Integer> = map_values([31])
	end
end

def main()
	defaulted := Services::Config.new()
	provided := Services::Config.new(values: [32])
	puts(defaulted.values[0].to_s())
	puts(provided.values[0].to_s())
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "nested_record_defaults.trb", source, "31\n32")
		})
	}
}

func TestQualifiedGenericNestedRecordConstructionAcrossBackends(t *testing.T) {
	source := []byte(`module Services
	record Box<T>
		value: T
	end

	record Empty
	end
end

def main()
	box := Services::Box<Integer>.new(value: 17)
	_empty := Services::Empty.new()
	puts(box.value.to_s())
	return
end
`)
	typeScript, err := Compile("nested_generic_record.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	if output := string(typeScript.Output); !strings.Contains(output, "const box: Services.Box<number>") {
		t.Fatalf("generated TypeScript record binding is not qualified:\n%s", output)
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "nested_generic_record.trb", source, "17")
		})
	}
}

func TestQualifiedNestedTypeAnnotationsPassTypeScriptCompiler(t *testing.T) {
	source := []byte(`module Services
	enum Status
		Ready
	end

	record Box<T>
		value: T
	end
end

def main()
	status := Services::Status::Ready
	box := Services::Box<String>.new(value: "ok")
	if status == Services::Status::Ready
		puts(box.value)
	end
	return
end
`)
	artifact, err := Compile("qualified_nested_types.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	for _, expected := range []string{
		"const status: Services.Status = Services.Status.Ready;",
		"const box: Services.Box<string> = ({value: \"ok\"} satisfies Services.Box<string>);",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated TypeScript is missing qualified annotation %q:\n%s", expected, output)
		}
	}
	checkTypeScriptArtifacts(t, []*Artifact{artifact}, "qualified_nested_types")
}

func TestNestedTypesStayQualifiedAtTypeScriptFunctionBoundaries(t *testing.T) {
	source := []byte(`module Services
	record Box<T>
		value: T
		label: String = "default"
	end
end

def passthrough(box: Box<String>): Box<String>
	return box
end

def main()
	box := if true
		passthrough(Services::Box<String>.new(value: "ok"))
	else
		passthrough(Services::Box<String>.new(value: "fallback"))
	end
	puts(box.value)
	return
end
`)
	artifact, err := Compile("qualified_function_boundary.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	if expected := "function passthrough(box: Services.Box<string>): Services.Box<string>"; !strings.Contains(output, expected) {
		t.Fatalf("generated TypeScript is missing qualified function boundary %q:\n%s", expected, output)
	}
	if expected := "((): Services.Box<string> =>"; !strings.Contains(output, expected) {
		t.Fatalf("generated TypeScript is missing qualified if-expression boundary %q:\n%s", expected, output)
	}
	checkTypeScriptArtifacts(t, []*Artifact{artifact}, "qualified_function_boundary")
}

func TestNestedTypesStayQualifiedInTypeScriptJSONCodecs(t *testing.T) {
	source := []byte(`import { JsonError, decode, encode } from trb/std/json
import { Result } from trb/std/result

module Services
	record Box
		value: String
	end

	enum Status
		Ready = "READY"
	end
end

def decode_box(source: String): Result<Box, JsonError>
	return decode<Box>(source)
end

def encode_box(box: Box): Result<String, JsonError>
	return encode(box)
end

def decode_status(source: String): Result<Status, JsonError>
	return decode<Status>(source)
end
`)
	artifacts, err := CompileProject([]SourceUnit{{
		Filename: "main.trb", ModulePath: "main", Package: "main", Source: source,
	}}, Options{Mode: "typescript", TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	artifact := artifactForModule(artifacts, "main")
	output := string(artifact.Output)
	for _, expected := range []string{
		"function decode_box(source: string): Result<Services.Box, JsonError>",
		"(value: Services.Box)",
		"Result.Ok<Services.Box, __trb_json.JsonError>",
		"case \"READY\": return Services.Status.Ready;",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated TypeScript JSON codec is missing qualified type %q:\n%s", expected, output)
		}
	}
	checkTypeScriptArtifacts(t, artifacts, "qualified_json_codecs")
}

func TestNestedTypesStayQualifiedInTypeScriptContextFetch(t *testing.T) {
	source := []byte(`import { Context, ContextKey, ContextValueError } from trb/web
import { Result } from trb/std/result

module Services
	record User
		name: String
	end
end

CURRENT_USER := ContextKey<User>.new("current_user")

def current_user(context: Context): Result<User, ContextValueError>
	updated := context.with(CURRENT_USER, Services::User.new(name: "Ada"))
	return updated.fetch(CURRENT_USER)
end
`)
	artifacts, err := CompileProject([]SourceUnit{{
		Filename: "main.trb", ModulePath: "main", Package: "main", Source: source,
	}}, Options{Mode: "typescript", TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifactForModule(artifacts, "main").Output)
	if expected := "Result<Services.User, ContextValueError>"; !strings.Contains(output, expected) {
		t.Fatalf("generated TypeScript context lookup is missing qualified type %q:\n%s", expected, output)
	}
	checkTypeScriptArtifacts(t, artifacts, "qualified_context_fetch")
}

func TestNestedPayloadEnumConstructionUsesQualifiedTypeScriptOwner(t *testing.T) {
	source := []byte(`module Services
	enum Event
		Value(value: String)
	end
end

def main()
	event := Services::Event::Value("ok")
	case event
	when Services::Event::Value(value)
		puts(value)
	end
	return
end
`)
	artifact, err := Compile("qualified_payload_enum.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	if expected := `const event: Services.Event = Services.Event.Value("ok");`; !strings.Contains(output, expected) {
		t.Fatalf("generated TypeScript is missing qualified payload enum construction %q:\n%s", expected, output)
	}
	checkTypeScriptArtifacts(t, []*Artifact{artifact}, "qualified_payload_enum")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "qualified_payload_enum.trb", source, "ok")
		})
	}
}

func TestRecordDefaultHelpersAvoidFieldNameCollisionsAcrossBackends(t *testing.T) {
	source := []byte(`record Collision
	trb_value_provided: Boolean
	args: String = "fallback"
	value: Integer = 7
	Object: String = "global"
end

record Pair
	a: Integer = 0
	b: Integer = 0
end

def trb_record_new_collision(): Integer
	return 2
end

def main()
	item := Collision.new(trb_value_provided: false)
	trbRecordArg1 := 9
	pair := Pair.new(a: 1, b: trbRecordArg1)
	puts(item.args)
	puts(item.value.to_s())
	puts(item.Object)
	puts(pair.b.to_s())
	puts(trb_record_new_collision().to_s())
	return
end
`)
	typeScript, err := Compile("record_helper_names.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	output := string(typeScript.Output)
	if !strings.Contains(output, "function __trbRecordNewCollision(__trbArgs:") {
		t.Fatalf("generated TypeScript record helper does not use a reserved argument name:\n%s", output)
	}
	checkTypeScriptArtifacts(t, []*Artifact{typeScript}, "record_helper_names")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "record_helper_names.trb", source, "fallback\n7\nglobal\n9\n2")
		})
	}
}

func TestGoRecordDefaultCallerDoesNotDependOnShadowableBuiltins(t *testing.T) {
	source := []byte(`record Config
	count: Integer = 1
end

def main()
	new := "shadow"
	config := Config.new()
	puts(new)
	puts(config.count.to_s())
	return
end
`)
	runEffectSource(t, "go", "record_default_builtin_shadow.trb", source, "shadow\n1")
}

func TestRecordDefaultHelpersPreserveDefinitionScopeAcrossBackends(t *testing.T) {
	source := []byte(`def value(): Integer
	return 1
end

def _default_value(): Integer
	return 1
end

record Config
	first: Integer = value()
	value: Integer = 2
end

record PrivateConfig
	first: Integer = _default_value()
	_default_value: Integer = 2
	_seed: Integer = 3
	copied: Integer = _seed
end

def main()
	item := Config.new()
	private_item := PrivateConfig.new()
	puts(item.first.to_s())
	puts(item.value.to_s())
	puts(private_item.first.to_s())
	puts(private_item.copied.to_s())
	return
end
`)
	typeScript, err := Compile("record_default_scope.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	output := string(typeScript.Output)
	if strings.Contains(output, "const value =") || !strings.Contains(output, "const __trbField1 =") {
		t.Fatalf("generated TypeScript record helper does not isolate field locals:\n%s", output)
	}
	checkTypeScriptArtifacts(t, []*Artifact{typeScript}, "record_default_scope")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "record_default_scope.trb", source, "1\n2\n1\n3")
		})
	}
}

func TestQualifiedRecordAnnotationsKeepSameLeafNamespacesSeparate(t *testing.T) {
	model := SourceUnit{
		Filename: "models/box.trb", ModulePath: "models/box", Package: "models",
		Source: []byte(`record Box<T>
	value: T
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import models/box as models

module Services
	record Box<T>
		value: T
	end
end

def main()
	local := Services::Box<String>.new(value: "local")
	remote := models::Box<String>.new(value: "remote")
	puts(local.value)
	puts(remote.value)
	return
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{model, main}, Options{
		Mode: "typescript", TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifactForModule(artifacts, "main").Output)
	for _, expected := range []string{"const local: Services.Box<string>", "const remote: models.Box<string>"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated TypeScript is missing distinct qualified annotation %q:\n%s", expected, output)
		}
	}
	checkTypeScriptArtifacts(t, artifacts, "qualified_record_namespaces")
}

func TestNamedImportedTypesTakePrecedenceOverNestedLeafTypesInTypeScript(t *testing.T) {
	model := SourceUnit{
		Filename: "models/box.trb", ModulePath: "models/box", Package: "models",
		Source: []byte(`record Box
	remote: String
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { Box } from models/box

module Services
	record Box
		local: String
	end
end

def read(box: Box): String
	return box.remote
end

def main()
	box := Box.new(remote: "ok")
	locals := [Services::Box.new(local: "local")]
	puts(read(box))
	puts(locals[0].local)
	return
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{model, main}, Options{
		Mode: "typescript", TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifactForModule(artifacts, "main").Output)
	for _, expected := range []string{
		"function read(box: Box): string",
		"const locals: Array<Services.Box>",
		"values: Array<Services.Box>",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated TypeScript did not preserve named-import type identity %q:\n%s", expected, output)
		}
	}
	checkTypeScriptArtifacts(t, artifacts, "named_import_nested_collision")
}

func TestTypeScriptExactNestedIdentitiesSurviveCompositeExpressions(t *testing.T) {
	model := SourceUnit{
		Filename: "models/box.trb", ModulePath: "models/box", Package: "models",
		Source: []byte(`record Box
	remote: String = "remote"
end

record Wrapper<T>
	value: T
	label: String = "wrapper"
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import models/box as models
import { Result } from trb/std/result

module Services
	record Box
		local: String = "local"
	end
end

enum Toggle
	On
	Off
end

def main()
	boxes := [models::Box.new()]
	mut mutable_boxes := [models::Box.new()]
	mutable_boxes = [models::Box.new()]
	by_name := {"remote" => models::Box.new()}
	combined := boxes.concat([models::Box.new()])
	merged := by_name.merge({"second" => models::Box.new()})
	_nullable: Box? := models::Box.new()
	first := boxes.first()
	mutable_first := mutable_boxes.first()
	mapped := boxes.map do |box|
		copy := box
		copy
	end
	concurrent := boxes.concurrent_map do |box|
		copy := box
		copy
	end
	found := boxes.find do |box|
		box.remote == "remote"
	end
	fetched := boxes.try_fetch(0)
	wrapped := Result<Box, String>::Ok(models::Box.new())
	failed := Result<String, Box>::Err(models::Box.new())
	generic := models::Wrapper<Box>.new(value: models::Box.new())
	conditional := if true
		models::Box.new()
	else
		models::Box.new()
	end
	conditional_local := if true
		inside := models::Box.new()
		inside
	else
		fallback := models::Box.new()
		fallback
	end
	selected := case Toggle::On
	when Toggle::On
		models::Box.new()
	when Toggle::Off
		models::Box.new()
	end
	selected_local := case Toggle::On
	when Toggle::On
		inside := models::Box.new()
		inside
	when Toggle::Off
		fallback := models::Box.new()
		fallback
	end
	selected_payload := case fetched
	when Result::Ok(value)
		copied := value
		copied
	when Result::Err(_error)
		models::Box.new()
	end
	boxes.each do |box|
		each_copy := box
		puts(each_copy.remote)
	end
	puts(boxes[0].remote)
	puts(by_name["remote"].remote)
	puts(combined[1].remote)
	puts(merged["second"].remote)
	puts(first.remote)
	puts(mutable_first.remote)
	puts(mapped[0].remote)
	puts(concurrent[0].remote)
	if found != nil
		puts(found.remote)
	end
	puts(conditional.remote)
	puts(conditional_local.remote)
	puts(selected.remote)
	puts(selected_local.remote)
	puts(selected_payload.remote)
	puts(generic.value.remote)
	case fetched
	when Result::Ok(value)
		statement_copy := value
		puts(statement_copy.remote)
	when Result::Err(error)
		puts(error.message)
	end
	case wrapped
	when Result::Ok(value)
		puts(value.remote)
	when Result::Err(error)
		puts(error)
	end
	case failed
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.remote)
	end
	return
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{model, main}, Options{
		Mode: "typescript", TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifactForModule(artifacts, "main").Output)
	for _, expected := range []string{
		"const boxes: Array<models.Box>",
		"const by_name: Record<string, models.Box>",
		"const combined: Array<models.Box>",
		"const merged: Record<string, models.Box>",
		"const _nullable: models.Box | null",
		"const first: models.Box",
		"const mutable_first: models.Box",
		"const mapped: Array<models.Box>",
		"const concurrent: Array<models.Box>",
		"const found: models.Box | null",
		"const fetched: Result<models.Box, IndexLookupError>",
		"const wrapped: Result<models.Box, string>",
		"Result.Ok<models.Box, string>",
		"const failed: Result<string, models.Box>",
		"Result.Err<string, models.Box>",
		"const generic: models.Wrapper<models.Box>",
		"models.__trbRecordNewWrapper<models.Box>",
		"const conditional: models.Box",
		"const conditional_local: models.Box",
		"const selected: models.Box",
		"const selected_local: models.Box",
		"const selected_payload: models.Box",
		"const copied: models.Box",
		"const each_copy: models.Box",
		"const statement_copy: models.Box",
		"values: Array<models.Box>",
		"values: Record<string, models.Box>",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated TypeScript lost exact nested identity %q:\n%s", expected, output)
		}
	}
	checkTypeScriptArtifacts(t, artifacts, "exact_nested_composites")
}

func TestGeneratedCodecTypesTakePrecedenceOverNestedLeafTypesInTypeScript(t *testing.T) {
	model := SourceUnit{
		Filename: "models/user.trb", ModulePath: "models/user", Package: "models",
		Source: []byte(`enum Status
	Remote = "REMOTE"
end

record User
	status: Status
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { User } from models/user
import { JsonError, decode } from trb/std/json
import { Result } from trb/std/result

module Services
	enum Status
		Local = "LOCAL"
	end
end

def decode_user(source: String): Result<User, JsonError>
	return decode<User>(source)
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{model, main}, Options{
		Mode: "typescript", TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifactForModule(artifacts, "main").Output)
	for _, expected := range []string{
		`import { Status } from "./models/user.ts";`,
		`case "REMOTE": return Status.Remote;`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated TypeScript codec did not preserve generated type identity %q:\n%s", expected, output)
		}
	}
	checkTypeScriptArtifacts(t, artifacts, "generated_codec_nested_collision")
}

func checkTypeScriptArtifacts(t *testing.T, artifacts []*Artifact, entry string) {
	t.Helper()
	tsc, err := exec.LookPath("tsc")
	if err != nil {
		t.Skip("tsc is not installed")
	}
	root := t.TempDir()
	paths := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		path := filepath.Join(root, filepath.FromSlash(artifact.IR.ModulePath)+".ts")
		writeCompilerRuntimeFile(t, path, artifact.Output)
		paths = append(paths, path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	arguments := []string{
		"--noEmit", "--pretty", "false", "--strict", "--skipLibCheck",
		"--target", "ES2022", "--module", "ESNext", "--moduleResolution", "Bundler",
		"--allowImportingTsExtensions",
	}
	arguments = append(arguments, paths...)
	command := exec.CommandContext(ctx, tsc, arguments...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("tsc rejected generated %s project: %v\n%s", entry, err, output)
	}
}

func TestNamespaceImportedRecordDefaultsUseHelpersAcrossBackends(t *testing.T) {
	model := SourceUnit{
		Filename: "models/config.trb", ModulePath: "models/config", Package: "models",
		Source: []byte(`def map_values(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		value
	end
end

record Config
	values: Array<Integer> = map_values([41])
end

record Required
	value: Integer
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import models/config as configs

def main()
	puts(configs::Config.new().values[0].to_s())
	puts(configs::Required.new(value: 42).value.to_s())
	return
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{model, main}, Options{
				Mode: mode, GoModule: "example.com/namespace-record-default", RubyLoader: "require_relative",
				TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := runEffectProject(t, mode, artifacts, "example.com/namespace-record-default"); got != "41\n42\n" {
				t.Fatalf("unexpected namespace-imported record output for %s: got %q, want %q", mode, got, "41\n42\n")
			}
		})
	}
}
