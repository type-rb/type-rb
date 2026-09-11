package bootstrapsnapshot

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSnapshotArrayIterationExecutesLiveAndLexicalSemantics(t *testing.T) {
	cases := []struct {
		name, body string
		want       int64
	}{
		{"live receiver and independent index", `mut values := [1, 2]
mut original := values
mut total := 0
values.each.with_index do |value, index|
 total += value + index
 if index == 0
  values.push(3)
  values[1] = 7
  values = [90]
 end
 index = 100
end
return total + original.size()`, 17},
		{"shadowed outer bindings", `mut value := 40
mut index := 50
mut total := 0
[1, 2].each.with_index do |value, index|
 value += 10
 index += 20
 total += value + index
end
return total + value + index`, 154},
		{"nested loop transfers", `mut total := 0
[1, 2, 3].each do |value|
 if value == 2
  next
 end
 [10, 20].each do |inner|
  total += value + inner
  break
 end
end
return total`, 24},
		{"mixed while and iteration transfers", `mut outer := 0
mut total := 0
while outer < 3
 outer += 1
 [1, 2, 3].each do |value|
  if value == 2
   next
  end
  mut inner := 0
  while inner < 3
   inner += 1
   if inner == 1
    next
   end
   total += value + outer
   break
  end
 end
 if outer == 2
  break
 end
end
return total`, 14},
		{"return from the method", `[1, 2, 3].each do |value|
 if value == 2
  return value
 end
end
return 0`, 2},
		{"empty source", `mut values: Array<Integer> := []
mut count := 8
values.each do |value|
 count += value
end
return count`, 8},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			source := "def probe(): Integer\n" + item.body + "\nend\ndef main()\nend\n"
			artifacts := analyzeV4Program(t, source)
			snapshot, err := BuildV4(artifacts, "/project/src")
			if err != nil {
				t.Fatal(err)
			}
			function := v4FunctionWithSuffix(t, snapshot.Functions, "#probe")
			if got := executeArraySnapshot(t, function); got != item.want {
				t.Fatalf("got %d, want %d", got, item.want)
			}
			first, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			again, err := BuildV4(artifacts, "/project/src")
			if err != nil {
				t.Fatal(err)
			}
			second, err := json.Marshal(again)
			if err != nil || string(first) != string(second) {
				t.Fatal("nondeterministic snapshot", err)
			}
		})
	}
}

func TestSnapshotArrayIterationCapturesAndManagedValues(t *testing.T) {
	source := `record Row
 text: String
end
alias Read = () -> String
def main()
 rows := [Row.new(text: "original")]
 text := "outer"
 action := fn(): String
  rows.each do |row|
   saved := fn(): String
    return row.text + text
   end
   ["replacement"].each do |text|
    text = "local"
   end
   return saved()
  end
  return text
 end
 puts(action())
end
`
	snapshot, err := BuildV4(analyzeV4Program(t, source), "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	outer := v4FunctionWithSuffix(t, snapshot.Functions, "#main$lambda0")
	if len(outer.Captures) != 2 {
		t.Fatalf("lost captures in iteration source/body: %#v", outer.Captures)
	}
	inner := v4FunctionWithSuffix(t, snapshot.Functions, "#main$lambda0$lambda0")
	if len(inner.Captures) != 2 {
		t.Fatalf("lost current record/String capture: %#v", inner.Captures)
	}
	var capturedRow bool
	for _, capture := range inner.Captures {
		if strings.Contains(capture.Type, "Row") {
			capturedRow = true
		}
	}
	if !capturedRow {
		t.Fatal("current managed element has no typed capture")
	}
}

func TestSnapshotArrayIterationKeepsUnsupportedBoundaries(t *testing.T) {
	for _, expression := range []string{"[1].each_slice(1) do |batch|\nputs(batch.size())\nend", "(0..2).each do |value|\nputs(value)\nend"} {
		_, err := BuildV4(analyzeV4Program(t, "def main()\n"+expression+"\nend\n"), "/project/src")
		if err == nil || !strings.Contains(err.Error(), "does not support") {
			t.Fatalf("accepted unsupported iteration: %v", err)
		}
	}
	artifacts := analyzeV4Program(t, "def main()\n[1].each do |value|\nputs(value)\nend\nend\n")
	if _, err := BuildV3(artifacts, "/project/src"); err == nil {
		t.Fatal("version 3 accepted Array iteration")
	}
	if _, err := BuildV2(artifacts, "/project/src"); err == nil {
		t.Fatal("version 2 accepted Array iteration")
	}
}

// Execute the data contract without referring to source names or the lowering
// implementation. Values are local to a block: edges must carry every live ID.
func executeArraySnapshot(t *testing.T, function FunctionV4) int64 {
	t.Helper()
	blocks := map[string]Block{}
	for _, block := range function.Blocks {
		blocks[block.ID] = block
	}
	block := blocks[function.Entry]
	values := map[string]any{}
	read := func(id string) any {
		value, ok := values[id]
		if !ok {
			t.Fatalf("unavailable value %s in %s", id, block.ID)
		}
		return value
	}
	edge := func(target string, arguments []string) {
		next, ok := blocks[target]
		if !ok || len(arguments) != len(next.Parameters) {
			t.Fatalf("invalid edge to %s", target)
		}
		incoming := map[string]any{}
		for i, argument := range arguments {
			incoming[next.Parameters[i].ID] = read(argument)
		}
		block, values = next, incoming
	}
	for budget := 0; budget < 1000; budget++ {
		for _, instruction := range block.Instructions {
			switch op := instruction.(type) {
			case IntegerLiteral:
				values[op.Result] = op.Value
			case BooleanLiteral:
				values[op.Result] = op.Value
			case ArrayConstruct:
				array := []any{}
				for _, id := range op.Arguments {
					array = append(array, read(id))
				}
				values[op.Result] = &array
			case ArraySize:
				values[op.Result] = int64(len(*read(op.Array).(*[]any)))
			case ArrayGet:
				values[op.Result] = (*read(op.Array).(*[]any))[read(op.Index).(int64)]
			case ArraySet:
				(*read(op.Array).(*[]any))[read(op.Index).(int64)] = read(op.Value)
			case ArrayPush:
				array := read(op.Array).(*[]any)
				*array = append(*array, read(op.Value))
			case BinaryInstruction:
				left, right := read(op.Left).(int64), read(op.Right).(int64)
				switch op.Operator {
				case "add":
					values[op.Result] = left + right
				case "subtract":
					values[op.Result] = left - right
				case "equal":
					values[op.Result] = left == right
				case "not_equal":
					values[op.Result] = left != right
				case "less_than":
					values[op.Result] = left < right
				case "less_than_or_equal":
					values[op.Result] = left <= right
				default:
					t.Fatalf("unsupported test operation: %#v", op)
				}
			default:
				t.Fatalf("unsupported test instruction: %s", reflect.TypeOf(op))
			}
		}
		switch term := block.Terminator.(type) {
		case Jump:
			edge(term.Target, term.Arguments)
		case Branch:
			if read(term.Condition).(bool) {
				edge(term.WhenTrue, term.TrueArguments)
			} else {
				edge(term.WhenFalse, term.FalseArguments)
			}
		case Return:
			if term.Value == nil {
				t.Fatal("missing Integer result")
			}
			return read(*term.Value).(int64)
		default:
			t.Fatalf("unsupported test terminator: %#v", term)
		}
	}
	t.Fatal("snapshot did not terminate within the test budget")
	return 0
}

func TestSnapshotArrayIterationEvaluatesReceiverOnlyBeforeTheLoop(t *testing.T) {
	source := `def choose(mut calls: Array<Integer>, values: Array<Integer>): Array<Integer>
 calls[0] += 1
 return values
end
def probe(): Integer
 mut calls := [0]
 mut sum := 0
 choose(calls, [1, 2]).each do |value|
  sum += value
 end
 return sum + calls[0]
end
def main()
end
`
	snapshot, err := BuildV4(analyzeV4Program(t, source), "/project/src")
	if err != nil {
		t.Fatal(err)
	}
	function := v4FunctionWithSuffix(t, snapshot.Functions, "#probe")
	count := 0
	for _, block := range function.Blocks {
		for _, instruction := range block.Instructions {
			if call, ok := instruction.(Call); ok && strings.HasSuffix(call.Function, "#choose") {
				count++
				if block.ID != function.Entry {
					t.Fatal("receiver evaluation moved into the loop")
				}
			}
		}
	}
	if count != 1 {
		t.Fatalf("receiver evaluated %d times in the snapshot", count)
	}
}
