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
	want := "def values()\n\t[1, 2, 3].each do |value, index| # header\n\t\tputs(value) # body\n\tend\n\t(0..10).each { |value| puts(value) }\n\treturn\nend\n"
	if string(formatted) != want {
		t.Fatalf("unexpected iteration formatting:\n%s", formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) > 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("iteration formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatPortableCollectionTransformations(t *testing.T) {
	source := []byte("def values():Array<Integer>\nmapped := [1,2].map do |value| # map\nvalue*2 # result\nend\nreturn mapped.select.with_index{|value,index| value>index}\nend\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	want := "def values(): Array<Integer>\n\tmapped := [1, 2].map do |value| # map\n\t\tvalue * 2 # result\n\tend\n\treturn mapped.select.with_index { |value, index| value > index }\nend\n"
	if string(formatted) != want {
		t.Fatalf("unexpected collection-transformation formatting:\n%s", formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) > 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("collection-transformation formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatEnumCaseUsesTabsAndPreservesComments(t *testing.T) {
	source := []byte("enum  State # enum\nOpen # open\nClosed\nend\ndef label(value:State):String\ncase value # select\nwhen State::Open # branch\nreturn \"open\" # result\nwhen State::Closed\nreturn \"closed\"\nend\nend\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	want := "enum State # enum\n\tOpen # open\n\tClosed\nend\ndef label(value: State): String\n\tcase value # select\n\twhen State::Open # branch\n\t\treturn \"open\" # result\n\twhen State::Closed\n\t\treturn \"closed\"\n\tend\nend\n"
	if string(formatted) != want {
		t.Fatalf("unexpected enum/case formatting:\n%s", formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) > 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("enum/case formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatCaseExpressionUsesCanonicalIndentationAndPreservesComments(t *testing.T) {
	source := []byte("enum State\nOpen\nClosed\nend\ndef label(value:State):String\nresult:=case value # select\nwhen State::Open # open\n\"open\" # result\nwhen State::Closed\n\"closed\"\nend\nreturn result\nend\n")
	want := "enum State\n\tOpen\n\tClosed\nend\ndef label(value: State): String\n\tresult := case value # select\n\twhen State::Open # open\n\t\t\"open\" # result\n\twhen State::Closed\n\t\t\"closed\"\n\tend\n\treturn result\nend\n"
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected case expression formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("case expression formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatIfExpressionUsesCanonicalIndentationAndPreservesComments(t *testing.T) {
	source := []byte("def label(enabled:Boolean):String\nresult:=if enabled # choose\n\"on\" # yes\nelsif false\n\"never\"\nelse # fallback\n\"off\"\nend\nreturn result\nend\n")
	want := "def label(enabled: Boolean): String\n\tresult := if enabled # choose\n\t\t\"on\" # yes\n\telsif false\n\t\t\"never\"\n\telse # fallback\n\t\t\"off\"\n\tend\n\treturn result\nend\n"
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected if expression formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("if expression formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatPayloadEnumAndPatternBindings(t *testing.T) {
	source := []byte("enum  Token # token\nText(value:String) # text\nPair(left:Integer,right:Integer)\nEOF\nend\ndef render(value:Token):String\ncase value\nwhen Token::Text(text) # bind\nreturn text\nwhen Token::Pair(left,right)\nreturn \"pair\"\nwhen Token::EOF\nreturn \"eof\"\nend\nend\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	want := "enum Token # token\n\tText(value: String) # text\n\tPair(left: Integer, right: Integer)\n\tEOF\nend\ndef render(value: Token): String\n\tcase value\n\twhen Token::Text(text) # bind\n\t\treturn text\n\twhen Token::Pair(left, right)\n\t\treturn \"pair\"\n\twhen Token::EOF\n\t\treturn \"eof\"\n\tend\nend\n"
	if string(formatted) != want {
		t.Fatalf("unexpected payload enum formatting:\n%s", formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) > 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("payload enum formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatExplicitUserGenerics(t *testing.T) {
	source := []byte("enum Result<T,E>\nOk(value:T)\nErr(error:E)\nend\ndef identity<T>(value:T):T\nreturn value\nend\ndef use():Result<Integer,String>\nnames:=identity<Array<String>>([\"Ada\"])\nreturn Result<Integer,String>::Ok(identity<Integer>(1))\nend\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	want := "enum Result<T, E>\n\tOk(value: T)\n\tErr(error: E)\nend\ndef identity<T>(value: T): T\n\treturn value\nend\ndef use(): Result<Integer, String>\n\tnames := identity<Array<String>>([\"Ada\"])\n\treturn Result<Integer, String>::Ok(identity<Integer>(1))\nend\n"
	if string(formatted) != want {
		t.Fatalf("unexpected generic formatting:\n%s", formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) > 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("generic formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatExpandsStatementSeparatorsAndPreservesNestedSemicolons(t *testing.T) {
	source := []byte("enum State; Open; Closed; end # enum\n" +
		"def label(value:State):String; case value; when State::Open; return \"a;b\"; when State::Closed; return \"closed\"; end; end\n" +
		"value:=1; next_value:=2 # second\n" +
		"value; # following\n" +
		"[1,2].each { |item| puts(item); puts(item+1) } # inline\n")
	want := "enum State\n" +
		"\tOpen\n" +
		"\tClosed\n" +
		"end # enum\n" +
		"def label(value: State): String\n" +
		"\tcase value\n" +
		"\twhen State::Open\n" +
		"\t\treturn \"a;b\"\n" +
		"\twhen State::Closed\n" +
		"\t\treturn \"closed\"\n" +
		"\tend\n" +
		"end\n" +
		"value := 1\n" +
		"next_value := 2 # second\n" +
		"value\n" +
		"# following\n" +
		"[1, 2].each { |item| puts(item); puts(item + 1) } # inline\n"

	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected separator formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) > 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("separator formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatPreservesLoopControlAndComments(t *testing.T) {
	source := []byte("def scan()\nwhile true\nnext # skip\nbreak # stop\nend\nreturn\nend\n")
	want := "def scan()\n\twhile true\n\t\tnext # skip\n\t\tbreak # stop\n\tend\n\treturn\nend\n"
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected loop control formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
}

func TestFormatTypedHashAndNestedGenericsPreservesComments(t *testing.T) {
	source := []byte("def configure()\nmut values:Hash<String,Hash<String,Integer>>:={primary:{}} # nested\nvalues[\"primary\"][\"count\"]=1 # update\nreturn\nend\n")
	want := "def configure()\n\tmut values: Hash<String, Hash<String, Integer>> := { primary: {} } # nested\n\tvalues[\"primary\"][\"count\"] = 1 # update\n\treturn\nend\n"
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected Hash formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) > 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("Hash formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatUnionTypesAndPatterns(t *testing.T) {
	source := []byte("def describe(value:Integer|String):String\ncase value\nwhen Integer(number)\nreturn number.to_s()\nwhen String(text)\nreturn text\nend\nend\n")
	want := "def describe(value: Integer | String): String\n\tcase value\n\twhen Integer(number)\n\t\treturn number.to_s()\n\twhen String(text)\n\t\treturn text\n\tend\nend\n"
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected union formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
}
