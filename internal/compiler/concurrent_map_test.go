package compiler

import (
	"context"
	goast "go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	gotypes "go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/nativepackage"
)

const concurrentMapSource = `def transform(values: Array<Integer>): Array<Integer>
	return values.concurrent_map(limit: 2) do |value|
		value * 2
	end
end
`

func TestConcurrentMapLowersAcrossBackends(t *testing.T) {
	artifacts := map[string]*Artifact{}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("concurrent_map.trb", []byte(concurrentMapSource), mode)
		if err != nil {
			t.Fatalf("%s compile failed: %v", mode, err)
		}
		artifacts[mode] = artifact
	}

	method := artifacts["go"].IR.Statements[0].(*ir.Method)
	returned := method.Body[0].(*ir.Return)
	transform, ok := returned.Value.(*ir.Transform)
	if !ok || transform.Operation != "concurrent_map" || transform.Limit == nil || transform.ExprType().String() != "Array<Integer>" {
		t.Fatalf("concurrent_map semantics were not retained in typed IR: %#v", returned.Value)
	}

	goOutput := string(artifacts["go"].Output)
	for _, expected := range []string{"sync.WaitGroup", `Value("type-rb/concurrency-group")`, "make([]int, len(", "__trbRequested"} {
		if !strings.Contains(goOutput, expected) {
			t.Fatalf("generated Go is missing %q:\n%s", expected, goOutput)
		}
	}
	if strings.Contains(goOutput, "__trbScope = trbcontext.WithValue") {
		t.Fatalf("generated Go leaked a completed root task group into later calls:\n%s", goOutput)
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "concurrent_map.go", artifacts["go"].Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("invalid generated Go: %v\n%s", err, goOutput)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", err, goOutput)
	}

	rubyOutput := string(artifacts["ruby"].Output)
	for _, expected := range []string{"Thread.new do", "SizedQueue.new", "Array.new", "concurrency_held"} {
		if !strings.Contains(rubyOutput, expected) {
			t.Fatalf("generated Ruby is missing %q:\n%s", expected, rubyOutput)
		}
	}
	if strings.Contains(rubyOutput, "__trb_scope.concurrency_group =") {
		t.Fatalf("generated Ruby leaked a completed root task group into later calls:\n%s", rubyOutput)
	}

	typeScriptOutput := string(artifacts["typescript"].Output)
	for _, expected := range []string{"export async function transform", "Array.from({ length:", "__trbConcurrencyGroup", "await Promise.all"} {
		if !strings.Contains(typeScriptOutput, expected) {
			t.Fatalf("generated TypeScript is missing %q:\n%s", expected, typeScriptOutput)
		}
	}
}

func TestGeneratedConcurrentMapRunsAndNestedMapsShareCapacity(t *testing.T) {
	source := []byte(`def main()
	results := [1, 2].concurrent_map(limit: 2) do |outer|
		[1, 2].concurrent_map(limit: 2) do |inner|
			outer * 10 + inner
		end
	end
	puts(results[0][0])
	puts(results[0][1])
	puts(results[1][0])
	puts(results[1][1])
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "ruby" {
				if _, err := exec.LookPath("ruby"); err != nil {
					t.Skip("ruby is not installed")
				}
			}
			if mode == "typescript" {
				if _, err := exec.LookPath("bun"); err != nil {
					t.Skip("bun is not installed")
				}
			}
			artifact, err := Compile("concurrent_map.trb", source, mode)
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			var command *exec.Cmd
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			switch mode {
			case "go":
				writeCompilerRuntimeFile(t, filepath.Join(root, "main.go"), artifact.Output)
				writeCompilerRuntimeFile(t, filepath.Join(root, "go.mod"), []byte("module example.com/concurrent-map-test\n\ngo 1.27\n"))
				command = exec.CommandContext(ctx, "go", "run", ".")
				command.Env = append(os.Environ(), "GOCACHE="+filepath.Join(root, "go-cache"))
			case "ruby":
				writeCompilerRuntimeFile(t, filepath.Join(root, "main.rb"), artifact.Output)
				command = exec.CommandContext(ctx, "ruby", "main.rb")
			case "typescript":
				writeCompilerRuntimeFile(t, filepath.Join(root, "main.ts"), artifact.Output)
				command = exec.CommandContext(ctx, "bun", "run", "main.ts")
			}
			command.Dir = root
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("generated %s concurrent_map failed: %v\n%s\n%s", mode, err, output, artifact.Output)
			}
			if got := strings.TrimSpace(string(output)); got != "11\n12\n21\n22" {
				t.Fatalf("unexpected %s output: %q", mode, got)
			}
		})
	}
}

func TestConcurrentMapEnforcesExplicitAndDefaultCapacity(t *testing.T) {
	tests := []struct {
		name      string
		receiver  string
		arguments string
		wantMax   string
	}{
		{name: "explicit", receiver: "[1, 2, 3, 4]", arguments: "(limit: 2)", wantMax: "2"},
		{name: "default", receiver: "[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]", wantMax: "8"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				if mode == "ruby" {
					if _, err := exec.LookPath("ruby"); err != nil {
						t.Skip("ruby is not installed")
					}
				}
				if mode == "typescript" {
					if _, err := exec.LookPath("bun"); err != nil {
						t.Skip("bun is not installed")
					}
				}
				source := []byte("import { probe, maximum } from github.com/acme/concurrency/probe\n\n" +
					"def main()\n" +
					"\tresults := " + test.receiver + ".concurrent_map" + test.arguments + " do |value|\n" +
					"\t\tprobe(value)\n" +
					"\tend\n" +
					"\tputs(results[0])\n" +
					"\tputs(results[results.size() - 1])\n" +
					"\tputs(maximum())\n" +
					"\treturn\nend\n")
				options := Options{Mode: mode, ModulePath: "main", NativePackages: concurrencyProbeCatalog(mode)}
				if mode == "go" {
					options.Package = "main"
					options.GoModule = "example.com/concurrent-map-app"
				}
				if mode == "typescript" {
					options.TypeScriptRuntime = "bun"
				}
				artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: options.Package, Source: source}}, options)
				if err != nil {
					t.Fatal(err)
				}
				output := runConcurrentProbeArtifact(t, mode, artifacts[0].Output)
				lines := strings.Split(strings.TrimSpace(output), "\n")
				if len(lines) != 3 || lines[0] != "1" || lines[2] != test.wantMax {
					t.Fatalf("unexpected %s concurrent capacity output: %q", mode, output)
				}
			})
		}
	}
}

func concurrencyProbeCatalog(mode string) *nativepackage.Catalog {
	dependency, module, probe, maximum := "", "", "", ""
	switch mode {
	case "go":
		dependency, module, probe, maximum = "example.com/concurrency-probe", "example.com/concurrency-probe", "Probe", "Maximum"
	case "ruby":
		dependency, module, probe, maximum = "acme-concurrency-probe", "acme/concurrency_probe", "Acme::ConcurrencyProbe.probe", "Acme::ConcurrencyProbe.maximum"
	default:
		dependency, module, probe, maximum = "@acme/concurrency-probe", "@acme/concurrency-probe", "probe", "maximum"
	}
	integer := nativepackage.Type{Kind: "int", Name: "Integer"}
	return &nativepackage.Catalog{
		FormatVersion: nativepackage.FormatVersion,
		Dependencies:  map[string]string{dependency: "1.0.0"},
		Modules: map[string]nativepackage.Module{
			"github.com/acme/concurrency/probe": {Exports: map[string]nativepackage.Export{
				"probe": {
					Kind: "function", Type: integer, Parameters: []nativepackage.Type{integer}, Required: 1,
					Runtime: &nativepackage.RuntimeBinding{
						Identity: "github.com/acme/concurrency/probe#probe", Dependency: dependency, Module: module, Symbol: probe,
						CallConvention: "function", MaySuspend: true, PropagatesExecutionScope: true,
					},
				},
				"maximum": {
					Kind: "function", Type: integer,
					Runtime: &nativepackage.RuntimeBinding{
						Identity: "github.com/acme/concurrency/probe#maximum", Dependency: dependency, Module: module, Symbol: maximum, CallConvention: "function",
					},
				},
			}},
		},
	}
}

func runConcurrentProbeArtifact(t *testing.T, mode string, source []byte) string {
	t.Helper()
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var command *exec.Cmd
	switch mode {
	case "go":
		writeCompilerRuntimeFile(t, filepath.Join(root, "main.go"), source)
		writeCompilerRuntimeFile(t, filepath.Join(root, "go.mod"), []byte("module example.com/concurrent-map-app\n\ngo 1.27\n\nrequire example.com/concurrency-probe v0.0.0\n\nreplace example.com/concurrency-probe => ./concurrency-probe\n"))
		writeCompilerRuntimeFile(t, filepath.Join(root, "concurrency-probe", "go.mod"), []byte("module example.com/concurrency-probe\n\ngo 1.27\n"))
		writeCompilerRuntimeFile(t, filepath.Join(root, "concurrency-probe", "probe.go"), []byte(`package concurrencyprobe

import (
	"context"
	"sync/atomic"
	"time"
)

var current atomic.Int64
var maximum atomic.Int64

func Probe(scope context.Context, value int) int {
	active := current.Add(1)
	for observed := maximum.Load(); active > observed && !maximum.CompareAndSwap(observed, active); observed = maximum.Load() {}
	defer current.Add(-1)
	select {
	case <-time.After(25 * time.Millisecond):
	case <-scope.Done():
		panic(scope.Err())
	}
	return value
}

func Maximum() int { return int(maximum.Load()) }
`))
		command = exec.CommandContext(ctx, "go", "run", ".")
		command.Env = append(os.Environ(), "GOCACHE=/tmp/type-rb-go-cache")
	case "ruby":
		writeCompilerRuntimeFile(t, filepath.Join(root, "main.rb"), source)
		nativeRoot := filepath.Join(root, "native")
		writeCompilerRuntimeFile(t, filepath.Join(nativeRoot, "acme", "concurrency_probe.rb"), []byte(`module Acme
  module ConcurrencyProbe
    @mutex = Mutex.new
    @current = 0
    @maximum = 0
    def self.probe(scope, value)
      scope.check!
      @mutex.synchronize do
        @current += 1
        @maximum = [@maximum, @current].max
      end
      begin
        sleep(0.025)
        scope.check!
        value
      ensure
        @mutex.synchronize { @current -= 1 }
      end
    end
    def self.maximum
      @mutex.synchronize { @maximum }
    end
  end
end
`))
		command = exec.CommandContext(ctx, "ruby", "-I", nativeRoot, "main.rb")
	case "typescript":
		writeCompilerRuntimeFile(t, filepath.Join(root, "main.ts"), source)
		moduleRoot := filepath.Join(root, "node_modules", "@acme", "concurrency-probe")
		writeCompilerRuntimeFile(t, filepath.Join(moduleRoot, "package.json"), []byte(`{"name":"@acme/concurrency-probe","type":"module","exports":"./index.ts"}`))
		writeCompilerRuntimeFile(t, filepath.Join(moduleRoot, "index.ts"), []byte(`let current = 0;
let observedMaximum = 0;
export async function probe(scope: AbortSignal | undefined, value: number): Promise<number> {
  if (scope?.aborted) throw new DOMException("cancelled", "AbortError");
  current += 1;
  observedMaximum = Math.max(observedMaximum, current);
  try {
    await Bun.sleep(25);
    if (scope?.aborted) throw new DOMException("cancelled", "AbortError");
    return value;
  } finally {
    current -= 1;
  }
}
export function maximum(): number { return observedMaximum; }
`))
		command = exec.CommandContext(ctx, "bun", "run", "main.ts")
	}
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated %s concurrency probe failed: %v\n%s\n%s", mode, err, output, source)
	}
	return string(output)
}

func TestConcurrentMapUsesImportFreeDefaultLimit(t *testing.T) {
	source := []byte(`def transform(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		value
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("concurrent_map.trb", source, mode)
		if err != nil {
			t.Fatalf("%s compile failed: %v", mode, err)
		}
		if strings.Contains(string(artifact.Output), "trb/std/concurrent") {
			t.Fatalf("%s generated an import for concurrent_map:\n%s", mode, artifact.Output)
		}
		method := artifact.IR.Statements[0].(*ir.Method)
		transform := method.Body[0].(*ir.Return).Value.(*ir.Transform)
		if transform.Limit != nil {
			t.Fatalf("default limit should stay implicit in IR: %#v", transform.Limit)
		}
	}
}

func TestConcurrentMapDoesNotSpecialCaseResultValues(t *testing.T) {
	source := []byte(`import { Result } from trb/std/result

def transform(values: Array<Integer>): Array<Result<Integer, String>>
	return values.concurrent_map do |value|
		Result<Integer, String>::Ok(value)
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("concurrent_result_map.trb", source, mode)
		if err != nil {
			t.Fatalf("%s compile failed: %v", mode, err)
		}
		method := artifact.IR.Statements[1].(*ir.Method)
		transform := method.Body[0].(*ir.Return).Value.(*ir.Transform)
		if got, want := transform.ExprType().String(), "Array<Result<Integer, String>>"; got != want {
			t.Fatalf("%s concurrent Result map type=%s, want %s", mode, got, want)
		}
	}
}

func TestConcurrentMapDiagnosticsArePortable(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "non array receiver",
			source: `def transform(values: Iterable<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		value
	end
end`,
			message: "concurrent_map is available only on Array",
		},
		{
			name: "non positive literal limit",
			source: `def transform(values: Array<Integer>): Array<Integer>
	return values.concurrent_map(limit: 0) do |value|
		value
	end
end`,
			message: "concurrent_map limit must be greater than zero",
		},
		{
			name: "positional limit",
			source: `def transform(values: Array<Integer>): Array<Integer>
	return values.concurrent_map(2) do |value|
		value
	end
end`,
			message: "concurrent_map accepts only the named argument limit",
		},
		{
			name: "outer assignment",
			source: `def transform(values: Array<Integer>): Array<Integer>
	mut total := 0
	return values.concurrent_map do |value|
		total += value
		total
	end
end`,
			message: "concurrent_map cannot assign to outer binding total",
		},
		{
			name: "unsafe capture",
			source: `def transform(values: Array<Integer>): Array<Integer>
	offsets := [1]
	return values.concurrent_map do |value|
		value + offsets[0]
	end
end`,
			message: "offsets because Array<Integer> is not concurrency-safe",
		},
		{
			name: "interface dispatch has no safety contract",
			source: `interface Counter
	add(): Integer
end

def transform(values: Array<Counter>): Array<Integer>
	return values.concurrent_map do |counter|
		counter.add()
	end
end`,
			message: "concurrent_map cannot call interface method add without an explicit concurrency-safety contract",
		},
		{
			name: "function value has no safety contract",
			source: `alias Mapper = (Integer) -> Integer

def transform(values: Array<Mapper>): Array<Integer>
	return values.concurrent_map do |callable|
		callable(1)
	end
end`,
			message: "concurrent_map cannot call a function value without an explicit concurrency-safety contract",
		},
		{
			name: "helper reaches shared mutation",
			source: `mut shared := 0

def mutate(value: Integer): Integer
	shared += value
	return shared
end

def transform(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		mutate(value)
	end
end`,
			message: "concurrent_map cannot assign to outer binding shared",
		},
		{
			name: "class method reaches shared mutation",
			source: `mut shared := 0

class Counter
	def self.add(value: Integer): Integer
		shared += value
		return shared
	end
end

def transform(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		Counter.add(value)
	end
end`,
			message: "concurrent_map cannot assign to outer binding shared",
		},
		{
			name: "module method reaches shared mutation",
			source: `mut shared := 0

module Counter
	def self.add(value: Integer): Integer
		shared += value
		return shared
	end
end

def transform(values: Array<Integer>): Array<Any>
	return values.concurrent_map do |value|
		Counter.add(value)
	end
end`,
			message: "concurrent_map cannot assign to outer binding shared",
		},
		{
			name: "instance method reaches shared mutation",
			source: `mut shared := 0

class Counter
	def add(): Integer
		shared += 1
		return shared
	end
end

def transform(values: Array<Counter>): Array<Integer>
	return values.concurrent_map do |counter|
		counter.add()
	end
end`,
			message: "concurrent_map cannot assign to outer binding shared",
		},
		{
			name: "constructor reaches shared mutation",
			source: `mut shared := 0

class Counter
	@value: Integer

	def initialize(value: Integer)
		@value = value
		shared += value
		return
	end
end

def transform(values: Array<Integer>): Array<Counter>
	return values.concurrent_map do |value|
		Counter.new(value)
	end
end`,
			message: "concurrent_map cannot assign to outer binding shared",
		},
		{
			name: "class field default reaches shared mutation",
			source: `mut shared := 0

def bump(): Integer
	shared += 1
	return shared
end

class Counter
	@value: Integer := bump()

	def initialize()
		return
	end
end

def transform(values: Array<Integer>): Array<Counter>
	return values.concurrent_map do |_value|
		Counter.new()
	end
end`,
			message: "concurrent_map cannot assign to outer binding shared",
		},
		{
			name: "constructor parameter default captures unsafe value",
			source: `mut shared := [1]

class Counter
	@values: Array<Integer>

	def initialize(values: Array<Integer> = shared)
		@values = values
		return
	end
end

def transform(values: Array<Integer>): Array<Counter>
	return values.concurrent_map do |_value|
		Counter.new()
	end
end`,
			message: "shared because Array<Integer> is not concurrency-safe",
		},
		{
			name: "implicit constructor field default reaches shared mutation through factory",
			source: `mut shared := 0

def bump(): Integer
	shared += 1
	return shared
end

class Counter
	@value: Integer := bump()
end

def build_counter(): Counter
	return Counter.new()
end

def transform(values: Array<Integer>): Array<Counter>
	return values.concurrent_map do |_value|
		build_counter()
	end
end`,
			message: "concurrent_map cannot assign to outer binding shared",
		},
		{
			name: "transitive class method reaches shared mutation",
			source: `mut shared := 0

class Counter
	def self.add(value: Integer): Integer
		shared += value
		return shared
	end
end

def relay(value: Integer): Integer
	return Counter.add(value)
end

def transform(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		relay(value)
	end
end`,
			message: "concurrent_map cannot assign to outer binding shared",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				_, err := Compile("bad_concurrent_map.trb", []byte(test.source), mode)
				if err == nil || !strings.Contains(err.Error(), test.message) {
					t.Fatalf("%s error = %v, want message containing %q", mode, err, test.message)
				}
			}
		})
	}
}

func TestConcurrentMapRejectsAliasedArrayElementInstanceMutation(t *testing.T) {
	source := []byte(`class Counter
	@value: Integer

	def initialize(value: Integer)
		@value = value
		return
	end

	def increment(): Integer
		@value += 1
		return @value
	end
end

	def transform(counter: Counter): Array<Integer>
		values := [counter, counter]
		return values.concurrent_map do |counter|
			counter.increment()
		end
	end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("aliased_concurrent_counter.trb", source, mode)
		if err == nil || !strings.Contains(err.Error(), "concurrent_map cannot assign to outer binding @value") {
			t.Fatalf("%s error = %v, want aliased instance mutation rejection", mode, err)
		}
	}
}

func TestConcurrentMapAllowsConstructorFieldInitialization(t *testing.T) {
	source := []byte(`class Counter
	@value: Integer

	def initialize(value: Integer)
		@value = value
		return
	end
end

def build_counter(value: Integer): Counter
	return Counter.new(value)
end

def transform(values: Array<Integer>): Array<Counter>
	return values.concurrent_map do |value|
		build_counter(value)
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("concurrent_constructor.trb", source, mode); err != nil {
			t.Fatalf("%s rejected constructor field initialization: %v", mode, err)
		}
	}
}

func TestConcurrentMapAllowsSafeClassFieldDefault(t *testing.T) {
	source := []byte(`def initial_value(): Integer
	return 1
end

class Counter
	@value: Integer := initial_value()
end

def build_counter(): Counter
	return Counter.new()
end

def transform(values: Array<Integer>): Array<Counter>
	return values.concurrent_map do |_value|
		build_counter()
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("safe_concurrent_field_default.trb", source, mode); err != nil {
			t.Fatalf("%s rejected concurrency-safe class field default: %v", mode, err)
		}
	}
}

func TestConcurrentMapDoesNotTreatDirectInitializeCallAsConstruction(t *testing.T) {
	source := []byte(`class Counter
	@value: Integer

	def initialize(value: Integer)
		@value = value
		return
	end
end

def transform(values: Array<Counter>): Array<Integer>
	return values.concurrent_map do |counter|
		counter.initialize(1)
		0
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("direct_concurrent_initialize.trb", source, mode)
		if err == nil || !strings.Contains(err.Error(), "concurrent_map cannot assign to outer binding @value") {
			t.Fatalf("%s error = %v, want direct initialize mutation rejection", mode, err)
		}
	}
}

func TestConcurrentMapRejectsBorrowedContainerMutation(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{
			name: "array through helper",
			source: `def mutate(items: Array<Integer>): Integer
	items[0] = items[0] + 1
	return items[0]
end

def transform(values: Array<Array<Integer>>): Array<Integer>
	return values.concurrent_map do |items|
		mutate(items)
	end
end`,
			message: "concurrent_map cannot mutate borrowed binding items because Array<Integer> is not uniquely owned",
		},
		{
			name: "hash directly",
			source: `def transform(values: Array<Hash<String, Integer>>): Array<Integer>
	return values.concurrent_map do |items|
		items["count"] = 1
		1
	end
end`,
			message: "concurrent_map cannot mutate borrowed binding items because Hash<String, Integer> is not uniquely owned",
		},
		{
			name: "string builder through helper",
			source: `import trb/std/string_builder

def mutate(builder: StringBuilder): String
	builder.append("x")
	return builder.to_s()
end

def transform(values: Array<StringBuilder>): Array<String>
	return values.concurrent_map do |builder|
		mutate(builder)
	end
end`,
			message: "concurrent_map cannot mutate borrowed binding builder because StringBuilder is not uniquely owned",
		},
		{
			name: "local alias of array element",
			source: `def transform(values: Array<Array<Integer>>): Array<Integer>
	return values.concurrent_map do |items|
		mut alias_items := items
		alias_items[0] = alias_items[0] + 1
		alias_items[0]
	end
end`,
			message: "concurrent_map cannot mutate borrowed binding alias_items because Array<Integer> is not uniquely owned",
		},
		{
			name: "nested iteration element",
			source: `def transform(values: Array<Array<Array<Integer>>>): Array<Integer>
	return values.concurrent_map do |items|
		items.each do |entry|
			entry[0] = entry[0] + 1
		end
		0
	end
end`,
			message: "concurrent_map cannot mutate borrowed binding entry because Array<Integer> is not uniquely owned",
		},
		{
			name: "borrowed element wrapped in fresh array",
			source: `def transform(values: Array<Array<Integer>>): Array<Integer>
	return values.concurrent_map do |items|
		mut aliases := [items]
		aliases[0][0] = aliases[0][0] + 1
		aliases[0][0]
	end
end`,
			message: "concurrent_map cannot mutate borrowed binding aliases because Array<Array<Integer>> is not uniquely owned",
		},
		{
			name: "borrowed element selected by conditional",
			source: `def transform(values: Array<Array<Integer>>): Array<Integer>
	return values.concurrent_map do |items|
		mut selected := if true
			items
		else
			[0]
		end
		selected[0] = selected[0] + 1
		selected[0]
	end
end`,
			message: "concurrent_map cannot mutate borrowed binding selected because Array<Integer> is not uniquely owned",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				_, err := Compile("borrowed_concurrent_container.trb", []byte(test.source), mode)
				if err == nil || !strings.Contains(err.Error(), test.message) {
					t.Fatalf("%s error = %v, want message containing %q", mode, err, test.message)
				}
			}
		})
	}
}

func TestConcurrentMapAllowsBorrowedContainerReadsAndTaskOwnedMutation(t *testing.T) {
	source := []byte(`def read_first(items: Array<Integer>): Integer
	return items[0]
end

def read_rows(values: Array<Array<Integer>>): Array<Integer>
	return values.concurrent_map do |items|
		read_first(items)
	end
end

def mutate_fresh(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		mut items := [value]
		items[0] = items[0] + 1
		items[0]
	end
end

def mutate_nested_fresh(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		mut rows := [[value]]
		rows.each do |items|
			items[0] = items[0] + 1
		end
		rows[0][0]
	end
end

def mutate_conditional_fresh(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		mut items := if value > 0
			[value]
		else
			[0]
		end
		items[0] = items[0] + 1
		items[0]
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("safe_concurrent_containers.trb", source, mode); err != nil {
			t.Fatalf("%s rejected borrowed reads or task-owned mutation: %v", mode, err)
		}
	}
}
