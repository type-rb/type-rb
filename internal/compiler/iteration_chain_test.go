package compiler

import "testing"

func TestIterationBlockPostfixChainsCompileAcrossModes(t *testing.T) {
	source := []byte(`def mapped_size(ids: Array<Integer>): Integer
	count := ids.map do |id|
		id * 2
	end.size()
	return count
end

def concurrent_total(ids: Array<Integer>): Integer
	return ids.concurrent_map do |id|
		id * 2
	end.reduce(0) do |sum, value|
		sum + value
	end
end

def brace_total(ids: Array<Integer>): Integer
	return ids.map { |id| id * 2 }.reduce(0) do |sum, value|
		sum + value
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("iteration_chain.trb", source, mode); err != nil {
			t.Fatalf("%s rejected iteration block postfix chains: %v", mode, err)
		}
	}
}
