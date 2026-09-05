package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/nativepackage"
)

const newtypeMemberExample = `newtype Label = String do
	private new

	def self.parse(source: String): Label?
		if source.empty?()
			return nil
		end
		return self.new(source)
	end

	def decorated(*, prefix: String = "["): String
		return prefix + value() + "]"
	end

	def to_s(): String
		return decorated()
	end
end

def main()
	label := Label.parse("hello")
	if label != nil
		puts(label.to_s())
		puts(label.decorated(prefix: "<"))
	end
end
`

func TestNewtypeMembersRunAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "newtype_members.trb", []byte(newtypeMemberExample), "[hello]\n<hello]")
		})
	}
}

func TestNewtypeMemberSafeNavigationEvaluatesReceiverOnceAndSkipsArguments(t *testing.T) {
	source := []byte(`newtype Label = String do
	def append(other: String): String
		return value() + other
	end
end
def receiver(present: Boolean): Label?
	puts("receiver")
	if present
		return Label.new("a")
	end
	return nil
end
def argument(): String
	puts("argument")
	return "b"
end
def main()
	missing := receiver(false)&.append(argument())
	puts((missing == nil).to_s())
	present := receiver(true)&.append(argument())
	if present != nil
		puts(present)
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "safe_newtype.trb", source, "receiver\ntrue\nreceiver\nargument\nab")
		})
	}
}

func TestTransitiveNewtypeMemberDependencyRunsAcrossBackends(t *testing.T) {
	units := []SourceUnit{
		{Filename: "/project/labels/index.trb", ModulePath: "labels/index", Package: "labels", Source: []byte(`newtype Label = String do
	private new
	def self.from(value: String): Label
		return self.new(value)
	end
	def text(): String
		return value()
	end
end
def make_label(): Label
	return Label.from("hello")
end
`)},
		{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte("import { make_label } from labels\ndef main()\nputs(make_label().text())\nend\n")},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject(units, Options{Mode: mode, GoModule: "example.com/labels", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/labels")); got != "hello" {
				t.Fatal(got)
			}
		})
	}
}

func TestNewtypeMemberDispatchUsesImportedDeclarationIdentity(t *testing.T) {
	units := []SourceUnit{
		{Filename: "/project/labels/index.trb", ModulePath: "labels/index", Package: "labels", Source: []byte("newtype Label = Integer do\nprivate new\ndef self.from(value: Integer): Label\nreturn self.new(value)\nend\ndef text(): String\nreturn \"remote:\" + value().to_s()\nend\nend\n")},
		{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte("import { Label as Count } from labels\nnewtype Label = String do\ndef text(): String\nreturn \"local:\" + value()\nend\nend\ndef main()\nputs(Count.from(7).text())\nputs((Count.from(7).value() + 1).to_s())\nputs(Label.new(\"hello\").text())\nend\n")},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject(units, Options{Mode: mode, GoModule: "example.com/labels", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/labels")); got != "remote:7\n8\nlocal:hello" {
				t.Fatal(got)
			}
		})
	}
}

func TestClosedNewtypeRejectsNativeReturn(t *testing.T) {
	catalog := &nativepackage.Catalog{FormatVersion: nativepackage.FormatVersion, Dependencies: map[string]string{"example": "1.0.0"}, Modules: map[string]nativepackage.Module{
		"example": {Exports: map[string]nativepackage.Export{"raw_label": {Kind: "function", Type: nativepackage.Type{Kind: "named", Name: "Label"}}}},
	}}
	unit := SourceUnit{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { raw_label } from example
newtype Label = String do
	private new
end
def unsafe_label(): Label
	return raw_label()
end
`)}
	_, err := CompileProject([]SourceUnit{unit}, Options{Mode: "typescript", NativePackages: catalog})
	if err == nil || !strings.Contains(err.Error(), "cannot construct closed newtype") {
		t.Fatalf("native return error = %v", err)
	}
}

func TestClosedNewtypeNativeCallbackDirectionality(t *testing.T) {
	label := nativepackage.Type{Kind: "named", Name: "Label"}
	unit := nativepackage.Type{Kind: "void", Name: "Void"}
	text := nativepackage.Type{Kind: "string", Name: "String"}
	for _, test := range []struct {
		name   string
		args   []nativepackage.Type
		source string
		reject bool
	}{
		{"incoming callback argument", []nativepackage.Type{label, unit}, "fn(value: Label)\nputs(value.value())\nend", true},
		{"outgoing callback result", []nativepackage.Type{text, label}, "fn(value: String): Label\nreturn Label.from(value)\nend", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog := &nativepackage.Catalog{FormatVersion: nativepackage.FormatVersion, Dependencies: map[string]string{"example": "1.0.0"}, Modules: map[string]nativepackage.Module{
				"example": {Exports: map[string]nativepackage.Export{"register": {Kind: "function", Type: unit, Required: 1, Parameters: []nativepackage.Type{{Kind: "function", Args: test.args}}}}},
			}}
			source := "import { register } from example\nnewtype Label = String do\nprivate new\ndef self.from(value: String): Label\nreturn self.new(value)\nend\nend\ndef main()\ncallback := " + test.source + "\nregister(callback)\nend\n"
			_, err := CompileWithOptions("main.trb", []byte(source), Options{Mode: "typescript", NativePackages: catalog})
			if test.reject {
				if err == nil || !strings.Contains(err.Error(), "cannot construct closed newtype") {
					t.Fatalf("native callback error = %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClosedNewtypeNativeValueAndRecordEdges(t *testing.T) {
	label := nativepackage.Type{Kind: "named", Name: "Label"}
	payload := nativepackage.Type{Kind: "named", Name: "NativePayload"}
	catalog := &nativepackage.Catalog{FormatVersion: nativepackage.FormatVersion, Dependencies: map[string]string{"example": "1.0.0"}, Modules: map[string]nativepackage.Module{
		"example": {Exports: map[string]nativepackage.Export{
			"raw_label":     {Kind: "value", Type: label},
			"NativePayload": {Kind: "record", Type: payload, Fields: []nativepackage.Field{{Name: "label", Type: label}}},
			"receive":       {Kind: "function", Type: payload},
			"send":          {Kind: "function", Type: nativepackage.Type{Kind: "void", Name: "Void"}, Required: 1, Parameters: []nativepackage.Type{payload}},
		}},
	}}
	for _, test := range []struct {
		name, imports, body string
		reject              bool
	}{
		{"incoming value", "raw_label", "puts(raw_label.value())", true},
		{"incoming record", "receive", "puts(receive().label.value())", true},
		{"outgoing record", "NativePayload, send", "send(NativePayload.new(label: Label.from(\"hello\")))", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := "import { " + test.imports + " } from example\nnewtype Label = String do\nprivate new\ndef self.from(value: String): Label\nreturn self.new(value)\nend\nend\ndef main()\n" + test.body + "\nend\n"
			_, err := CompileWithOptions("main.trb", []byte(source), Options{Mode: "typescript", NativePackages: catalog})
			if test.reject {
				if err == nil || !strings.Contains(err.Error(), "cannot construct closed newtype") {
					t.Fatalf("native edge error = %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClosedNewtypeRejectsWebBodyAndQuery(t *testing.T) {
	for _, operation := range []string{"json", "query"} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			source := `import { Request } from trb/web
newtype Label = String do
	private new
end
record Payload
	label: Label
end
def read(request: Request): Payload?
	return request.` + operation + `<Payload>() catch |_error|
		return nil
	end
end
`
			_, err := CompileProject([]SourceUnit{{Filename: "/project/main.trb", ModulePath: "main", Source: []byte(source)}}, Options{Mode: mode})
			if err == nil || !strings.Contains(err.Error(), "Payload.label -> Label") {
				t.Fatalf("%s %s error = %v", mode, operation, err)
			}
		}
	}
}

func TestClosedNewtypeRejectsJobInput(t *testing.T) {
	sources := []SourceUnit{
		{Filename: "/project/contracts/index.trb", ModulePath: "contracts/index", Package: "contracts", Source: []byte("newtype Label = String do\nprivate new\nend\n")},
		{Filename: "/project/jobs/print_job.trb", ModulePath: "jobs/print_job", Package: "jobs", Source: []byte("import { Label } from contracts\nimport { Job } from trb/jobs\nclass PrintJob < Job\ndef perform(label: Label)\nputs(label.value())\nend\nend\n")},
		{Filename: "/project/config/jobs.trb", ModulePath: "config/jobs", Package: "config", Source: []byte(jobsSQLConfigurationSource)},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := CompileProject(sources, Options{Mode: mode, GoModule: "example.com/jobs", TypeScriptRuntime: "bun", SourceRoot: "/project", ProjectRoot: "/project", JobsConfiguration: "config/jobs"})
		if err == nil {
			t.Fatalf("%s allowed a closed newtype Job input", mode)
		}
		if !strings.Contains(err.Error(), "PrintJob") || !strings.Contains(err.Error(), "label") {
			t.Fatalf("%s Job input error = %v", mode, err)
		}
	}
}

func TestImportedNewtypeMembersRunAcrossBackends(t *testing.T) {
	boundary := strings.Index(newtypeMemberExample, "def main()")
	units := []SourceUnit{
		{Filename: "/project/labels/index.trb", ModulePath: "labels/index", Package: "labels", Source: []byte(newtypeMemberExample[:boundary])},
		{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte("import { Label as Text } from labels\n" + strings.ReplaceAll(newtypeMemberExample[boundary:], "Label", "Text"))},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject(units, Options{Mode: mode, GoModule: "example.com/labels", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/labels")); got != "[hello]\n<hello]" {
				t.Fatal(got)
			}
		})
	}
}

func TestClosedNewtypeDiagnostics(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"external construction", "newtype Label = String do\nprivate new\nend\ndef bad(): Label\nreturn Label.new(\"bad\")\nend\n", "raw constructor for closed newtype Label is private"},
		{"mutable representation", "newtype Labels = Array<String> do\nprivate new\nend\n", "requires a recursively immutable representation"},
		{"nested mutable representation", "record Data\nvalues: Array<String>\nend\nnewtype Labels = Data do\nprivate new\nend\n", "requires a recursively immutable representation"},
		{"constructor override", "newtype Label = String do\ndef self.new(): String\nreturn \"bad\"\nend\nend\n", "newtype method name new is reserved"},
		{"duplicate directive", "newtype Label = String do\nprivate new\nprivate new\nend\n", "private new must occur once"},
		{"no field", "newtype Label = String do\n@name: String\nend\n", "newtype body may only contain"},
		{"no forwarding", "newtype Label = String do\nend\ndef bad(label: Label): Integer\nreturn label.size()\nend\n", "has no instance member size"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				_, err := Compile("bad_newtype.trb", []byte(tc.source), mode)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("%s error = %v, want %q", mode, err, tc.want)
				}
			}
		})
	}
}

func TestClosedNewtypeRejectsAliasedMutableRecordWithSameName(t *testing.T) {
	units := []SourceUnit{
		{Filename: "/project/data/index.trb", ModulePath: "data/index", Package: "data", Source: []byte("record Payload\nvalues: Array<String>\nend\n")},
		{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte("import { Payload as Remote } from data\nrecord Payload\nvalues: String\nend\nnewtype Validated = Remote do\nprivate new\nend\n")},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := CompileProject(units, Options{Mode: mode})
		if err == nil || !strings.Contains(err.Error(), "requires a recursively immutable representation") {
			t.Fatalf("%s error = %v", mode, err)
		}
	}
}

func TestClosedNewtypeRejectsInboundNullableEnum(t *testing.T) {
	source := []byte("import trb/std/json\nnewtype Label = String do\nprivate new\nend\nenum Item\nText(label: Label)\nEmpty\nend\ndef decode(source: String): Item?\nreturn JSON.decode<Item?>(source) catch |_error|\nreturn nil\nend\nend\n")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := CompileProject([]SourceUnit{{Filename: "/project/main.trb", ModulePath: "main", Source: source}}, Options{Mode: mode})
		if err == nil || !strings.Contains(err.Error(), "Item::Text.label -> Label") {
			t.Fatalf("%s error = %v", mode, err)
		}
	}
}

func TestNewtypeGenericAndEffectfulMembers(t *testing.T) {
	source := []byte(`newtype Counter = Integer do
	def map<T>(value: T): Array<T>
		return [value]
	end
	def parallel(): Integer
		values := [1, 2].concurrent_map do |number|
			number + self.value()
		end
		return values[0]
	end
end
def main()
	value := Counter.new(7)
	puts(value.map<String>("ok")[0])
	puts(value.parallel().to_s())
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "newtype_effects.trb", source, "ok\n8")
		})
	}
}

func TestModuleOwnedNewtypeMethods(t *testing.T) {
	source := []byte(`module Labels
	newtype Label = String do
		private new
		def self.from(value: String): Label
			return self.new(value)
		end
		def text(): String
			return value()
		end
	end
end
def main()
	puts(Labels::Label.from("hello").text())
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "nested_newtype.trb", source, "hello")
		})
	}
}

func TestClosedNewtypeValuesInSourceCollections(t *testing.T) {
	source := []byte(`newtype Label = String do
	private new
	def self.from(source: String): Label
		return self.new(source)
	end
	def labels(): Array<Label>
		mut values := []
		values.push(self)
		return values
	end
end
record Link
	value: String
	following: Link?
end
newtype Chain = Link do
	private new
end
def main()
	label := Label.from("hello")
	values := label.labels().map do |item|
		item
	end
	first := values.try_fetch(0) catch |_error|
		return
	end
	puts(first.value())
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			unit := SourceUnit{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: source}
			artifacts, err := CompileProject([]SourceUnit{unit}, Options{Mode: mode, GoModule: "example.com/labels", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/labels")); got != "hello" {
				t.Fatal(got)
			}
		})
	}
}

func TestClosedNewtypeJSONDirectionality(t *testing.T) {
	declaration := `import trb/std/json
newtype Label = String do
	private new
	def self.from(source: String): Label
		return self.new(source)
	end
end
record Payload
	label: Label?
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			outbound := declaration + `def main()
	encoded := JSON.encode(Payload.new(label: Label.from("ok"))) catch |_error|
		return
	end
	puts(encoded)
end
`
			unit := SourceUnit{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(outbound)}
			artifacts, err := CompileProject([]SourceUnit{unit}, Options{Mode: mode, GoModule: "example.com/labels", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/labels")); got != `{"label":"ok"}` {
				t.Fatal(got)
			}
			unit.Source = []byte(declaration + `def decode(source: String): Payload?
	return JSON.decode<Payload>(source) catch |_error|
		return nil
	end
end
`)
			_, err = CompileProject([]SourceUnit{unit}, Options{Mode: mode})
			if err == nil || !strings.Contains(err.Error(), "Payload.label -> Label") {
				t.Fatalf("decode error = %v", err)
			}
		})
	}
}
