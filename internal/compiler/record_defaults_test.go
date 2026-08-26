package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
)

const recordDefaultsSource = `record Window
	start: Integer
	finish: Integer = start + 10
	tags: Array<String> = []
end

def sample(): Window
	return Window.new(start: 2)
end
`

func TestRecordFieldDefaultsAreFirstClassAcrossPortableBackends(t *testing.T) {
	expected := map[string][]string{
		"go": {
			"func TrbRecordNewWindow(start int, finish int, trbFinishProvided bool, tags []string, trbTagsProvided bool) Window",
			"finish = trbIntegerAdd_",
			"tags = []string{}",
			"return TrbRecordNewWindow(trbRecordArg1, *new(int), false, *new([]string), false)",
		},
		"ruby": {
			"Window = Data.define(:start, :finish, :tags) do",
			"finish = __trb_integer_add(start, 10) unless trb_finish_provided",
			"tags = [] unless trb_tags_provided",
			"Window.__trb_record_new(__trb_record_arg_1, nil, false, nil, false)",
		},
		"typescript": {
			"export function __trbRecordNewWindow(args: { start: number; finish?: number; tags?: Array<string> }): Window",
			"const finish = Object.prototype.hasOwnProperty.call(args, \"finish\") ? args.finish as number : __trbIntegerAdd(start, 10);",
			"const tags = Object.prototype.hasOwnProperty.call(args, \"tags\") ? args.tags as Array<string> : [];",
			"return __trbRecordNewWindow({ start: 2 });",
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := Compile("window.trb", []byte(recordDefaultsSource), mode)
			if err != nil {
				t.Fatal(err)
			}
			record, ok := artifact.AST.Statements[0].(*ast.RecordStatement)
			if !ok {
				t.Fatalf("expected record AST, got %#v", artifact.AST.Statements[0])
			}
			if field := record.Body[2].(*ast.RecordFieldStatement); field.Default == nil {
				t.Fatal("record field default was not preserved in the AST")
			}
			lowered, ok := artifact.IR.Statements[0].(*ir.Record)
			if !ok {
				t.Fatalf("expected record IR, got %#v", artifact.IR.Statements[0])
			}
			if field := lowered.Body[2].(*ir.RecordField); field.Default == nil {
				t.Fatal("record field default was not preserved in the IR")
			}
			output := string(artifact.Output)
			for _, fragment := range expected[mode] {
				if !strings.Contains(output, fragment) {
					t.Fatalf("generated %s is missing %q:\n%s", mode, fragment, output)
				}
			}
		})
	}
}

func TestRecordFieldDefaultDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "wrong type",
			source: "record Config\n\tport: Integer = \"8080\"\nend\n",
			want:   "record field default has type String, expected Integer",
		},
		{
			name:   "required after default",
			source: "record Config\n\tport: Integer = 8080\n\thost: String\nend\n",
			want:   "required record field cannot follow a default field",
		},
		{
			name:   "later field reference",
			source: "record Range\n\tfinish: Integer = start + 1\n\tstart: Integer = 0\nend\n",
			want:   "operator + does not support Any and Integer",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Compile("config.trb", []byte(test.source), "go")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Compile() error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestEnumMembersPreservePostfixAttributes(t *testing.T) {
	source := []byte(`enum Command
	Serve(port: Integer) @cli(about: "Start the server")
	Version @cli(about: "Print the version")
end
`)
	artifact, err := Compile("command.trb", source, "go")
	if err != nil {
		t.Fatal(err)
	}
	parsed := artifact.AST.Statements[0].(*ast.EnumStatement)
	serve := parsed.Body[0].(*ast.EnumMemberStatement)
	if len(serve.Attributes) != 1 || serve.Attributes[0].Name != "cli" || len(serve.Attributes[0].Arguments) != 1 {
		t.Fatalf("unexpected enum member attributes in AST: %#v", serve.Attributes)
	}
	lowered := artifact.IR.Statements[0].(*ir.Enum)
	version := lowered.Body[1].(*ir.EnumMember)
	if len(version.Attributes) != 1 || version.Attributes[0].Name != "cli" {
		t.Fatalf("unexpected enum member attributes in IR: %#v", version.Attributes)
	}
}
