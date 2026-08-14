package languageservice

import "testing"

func TestSelectionRangesBuildTokenLineAndStructuralParents(t *testing.T) {
	source := "class User\n\tdef name(): String\n\t\tvalue := \"Ada\"\n\t\tputs(value)\n\tend\nend\n"
	cursor := 0
	for index := 0; index < len(source); index++ {
		if source[index:] == "value)\n\tend\nend\n" {
			cursor = index + 1
			break
		}
	}
	ranges := SelectionRanges(source, []int{cursor})
	if len(ranges) != 1 {
		t.Fatalf("ranges=%#v", ranges)
	}
	want := []string{"value", "\t\tputs(value)", "def name(): String\n\t\tvalue := \"Ada\"\n\t\tputs(value)\n\tend", "class User\n\tdef name(): String\n\t\tvalue := \"Ada\"\n\t\tputs(value)\n\tend\nend", source}
	current := &ranges[0]
	for index, expected := range want {
		if current == nil {
			t.Fatalf("selection chain stopped at %d", index)
		}
		if selected := source[current.Range.Start:current.Range.End]; selected != expected {
			t.Fatalf("selection %d=%q want=%q", index, selected, expected)
		}
		current = current.Parent
	}
	if current != nil {
		t.Fatalf("unexpected extra parent=%#v", current)
	}
}

func TestSelectionRangesClampCursorsAndIncludeComments(t *testing.T) {
	source := "# comment\n"
	ranges := SelectionRanges(source, []int{2, len(source) + 10})
	if len(ranges) != 2 || source[ranges[0].Range.Start:ranges[0].Range.End] != "# comment" {
		t.Fatalf("ranges=%#v", ranges)
	}
	if ranges[1].Range != (OffsetRange{Start: 0, End: len(source)}) {
		t.Fatalf("clamped range=%#v", ranges[1])
	}
}
