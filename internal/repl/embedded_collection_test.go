package repl

import "testing"

func TestCompleteEmbeddedCollectionBlocks(t *testing.T) {
	for _, source := range []string{
		"puts([1].map do |item|\nif item > 0\nitem\nelse\n0\nend\nend)",
		"def main()\nputs([1].map do |item|\nitem\nend)\nend",
		"puts([1].map do |item|; item; end)",
		"puts([[1].map do |item|\nitem\nend])",
	} {
		if !Complete(source) {
			t.Errorf("complete submission rejected: %s", source)
		}
	}
	for _, source := range []string{
		"def main()\nputs([1].map do |item|\nitem\nend)",
		"puts([1].map do |item|\nif item > 0\nitem\nend",
		"puts([1].map do |item|\nitem\nend",
	} {
		if Complete(source) {
			t.Errorf("incomplete submission accepted: %s", source)
		}
	}
}
