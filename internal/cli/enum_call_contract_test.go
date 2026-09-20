package cli

import "testing"

func TestEnumSafeCallsKeepNilAndArgumentLazinessAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, `enum Status
Ready = "ready"
def label(value: String): String
return self.raw_value() + value
end
end
def select(present: Boolean): Status?
puts("receiver")
if present
return Status::Ready
end
return nil
end
def argument(): String
puts("argument")
return "!"
end
def raw(value: Status?): String?
return value&.raw_value()
end
def label(present: Boolean): String?
return select(present)&.label(argument())
end
def main()
puts(label(false) == nil)
present := label(true)
if present != nil
puts(present)
end
puts(raw(nil) == nil)
text := raw(Status::Ready)
if text != nil
puts(text)
end
end
`, "receiver\ntrue\nreceiver\nargument\nready!\ntrue\nready\n", "")
}

func TestEnumMethodReturningCallableAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, `enum State
Ready
def reader(prefix: String): () -> String
return fn(): String; return prefix + "held"; end
end
end
def main()
read := State::Ready.reader("value:")
puts(read())
end
`, "value:held\n", "")
}

func TestRawEnumTypeAliasClassCallsAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, `enum State
Ready = "ready"
end
alias Status = State
def main()
case Status.from_raw("ready")
when Result::Ok(value)
puts(value.raw_value())
when Result::Err(_error)
puts("unexpected")
end
end
`, "ready\n", "")
}

func TestEnumReceiverBeforeExplicitArgumentsAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, `enum State
Ready
def label(value: String): String
return value
end
end
def receiver(): State
puts("receiver")
return State::Ready
end
def argument(): String
puts("argument")
return "value"
end
def main()
puts(receiver().label(argument()))
end
`, "receiver\nargument\nvalue\n", "")
}

func TestImportedEnumSafeCallsAndCallableResultsAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"state.trb": `enum State
Ready = "ready"
def reader(prefix: String): () -> String
return fn(): String; return prefix + self.raw_value(); end
end
def report(value: String)
puts(value)
end
end
alias Status = State
`,
		"main.trb": `import { Status } from state
def argument(): String
puts("argument")
return "reported"
end
def report(value: Status?)
value&.report(argument())
end
def main()
case Status.from_raw("ready")
when Result::Ok(value)
read := value.reader("value:")
puts(read())
report(nil)
report(value)
when Result::Err(_error)
puts("unexpected")
end
end
`,
	}, "value:ready\nargument\nreported\n", "")
}

func TestPlainEnumRawMembersRejectedAcrossTargetsAndREPL(t *testing.T) {
	for _, expression := range []string{"State.from_raw(1)", "puts(State::Ready.raw_value())"} {
		t.Run(expression, func(t *testing.T) {
			runPortableExecutionCase(t, "enum State\nReady\nend\ndef main()\n"+expression+"\nend\n", "", "has no member")
		})
	}
}

func TestNestedRawEnumAliasChainAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"codes.trb": `module Codes
enum Level
Low = -1
High = 3
end
alias Selected = Level
alias Status = Selected
end
`,
		"main.trb": `import codes as Wire
def main()
case Wire::Status.from_raw(-1)
when Result::Ok(value)
puts(value.raw_value())
when Result::Err(_error)
puts("unexpected")
end
end
`,
	}, "-1\n", "")
}
