package cli

import "testing"

func TestLibraryResultsKeepOuterGenericParameters(t *testing.T) {
	source := `record Box
text: String
end
def duplicate<T>(items: Array<T>): Array<T>
return items.dup().reverse().slice(0...items.size()).reverse()
end
def edge<T>(items: Array<T>): T
return items.first()
end
def values<V>(items: Hash<String, V>): Array<V>
return items.dup().values()
end
def main()
boxes := [Box.new(text: "held")]
copied := duplicate<Box>(boxes)
puts(edge<Box>(copied).text)
puts(values<Integer>({"key" => 7})[0])
puts(duplicate<Integer>([3, 4])[1])
end
`
	runPortableExecutionCase(t, source, "held\n7\n4\n", "")
}

func TestLibraryResultsKeepNominalParameterName(t *testing.T) {
	source := `record T
value: Integer
end
def main()
values := [T.new(value: 9)]
puts(values.dup().first().value)
end
`
	runPortableExecutionCase(t, source, "9\n", "")
}
