package cli

import "testing"

func TestReduceFailureStopsLaterEvaluationAcrossExecutionPaths(t *testing.T) {
	for _, test := range []struct {
		name, sourceBody, initialBody, want string
	}{
		{"source", "values: Array<Integer> := []\nreturn [values[0]]", "return 0", "source\n"},
		{"initial", "return [1, 2]", "values: Array<Integer> := []\nreturn values[0]", "source\ninitial\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := "def source(): Array<Integer>\nputs(\"source\")\n" + test.sourceBody + "\nend\n" +
				"def initial(): Integer\nputs(\"initial\")\n" + test.initialBody + "\nend\n" +
				`def main()
total := source().reduce(initial()) do |sum, item|
puts("block")
sum + item
end
puts(total)
end
`
			runPortableExecutionCase(t, source, test.want, "out of bounds")
		})
	}
}
