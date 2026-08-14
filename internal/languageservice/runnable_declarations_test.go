package languageservice

import "testing"

func TestRunnableDeclarationsFindsOnlyPortableMain(t *testing.T) {
	source := `def helper()
	return
end

class Application
	def main()
		return
	end
end

def main()
	missing()
	return
end
`
	items := RunnableDeclarations(source)
	if len(items) != 1 || items[0].Kind != RunnableDeclarationMain {
		t.Fatalf("runnable declarations=%#v", items)
	}
	if got := source[items[0].Range.Start:items[0].Range.End]; got != "main" {
		t.Fatalf("runnable range=%q", got)
	}
}

func TestRunnableDeclarationsRejectsInvalidMainSignatures(t *testing.T) {
	for _, source := range []string{
		"def main(value: String)\n\treturn\nend\n",
		"def main<T>()\n\treturn\nend\n",
		"def main(): String\n\treturn \"main\"\nend\n",
		"def main() fails Error\n\treturn\nend\n",
		"def self.main()\n\treturn\nend\n",
	} {
		if items := RunnableDeclarations(source); len(items) != 0 {
			t.Fatalf("source %q produced runnable declarations %#v", source, items)
		}
	}
}
