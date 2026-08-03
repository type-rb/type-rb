package repl

import "testing"

func TestCompleteTracksBlocksAndDelimiters(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"1 + 2", true},
		{"class User", false},
		{"class User\nend", true},
		{"if true\n  1", false},
		{"if true\n  1\nend", true},
		{"call(\n  1,", false},
		{"call(\n  1,\n)", true},
		{"# class does not open a block", true},
		{"\"class\"", true},
	}
	for _, test := range tests {
		if actual := Complete(test.source); actual != test.want {
			t.Errorf("Complete(%q)=%v, want %v", test.source, actual, test.want)
		}
	}
}
