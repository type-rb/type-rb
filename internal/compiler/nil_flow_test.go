package compiler

import "testing"

func TestAssignedNilRetainsAbsenceAcrossBackends(t *testing.T) {
	source := []byte(`def absent(value: String?): Boolean
  return value == nil
end
def missing(): String?
  mut result: String? := "initial"
  result = nil
  return result
end
def main()
  mut text: String? := "hello"
  text = nil
  puts(text == nil)
  puts(nil != text)
  puts(absent(text))
  puts(missing() == nil)
  text = "again"
  puts(text.size())
  values: Array<String?> := ["inner"]
  values.each do |text|
    text = nil
    puts(text == nil)
  end
  puts(text.size())
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{Mode: mode, GoModule: "example.com/nil-flow", RubyLoader: "require_relative", SourceRoot: "/project", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			got := runEffectProject(t, mode, artifacts, "example.com/nil-flow")
			if want := "true\nfalse\ntrue\ntrue\n5\ntrue\n5\n"; got != want {
				t.Fatalf("output=%q, want %q", got, want)
			}
		})
	}
}
