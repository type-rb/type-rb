package compiler

import (
	"strings"
	"testing"
)

func TestUnionPrivateFieldsAreCheckedBeforeMemberProjection(t *testing.T) {
	for _, declarations := range []string{
		"class First\n@_value: String := \"first\"\nend\nclass Second\n@_value: String := \"second\"\nend\n",
		"record First\n_value: String\nend\nrecord Second\n_value: String\nend\n",
		"class First<T>\n@_value: T\ndef initialize(value: T)\n@_value = value\nend\nend\nalias FirstValue = First<String>\nclass Second\n@_value: String := \"second\"\nend\n",
	} {
		first := "First"
		if strings.Contains(declarations, "FirstValue") {
			first = "FirstValue"
		}
		source := declarations + "def access(value: " + first + " | Second): String\nreturn value._value\nend\n"
		for _, mode := range []string{"go", "ruby", "typescript"} {
			_, err := Compile("private_union.trb", []byte(source), mode)
			if err == nil || !strings.Contains(err.Error(), "private member _value cannot be accessed externally") {
				t.Fatalf("%s accepted private common field: %v\n%s", mode, err, source)
			}
		}
	}
}
