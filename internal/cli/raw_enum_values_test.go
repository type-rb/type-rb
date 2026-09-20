package cli

import (
	"strings"
	"testing"
)

func TestRawEnumDirectAndStoredValuesAcrossTargetsAndREPL(t *testing.T) {
	source := `enum Status
Ready = "r"
Done = "d"
def label(): String
return self.raw_value()
end
end
enum Level
Low = -1
High = 3
end
def main()
puts(Status::Ready.raw_value())
stored := Status::Done
puts(stored.raw_value())
puts(Status::Ready.label())
puts(Level::Low.raw_value())
end
`
	runPortableExecutionCase(t, source, "r\nd\nr\n-1\n", "")
}

func TestRawEnumConversionCaseAcrossTargetsAndREPL(t *testing.T) {
	source := `enum Status
Ready = "r"
Done = "d"
end
module Codes
enum Level
Low = -1
High = 3
end
end
def main()
case Status.from_raw("r")
when Result::Ok(value)
puts(value.raw_value())
when Result::Err(_error)
puts("unexpected")
end
case Status.from_raw("missing")
when Result::Ok(_value)
puts("unexpected")
when Result::Err(_error)
puts("unknown")
end
case Codes::Level.from_raw(-1)
when Result::Ok(value)
puts(value.raw_value())
when Result::Err(_error)
puts("unexpected")
end
end
`
	runPortableExecutionCase(t, source, "r\nunknown\n-1\n", "")
}

func TestImportedNestedRawEnumAliasAcrossTargetsAndREPL(t *testing.T) {
	files := map[string]string{
		"messages/codes.trb": `module Codes
enum Status
Ready = "r"
Done = "d"
end
record Message
text: String = "default"
end
end
`,
		"consumer.trb": `import messages/codes as Wire
def report()
puts(Wire::Status::Ready.raw_value())
case Wire::Status.from_raw("d")
when Result::Ok(value)
puts(value.raw_value())
when Result::Err(_error)
puts("unexpected")
end
message := Wire::Message.new()
puts(message.text)
end
`,
		"main.trb": `import { report } from consumer
def main()
report()
end
`,
	}
	for _, spelling := range []string{"import messages/codes as Wire", "import { Codes } from messages/codes"} {
		t.Run(spelling, func(t *testing.T) {
			consumer := files["consumer.trb"]
			if strings.Contains(spelling, "{") {
				consumer = strings.ReplaceAll(consumer, "import messages/codes as Wire", spelling)
				consumer = strings.ReplaceAll(consumer, "Wire::", "Codes::")
			}
			program := map[string]string{"messages/codes.trb": files["messages/codes.trb"], "consumer.trb": consumer, "main.trb": files["main.trb"]}
			runPortableExecutionFiles(t, program, "r\nd\ndefault\n", "")
		})
	}
}

func TestRawEnumValueAndOwnerDiagnosticsAcrossTargetsAndREPL(t *testing.T) {
	for _, test := range []struct{ expression, failure string }{
		{"puts(Status.raw_value())", "instance member"},
		{"Status::Ready.from_raw(\"r\")", "class member"},
		{"Status.from_raw(7)", "expected string"},
		{"Status.from_raw(\"r\")", "must be used"},
		{"value: String := Status::Ready\nputs(value)", "cannot assign"},
	} {
		t.Run(test.expression, func(t *testing.T) {
			source := "enum Status\nReady = \"r\"\nend\ndef main()\n" + test.expression + "\nend\n"
			runPortableExecutionCase(t, source, "", test.failure)
		})
	}
}
