package compiler

import (
	"strings"
	"testing"
)

func TestGenericApplicationsRespectLexicalShadowingAcrossModes(t *testing.T) {
	declaration := `def identity<T>(value: T): T
	return value
end
`
	cases := []struct {
		name   string
		source string
	}{
		{"local", "def main()\nidentity := 1\nputs(identity<Integer>(7))\nend\n"},
		{"parameter", "def use(identity: Integer): Integer\nreturn identity<Integer>(7)\nend\n"},
		{"block parameter", "def main()\n[1].each do |identity|\nputs(identity<Integer>(7))\nend\nend\n"},
		{"enclosing local", "def main()\nidentity := 1\nif true\nputs(identity<Integer>(7))\nend\nend\n"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				_, err := Compile("generic_shadow.trb", []byte(declaration+test.source), mode)
				if err == nil || !strings.Contains(err.Error(), "local value identity does not accept type arguments") {
					t.Fatalf("%s: expected lexical shadow diagnostic, got %v", mode, err)
				}
			}
		})
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		source := declaration + `def main()
	[1].each do |identity|
		puts(identity)
	end
	puts(identity<Integer>(7))
	puts(identity<String>("held"))
end
`
		if _, err := Compile("generic_scope.trb", []byte(source), mode); err != nil {
			t.Fatalf("%s: generic declaration must remain visible outside the shadowing scope: %v", mode, err)
		}
	}
}
