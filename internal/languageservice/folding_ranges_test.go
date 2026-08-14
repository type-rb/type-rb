package languageservice

import "testing"

func TestFoldingRangesReturnsNestedStructuralRegions(t *testing.T) {
	source := `module Accounts
	class User
		def label(): String
			if true
				return "yes"
			else
				return "no"
			end
		end
	end
end

formatter := fn(value: String): String
	return value
end

[1, 2].each do |value|
	puts(value)
end
`
	ranges := FoldingRanges(source)
	pairs := make([][2]int, 0, len(ranges))
	for _, item := range ranges {
		start := lineAtOffset(source, item.Range.Start)
		end := lineAtOffset(source, item.Range.End)
		pairs = append(pairs, [2]int{start, end})
	}
	want := [][2]int{{0, 10}, {1, 9}, {2, 8}, {3, 7}, {12, 14}, {16, 18}}
	if len(pairs) != len(want) {
		t.Fatalf("ranges=%v want=%v", pairs, want)
	}
	for index := range want {
		if pairs[index] != want[index] {
			t.Fatalf("ranges=%v want=%v", pairs, want)
		}
	}
}

func TestFoldingRangesRemainAvailableForIncompleteDocuments(t *testing.T) {
	source := "class User\n\tdef name(): Missing\n\tend\nend\n"
	ranges := FoldingRanges(source)
	if len(ranges) != 2 {
		t.Fatalf("ranges=%#v", ranges)
	}
}

func lineAtOffset(source string, offset int) int {
	line := 0
	for index := 0; index < offset && index < len(source); index++ {
		if source[index] == '\n' {
			line++
		}
	}
	return line
}
