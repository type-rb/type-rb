package compiler

import (
	"strings"
	"testing"
)

func TestUnusedLocalBindingsAreRejectedAcrossModes(t *testing.T) {
	source := []byte("def main()\n\tanswer := 42\n\treturn\nend\n")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("unused_local.trb", source, mode)
		if err == nil || !strings.Contains(err.Error(), "local variable answer is not used") {
			t.Fatalf("%s: expected unused local diagnostic, got %v", mode, err)
		}
	}
}

func TestUnusedBlockAndPatternBindingsAreRejected(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "block parameter",
			source: "def main()\n" +
				"\t[1].each do |value|\n" +
				"\t\tputs(\"item\")\n" +
				"\tend\n" +
				"\treturn\n" +
				"end\n",
			want: "block parameter value is not used",
		},
		{
			name: "pattern binding",
			source: "import { Result } from trb/std/result\n\n" +
				"def inspect(result: Result<Integer, String>): Integer\n" +
				"\tcase result\n" +
				"\twhen Result::Ok(value)\n" +
				"\t\treturn 1\n" +
				"\twhen Result::Err(error)\n" +
				"\t\treturn 0\n" +
				"\tend\n" +
				"end\n",
			want: "pattern binding value is not used",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Compile("unused_binding.trb", []byte(test.source), "go")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q diagnostic, got %v", test.want, err)
			}
		})
	}
}

func TestBlankBindingsExplicitlyDiscardValuesAcrossModes(t *testing.T) {
	source := []byte(`import { Result } from trb/std/result

def inspect(result: Result<Integer, String>): Integer
	case result
	when Result::Ok(_)
		return 1
	when Result::Err(_)
		return 0
	end
end

def main()
	[1].each do |_|
		puts("item")
	end
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("blank_binding.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected blank bindings: %v", mode, err)
		}
		if strings.Contains(string(artifact.Output), "_ :=") || strings.Contains(string(artifact.Output), "const _ =") {
			t.Fatalf("%s emitted a payload declaration for a blank pattern binding:\n%s", mode, artifact.Output)
		}
	}
}

func TestUnusedImportsAreRejectedAcrossModes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "package import",
			source: "import trb/std/io\n\ndef main()\n\tputs(1)\n\treturn\nend\n",
			want:   "import trb/std/io is not used",
		},
		{
			name:   "named import",
			source: "import { Result } from trb/std/result\n\ndef main()\n\treturn\nend\n",
			want:   "imported symbol Result is not used",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				_, err := Compile("unused_import.trb", []byte(test.source), mode)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q diagnostic, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestImportsUsedByPackagesAndTypesAreAccepted(t *testing.T) {
	source := []byte(`import trb/std/io
import { Result } from trb/std/result

def print_result(result: Result<Integer, String>)
	io.puts(result)
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("used_imports.trb", source, mode); err != nil {
			t.Fatalf("%s rejected used imports: %v", mode, err)
		}
	}
}

func TestImportedTypesUsedAsGenericArgumentsAreAccepted(t *testing.T) {
	source := []byte(`import { JsonError } from trb/std/json

def identity<T>(value: T): T
	return value
end

def discard()
	identity<JsonError?>(nil)
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("generic_type_import.trb", source, mode); err != nil {
			t.Fatalf("%s rejected an imported generic type argument: %v", mode, err)
		}
	}
}

func TestUnusedParametersRemainValidAcrossModes(t *testing.T) {
	source := []byte("def ignore(value: Integer)\n\treturn\nend\n")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("unused_parameter.trb", source, mode); err != nil {
			t.Fatalf("%s rejected an unused function parameter: %v", mode, err)
		}
	}
}
