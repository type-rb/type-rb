package compiler

import (
	"strings"
	"testing"
)

func TestNullableConditionalRejectsStaleFacts(t *testing.T) {
	for name, body := range map[string]string{
		"if":              "if flag\ntext = nil\nend",
		"elsif":           "if flag\nputs(1)\nelsif true\ntext = nil\nend",
		"else":            "if flag\nputs(1)\nelse\ntext = nil\nend",
		"nested":          "if flag\nif true\ntext = nil\nend\nend",
		"case":            "case 1\nwhen 1\ntext = nil\nelse\nputs(2)\nend",
		"expression":      "puts(if flag\ntext = nil\n1\nelse\n2\nend)",
		"case expression": "puts(case 1\nwhen 1\ntext = nil\n1\nelse\n2\nend)",
		"enum":            "case Choice::One\nwhen Choice::One\ntext = nil\nwhen Choice::Two\nputs(2)\nend",
		"union":           "input: Integer | String := 1\ncase input\nwhen Integer(number)\nputs(number)\ntext = nil\nwhen String(label)\nputs(label)\nend",
	} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				source := "enum Choice\nOne\nTwo\nend\ndef check(flag: Boolean)\nputs(flag)\nmut text: String? := nil\ntext = \"kept\"\n" + body + "\nputs(text.size())\nend\n"
				_, err := Compile("conditional.trb", []byte(source), mode)
				if err == nil || !strings.Contains(err.Error(), "type String? has no member size") {
					t.Fatalf("expected stale fact rejection, got %v", err)
				}
			})
		}
	}
}

func TestNullableConditionalRejectsReplacedFieldReceiver(t *testing.T) {
	source := []byte("record Cell\nvalue: String?\nend\ndef main()\nmut cell := Cell.new(value: \"kept\")\nif cell.value != nil\nif true\ncell = Cell.new(value: nil)\nend\nputs(cell.value.size())\nend\nend\n")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("conditional_field.trb", source, mode); err == nil || !strings.Contains(err.Error(), "type String? has no member size") {
			t.Fatalf("%s: expected replaced field rejection, got %v", mode, err)
		}
	}
}

func TestNullableConditionalKeepsBranchEntriesAndFreshGuards(t *testing.T) {
	source := []byte(`record Cell
value: String?
end
def size(mut text: String?): Integer
  if text == nil
    text = "ignored"
    return 0
  end
  return text.size()
end
def choose(flag: Boolean)
  mut text: String? := nil
  text = "kept"
  if flag
    puts(text.size())
    text = nil
  else
    puts(text.size())
  end
  puts(text == nil)
  if text != nil
    puts(text.size())
  end
  text = "again"
  puts(text.size())
end
def main()
  choose(true)
  choose(false)
  mut text: String? := nil
  text = "kept"
  puts(case 1
  when 1
    text = nil
    7
  else
    8
  end)
  puts(text == nil)
  mut cell := Cell.new(value: "kept")
  if cell.value != nil
    if true
      puts(cell.value.size())
      cell = Cell.new(value: nil)
    end
    puts(cell.value == nil)
  end
  puts(size("kept"))
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{Mode: mode, GoModule: "example.com/nullable-conditional", RubyLoader: "require_relative", SourceRoot: "/project", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			if got, want := runEffectProject(t, mode, artifacts, "example.com/nullable-conditional"), "4\ntrue\n5\n4\nfalse\n4\n5\n7\ntrue\n4\ntrue\n4\n"; got != want {
				t.Fatalf("output=%q, want %q", got, want)
			}
		})
	}
}

func TestNullableConditionalShadowingPreservesOuterFacts(t *testing.T) {
	source := []byte(`def main()
  mut text: String? := nil
  text = "kept"
  if true
    mut text: String? := nil
    text = "inner"
    puts(text.size())
  end
  puts(text.size())
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("conditional_shadowing.trb", source, mode); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
	}
}
