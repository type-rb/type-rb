package repl

import "testing"

func TestUTF8WithReplacementUsesMaximalSubparts(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{name: "valid", input: []byte("A😀"), want: "A😀"},
		{name: "encoded replacement character remains valid", input: []byte{0xef, 0xbf, 0xbd}, want: "�"},
		{name: "adjacent stray continuations", input: []byte{0x80, 0x80}, want: "��"},
		{name: "truncated three byte prefix", input: []byte{0xe2, 0x82}, want: "�"},
		{name: "truncated four byte prefix", input: []byte{0xf0, 0x9f, 0x98}, want: "�"},
		{name: "invalid continuation after lead", input: []byte{0xe2, 0x28, 0xa1}, want: "�(�"},
		{name: "overlong sequence", input: []byte{0xc0, 0xaf}, want: "��"},
		{name: "surrogate sequence", input: []byte{0xed, 0xa0, 0x80}, want: "���"},
		{name: "out of range sequence", input: []byte{0xf4, 0x90, 0x80, 0x80}, want: "����"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := utf8WithReplacement(test.input); got != test.want {
				t.Fatalf("utf8WithReplacement(%x) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}
