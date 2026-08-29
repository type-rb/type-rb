package playground

import (
	"context"
	"fmt"
)

type Lesson struct {
	ID          string `json:"id"`
	Chapter     string `json:"chapter"`
	Title       string `json:"title"`
	Eyebrow     string `json:"eyebrow"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Expected    string `json:"expected"`
	Hint        string `json:"hint"`
}

var tourLessons = []Lesson{
	{
		ID: "hello", Chapter: "Start", Title: "First TypeRB", Eyebrow: "01 · Start",
		Description: "The browser scratch runner evaluates ordinary expressions directly. Comments begin with #, and the portable puts function writes values to standard output.",
		Source:      "# Change either value and evaluate the scratch source again.\nputs(\"Hello, TypeRB!\")\nputs(1 + 2)\n",
		Expected:    "Hello, TypeRB!\n3\n",
		Hint:        "Change the message or arithmetic expression, then press Run or Cmd/Ctrl-Enter.",
	},
	{
		ID: "values", Chapter: "Start", Title: "Values, types, and methods", Eyebrow: "02 · Start",
		Description: "Integer, Float, String, and Boolean values have portable operators and receiver methods. Types are checked before the selected target runs.",
		Source:      "count := 6\nratio := 1.5\nlanguage := \"TypeRB\"\nready := true\n\nputs(count + 1)\nputs(ratio + 0.25)\nputs(language.size())\nputs(language.upcase())\nputs(ready)\nputs(\"42\".to_i())\nputs(count.to_s())\n",
		Expected:    "7\n1.75\n6\nTYPERB\ntrue\n42\n6\n",
		Hint:        "Try a different receiver method, or pass the wrong type to an operator to inspect its diagnostic.",
	},
	{
		ID: "bindings", Chapter: "Start", Title: "Bindings and mutability", Eyebrow: "03 · Start",
		Description: ":= infers an immutable binding. Add mut when rebinding or using a destructive operation; an uppercase name creates an immutable constant.",
		Source:      "MAX_STEPS := 3\nname := \"Ada\"\nmut score := 1\nmut tags := [\"typed\"]\n\nscore += 2\ntags.push(\"portable\")\n\nputs(name)\nputs(score)\nputs(MAX_STEPS)\nputs(tags)\n",
		Expected:    "Ada\n3\n3\n[\"typed\", \"portable\"]\n",
		Hint:        "Try assigning name = \"Grace\" or removing mut from tags to see the mutation rules.",
	},
	{
		ID: "functions", Chapter: "Write programs", Title: "Functions and decisions", Eyebrow: "04 · Write programs",
		Description: "Parameters and return values are checked before execution. if, elsif, and else branch on Boolean conditions in every target mode.",
		Source:      "def describe(score: Integer): String\n\tif score >= 10\n\t\treturn \"great\"\n\telsif score > 0\n\t\treturn \"keep going\"\n\telse\n\t\treturn \"start\"\n\tend\nend\n\nputs(describe(12))\nputs(describe(3))\nputs(describe(0))\n",
		Expected:    "great\nkeep going\nstart\n",
		Hint:        "Pass a String to describe, or use a non-Boolean if condition, to inspect static checking.",
	},
	{
		ID: "repetition", Chapter: "Write programs", Title: "Repetition", Eyebrow: "05 · Write programs",
		Description: "while handles stateful loops. Range values and iterator blocks provide portable traversal with next, break, and optional indexes.",
		Source:      "mut countdown := 3\nwhile countdown > 0\n\tputs(countdown)\n\tcountdown -= 1\nend\n\n(0...4).each.with_index do |value, index|\n\tif value == 1\n\t\tnext\n\tend\n\tif value == 3\n\t\tbreak\n\tend\n\tputs(index.to_s() + \": \" + value.to_s())\nend\nputs(\"loops complete\")\n",
		Expected:    "3\n2\n1\n0: 0\n2: 2\nloops complete\n",
		Hint:        "Change ... to .. to include the range endpoint, or move the break condition.",
	},
	{
		ID: "collections", Chapter: "Write programs", Title: "Collections", Eyebrow: "06 · Write programs",
		Description: "Array and Hash element types are inferred. map, select, and reduce transform values, while Hash fetch performs a checked key lookup.",
		Source:      "numbers := [1, 2, 3, 4]\nlabels := {\"language\" => \"TypeRB\"}\n\ndoubled := numbers.map do |number|\n\tnumber * 2\nend\neven := numbers.select do |number|\n\tnumber % 2 == 0\nend\ntotal := even.reduce(0) do |sum, number|\n\tsum + number\nend\n\nputs(doubled)\nputs(even)\nputs(total)\nputs(labels.fetch(\"language\"))\n",
		Expected:    "[2, 4, 6, 8]\n[2, 4]\n6\nTypeRB\n",
		Hint:        "Change the select predicate, or add another key and fetch it from labels.",
	},
	{
		ID: "records", Chapter: "Model data and errors", Title: "Records for data", Eyebrow: "07 · Model data",
		Description: "A record is a closed product type with keyword-only construction and checked fields. The same declaration can define data contracts across targets.",
		Source:      "record User\n\tname: String\n\tactive: Boolean\nend\n\nuser := User.new(name: \"Ada\", active: true)\nputs(user.name)\nputs(user.active)\n",
		Expected:    "Ada\ntrue\n",
		Hint:        "Remove active from the constructor, or give it a String value, to see complete-field checking.",
	},
	{
		ID: "classes", Chapter: "Model data and errors", Title: "Classes for behavior", Eyebrow: "08 · Model data",
		Description: "Classes combine declared fields and methods. readonly controls external assignment, underscore-prefixed members are private, and self methods belong to the class.",
		Source:      "class Counter\n\treadonly @label: String\n\t@_value: Integer\n\n\tdef initialize(label: String, value: Integer)\n\t\t@label = label\n\t\t@_value = value\n\t\treturn\n\tend\n\n\tdef increment(): Integer\n\t\tself._advance()\n\t\treturn @_value\n\tend\n\n\tdef _advance()\n\t\t@_value += 1\n\t\treturn\n\tend\n\n\tdef self.zero(label: String): Counter\n\t\treturn Counter.new(label, 0)\n\tend\nend\n\ncounter := Counter.zero(\"items\")\nputs(counter.label)\nputs(counter.increment())\nputs(counter.increment())\n",
		Expected:    "items\n1\n2\n",
		Hint:        "Try counter._advance() or counter.label = \"other\" to see private and readonly diagnostics.",
	},
	{
		ID: "result", Chapter: "Model data and errors", Title: "Enums and Result", Eyebrow: "09 · Model errors",
		Description: "Result<T, E> is a standard generic payload enum. Safe standard-library operations return structured errors, while exhaustive case matching keeps both paths visible.",
		Source:      "import { NumberParseError } from trb/std/errors\nimport trb/std/result\n\ndef display(result: Result<Integer, NumberParseError>): String\n\tcase result\n\twhen Result::Ok(value)\n\t\treturn \"number: \" + value.to_s()\n\twhen Result::Err(error)\n\t\treturn \"error: \" + error.message\n\tend\nend\n\nputs(display(\"42\".try_to_i()))\nputs(display(\"nope\".try_to_i()))\n",
		Expected:    "number: 42\nerror: invalid Integer\n",
		Hint:        "Define your own payload enum and remove one when branch to see exhaustive checking.",
	},
	{
		ID: "json", Chapter: "Model data and errors", Title: "JSON and typed codecs", Eyebrow: "10 · Model data",
		Description: "Portable JSON and JSONC packages return Result values. Typed codecs decode checked records without passing untyped maps through the application.",
		Source:      "import trb/std/json\nimport trb/std/result\n\nrecord User\n\tname: String\n\tactive: Boolean\nend\n\ndecoded: Result<User, JSON::Error> := JSON.decode<User>(\"{\\\"name\\\":\\\"Ada\\\",\\\"active\\\":true}\")\ncase decoded\nwhen Result::Ok(user)\n\tputs(user.name)\n\tputs(user.active)\nwhen Result::Err(error)\n\tputs(error.message)\nend\n",
		Expected:    "Ada\ntrue\n",
		Hint:        "Remove a JSON field to inspect its path-aware error, or import encode and serialize a User.",
	},
	{
		ID: "standard-library", Chapter: "Portability", Title: "Portable standard library", Eyebrow: "11 · Portability",
		Description: "Portable packages expose the same contracts in every mode. Bytes, Unicode, StringBuilder, and path cover common compiler and application work.",
		Source:      "import trb/std/path\nimport trb/std/string_builder\nimport trb/std/unicode\n\nmut builder := StringBuilder.from_string(\"Type\")\nbuilder.append(\"RB\")\nbuilder.append_codepoint(128512)\ntext := builder.to_s()\nbytes := text.to_bytes()\n\nputs(text)\nputs(bytes.size())\nputs(Unicode.letter(12354))\nputs(Path.clean(\"/docs/../tour\"))\n",
		Expected:    "TypeRB😀\n10\ntrue\n/tour\n",
		Hint:        "Try Unicode.from_codepoint(), Path.join(), or convert the Bytes value back with to_s().",
	},
	{
		ID: "targets", Chapter: "Portability", Title: "One language, three targets", Eyebrow: "12 · Portability",
		Description: "TypeRB keeps one grammar and portable meaning. Target mode selects code generation and packages; target-specific capabilities require explicit imports.",
		Source:      "import trb/std/io\n\nrecord Greeting\n\tlanguage: String\n\ttargets: String\nend\n\ndef message(greeting: Greeting): String\n\treturn greeting.language + \" targets \" + greeting.targets\nend\n\ngreeting := Greeting.new(language: \"TypeRB\", targets: \"Go, Ruby, and TypeScript\")\nIO.puts(message(greeting))\n",
		Expected:    "TypeRB targets Go, Ruby, and TypeScript\n",
		Hint:        "Switch Target and open Target code. The TypeRB source and result stay the same while generated code changes.",
	},
}

func Tour() []Lesson {
	return append([]Lesson(nil), tourLessons...)
}

// validateTour compiles and evaluates every lesson in every output mode.
func validateTour(ctx context.Context) (int, error) {
	count := 0
	for _, lesson := range tourLessons {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			result := Run(ctx, lesson.Source, mode)
			if !result.OK {
				message := "unknown failure"
				if len(result.Diagnostics) > 0 {
					message = result.Diagnostics[0].Message
				}
				return count, fmt.Errorf("lesson %s in mode %s: %s", lesson.ID, mode, message)
			}
			if result.Output != lesson.Expected {
				return count, fmt.Errorf("lesson %s in mode %s: expected output %q, got %q", lesson.ID, mode, lesson.Expected, result.Output)
			}
			count++
		}
	}
	return count, nil
}
