#!/usr/bin/env python3
"""Verify buffered construction, immutable snapshots, aliases and GC ownership."""
import argparse
import os
from pathlib import Path
import re
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument("binary", type=Path)
parser.add_argument("--reference", type=Path)
args = parser.parse_args()
native = args.binary.resolve()
reference = args.reference.resolve() if args.reference else None
env=dict(os.environ,NO_COLOR='1',TERM='dumb',TRBN_QBE=str(native.with_name("qbe")))
def run(args, data=None, success=True, env=env):
    result = subprocess.run(list(map(str, args)), input=data, env=env,
                            text=True, capture_output=True, timeout=180)
    assert (result.returncode == 0) == success, (args, result.returncode, result.stderr, result.stdout)
    return result

with tempfile.TemporaryDirectory(prefix="native-builder-") as temporary:
    tmp = Path(temporary)
    env["TRBN_HISTORY"] = str(tmp / "history")
    cases = [
        (
            'boundaries',
            """import trb/std/string_builder

def extend(mut writer: StringBuilder)
	writer.append("!")
end

def main()
	mut writer := StringBuilder.from_string("A")
	mut shared := writer
	writer.append("\\xf0\\x9f")
	puts(writer.size())
	early := writer.to_s()
	shared.append("\\x98\\x80")
	extend(shared)
	snapshot := writer.to_s()
	puts(snapshot)
	puts(writer.size())
	puts(early.to_bytes().size())
	writer.clear()
	puts(writer.empty?())
	writer.append_codepoint(12354)
	writer.append_codepoint(0)
	writer.append("Z")
	puts(writer.size())
	puts(writer.to_s().to_bytes().size())
	puts(snapshot)
	puts(shared.to_s() == writer.to_s())
	mut separate := StringBuilder.new()
	separate.append("x")
	puts(writer.size())
end
""",
            """3
A😀!
3
3
true
3
5
A😀!
true
3
""",
        ),
        (
            'growth',
            """import trb/std/string_builder

record Held
	writer: StringBuilder
	text: String
end

def update(mut writer: StringBuilder, count: Integer)
	mut i := 0
	while i < count
		writer.append("a😀")
		i += 1
	end
end

def main()
	mut writer := StringBuilder.new()
	update(writer, 1024)
	snapshot := writer.to_s()
	held := Held.new(writer: writer, text: snapshot)
	mut writers := [writer]
	mut shared := writers[0]
	update(shared, 1024)
	puts(held.text.size())
	puts(writer.size())
	puts(held.writer.to_s().to_bytes().size())
	writer.clear()
	writer.append("after")
	puts(held.text.size())
	puts(held.writer.to_s())
	mut retained: Array<String> := []
	mut i := 0
	while i < 512
		mut item := StringBuilder.from_string("saved-")
		item.append(i.to_s())
		retained.push(item.to_s())
		item.clear()
		item.append("changed")
		i += 1
	end
	puts(retained[0])
	puts(retained[511])
end
""",
            """2048
4096
10240
2048
after
saved-0
saved-511
""",
        ),
        (
            'reuse',
            """import trb/std/string_builder

def main()
	mut writer := StringBuilder.new()
	mut shared := writer
	mut saved: Array<String> := []
	mut index := 0
	while index < 256
		writer.clear()
		writer.append("a😀")
		writer.append(index.to_s())
		saved.push(shared.to_s())
		shared.clear()
		shared.append("Z")
		index += 1
	end
	puts(saved[0])
	puts(saved[255])
	puts(writer.to_s())
	puts(writer.size())
	writer.clear()
	puts(shared.empty?())
	puts(shared.to_s().size())
end
""",
            "a😀0\na😀255\nZ\n1\ntrue\n0\n",
        ),
        (
            'ascii-codepoints',
            """import trb/std/string_builder

def main()
	mut writer := StringBuilder.new()
	mut shared := writer
	mut saved: Array<String> := []
	mut point := 0
	while point < 128
		writer.clear()
		writer.append_codepoint(point)
		saved.push(shared.to_s())
		shared.clear()
		shared.append_codepoint(127 - point)
		point += 1
	end
	mut checksum := 0
	point = 0
	while point < 128
		bytes := saved[point].to_bytes()
		checksum += bytes.at(0)
		if bytes.size() != 1 || bytes.at(0) != point
			puts("mismatch")
		end
		point += 1
	end
	puts(checksum)
	puts(writer.to_s().to_bytes().at(0))
	writer.clear()
	writer.append_codepoint(128)
	writer.append_codepoint(1114111)
	puts(writer.size())
	puts(writer.to_s().to_bytes().size())
end
""",
            "8128\n0\n2\n6\n",
        ),
        (
            'display',
            """import trb/std/string_builder
def show(value: Any)
puts(value)
end
def main()
mut writer := StringBuilder.from_string("text")
puts(writer)
show(writer)
writer.append("!")
show(writer)
writer.clear()
puts(writer.to_s().to_bytes().size())
puts(writer)
end
""",
            """text
text
text!
0

""",
        ),
    ]
    for name, code, expected in cases:
        source = tmp / (name + ".trb")
        source.write_text(code)
        assert run([native, "check", source]).stdout == "ok\n"
        program = tmp / name
        run([native, "build", "--compile", source, "--outfile", program])
        result = run([program], env=dict(env, TYPE_RB_NATIVE_RUNTIME_STATS="1"))
        assert result.stdout == expected, (name, result.stdout, expected)
        statistics = {line.split(",")[1]: int(line.split(",")[2])
                      for line in result.stderr.splitlines()
                      if line.startswith("type-rb-native-gc-stat-v1,")}
        assert statistics["live-bytes"] == 0
        assert statistics["allocated-bytes"] == statistics["reclaimed-bytes"]
        if reference:
            assert run([reference, "run", source]).stdout == expected
        emitted = run([native, "--internal-driver", "emit-qbe", source]).stdout
        for function in ["trbn_builder_new", "trbn_builder_append",
                         "trbn_builder_append_codepoint", "trbn_builder_to_string"]:
            match = re.search(r"^function(?: l)? \$" + function + r"\([^\n]*\) \{.*?^\}",
                              emitted, re.M | re.S)
            assert match, function
            emitted = emitted.replace(match[0], match[0].replace(
                "@start\n", "@start\n call $trbn_gc_collect(w 0)\n", 1))
        il = tmp / "forced.ssa"
        il.write_text(emitted)
        assembly = tmp / "forced.s"
        run([env["TRBN_QBE"], "-o", assembly, il])
        run(["cc", assembly, "-lm", "-o", tmp / "forced"])
        assert run([tmp / "forced"]).stdout == expected
    # Clearing an unusually large buffer must not keep that storage alive.
    # Inject a collection at the real clear boundary while the ordinary caller
    # retains the builder; this checks traced ownership rather than host RSS.
    for length in [0, 1, 64, 65, 4096]:
        source = tmp / "clear-retention.trb"
        source.write_text('import trb/std/string_builder\ndef main()\n'
                          'mut writer := StringBuilder.new()\n'
                          'writer.append("' + 'x' * length + '")\n'
                          'writer.clear()\nputs(writer.empty?())\nend\n')
        emitted = run([native, "--internal-driver", "emit-qbe", source]).stdout
        function = re.search(r"^function \$trbn_builder_clear\([^\n]*\) \{.*?^\}",
                             emitted, re.M | re.S)
        assert function
        limit = 256 if length <= 64 else 64
        hook = (" call $trbn_gc_collect(w 0)\n"
                " %retained_bytes =l loadl $trbn_gc_heap_bytes\n"
                f" %bounded =w culel %retained_bytes, {limit}\n"
                " jnz %bounded, @retention_ok, @retention_failed\n"
                "@retention_failed\n call $abort()\n hlt\n@retention_ok\n")
        changed = function[0].replace(" ret\n", hook + " ret\n")
        assert changed != function[0]
        il = tmp / "clear-retention.ssa"
        il.write_text(emitted.replace(function[0], changed))
        run([env["TRBN_QBE"], "-o", tmp / "clear-retention.s", il])
        run(["cc", tmp / "clear-retention.s", "-lm", "-o", tmp / "clear-retention"])
        assert run([tmp / "clear-retention"]).stdout == "true\n"

    # The private span operation is inaccessible to authored source. Alter only
    # one already checked application's call to exercise its defensive bounds.
    source = tmp / "span.trb"
    source.write_text('import trb/std/string_builder\ndef main()\n'
                      'mut writer := StringBuilder.new()\nwriter.append("abc")\nend\n')
    original = run([native, "--internal-driver", "emit-qbe", source]).stdout
    function = next(match for match in re.finditer(
        r"^function[^\n]*\$trbnf[0-9]+\([^\n]*\) \{.*?^\}", original, re.M | re.S)
                    if "call $trbn_builder_append(" in match[0])
    call = re.search(r"call \$trbn_builder_append\((l [^,]+), (l [^)]+)\)", function[0])
    assert call
    for start, finish in [(-1, 2), (0, 4), (2, 1), (0, 9007199254740991)]:
        replacement = f"call $trbn_builder_append_bytes({call[1]}, {call[2]}, l {start}, l {finish})"
        modified = original.replace(function[0], function[0].replace(call[0], replacement))
        il = tmp / "span.ssa"
        il.write_text(modified)
        run([env["TRBN_QBE"], "-o", tmp / "span.s", il])
        run(["cc", tmp / "span.s", "-lm", "-o", tmp / "span"])
        assert "byte range is out of bounds" in run([tmp / "span"], success=False).stderr
    for body in ['writer := StringBuilder.new()\nwriter.append("x")',
                 'mut writer := StringBuilder.new()\nwriter.append(7)',
                 'mut writer := StringBuilder.new()\nwriter.size(1)']:
        source = tmp / "rejected.trb"
        source.write_text("import trb/std/string_builder\ndef main()\n" + body + "\nend\n")
        run([native, "check", source], success=False)
    for point in [-1, 55296, 57343, 1114112]:
        source = tmp / "invalid.trb"
        source.write_text("import trb/std/string_builder\ndef main()\n"
                          "mut writer := StringBuilder.new()\n"
                          f"writer.append_codepoint({point})\nend\n")
        run([native, "build", "--compile", source, "--outfile", tmp / "invalid"])
        result = run([tmp / "invalid"], success=False)
        assert "invalid Unicode code point" in result.stderr
    commands='''import trb/std/string_builder
mut writer := StringBuilder.from_string("A")
mut shared := writer
writer.append("😀")
shared.append_codepoint(33)
writer
writer.size()
snapshot := writer.to_s()
writer.clear()
writer.empty?()
writer.to_s()
writer.append("new")
snapshot
shared.to_s()
:quit
'''
    want='StringBuilder("A") : StringBuilder [mut]\nStringBuilder("A") : StringBuilder [mut]\nStringBuilder("A😀!") : StringBuilder [mut]\n3 : Integer\n"A😀!" : String\ntrue : Boolean\n"" : String\n"A😀!" : String\n"new" : String\n'
    value=run([native,'repl'],data=commands)
    assert value.stdout==want,(value.stdout,want)

print("PASS StringBuilder snapshots, shared mutation, Unicode, growth, GC and retained REPL")
