package formatter

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatPreservesCommentsAndIsIdempotent(t *testing.T) {
	source := []byte("# target\nclass  Post<ApplicationRecord\n# association\nbelongs_to :user # owner\ndef summary( limit:Integer=80 ):String\nvalue:=body.to_s() # keep me\nreturn value\nend\nend\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	text := string(formatted)
	for _, comment := range []string{"# target", "# association", "# owner", "# keep me"} {
		if !strings.Contains(text, comment) {
			t.Fatalf("comment %q was lost:\n%s", comment, text)
		}
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) > 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("formatter is not idempotent:\nfirst:\n%s\nsecond:\n%s\ndiags=%v", formatted, formattedAgain, diagnostics)
	}
}

func TestFormatPreservesHeredocBody(t *testing.T) {
	source := []byte("class Query\ndef sql():String\nreturn <<~SQL\n  SELECT  *\n+    FROM posts # SQL comment, not TypeRB\nSQL\nend\nend\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	if !strings.Contains(string(formatted), "  SELECT  *\n+    FROM posts # SQL comment, not TypeRB\nSQL") {
		t.Fatalf("heredoc body changed:\n%s", formatted)
	}
}

func TestFormatDistinguishesNamespaceAndTypedKeyword(t *testing.T) {
	source := []byte("class Admin::Post<ActiveRecord::Base\ndef configure(cache::Boolean=false)\nreturn cache\nend\nend\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	text := string(formatted)
	if !strings.Contains(text, "class Admin::Post < ActiveRecord::Base") || !strings.Contains(text, "cache:: Boolean = false") {
		t.Fatalf("namespace/keyword formatting is ambiguous:\n%s", text)
	}
}

func TestFormatPreservesRailsRegexAndPercentLiterals(t *testing.T) {
	source := []byte("class User<ApplicationRecord\nvalidates :code,format:{with:/\\A[a-z#]+\\z/i}\nTAGS=%(alpha beta)\nend\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	text := string(formatted)
	if !strings.Contains(text, `/\A[a-z#]+\z/i`) || !strings.Contains(text, `%(alpha beta)`) {
		t.Fatalf("Ruby literal contents changed:\n%s", text)
	}
}

func TestFormatKeepsExplicitImportPathsCompact(t *testing.T) {
	source := []byte("import  trb / std / io # portable output\nimport {User} from app / models / user\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	want := "import trb/std/io # portable output\nimport { User } from app/models/user\n"
	if string(formatted) != want {
		t.Fatalf("unexpected import formatting:\n%s", formatted)
	}
}

func TestFormatPortableIterationAndRanges(t *testing.T) {
	source := []byte("def values()\n[1,2,3].each do |value,index| # header\nputs(value) # body\nend\n(0 .. 10).each{|value| puts(value)}\nreturn\nend\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	want := "def values()\n  [1, 2, 3].each do |value, index| # header\n    puts(value) # body\n  end\n  (0..10).each { |value| puts(value) }\n  return\nend\n"
	if string(formatted) != want {
		t.Fatalf("unexpected iteration formatting:\n%s", formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) > 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("iteration formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}
