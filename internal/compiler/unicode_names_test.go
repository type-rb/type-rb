package compiler

import (
	"strings"
	"testing"
)

func TestGoUnicodeBindingsPreserveExactSpelling(t *testing.T) {
	source := []byte(`def combine(Å: Integer, Å: Integer): Integer
  return Å * 10 + Å
end
def main()
  Å := 1
  Å := 2
  å := 3
  Σ := 4
  σ := 5
  x__trb_unicode_c385 := 6
  saved := fn(): Integer; return Å + Å + å + Σ + σ; end
  puts(combine(Å, Å))
  puts(saved())
  puts(x__trb_unicode_c385)
end
`)
	runEffectSource(t, "go", "unicode_bindings.trb", source, "12\n15\n6")
}

func TestGoUnicodeRecordFieldsAndMethodsPreserveExactSpelling(t *testing.T) {
	source := []byte(`record Pair
  å: Integer
  Å: Integer
end
class Counter
  def å(): Integer
    return 3
  end
  def Å(): Integer
    return 4
  end
end
def main()
  pair := Pair.new(å: 1, Å: 2)
  counter := Counter.new()
  puts(pair.å)
  puts(pair.Å)
  puts(counter.å())
  puts(counter.Å())
end
`)
	runEffectSource(t, "go", "unicode_members.trb", source, "1\n2\n3\n4")
}

func TestGoUnicodeDeclarationsRemainUsableAcrossPackages(t *testing.T) {
	requireEffectRuntime(t, "go")
	units := []SourceUnit{
		{Filename: "/project/domain/values.trb", ModulePath: "domain/values", Package: "domain", Source: []byte(`record Box日本
  値: Integer
end
enum Choice日本
  Some値(中身: Box日本)
end
def 足す(値: Integer, *, 増分: Integer = 3): Integer
  return 値 + 増分
end
`)},
		{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { Box日本, Choice日本, 足す as 加える } from domain/values
def main()
  値 := Box日本.new(値: 加える(2, 増分: 4))
  選択 := Choice日本::Some値(値)
  case 選択
  when Choice日本::Some値(中身)
    puts(中身.値)
  end
  puts(加える(1))
end
`)},
	}
	options := Options{Mode: "go", GoModule: "example.com/unicode-names", ProjectRoot: "/project", SourceRoot: "/project"}
	artifacts, err := CompileProject(units, options)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(runEffectProject(t, "go", artifacts, options.GoModule)); got != "6\n4" {
		t.Fatalf("Unicode imported declarations: %q", got)
	}
}
