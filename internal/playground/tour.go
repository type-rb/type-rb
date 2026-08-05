package playground

import (
	"context"
	"fmt"
)

type Lesson struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Eyebrow     string `json:"eyebrow"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Expected    string `json:"expected"`
	Hint        string `json:"hint"`
}

var tourLessons = []Lesson{
	{
		ID: "hello", Title: "Hello, TypeRB", Eyebrow: "01 · Start here",
		Description: "Every expression is parsed, type checked, lowered to typed IR, and then evaluated. Start with the portable puts function.",
		Source:      "puts(\"Hello, TypeRB!\")\n",
		Expected:    "Hello, TypeRB!\n",
		Hint:        "Change the message, then press Run or Cmd/Ctrl-Enter.",
	},
	{
		ID: "bindings", Title: "Bindings and inferred types", Eyebrow: "02 · Values",
		Description: ":= creates an immutable binding and infers its type. Receiver methods such as size are portable standard-library contracts.",
		Source:      "name := \"TypeRB\"\nletters := name.size()\nputs(name)\nputs(letters)\n",
		Expected:    "TypeRB\n6\n",
		Hint:        "Try assigning name = \"other\" to see the immutable-binding diagnostic.",
	},
	{
		ID: "functions", Title: "Typed functions", Eyebrow: "03 · Functions",
		Description: "Parameters and return values are checked before execution. Calls always use parentheses, and every function has an explicit return.",
		Source:      "def greet(name: String): String\n\treturn \"Hello, \" + name\nend\n\nputs(greet(\"Ada\"))\n",
		Expected:    "Hello, Ada\n",
		Hint:        "Pass an Integer to greet and inspect the compiler diagnostic.",
	},
	{
		ID: "collections", Title: "Collection transformations", Eyebrow: "04 · Collections",
		Description: "map is structured TypeRB syntax, not a target-language callback. Its result type is inferred from the block expression.",
		Source:      "numbers := [1, 2, 3, 4]\ndoubled := numbers.map do |number|\n\tnumber * 2\nend\nputs(doubled)\n",
		Expected:    "[2, 4, 6, 8]\n",
		Hint:        "Replace map with select and return a Boolean expression.",
	},
	{
		ID: "records", Title: "Records as data contracts", Eyebrow: "05 · Data",
		Description: "A record is a closed product type with keyword-only construction and checked fields. It can be shared across output modes.",
		Source:      "record User\n\tname: String\n\tactive: Boolean\nend\n\nuser := User.new(name: \"Ada\", active: true)\nputs(user.name)\nputs(user.active)\n",
		Expected:    "Ada\ntrue\n",
		Hint:        "Remove active from the constructor to see complete-field checking.",
	},
	{
		ID: "enums", Title: "Closed choices and payloads", Eyebrow: "06 · Control flow",
		Description: "Payload enums model closed alternatives. case binds payload values and must handle every member when there is no else branch.",
		Source:      "enum Status\n\tActive\n\tPaused(reason: String)\nend\n\ndef describe(status: Status): String\n\tcase status\n\twhen Status::Active\n\t\treturn \"active\"\n\twhen Status::Paused(reason)\n\t\treturn \"paused: \" + reason\n\tend\nend\n\nputs(describe(Status::Paused(\"maintenance\")))\n",
		Expected:    "paused: maintenance\n",
		Hint:        "Delete one when branch to see exhaustive-case checking.",
	},
	{
		ID: "result", Title: "Errors as values", Eyebrow: "07 · Result",
		Description: "Safe operations return Result<T, E>. Ordinary exhaustive matching keeps both success and failure paths visible.",
		Source:      "import { Result } from trb/std/result\n\ndef display(result: Result<Integer, String>): String\n\tcase result\n\twhen Result::Ok(value)\n\t\treturn \"number: \" + value.to_s()\n\twhen Result::Err(error)\n\t\treturn \"error: \" + error\n\tend\nend\n\nputs(display(\"42\".try_to_i()))\nputs(display(\"nope\".try_to_i()))\n",
		Expected:    "number: 42\nerror: invalid Integer\n",
		Hint:        "Change the input strings and compare the two Result variants.",
	},
}

func Tour() []Lesson {
	return append([]Lesson(nil), tourLessons...)
}

// ValidateTour compiles and evaluates every lesson in every output mode. It is
// intentionally exposed through `trb tour --check` rather than run as part of
// every ordinary project build.
func ValidateTour(ctx context.Context) (int, error) {
	count := 0
	for _, lesson := range tourLessons {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			result := evaluate(ctx, lesson.Source, mode)
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
