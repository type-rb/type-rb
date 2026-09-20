package repl

import "testing"

func TestCompleteSymbolKeywordBoundaries(t *testing.T) {
	for _, source := range []string{
		"value := :if", "puts(:case)", "value := :fn", "value := :catch", "value := :do",
		"def label(value: String = :if): String\nreturn :if\nend",
		"def label(): String\nreturn :if if true\nreturn :end\nend",
		"values := {if: :case, end: :if}",
		"values := {\nwhile: 1,\nend: 2\n}",
		"value := (true ? :if : :case)",
		"puts([1].map { |item| :if + item.to_s() })",
		"puts([1].map do |item|\n:if + item.to_s()\nend)",
	} {
		if !Complete(source) {
			t.Errorf("complete Symbol expression rejected: %s", source)
		}
	}
	for _, source := range []string{
		"values := {label: if true\n:if\nelse\n:case",
		"identity(value: if true\n:if\nelse\n:case",
		"def label(): String\nreturn :if",
		"puts([1].map do |item|\n:if + item.to_s()",
	} {
		if Complete(source) {
			t.Errorf("open syntax hidden by a Symbol: %s", source)
		}
	}
}
