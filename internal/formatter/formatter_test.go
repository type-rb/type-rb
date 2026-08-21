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

func TestFormatRejectsInvalidUTF8WithoutProducingOutput(t *testing.T) {
	source := append([]byte("value := 1\n"), 0xff)
	formatted, diagnostics := Format(source)
	if formatted != nil {
		t.Fatalf("formatter produced invalid output: %q", formatted)
	}
	if len(diagnostics) != 1 || diagnostics[0].Message != "source is not valid UTF-8" {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestFormatFunctionValuesAndSemicolonForm(t *testing.T) {
	source := []byte("double:(Integer)->Integer:=fn(value:Integer):Integer; return value*2; end # closure\n")
	want := "double: (Integer) -> Integer := fn(value: Integer): Integer\n\treturn value * 2\nend # closure\n"
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected fn formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("fn formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatPreservesStructuredJSXAndIsIdempotent(t *testing.T) {
	source := []byte("import { ReactNode } from trb/platform/typescript/react\ndef Card(props:CardProps):ReactNode\nreturn <article className=\"card\">\n\t<h2>{props.title}</h2>\n</article> # card\nend\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	text := string(formatted)
	for _, expected := range []string{
		"def Card(props: CardProps): ReactNode",
		"\treturn <article className=\"card\">\n\t<h2>{props.title}</h2>\n</article> # card",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("formatted JSX is missing %q:\n%s", expected, text)
		}
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) > 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("JSX formatting is not idempotent:\nfirst:\n%s\nsecond:\n%s\ndiags=%v", formatted, formattedAgain, diagnostics)
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

func TestFormatFollowsChainedMultilineTokensToTheirFinalLine(t *testing.T) {
	source := []byte("'\n''\n'E")
	want := "'\n' '\n' E\n"
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected multiline token formatting\nwant:\n%q\ngot:\n%q", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("multiline token formatting is not idempotent:\nfirst:\n%q\nsecond:\n%q\ndiagnostics=%v", formatted, formattedAgain, diagnostics)
	}
}

func TestFormatKeepsSymbolColonAfterOperatorIdempotent(t *testing.T) {
	source := []byte("value|:active")
	want := "value | :active\n"
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected symbol formatting\nwant:\n%q\ngot:\n%q", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("symbol formatting is not idempotent:\nfirst:\n%q\nsecond:\n%q\ndiagnostics=%v", formatted, formattedAgain, diagnostics)
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

func TestFormatKeepsNullableTypeMarkersAttached(t *testing.T) {
	source := []byte("record User\nnickname:String ?\nend\ndef find_name():String ?\nreturn nil\nend\n")
	want := "record User\n\tnickname: String?\nend\ndef find_name(): String?\n\treturn nil\nend\n"

	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected nullable type formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("nullable type formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatTransparentGenericTypeAlias(t *testing.T) {
	source := []byte("type  DbResult < T > =Result<T,DbError> # database result\n")
	want := "type DbResult<T> = Result<T, DbError> # database result\n"

	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected type alias formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
}

func TestFormatResultSignaturesAndPropagation(t *testing.T) {
	source := []byte("def load():Result<String,LoadError> # result\nvalue:=try read() # propagate\nreturn Result::Ok(value)\nend\n")
	want := "def load(): Result<String, LoadError> # result\n\tvalue := try read() # propagate\n\treturn Result::Ok(value)\nend\n"

	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected Result formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("Result formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatTryAndCatchExpressions(t *testing.T) {
	source := []byte("def load()\nvalue:=try read_value() # propagate\nrecovered:=read_value() catch | error | # recover\nreturn recover(error)\nend\nreturn recovered\nend\n")
	want := "def load()\n\tvalue := try read_value() # propagate\n\trecovered := read_value() catch |error| # recover\n\t\treturn recover(error)\n\tend\n\treturn recovered\nend\n"

	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected try/catch formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("try/catch formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatCatchAfterCallBlock(t *testing.T) {
	source := []byte("def store()\nresult:=Database.transaction() do | tx | # transaction\ntry save(tx)\nend catch | error | # rollback complete\nreturn recover(error)\nend\nreturn result\nend\n")
	want := "def store()\n\tresult := Database.transaction() do |tx| # transaction\n\t\ttry save(tx)\n\tend catch |error| # rollback complete\n\t\treturn recover(error)\n\tend\n\treturn result\nend\n"

	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected call-block catch formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("call-block catch formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatCatchAfterMultilineCall(t *testing.T) {
	source := []byte("def save()\nsaved:=row.with(\nvalue:1,\n).save() catch | error |\nreturn recover(error)\nend\nreturn saved\nend\n")
	want := "def save()\n\tsaved := row.with(\n\t\tvalue: 1,\n\t).save() catch |error|\n\t\treturn recover(error)\n\tend\n\treturn saved\nend\n"

	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected multiline-call catch formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("multiline-call catch formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatResultFunctionValues(t *testing.T) {
	source := []byte("loader:()->Result<String,LoadError>:=fn():Result<String,LoadError>; return read(); end\n")
	want := "loader: () -> Result<String, LoadError> := fn(): Result<String, LoadError>\n\treturn read()\nend\n"

	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected fallible function formatting\nwant:\n%s\ngot:\n%s", want, formatted)
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
	source := []byte("import  trb / std / io # portable output\nimport {User} from app / models / user\nimport {Widget} from app / widgets / index\nimport {\nFirst,\nSecond,\n} from domain / insurer\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	want := "import trb/std/io # portable output\nimport { User } from app/models/user\nimport { Widget } from app/widgets/index\nimport {\n\tFirst,\n\tSecond,\n} from domain/insurer\n"
	if string(formatted) != want {
		t.Fatalf("unexpected import formatting:\n%s", formatted)
	}
}

func TestFormatCanonicalizesResolvedImportPathsAndPreservesComments(t *testing.T) {
	source := []byte("import { DataTable } from shared / ui / DataTable / index # directory entry\nimport { User } from models / user / index # ambiguous\n")
	formatted, diagnostics := FormatWithOptions(source, Options{CanonicalImportPath: func(path string) string {
		if path == "shared/ui/DataTable/index" {
			return "shared/ui/DataTable"
		}
		return path
	}})
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	want := "import { DataTable } from shared/ui/DataTable # directory entry\nimport { User } from models/user/index # ambiguous\n"
	if string(formatted) != want {
		t.Fatalf("unexpected canonical import formatting:\n%s", formatted)
	}
	formattedAgain, diagnostics := FormatWithOptions(formatted, Options{CanonicalImportPath: func(path string) string { return path }})
	if len(diagnostics) > 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("canonical import formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
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

func TestFormatStructuredBlockValues(t *testing.T) {
	source := []byte("def process()\nresult:=Product.find_each(batch_size:100) do |product| # batch\nputs(product)\nend\nreturn Product.find_in_batches() do |products|\nputs(products)\nend\nend\n")
	want := "def process()\n\tresult := Product.find_each(batch_size: 100) do |product| # batch\n\t\tputs(product)\n\tend\n\treturn Product.find_in_batches() do |products|\n\t\tputs(products)\n\tend\nend\n"

	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected structured block formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("structured block formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}

func TestFormatPortableCollectionTransformations(t *testing.T) {
	source := []byte("def values():Array<Integer>\nmapped := [1,2].map do |value| # map\nvalue*2 # result\nend\nordered := mapped.sort_by(){|value| -value}\nall_positive := ordered.all?(){|value| value>0}\nfound := ordered.find(){|value| value>1}\nputs(all_positive)\nputs(found)\nreturn ordered.select.with_index{|value,index| value>index}\nend\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	want := "def values(): Array<Integer>\n\tmapped := [1, 2].map do |value| # map\n\t\tvalue * 2 # result\n\tend\n\tordered := mapped.sort_by() { |value| - value }\n\tall_positive := ordered.all?() { |value| value > 0 }\n\tfound := ordered.find() { |value| value > 1 }\n\tputs(all_positive)\n\tputs(found)\n\treturn ordered.select.with_index { |value, index| value > index }\nend\n"
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

func TestFormatLiteralCaseAlternatives(t *testing.T) {
	source := []byte("def label(value:String):String\nreturn case value\nwhen \"receipts\",\"receipt_detail\" # shared\n\"receipts\"\nelse\n\"other\"\nend\nend\n")
	want := "def label(value: String): String\n\treturn case value\n\twhen \"receipts\", \"receipt_detail\" # shared\n\t\t\"receipts\"\n\telse\n\t\t\"other\"\n\tend\nend\n"
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected literal case formatting\nwant:\n%s\ngot:\n%s", want, formatted)
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

func TestFormatRawValueEnumMethodsAndComments(t *testing.T) {
	source := []byte("enum  OrderStatus # status\nPending=\"PENDING\" # pending\nCompleted = \"COMPLETED\"\ndef terminal?():Boolean # terminal\nreturn self==OrderStatus::Completed\nend\nend\n")
	want := "enum OrderStatus # status\n\tPending = \"PENDING\" # pending\n\tCompleted = \"COMPLETED\"\n\tdef terminal?(): Boolean # terminal\n\t\treturn self == OrderStatus::Completed\n\tend\nend\n"
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected raw enum formatting:\n%s", formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || string(formattedAgain) != want {
		t.Fatalf("raw enum formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
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

func TestFormatGenericClassesAndRecords(t *testing.T) {
	source := []byte("class Box<T>\n@value:T\ndef pair<U>(other:U):Pair<T,U>\nreturn Pair<T,U>.new(left:@value,right:other)\nend\nend\nrecord Pair<T,U>\nleft:T\nright:U\nend\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	want := "class Box<T>\n\t@value: T\n\tdef pair<U>(other: U): Pair<T, U>\n\t\treturn Pair<T, U>.new(left: @value, right: other)\n\tend\nend\nrecord Pair<T, U>\n\tleft: T\n\tright: U\nend\n"
	if string(formatted) != want {
		t.Fatalf("unexpected generic object formatting:\n%s", formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) > 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("generic object formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
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

func TestFormatLiteralTypesAndDiscriminantCase(t *testing.T) {
	source := []byte("record Response\nstatus:201\nkind:\"created\"\nend\ndef show(response:Response):String\ncase response.status\nwhen 201\nreturn response.kind\nend\nend\n")
	want := "record Response\n\tstatus: 201\n\tkind: \"created\"\nend\ndef show(response: Response): String\n\tcase response.status\n\twhen 201\n\t\treturn response.kind\n\tend\nend\n"
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if string(formatted) != want {
		t.Fatalf("unexpected literal type formatting\nwant:\n%s\ngot:\n%s", want, formatted)
	}
	formattedAgain, diagnostics := Format(formatted)
	if len(diagnostics) > 0 || !bytes.Equal(formatted, formattedAgain) {
		t.Fatalf("literal type formatting is not idempotent:\n%s\ndiags=%v", formattedAgain, diagnostics)
	}
}
