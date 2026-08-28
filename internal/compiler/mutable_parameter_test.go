package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

func TestParametersAreImmutableByDefaultAcrossBackends(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "reassignment",
			source: `def advance(value: Integer): Integer
	value += 1
	return value
end
`,
			want: "value is immutable; declare it with mut to use assignment",
		},
		{
			name: "destructive operation",
			source: `def append(values: Array<Integer>)
	values.push(2)
	return
end
`,
			want: "values is immutable; declare it with mut to use push()",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				_, err := Compile("immutable_parameter.trb", []byte(test.source), mode)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q, got %v", mode, test.want, err)
				}
			}
		})
	}
}

func TestExplicitMutableParametersCompileAcrossBackends(t *testing.T) {
	source := []byte(`def advance(mut value: Integer, *, mut amount: Integer = 1): Integer
	value += amount
	amount = 0
	return value + amount
end

def append(mut values: Array<Integer>): Array<Integer>
	values.push(2)
	return values
end

def main()
	puts(advance(1, amount: 2).to_s())
	puts(append([1]).size().to_s())
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		artifact, err := Compile("mutable_parameter.trb", source, mode)
		if err != nil {
			t.Fatalf("%s rejected mutable parameters: %v", mode, err)
		}
		method, ok := artifact.IR.Statements[0].(*ir.Method)
		if !ok || len(method.Parameters) != 2 || !method.Parameters[0].Mutable || !method.Parameters[1].Mutable {
			t.Fatalf("%s did not retain parameter mutability in typed IR: %#v", mode, artifact.IR.Statements[0])
		}
	}
}

func TestFunctionValueParametersAreImmutableByDefault(t *testing.T) {
	immutable := []byte(`def main()
	callable := fn(value: Integer): Integer
		value += 1
		return value
	end
	puts(callable(1).to_s())
	return
end
`)
	mutable := []byte(`def main()
	callable := fn(mut value: Integer): Integer
		value += 1
		return value
	end
	puts(callable(1).to_s())
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("immutable_fn_parameter.trb", immutable, mode); err == nil || !strings.Contains(err.Error(), "value is immutable; declare it with mut to use assignment") {
			t.Fatalf("%s accepted immutable fn parameter reassignment: %v", mode, err)
		}
		if _, err := Compile("mutable_fn_parameter.trb", mutable, mode); err != nil {
			t.Fatalf("%s rejected mutable fn parameter: %v", mode, err)
		}
	}
}

func TestParameterMutabilityIsNotPartOfCallableIdentity(t *testing.T) {
	source := []byte(`interface Counter
	advance(value: Integer): Integer
end

class MutableCounter implements Counter
	def advance(mut value: Integer): Integer
		value += 1
		return value
	end
end

class BaseCounter
	def advance(value: Integer): Integer
		return value
	end
end

class ChildCounter < BaseCounter
	def advance(mut value: Integer): Integer
		value += 1
		return value
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("mutable_callable_identity.trb", source, mode); err != nil {
			t.Fatalf("%s treated parameter mutability as callable identity: %v", mode, err)
		}
	}
}

func TestRejectMutableParametersWithoutImplementationBindings(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "interface",
			source: `interface Counter
	advance(mut value: Integer): Integer
end
`,
			want: "interface parameters cannot be declared with mut",
		},
		{
			name: "enum payload",
			source: `enum Change
	Updated(mut value: Integer)
end
`,
			want: "enum payload fields cannot be declared with mut",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"go", "ruby", "typescript"} {
				_, err := Compile("invalid_mutable_parameter.trb", []byte(test.source), mode)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("%s: expected %q, got %v", mode, test.want, err)
				}
			}
		})
	}
}
