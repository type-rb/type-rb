package compiler

import (
	"strings"
	"testing"
)

func TestTopLevelBindingsRemainVisibleAcrossMethodBoundaries(t *testing.T) {
	source := []byte(`mut count: Integer := 3
label := "outer"
items := [7]
callback: (Integer) -> Integer := fn(value: Integer): Integer
  return value + count
end

def change(): Integer
  count += 2
  return count
end

def shadow(count: Integer): Integer
  return count + 1
end

class Reader
  def read(): Integer
    return count
  end
end

def main()
  saved := fn(): Integer; return count; end
  puts(label)
  puts(items[0])
  puts(change())
  puts(saved())
  puts(callback(4))
  puts(shadow(10))
  puts(Reader.new().read())
  count := 17
  puts(count)
  puts(saved())
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "global_bindings.trb", source, "outer\n7\n5\n5\n9\n11\n5\n17\n5")
		})
	}
}

func TestTopLevelBindingsKeepTheirSourceModuleIdentity(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			sources := []SourceUnit{
				{Filename: "/project/left.trb", ModulePath: "left", Source: []byte("mut value := 11\ndef read_left(): Integer\nvalue += 1\nreturn value\nend\n")},
				{Filename: "/project/right.trb", ModulePath: "right", Source: []byte("value := 23\ndef read_right(): Integer\nreturn value\nend\n")},
				{Filename: "/project/main.trb", ModulePath: "main", Source: []byte("import { read_left } from left\nimport { read_right } from right\ndef main()\nputs(read_left())\nputs(read_right())\nputs(read_left())\nend\n")},
			}
			options := Options{Mode: mode, GoModule: "example.com/bindings", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"}
			artifacts, err := CompileProject(sources, options)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, options.GoModule)); got != "12\n23\n13" {
				t.Fatalf("independent module storage: %q", got)
			}
		})
	}
}
