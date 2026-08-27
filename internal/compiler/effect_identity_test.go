package compiler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassAndInstanceMethodsDoNotShareEffectABI(t *testing.T) {
	source := []byte(`class Base
	def self.values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end

class Child < Base
	def values(input: Array<Integer>): Array<Integer>
		return input
	end
end

module Settings
	VALUES := Child.new().values([2])
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := Compile("member_effect_identity.trb", source, mode)
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifact.Output)
			switch mode {
			case "go":
				if strings.Contains(output, "func (self *Child) Values(__trbScope") || strings.Contains(output, "NewChild().Values(trbcontext.Background()") {
					t.Fatalf("pure instance method received the class-method effect ABI:\n%s", output)
				}
			case "ruby":
				if strings.Contains(output, "def values(__trb_scope") || strings.Contains(output, "Child.new().values(TrbExecutionScope.root") {
					t.Fatalf("pure instance method received the class-method effect ABI:\n%s", output)
				}
			}
		})
	}
}

func TestPrivateClassesInDifferentModulesDoNotShareEffectABI(t *testing.T) {
	effectful := SourceUnit{
		Filename: "effectful.trb", ModulePath: "services/effectful",
		Source: []byte(`class _Box
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end
`),
	}
	pure := SourceUnit{
		Filename: "pure.trb", ModulePath: "services/pure",
		Source: []byte(`class _Box
	def values(input: Array<Integer>): Array<Integer>
		return input
	end
end

module Settings
	VALUES := _Box.new().values([2])
end
`),
	}
	assertPureEffectIdentityModule(t, []SourceUnit{effectful, pure}, "services/pure")
}

func TestImportedSuperclassKeepsClassAndInstanceMethodEffectsSeparate(t *testing.T) {
	base := SourceUnit{
		Filename: "base.trb", ModulePath: "models/base",
		Source: []byte(`class Base
	def self.values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end
`),
	}
	child := SourceUnit{
		Filename: "child.trb", ModulePath: "models/child",
		Source: []byte(`import { Base } from models/base

class Child < Base
	def values(input: Array<Integer>): Array<Integer>
		return input
	end
end

module Settings
	VALUES := Child.new().values([2])
end
`),
	}
	assertPureEffectIdentityModule(t, []SourceUnit{base, child}, "models/child")
}

func TestPrivateInterfacesInDifferentModulesDoNotShareEffectABI(t *testing.T) {
	effectful := SourceUnit{
		Filename: "effectful.trb", ModulePath: "services/effectful",
		Source: []byte(`interface _Worker
	values(input: Array<Integer>): Array<Integer>
end

class EffectWorker implements _Worker
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end
`),
	}
	pure := SourceUnit{
		Filename: "pure.trb", ModulePath: "services/pure",
		Source: []byte(`interface _Worker
	values(input: Array<Integer>): Array<Integer>
end

class PureWorker implements _Worker
	def values(input: Array<Integer>): Array<Integer>
		return input
	end
end

module Settings
	VALUES := PureWorker.new().values([2])
end
`),
	}
	assertPureEffectIdentityModule(t, []SourceUnit{effectful, pure}, "services/pure")
}

func TestImportedInterfaceSharesEffectABIWithItsImplementations(t *testing.T) {
	contract := SourceUnit{
		Filename: "worker.trb", ModulePath: "contracts/worker",
		Source: []byte(`interface Worker
	values(input: Array<Integer>): Array<Integer>
end
`),
	}
	effectful := SourceUnit{
		Filename: "effectful.trb", ModulePath: "services/effectful",
		Source: []byte(`import { Worker } from contracts/worker

class EffectWorker implements Worker
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end
`),
	}
	pure := SourceUnit{
		Filename: "pure.trb", ModulePath: "services/pure",
		Source: []byte(`import { Worker } from contracts/worker

class PureWorker implements Worker
	def values(input: Array<Integer>): Array<Integer>
		return input
	end
end

def main()
	puts(PureWorker.new().values([2])[0].to_s())
	return
end
`),
	}
	wants := map[string]string{
		"go":         "func (self *PureWorker) Values(__trbScope trbcontext.Context",
		"ruby":       "def values(__trb_scope, input)",
		"typescript": "async values(__trbScope: AbortSignal | undefined",
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{contract, effectful, pure}, Options{
				Mode: mode, GoModule: "example.com/imported-interface-effect", RubyLoader: "require_relative",
				SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifactForModule(artifacts, "services/pure").Output)
			if !strings.Contains(output, wants[mode]) {
				t.Fatalf("generated %s implementation is missing imported interface effect ABI %q:\n%s", mode, wants[mode], output)
			}
		})
	}
}

func TestImportedGenericInterfaceAliasSharesEffectABIAcrossBackends(t *testing.T) {
	contract := SourceUnit{
		Filename: "contracts/worker.trb", ModulePath: "contracts/worker", Package: "contracts",
		Source: []byte(`interface Worker<T>
	values(input: Array<T>): Array<T>
end
`),
	}
	aliases := SourceUnit{
		Filename: "contracts/alias.trb", ModulePath: "contracts/alias", Package: "contracts",
		Source: []byte(`import { Worker } from contracts/worker

alias WorkerAlias<T> = Worker<T>
alias WorkerChain<T> = WorkerAlias<T>
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { WorkerChain } from contracts/alias

class EffectWorker implements WorkerChain<Integer>
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end

def run_worker(worker: WorkerChain<Integer>): Array<Integer>
	return worker.values([4])
end

def main()
	puts(run_worker(EffectWorker.new())[0].to_s())
	return
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{contract, aliases, main}, Options{
				Mode: mode, GoModule: "example.com/interface-alias-effect", RubyLoader: "require_relative",
				TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/interface-alias-effect")); got != "4" {
				t.Fatalf("unexpected imported interface alias output for %s: got %q, want %q", mode, got, "4")
			}
		})
	}
}

func TestImportedAliasReturnDispatchSharesEffectABIAcrossBackends(t *testing.T) {
	contract := SourceUnit{
		Filename: "contracts/worker.trb", ModulePath: "contracts/worker", Package: "contracts",
		Source: []byte(`interface Worker
	values(input: Array<Integer>): Array<Integer>
end

alias WorkerAlias = Worker

class EffectWorker implements WorkerAlias
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end

def build_worker(): WorkerAlias
	return EffectWorker.new()
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { build_worker } from contracts/worker

module Services
	interface Worker
		local_value(): Integer
	end
end

def main()
	puts(build_worker().values([7])[0].to_s())
	return
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{contract, main}, Options{
				Mode: mode, GoModule: "example.com/imported-alias-return-effect", RubyLoader: "require_relative",
				TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/imported-alias-return-effect")); got != "7" {
				t.Fatalf("unexpected imported alias return output for %s: got %q, want %q", mode, got, "7")
			}
		})
	}
}

func TestImportedGenericIdentityReturnDispatchSharesEffectABIInRubyAndTypeScript(t *testing.T) {
	contract := SourceUnit{
		Filename: "contracts/worker.trb", ModulePath: "contracts/worker", Package: "contracts",
		Source: []byte(`interface Worker
	values(input: Array<Integer>): Array<Integer>
end

alias Identity<T> = T

class EffectWorker implements Identity<Worker>
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end

def build_worker(): Identity<Worker>
	return EffectWorker.new()
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { build_worker } from contracts/worker

def main()
	puts(build_worker().values([17])[0].to_s())
	return
end
`),
	}
	for _, mode := range []string{"ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{contract, main}, Options{
				Mode: mode, RubyLoader: "require_relative", TypeScriptRuntime: "bun",
				SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/imported-generic-alias-return-effect")); got != "17" {
				t.Fatalf("unexpected imported generic alias return output for %s: got %q, want %q", mode, got, "17")
			}
		})
	}
}

func TestLocalInterfaceAliasSharesEffectABIAcrossBackends(t *testing.T) {
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`interface Worker
	values(input: Array<Integer>): Array<Integer>
end

alias WorkerAlias = Worker

class EffectWorker implements WorkerAlias
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end

def run_worker(worker: WorkerAlias): Array<Integer>
	return worker.values([4])
end

def main()
	puts(run_worker(EffectWorker.new())[0].to_s())
	return
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{main}, Options{
				Mode: mode, GoModule: "example.com/local-interface-alias-effect", RubyLoader: "require_relative",
				TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/local-interface-alias-effect")); got != "4" {
				t.Fatalf("unexpected local interface alias output for %s: got %q, want %q", mode, got, "4")
			}
		})
	}
}

func TestGenericIdentityAliasSharesEffectABIInRubyAndTypeScript(t *testing.T) {
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`interface Worker
	values(input: Array<Integer>): Array<Integer>
end

alias Identity<T> = T

class EffectWorker implements Identity<Worker>
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end

def run_worker(worker: Identity<Worker>): Array<Integer>
	return worker.values([8])
end

def main()
	puts(run_worker(EffectWorker.new())[0].to_s())
	return
end
`),
	}
	for _, mode := range []string{"ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{main}, Options{
				Mode: mode, GoModule: "example.com/generic-identity-alias-effect", RubyLoader: "require_relative",
				TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/generic-identity-alias-effect")); got != "8" {
				t.Fatalf("unexpected generic identity alias output for %s: got %q, want %q", mode, got, "8")
			}
		})
	}
}

func TestLocalGenericAliasReapplicationFollowsImportedTypeArgumentInRubyAndTypeScript(t *testing.T) {
	contract := SourceUnit{
		Filename: "contracts/worker.trb", ModulePath: "contracts/worker", Package: "contracts",
		Source: []byte(`interface Worker
	values(input: Array<Integer>): Array<Integer>
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { Worker } from contracts/worker

alias Identity<T> = T
alias Wrapper<T> = Identity<T>

class EffectWorker implements Wrapper<Wrapper<Worker>>
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end

def run_worker(worker: Wrapper<Wrapper<Worker>>): Array<Integer>
	return worker.values([11])
end

def main()
	puts(run_worker(EffectWorker.new())[0].to_s())
	return
end
`),
	}
	for _, mode := range []string{"ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{contract, main}, Options{
				Mode: mode, RubyLoader: "require_relative", TypeScriptRuntime: "bun",
				SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/local-identity-import-effect")); got != "11" {
				t.Fatalf("unexpected local identity alias output for %s: got %q, want %q", mode, got, "11")
			}
		})
	}
}

func TestImportedIdentityAliasFollowsItsLocalTypeArgument(t *testing.T) {
	identity := SourceUnit{
		Filename: "contracts/identity.trb", ModulePath: "contracts/identity", Package: "contracts",
		Source: []byte(`alias Identity<T> = T
`),
	}
	unrelated := SourceUnit{
		Filename: "other/worker.trb", ModulePath: "other/worker", Package: "other",
		Source: []byte(`class Worker
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { Identity } from contracts/identity
import { Worker } from other/worker

module Services
	interface Worker
		values(input: Array<Integer>): Array<Integer>
	end

	class EffectWorker implements Identity<Worker>
		def values(input: Array<Integer>): Array<Integer>
			return input.concurrent_map do |value|
				value
			end
		end
	end
end
`),
	}
	if _, err := CompileProject([]SourceUnit{identity, unrelated, main}, Options{
		Mode: "ruby", SourceRoot: "/project", ProjectRoot: "/project", AllowUnusedImports: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNestedImportedIdentityAliasSharesEffectABIInRubyAndTypeScript(t *testing.T) {
	contract := SourceUnit{
		Filename: "contracts/worker.trb", ModulePath: "contracts/worker", Package: "contracts",
		Source: []byte(`interface Worker
	values(input: Array<Integer>): Array<Integer>
end

alias Identity<T> = T
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { Identity, Worker } from contracts/worker

class EffectWorker implements Identity<Identity<Worker>>
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end

def run_worker(worker: Identity<Identity<Worker>>): Array<Integer>
	return worker.values([12])
end

def main()
	puts(run_worker(EffectWorker.new())[0].to_s())
	return
end
`),
	}
	for _, mode := range []string{"ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{contract, main}, Options{
				Mode: mode, RubyLoader: "require_relative", TypeScriptRuntime: "bun",
				SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/nested-imported-identity-effect")); got != "12" {
				t.Fatalf("unexpected nested imported identity alias output for %s: got %q, want %q", mode, got, "12")
			}
		})
	}
}

func TestAliasTargetProvenanceDistinguishesLexicalAndImportedTypes(t *testing.T) {
	t.Run("lexical target wins over unrelated imported type", func(t *testing.T) {
		other := SourceUnit{
			Filename: "other/worker.trb", ModulePath: "other/worker", Package: "other",
			Source: []byte(`interface Worker
	value(): Integer
end

def touch(): Integer
	return 1
end
`),
		}
		main := SourceUnit{
			Filename: "main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`import { Worker, touch } from other/worker

module Services
	interface Worker
		values(input: Array<Integer>): Array<Integer>
	end

	alias WorkerAlias = Worker

	class EffectWorker implements WorkerAlias
		def values(input: Array<Integer>): Array<Integer>
			return input.concurrent_map do |value|
				value
			end
		end
	end
end

def main()
	puts(touch().to_s())
	return
end
`),
		}
		artifacts, err := CompileProject([]SourceUnit{other, main}, Options{
			Mode: "go", GoModule: "example.com/lexical-alias-target", SourceRoot: "/project", ProjectRoot: "/project",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(runEffectProject(t, "go", artifacts, "example.com/lexical-alias-target")); got != "1" {
			t.Fatalf("unexpected lexical alias target output: got %q, want %q", got, "1")
		}
	})

	t.Run("imported target survives a local leaf collision", func(t *testing.T) {
		contract := SourceUnit{
			Filename: "contracts/worker.trb", ModulePath: "contracts/worker", Package: "contracts",
			Source: []byte(`interface Worker
	values(input: Array<Integer>): Array<Integer>
end

alias WorkerAlias = Worker
`),
		}
		main := SourceUnit{
			Filename: "main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`import { WorkerAlias } from contracts/worker

module Services
	interface Worker
		local_value(): Integer
	end

	alias LocalAlias = WorkerAlias

	class EffectWorker implements LocalAlias
		def values(input: Array<Integer>): Array<Integer>
			return input.concurrent_map do |value|
				value
			end
		end
	end
end

def main()
	return
end
`),
		}
		artifacts, err := CompileProject([]SourceUnit{contract, main}, Options{
			Mode: "go", GoModule: "example.com/imported-alias-target", SourceRoot: "/project", ProjectRoot: "/project",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(runEffectProject(t, "go", artifacts, "example.com/imported-alias-target")); got != "" {
			t.Fatalf("unexpected imported alias target output: got %q, want empty output", got)
		}
	})
}

func TestGoLocalAliasChainDoesNotUseSemanticTargetImport(t *testing.T) {
	contract := SourceUnit{
		Filename: "contracts/worker.trb", ModulePath: "contracts/worker", Package: "contracts",
		Source: []byte(`interface Worker
	value(): Integer
end
`),
	}
	unrelated := SourceUnit{
		Filename: "other/base.trb", ModulePath: "other/base", Package: "other",
		Source: []byte(`class Base
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { Worker } from contracts/worker
import { Base } from other/base

module Services
	alias Base = Worker
	alias LocalAlias = Base
end

def main()
	return
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{contract, unrelated, main}, Options{
		Mode: "go", GoModule: "example.com/local-alias-chain", SourceRoot: "/project", ProjectRoot: "/project",
		AllowUnusedImports: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range artifacts {
		if artifact.IR.ModulePath != "main" {
			continue
		}
		output := string(artifact.Output)
		if !strings.Contains(output, "type LocalAlias = Base") {
			t.Fatalf("local alias chain used an unrelated imported authored target:\n%s", output)
		}
		return
	}
	t.Fatal("main artifact was not generated")
}

func TestImportedAliasToNonInterfaceDoesNotUseCollidingLocalInterface(t *testing.T) {
	contract := SourceUnit{
		Filename: "contracts/worker.trb", ModulePath: "contracts/worker", Package: "contracts",
		Source: []byte(`class Worker
end

alias WorkerAlias = Worker
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { WorkerAlias } from contracts/worker

module Services
	interface Worker
		local_value(): Integer
	end

	alias LocalAlias = WorkerAlias

	class Implementation implements LocalAlias
		def local_value(): Integer
			return 1
		end
	end
end
`),
	}
	_, err := CompileProject([]SourceUnit{contract, main}, Options{Mode: "ruby", SourceRoot: "/project", ProjectRoot: "/project"})
	if err == nil || !strings.Contains(err.Error(), "implemented type Worker must resolve to an interface") {
		t.Fatalf("expected imported non-interface alias diagnostic, got %v", err)
	}
}

func TestFileAndIndexModulesKeepSeparateEffectIdentities(t *testing.T) {
	effectful := SourceUnit{
		Filename: "models.trb", ModulePath: "models", Package: "main",
		Source: []byte(`def map_values(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		value
	end
end

class _Box
	@values: Array<Integer> := map_values([1])
end

def build_effect(): Integer
	_Box.new()
	return 1
end
`),
	}
	pure := SourceUnit{
		Filename: "models/index.trb", ModulePath: "models/index", Package: "models",
		Source: []byte(`class _Box
	@values: Array<Integer> := [2]
end

def build_pure(): Integer
	_Box.new()
	return 2
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { build_effect } from models
import { build_pure } from models/index

def main()
	puts(build_effect().to_s())
	puts(build_pure().to_s())
	return
end
`),
	}
	for _, mode := range []string{"go"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{effectful, pure, main}, Options{
				Mode: mode, GoModule: "example.com/exact-module-effect", RubyLoader: "require_relative",
				SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/exact-module-effect")); got != "1\n2" {
				t.Fatalf("unexpected exact module identity output for %s: got %q, want %q", mode, got, "1\n2")
			}
		})
	}
}

func TestOmittedIndexImportUsesResolvedEffectModuleIdentity(t *testing.T) {
	helper := SourceUnit{
		Filename: "helpers/index.trb", ModulePath: "helpers/index", Package: "helpers",
		Source: []byte(`def map_values(values: Array<Integer>): Array<Integer>
	return values.concurrent_map do |value|
		value
	end
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { map_values } from helpers

def main()
	puts(map_values([5])[0].to_s())
	return
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{helper, main}, Options{
				Mode: mode, GoModule: "example.com/index-effect", RubyLoader: "require_relative",
				TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/index-effect")); got != "5" {
				t.Fatalf("unexpected omitted index effect output for %s: got %q, want %q", mode, got, "5")
			}
		})
	}
}

func TestNestedModuleEffectfulMethodRunsInRubyAndTypeScript(t *testing.T) {
	source := []byte(`module Services
	class Worker
		def values(input: Array<Integer>): Array<Integer>
			return input.concurrent_map do |value|
				value
			end
		end
	end
end

def main()
	puts(Services::Worker.new().values([6])[0].to_s())
	return
end
`)
	for _, mode := range []string{"ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "nested_effect.trb", source, "6")
		})
	}
}

func TestNestedModuleSingletonMethodRunsInRubyAndTypeScript(t *testing.T) {
	source := []byte(`module Services
	def self.values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end

def main()
	puts(Services.values([6])[0])
	return
end
`)
	for _, mode := range []string{"ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "nested_singleton_method.trb", source, "6")
		})
	}
}

func TestNestedModuleSingletonReturnDispatchRunsInRubyAndTypeScript(t *testing.T) {
	source := []byte(`class Worker
	def values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end

module Services
	def self.build(): Worker
		return Worker.new()
	end
end

def main()
	puts(Services.build().values([6])[0])
	return
end
`)
	for _, mode := range []string{"ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "nested_singleton_return.trb", source, "6")
		})
	}
}

func TestModuleSingletonMethodWinsOverSameNamedTypeAliasInRuby(t *testing.T) {
	source := []byte(`module Services
	def self.values(input: Array<Integer>): Array<Integer>
		return input.concurrent_map do |value|
			value
		end
	end
end

class Worker
end

alias Services = Worker

def main()
	puts(Services.values([6])[0])
	return
end
`)
	runRubyEffectSource(t, "module_alias_identity_collision.trb", source, "6")
}

func TestNestedModuleQualifiedConstructorRetainsReceiverTypeInRubyAndTypeScript(t *testing.T) {
	source := []byte(`module Services
	class Worker
		def values(input: Array<Integer>): Array<Integer>
			return input.concurrent_map do |value|
				value
			end
		end
	end
end

def main()
	worker := Services::Worker.new()
	puts(worker.values([6])[0].to_s())
	return
end
`)
	for _, mode := range []string{"ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "nested_receiver_type.trb", source, "6")
		})
	}
}

func TestModuleLeafDoesNotHideNestedClassReceiverInRubyAndTypeScript(t *testing.T) {
	source := []byte(`module Worker
end

module Other
	class Worker
		def values(input: Array<Integer>): Array<Integer>
			return input.concurrent_map do |value|
				value
			end
		end
	end
end

def main()
	worker := Other::Worker.new()
	puts(worker.values([17])[0])
	return
end
`)
	for _, mode := range []string{"ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "module_class_leaf_collision.trb", source, "17")
		})
	}
}

func TestQualifiedNativeConstructorDoesNotUseLocalLeafTypeInRuby(t *testing.T) {
	source := []byte(`import trb/platform/ruby/native

module Services
	class Worker
		def initialize(value: Integer)
			return
		end
	end
end

module Other
end

class Other::Worker
	def initialize(value)
		@value = value
	end

	def value
		@value
	end
end

def main()
	puts(Other::Worker.new("ok").value())
	return
end
`)
	runRubyEffectSource(t, "native_qualified_constructor.trb", source, "ok")
}

func TestQualifiedNativeMemberDoesNotUseRootLeafTypeInRuby(t *testing.T) {
	source := []byte(`import trb/platform/ruby/native

class Stat
	def size(): Array<Integer>
		return [1].concurrent_map do |value|
			value
		end
	end
end

def main()
	File::Stat.new(__FILE__).size()
	return
end
`)
	runRubyEffectSource(t, "native_qualified_member.trb", source, "")
}

func TestNestedModuleRelativeQualifiedEffectsRunInRuby(t *testing.T) {
	t.Run("method", func(t *testing.T) {
		source := []byte(`module Services
	module Sub
		class Worker
			def values(input: Array<Integer>): Array<Integer>
				return input.concurrent_map do |value|
					value
				end
			end
		end
	end

	class Runner
		def run(): Array<Integer>
			return Sub::Worker.new().values([6])
		end
	end
end

def main()
	puts(Services::Runner.new().run()[0].to_s())
	return
end
`)
		runRubyEffectSource(t, "relative_nested_method.trb", source, "6")
	})

	t.Run("constructor", func(t *testing.T) {
		source := []byte(constructorEffectPrelude + `
module Services
	module Sub
		class Box
			@values: Array<Integer> := map_values([10])

			def first(): Integer
				return @values[0]
			end
		end
	end

	class Runner
		def run(): Integer
			return Sub::Box.new().first()
		end
	end
end

def main()
	puts(Services::Runner.new().run().to_s())
	return
end
`)
		runRubyEffectSource(t, "relative_nested_constructor.trb", source, "10")
	})
}

func TestNestedModuleOuterLexicalInheritanceEffectsRunInRuby(t *testing.T) {
	t.Run("superclass", func(t *testing.T) {
		source := []byte(`module Services
	class Base
		def values(input: Array<Integer>): Array<Integer>
			return input.concurrent_map do |value|
				value
			end
		end
	end

	module Sub
		class Child < Base
		end
	end
end

def main()
	puts(Services::Sub::Child.new().values([13])[0].to_s())
	return
end
`)
		runRubyEffectSource(t, "outer_lexical_superclass.trb", source, "13")
	})

	t.Run("interface", func(t *testing.T) {
		source := []byte(`module Services
	interface Worker
		values(input: Array<Integer>): Array<Integer>
	end

	module Sub
		class EffectWorker implements Worker
			def values(input: Array<Integer>): Array<Integer>
				return input.concurrent_map do |value|
					value
				end
			end
		end
	end

	class Runner
		def run(worker: Worker): Array<Integer>
			return worker.values([14])
		end
	end
end

def main()
	puts(Services::Runner.new().run(Services::Sub::EffectWorker.new())[0].to_s())
	return
end
`)
		runRubyEffectSource(t, "outer_lexical_interface.trb", source, "14")
	})
}

func TestNestedModuleClassFieldDefaultUsesConstructorScope(t *testing.T) {
	for _, test := range []struct {
		name        string
		initializer string
	}{
		{name: "explicit initializer", initializer: "\n\t\tdef initialize()\n\t\t\treturn\n\t\tend\n"},
		{name: "implicit initializer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(constructorEffectPrelude + `
module Services
	class Box
		@values: Array<Integer> := map_values([10])
` + test.initializer + `
		def first(): Integer
			return @values[0]
		end
	end
end

def main()
	puts(Services::Box.new().first().to_s())
	return
end
`)
			runRubyEffectSource(t, "nested_constructor_effect.trb", source, "10")
		})
	}
}

func runRubyEffectSource(t *testing.T, filename string, source []byte, want string) {
	t.Helper()
	runEffectSource(t, "ruby", filename, source, want)
}

func runEffectSource(t *testing.T, mode, filename string, source []byte, want string) {
	t.Helper()
	requireEffectRuntime(t, mode)
	artifact, err := Compile(filename, source, mode)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(runConcurrentProbeArtifact(t, mode, artifact.Output)); got != want {
		t.Fatalf("unexpected nested module effect output: got %q, want %q", got, want)
	}
}

func requireEffectRuntime(t *testing.T, mode string) {
	t.Helper()
	tool := mode
	if mode == "typescript" {
		tool = "bun"
	}
	if _, err := exec.LookPath(tool); err != nil {
		t.Skip(tool + " is not installed")
	}
}

func runEffectProject(t *testing.T, mode string, artifacts []*Artifact, goModule string) string {
	t.Helper()
	root := t.TempDir()
	extension := map[string]string{"go": ".go", "ruby": ".rb", "typescript": ".ts"}[mode]
	for _, artifact := range artifacts {
		path := filepath.Join(root, filepath.FromSlash(artifact.IR.ModulePath)+extension)
		writeCompilerRuntimeFile(t, path, artifact.Output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var command *exec.Cmd
	switch mode {
	case "go":
		writeCompilerRuntimeFile(t, filepath.Join(root, "go.mod"), []byte("module "+goModule+"\n\ngo 1.27\n"))
		command = exec.CommandContext(ctx, "go", "run", ".")
		command.Env = append(os.Environ(), "GOCACHE=/tmp/type-rb-go-cache")
	case "ruby":
		command = exec.CommandContext(ctx, "ruby", "main.rb")
	case "typescript":
		command = exec.CommandContext(ctx, "bun", "run", "main.ts")
	}
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated %s effect project failed: %v\n%s", mode, err, output)
	}
	return string(output)
}

func assertPureEffectIdentityModule(t *testing.T, units []SourceUnit, module string) {
	t.Helper()
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject(units, Options{
				Mode: mode, GoModule: "example.com/effect-identity", RubyLoader: "require_relative",
				SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifactForModule(artifacts, module).Output)
			switch mode {
			case "go":
				if strings.Contains(output, "Values(__trbScope") || strings.Contains(output, ".Values(trbcontext.Background()") {
					t.Fatalf("pure Go module received another module's effect ABI:\n%s", output)
				}
			case "ruby":
				if strings.Contains(output, "def values(__trb_scope") || strings.Contains(output, ".values(TrbExecutionScope.root") {
					t.Fatalf("pure Ruby module received another module's effect ABI:\n%s", output)
				}
			}
		})
	}
}
