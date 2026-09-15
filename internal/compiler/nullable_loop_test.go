package compiler

import (
	"strings"
	"testing"
)

func TestNullableLoopRejectsStaleFacts(t *testing.T) {
	tests := map[string]string{
		"body":              "while i < 2\nputs(text.size())\ntext = nil\ni += 1\nend",
		"condition":         "while text.size() > 0\ntext = nil\nend",
		"conditional":       "while i < 2\nputs(text.size())\nif i == 0\ntext = nil\nend\ni += 1\nend",
		"after loop":        "while i < 1\ntext = nil\ni += 1\nend\nputs(text.size())",
		"each":              "(0..1).each do |i|\nputs(i)\nputs(text.size())\ntext = nil\nend",
		"map":               "sizes := [0, 1].map do |i|\nsize := text.size() + i\ntext = nil\nsize\nend\nputs(sizes[0])",
		"expression branch": "while i < 2\nputs(text.size())\nputs(if i == 0\ntext = nil\n1\nelse\n2\nend)\ni += 1\nend",
	}
	for name, body := range tests {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				source := "def main()\nmut text: String? := \"hello\"\nif text != nil\nmut i := 0\n" + body + "\nputs(i)\nend\nend\n"
				_, err := Compile("nullable_loop.trb", []byte(source), mode)
				if err == nil || !strings.Contains(err.Error(), "type String? has no member size") {
					t.Fatalf("expected stale nullable receiver diagnostic, got %v", err)
				}
			})
		}
	}
}

func TestNullableLoopRejectsReplacedFieldReceiver(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		source := []byte("record Cell\nvalue: String?\nend\ndef main()\nmut cell := Cell.new(value: \"hello\")\nif cell.value != nil\nmut i := 0\nwhile i < 2\nputs(cell.value.size())\ncell = Cell.new(value: nil)\ni += 1\nend\nend\nend\n")
		if _, err := Compile("nullable_field_loop.trb", source, mode); err == nil || !strings.Contains(err.Error(), "type String? has no member size") {
			t.Fatalf("%s: expected replaced field receiver diagnostic, got %v", mode, err)
		}
	}
}

func TestNullableLoopPreservesFreshAndUnaffectedFacts(t *testing.T) {
	source := []byte(`def main()
  mut text: String? := "hello"
  while text != nil
    puts(text.size())
    text = nil
  end
  text = "again"
  [0, 1].each do |i|
    if text != nil
      puts(text.size() + i)
    end
    text = nil
  end
  stable: String? := "keep"
  if stable != nil
    mut i := 0
    while i < 2
      puts(stable.size())
      i += 1
    end
    puts(stable.size())
  end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{Mode: mode, GoModule: "example.com/nullable-loop", RubyLoader: "require_relative", SourceRoot: "/project", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			got := runEffectProject(t, mode, artifacts, "example.com/nullable-loop")
			if want := "5\n5\n4\n4\n4\n"; got != want {
				t.Fatalf("output=%q, want %q", got, want)
			}
		})
	}
}

func TestNullableLoopShadowingDoesNotEraseOuterFacts(t *testing.T) {
	source := []byte(`def main()
 stable: String? := "keep"
 if stable != nil
 mut i := 2
    values: Array<String?> := ["inner"]
    values.each do |stable|
      stable = nil
      puts(stable == nil)
    end
    while i < 3
      mut stable: String? := "local"
      stable = nil
      puts(stable == nil)
      i += 1
    end
    _callback := fn(mut stable: String?): String
      stable = nil
      return "unused"
    end
    puts(stable.size())
 end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("nullable_loop_shadowing.trb", source, mode); err != nil {
			t.Fatalf("%s rejected an unaffected outer binding: %v", mode, err)
		}
	}
}
