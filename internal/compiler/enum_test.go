package compiler

import (
	"strings"
	"testing"
)

const rawValueEnumSource = `import { JSON } from trb/std/json

enum OrderStatus
	Pending = "PENDING"
	Completed = "COMPLETED"

	def terminal?(): Boolean
		return self == OrderStatus::Completed
	end
end

enum Priority
	Low = -1
	High = 2
end

def main()
	status := OrderStatus::Completed
	puts(status.raw_value())
	puts(status.terminal?())
	encoded := JSON.encode(status)
	case encoded
	when Result::Ok(json)
		puts(json)
	when Result::Err(error)
		puts(error.message)
	end
	decoded := JSON.decode<OrderStatus>("\"COMPLETED\"")
	case decoded
	when Result::Ok(value)
		puts(value.terminal?())
	when Result::Err(error)
		puts(error.message)
	end
	parsed := OrderStatus.from_raw("PENDING")
	case parsed
	when Result::Ok(value)
		puts(value.raw_value())
	when Result::Err(error)
		puts(error.message)
	end
	low := Priority::Low
	puts(low.raw_value())
	priority := Priority.from_raw(2)
	case priority
	when Result::Ok(value)
		puts(value.raw_value())
	when Result::Err(error)
		puts(error.message)
	end
	return
end
`

func TestRawValueEnumsCompileAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", Source: []byte(rawValueEnumSource)}}, Options{
				Mode: mode, GoModule: "example.com/raw-enum", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			var output string
			for _, artifact := range artifacts {
				if artifact.Filename == "main.trb" {
					output = string(artifact.Output)
				}
			}
			if output == "" {
				t.Fatal("main artifact was not generated")
			}
			for _, forbidden := range []string{"raw_value()", "from_raw("} {
				if strings.Contains(output, forbidden) {
					t.Fatalf("generated %s retained source-only enum API %q:\n%s", mode, forbidden, output)
				}
			}
		})
	}
}

func TestRawValueEnumDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"mixed member", "enum Status\n\tReady = \"READY\"\n\tDone\nend\n", "every member of a raw-value enum"},
		{"mixed raw types", "enum Status\n\tReady = \"READY\"\n\tDone = 2\nend\n", "raw enum value has type Integer, expected String"},
		{"duplicate raw value", "enum Status\n\tReady = \"READY\"\n\tDone = \"READY\"\nend\n", "raw enum value duplicates Ready"},
		{"computed raw value", "enum Status\n\tReady = \"READY\" + \"!\"\nend\n", "raw enum values must be explicit"},
		{"payload raw value", "enum Status\n\tReady(value: String)\n\tDone = \"DONE\"\nend\n", "raw-value enum members cannot declare payload fields"},
		{"reserved method", "enum Status\n\tReady\n\tdef raw_value(): String\n\t\treturn \"ready\"\n\tend\nend\n", "enum method name raw_value is reserved"},
		{"dot member access", "enum Status\n\tReady = \"READY\"\nend\nvalue := Status.Ready\n", "enum member Ready must be accessed with ::"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Compile("status.trb", []byte(test.source), "go")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestPayloadEnumsMayDeclareInstanceMethodsAcrossBackends(t *testing.T) {
	source := []byte(`enum Token
	Text(value: String)
	EOF

	def text?(): Boolean
		case self
		when Token::Text(_)
			return true
		when Token::EOF
			return false
		end
	end
end

def main()
	puts(Token::Text("value").text?())
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("main.trb", source, mode); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
	}
}

func TestGenericEnumMethodsUseReceiverTypeArguments(t *testing.T) {
	source := []byte(`enum Box<T>
	Value(value: T)

	def replace(value: T): T
		return value
	end
end

def main()
	box := Box<String>::Value("first")
	puts(box.replace("second"))
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("main.trb", source, mode); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
	}

	invalid := []byte(`enum Box<T>
	Value(value: T)

	def replace(value: T): T
		return value
	end
end

def main()
	box := Box<String>::Value("first")
	puts(box.replace(2))
	return
end
`)
	_, err := Compile("main.trb", invalid, "go")
	if err == nil || !strings.Contains(err.Error(), "has type Integer, expected String") {
		t.Fatalf("expected specialized enum method argument error, got %v", err)
	}
}

func TestImportedRawValueEnumAPIsCompileAcrossBackends(t *testing.T) {
	model := SourceUnit{Filename: "/project/models/status.trb", ModulePath: "models/status", Package: "models", Source: []byte(`enum OrderStatus
	Pending = "PENDING"
	Completed = "COMPLETED"

	def terminal?(): Boolean
		return self == OrderStatus::Completed
	end
end
`)}
	main := SourceUnit{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { OrderStatus } from models/status

def main()
	status := OrderStatus::Completed
	puts(status.raw_value())
	puts(status.terminal?())
	puts(OrderStatus.from_raw("PENDING"))
	return
end
`)}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := CompileProject([]SourceUnit{model, main}, Options{Mode: mode, GoModule: "example.com/raw-enum", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRawValueEnumGeneratedAPIsSupportForwardTypeReferences(t *testing.T) {
	source := []byte(`def main()
	puts(OrderStatus.from_raw("READY"))
	return
end

enum OrderStatus
	Ready = "READY"
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("main.trb", source, mode); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
	}
}
