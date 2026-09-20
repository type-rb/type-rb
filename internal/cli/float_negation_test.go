package cli

import (
	"strings"
	"testing"
)

func TestFloatNegationPreservesSignedZeroAcrossBackendsAndREPL(t *testing.T) {
	source := `record Value
number: Float
end
def negative(): Float
return -0.0
end
def early_return(early: Boolean): Float
value := -(if early
return 7.0
else
0.0
end)
return value
end
def main()
values := [-0.0, -(0.0), -0.00, negative(), Value.new(number: -0.0).number]
values.each { |value| puts((1.0 / value) < 0.0) }
puts((1.0 / -(-0.0)) > 0.0)
puts((1.0 / values.uniq()[0]) < 0.0)
puts((1.0 / (-0.0).abs()) > 0.0)
puts((-(0.0 / 0.0)).nan?())
puts(-(1.0 / 0.0) < 0.0)
mut calls := 0
zero := fn(): Float
calls += 1
return 0.0
end
puts((1.0 / -zero()) < 0.0)
puts(calls == 1)
puts(early_return(true) == 7.0)
puts((1.0 / early_return(false)) < 0.0)
end
`
	runPortableExecutionCase(t, source, strings.Repeat("true\n", 14), "")
}
