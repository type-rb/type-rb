package compiler

import (
	"strings"
	"testing"
)

func TestTypeScriptIntrinsicsDoNotShadowValueBindings(t *testing.T) {
	source := []byte(`import { EnumValueError } from trb/std/errors
import { Result } from trb/std/result

enum Status
	Ready = "READY"
end

def render(value: Float): String
	return value.to_s()
end

def truncate(value: Float): Integer
	return value.to_i()
end

def remove_first(): Integer
	mut value := [1, 2, 3]
	return value.shift()
end

def parse_status(value: String): Result<Status, EnumValueError>
	return Status.from_raw(value)
end
`)
	artifact, err := Compile("value.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	if strings.Contains(output, "const value = value") {
		t.Fatalf("generated TypeScript shadows a source value binding:\n%s", output)
	}
	for _, want := range []string{
		`})(value)`,
		`const result = values.shift()`,
		`switch (value)`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated TypeScript does not contain %q:\n%s", want, output)
		}
	}
}
