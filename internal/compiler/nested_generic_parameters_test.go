package compiler

import (
	"strings"
	"testing"
)

func TestNestedGenericParametersRetainFollowingArgumentsAcrossModes(t *testing.T) {
	declaration := `def pick(rows: Array<Array<Integer>>, index: Integer): Integer
	return rows[index][0]
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := Compile("nested_parameters.trb", []byte(declaration+"def main()\n\tputs(pick([[7]], 0))\nend\n"), mode); err != nil {
				t.Fatal(err)
			}
			_, err := Compile("nested_parameters.trb", []byte(declaration+"def main()\n\tputs(pick([[7]], \"bad\"))\nend\n"), mode)
			if err == nil || !strings.Contains(err.Error(), "argument 2") || !strings.Contains(err.Error(), "expected Integer") {
				t.Fatalf("expected the following argument's type diagnostic, got %v", err)
			}
		})
	}
}
