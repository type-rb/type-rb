#!/usr/bin/env python3
"""Exercise the built CLI through public commands and a real terminal."""
import argparse
from compiler_sources import stage_sources
import errno
import json
import os
from pathlib import Path
import pty
import select
import signal
import shutil
import subprocess
import sys
import tempfile
import time

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
args = parser.parse_args()
binary = args.binary.resolve()
repository = Path(__file__).resolve().parent.parent
subprocess.run([sys.executable, str(repository / "tools/native-format-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-package-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-official-import-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-web-binding-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-json-codec-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-web-input-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-time-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-web-context-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-encoding-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-web-route-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-web-middleware-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-web-openapi-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-web-client-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-failure-recovery-test.py"), "--qbe", str(binary.with_name("qbe"))], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-repl-flow-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-enum-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-raw-enum-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-generic-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-result-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-builtin-values-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-integer-parsing-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-float-parsing-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-alias-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-callable-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-constants-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-global-bindings-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-repl-global-bindings-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-namespace-bindings-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-array-copy-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-array-mutation-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-array-query-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-array-sorting-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-keyed-array-sorting-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-safe-block-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-sliced-iteration-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-array-join-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-string-trimming-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-string-transforms-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-symbol-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-newtype-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-path-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-object-methods-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-literal-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-union-hash-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-scalar-strings-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-repl-display-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-union-test.py"), str(binary)], check=True)
subprocess.run([sys.executable, str(repository / "tools/native-hash-range-test.py"), str(binary)], check=True)

with tempfile.TemporaryDirectory(prefix='native cli ') as temporary:
    root = Path(temporary)
    env = dict(os.environ, TRBN_HISTORY=str(root / 'history.json'), TERM='xterm', NO_COLOR='1')

    def run(*arguments, text=None, cwd=root, success=True):
        result = subprocess.run([str(binary), *map(str, arguments)], input=text,
                                text=True, capture_output=True, cwd=cwd, env=env,
                                timeout=30)
        if (result.returncode == 0) != success:
            raise AssertionError((arguments, result.returncode, result.stdout, result.stderr))
        return result.stdout + result.stderr

    # Exercise internal pool cells through an ordinary Native-built consumer.
    # This is a lifetime fixture, not authored fn/REPL closure acceptance.
    cell_source = root / 'binding-source'
    cell_source.mkdir()
    stage_sources([repository / 'compiler/src', repository / 'compiler/cli'], cell_source)
    probe = cell_source / 'binding_probe.trb'
    probe.write_text('''import { compiler_new } from state/state
import { callable_type } from frontend/types/callable_types
import { repl_store, repl_environment, repl_integer, repl_string, repl_value, repl_activate } from repl/model
import { repl_bind, repl_capture_binding, repl_binding_value, repl_assign_binding, repl_compact } from repl/values

def probe(): Boolean
mut store := repl_store()
mut environment := repl_environment()
first := repl_integer(store, 1)
before := store.types.size()
repl_bind(environment, "value", first, 1)
if store.types.size() != before || environment.cells[0] != 0
return false
end
cell := repl_capture_binding(store, environment, 0)
if repl_capture_binding(store, environment, 0) != cell
return false
end
repl_assign_binding(store, environment, 0, repl_integer(store, 2))
if store.integers[repl_binding_value(store, environment, 0)] != 2 || store.integers[store.children[cell][0]] != 2
return false
end
managed := repl_value(store, "Array<String>", 0, 0.0, "", [repl_string(store, "kept")])
repl_assign_binding(store, environment, 0, managed)
carrier := repl_value(store, "$Capture", 0, 0.0, "", [cell, cell])
environment.count[0] = 0
repl_bind(environment, "replacement", repl_integer(store, 100), 1)
if environment.cells[0] != 0 || repl_capture_binding(store, environment, 0) == cell
return false
end
repl_bind(environment, "escaped", carrier, 0)
cycle := repl_value(store, "$Cycle", 0, 0.0, "", [])
store.children[cycle].push(cycle)
store.children[cycle].push(carrier)
repl_bind(environment, "cycle", cycle, 0)
mut iteration := repl_environment()
mut captured: Array<Integer> := []
mut index := 0
while index < 3
iteration.count[0] = 0
repl_bind(iteration, "item", repl_integer(store, index), 1)
captured.push(repl_capture_binding(store, iteration, 0))
index += 1
end
repl_bind(environment, "iterations", repl_value(store, "$Capture", 0, 0.0, "", captured), 0)
index = 0
while index < 25
repl_string(store, "unreachable temporary")
repl_assign_binding(store, environment, 0, repl_integer(store, index))
store = repl_compact(store, environment)
if store.integers[repl_binding_value(store, environment, 0)] != index || store.types.size() > 16
return false
end
saved := repl_binding_value(store, environment, 1)
if store.children[saved][0] != store.children[saved][1]
return false
end
retained := store.children[store.children[saved][0]][0]
if store.texts[store.children[retained][0]] != "kept"
return false
end
retained_cycle := repl_binding_value(store, environment, 2)
if store.children[retained_cycle][0] != retained_cycle || store.children[retained_cycle][1] != saved
return false
end
items := store.children[repl_binding_value(store, environment, 3)]
mut item := 0
while item < 3
if store.integers[store.children[items[item]][0]] != item
return false
end
item += 1
end
index += 1
end
return true
end

def program_probe(): Boolean
mut store := repl_store()
mut environment := repl_environment()
repl_activate(store, compiler_new(2))
closure := repl_value(store, callable_type([], "Integer"), 0, 0.0, "", [])
repl_bind(environment, "kept", closure, 0)
mut index := 0
while index < 12
repl_activate(store, compiler_new(2))
repl_value(store, callable_type([], "Integer"), 0, 0.0, "", [])
store = repl_compact(store, environment)
if store.programs.size() != 2 || store.types.size() != 2
return false
end
if store.value_programs[repl_binding_value(store, environment, 0)] == store.active_program[0]
return false
end
index += 1
end
environment.count[0] = 0
store = repl_compact(store, environment)
return store.programs.size() == 1 && store.types.size() == 1
end

def main()
puts(probe())
puts(program_probe())
end
''')
    cell_binary = root / 'binding-probe'
    run('build', '--compile', '--outfile', cell_binary, probe)
    assert subprocess.check_output([cell_binary], text=True, timeout=30) == 'true\ntrue\n'

    # These sources exercise the checked compiler and the independent REPL evaluator.
    for case_name in ('elsif-control', 'elsif-managed', 'loop-transfer-control',
                      'loop-transfer-effects', 'loop-transfer-managed', 'array-assignment-targets',
                      'array-assignment-managed', 'array-assignment-recovery',
                      'boolean-array-values', 'boolean-array-effects', 'boolean-array-managed',
                      'record-field-values', 'record-array-values',
                      'record-array-effects', 'record-array-managed', 'local-array-header-mir',
                      'stable-array-bindings', 'conditional-array-headers', 'loop-array-headers',
                      'hash-values', 'hash-managed', 'hash-cycles',
                      'array-iteration-live', 'array-iteration-control', 'array-iteration-managed',
                      'iteration-brace-blocks',
                      'logical-loop-stack',
                      'range-values', 'range-carriers', 'range-extrema', 'range-effects',
                      'range-precedence', 'range-managed', 'range-streaming',
                      'string-index-lifetime', 'default-arguments-mir',
                      'value-control-mir', 'value-transfer-mir', 'nullable-values-mir',
                      'namespace-binding-storage',
                      'builtin-result-values', 'unicode-identifier-values',
                      'boolean-array-depth', 'record-array-depth'):
        fixture = repository / 'compiler/conformance/valid' / (case_name + '.trb')
        expected = fixture.with_suffix('.out').read_text()
        case_source = root / (case_name + '.trb')
        case_source.write_text(fixture.read_text())
        assert run('check', case_source) == 'ok\n'
        case_output = root / case_name
        run('build', '--compile', '--outfile', case_output, case_source)
        assert subprocess.check_output([case_output], text=True, timeout=30) == expected
        if case_name in ('elsif-managed', 'loop-transfer-managed', 'array-assignment-managed',
                         'boolean-array-managed', 'record-array-managed', 'hash-managed', 'hash-cycles',
                         'array-iteration-managed', 'range-managed', 'string-index-lifetime'):
            collected = subprocess.run([case_output], text=True, capture_output=True,
                                       env=dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1'), timeout=30)
            assert collected.returncode == 0 and collected.stdout == expected
            automatic = [line.split(',')[-1] for line in collected.stderr.splitlines()
                         if line.startswith('type-rb-native-gc-stat-v1,automatic-collections,')]
            assert len(automatic) == 1 and int(automatic[0]) > 0, collected.stderr
        submission = fixture.read_text().replace('def main()', 'def exercise_control_case()')
        # Interactive top-level bindings display their initialized value.
        expected_repl = ('1 : Integer\n' if case_name == 'namespace-binding-storage' else '') + expected
        actual_repl = run('repl', text=submission + '\nexercise_control_case()\n:quit\n')
        assert actual_repl == expected_repl, (case_name, actual_repl, expected_repl)
    # Keep optional identities across distinct submissions and record-ID remapping.
    retained = run('repl', text=(
        'record Cell\nvalue: String?\nend\n'
        'empty: Cell? := nil\n'
        'present: Cell? := Cell.new(value: "hello")\n'
        'record Other\nnumber: Integer\nend\n'
        'missing := empty&.value\n'
        'puts(missing == nil)\n'
        'value := present&.value\n'
        'if value != nil\nputs(value)\nend\n'
        'zero: Integer? := 0\nputs(zero == nil)\n'
        'false_value: Boolean? := false\nputs(false_value == nil)\n'
        ':quit\n'))
    assert retained == ('nil : Cell?\nCell(value: "hello") : Cell?\n'
                        'nil : String?\ntrue\n"hello" : String?\nhello\n'
                        '0 : Integer?\nfalse\nfalse : Boolean?\nfalse\n'), retained
    # Corrected reference parser forms are separate from the exact pinned parity registry.
    transfer = repository / 'compiler/conformance/reference-parser-fixed/value-transfer-operands.source'
    transfer_source = root / 'value-transfer-operands.trb'
    transfer_source.write_text(transfer.read_text())
    expected_transfer = transfer.with_suffix('.out').read_text()
    assert run('check', transfer_source) == 'ok\n'
    transfer_binary = root / 'value-transfer-operands'
    run('build', '--compile', '--outfile', transfer_binary, transfer_source)
    assert subprocess.check_output([transfer_binary], text=True, timeout=30) == expected_transfer
    submission = transfer.read_text().replace('def main()', 'def exercise_transfers()')
    assert run('repl', text=submission + '\nexercise_transfers()\n:quit\n') == expected_transfer
    numeric = repository / 'compiler/conformance/reference-compiler-fixed/nullable-numeric-widening.source'
    numeric_source = root / 'nullable-numeric-widening.trb'
    numeric_source.write_text(numeric.read_text())
    numeric_expected = numeric.with_suffix('.out').read_text()
    assert run('check', numeric_source) == 'ok\n'
    numeric_binary = root / 'nullable-numeric-widening'
    run('build', '--compile', '--outfile', numeric_binary, numeric_source)
    assert subprocess.check_output([numeric_binary], text=True, timeout=30) == numeric_expected
    submission = numeric.read_text().replace('def main()', 'def exercise_numeric_conversion()')
    assert run('repl', text=submission + '\nexercise_numeric_conversion()\n:quit\n') == numeric_expected
    failure = repository / 'compiler/conformance/runtime-invalid/array-assignment-initial.trb'
    submission = failure.read_text().replace('def main()', 'def invalid_assignment_case()')
    failure_output = run('repl', text=submission + '\ninvalid_assignment_case()\n:quit\n')
    assert 'out of bounds' in failure_output, failure_output
    assert 'unexpected RHS' not in failure_output, failure_output
    for case_name in ('range-endpoint-failure', 'range-start-failure'):
        fixture = repository / 'compiler/conformance/runtime-invalid' / (case_name + '.trb')
        case_source = root / (case_name + '.trb')
        case_source.write_text(fixture.read_text())
        case_output = root / case_name
        run('build', '--compile', '--outfile', case_output, case_source)
        failed = subprocess.run([case_output], text=True, capture_output=True, timeout=30)
        assert failed.returncode != 0 and failed.stdout == '', failed
        assert failed.stderr == fixture.with_suffix('.stderr').read_text(), failed
        submission = fixture.read_text().replace('def main()', 'def endpoint_failure_case()')
        rejection = run('repl', text=submission + '\nendpoint_failure_case()\n:quit\n')
        assert 'error: division by zero' in rejection, rejection
        assert 'unexpected RHS' not in rejection and 'checked MIR' not in rejection, rejection
    for case_name in ('boolean-array-negative', 'boolean-array-past-end',
                      'record-array-negative', 'record-array-past-end', 'local-array-header-bounds',
                      'stable-array-bindings-bounds', 'conditional-array-header-bounds',
                      'loop-array-header-bounds', 'loop-array-header-negative'):
        fixture = repository / 'compiler/conformance/runtime-invalid' / (case_name + '.trb')
        case_source = root / (case_name + '.trb')
        case_source.write_text(fixture.read_text())
        assert run('check', case_source) == 'ok\n'
        case_output = root / case_name
        run('build', '--compile', '--outfile', case_output, case_source)
        failed = subprocess.run([case_output], text=True, capture_output=True, timeout=30)
        assert failed.returncode != 0 and failed.stdout == '', failed
        assert failed.stderr == fixture.with_suffix('.stderr').read_text(), failed
        submission = fixture.read_text().replace('def main()', 'def invalid_boolean_index()')
        assert 'out of bounds' in run('repl', text=submission + '\ninvalid_boolean_index()\n:quit\n')
    for case_name in ('elsif-after-else', 'elsif-branch-binding', 'elsif-condition',
                      'elsif-escaping-binding', 'elsif-missing-condition', 'elsif-outside-if',
                      'loop-transfer-break-outside-loop', 'loop-transfer-next-outside-loop',
                      'loop-transfer-break-value', 'loop-transfer-next-value',
                      'loop-transfer-break-condition', 'boolean-array-element', 'boolean-array-write',
                      'boolean-array-push', 'boolean-array-index', 'boolean-array-constant-mutation',
                      'boolean-array-parameter', 'boolean-array-readonly',
                      'boolean-array-invariance', 'record-field-write', 'record-field-compound',
                      'record-field-parenthesized', 'record-field-array-replace', 'record-field-call',
                      'record-field-array-immutable', 'record-field-nested', 'record-field-return',
                      'record-array-element', 'record-array-write', 'record-array-push',
                      'record-array-index', 'record-array-constant-mutation', 'record-array-readonly',
                      'record-array-invariance', 'record-array-field-mutation',
                      'record-array-unknown', 'array-iteration-arity', 'array-iteration-duplicate',
                      'array-iteration-escaping', 'array-iteration-receiver',
                      'array-iteration-constant-mutation', 'array-iteration-brace-close',
                      'range-float', 'range-string', 'range-comparison', 'range-index',
                      'range-element-type', 'range-duplicate', 'range-escaping',
                      'float-array-method-arity'):
        fixture = repository / 'compiler/conformance/invalid' / (case_name + '.source')
        case_source = root / (case_name + '.trb')
        case_source.write_text(fixture.read_text())
        assert 'TRBN' in run('check', case_source, success=False)
        assert 'TRBN' in run('build', '--compile', case_source, success=False)
        submission = fixture.read_text().replace('def main()', 'def invalid_control_case()')
        rejection = run('repl', text=submission + '\n:quit\n')
        assert 'TRBN' in rejection

    failed_condition = run('repl', text='def visited(): Boolean\nputs("unexpected effect")\nreturn true\nend\nif false\nputs("wrong")\nelsif 1 / 0 == 0\nputs("wrong")\nelsif visited()\nputs("wrong")\nend\n:quit\n')
    assert 'division by zero' in failed_condition
    assert 'unexpected effect' not in failed_condition
    assert 'wrong' not in failed_condition

    assert 'default mode: trb' in run('--version')
    assert 'Usage:' in run()
    assert 'Usage:' in run('-h')
    for arguments in [('fmt',), ('--mode=',), ('--config',), ('build', '--compile', '--stdout')]:
        run(*arguments, success=False)
    standard_fixture = repository / 'compiler/conformance/valid/standard-package-calls.trb'
    standard_expected = standard_fixture.with_suffix('.out').read_text()
    standard = root / 'standard-package-calls.trb'
    standard.write_text(standard_fixture.read_text())
    assert run('check', standard) == 'ok\n'
    assert run('run', standard) == standard_expected
    assert run('run', standard, '--', 'one', 'two') == standard_expected
    standard_binary = root / 'standard-calls'
    run('build', '--compile', '--outfile', standard_binary, standard)
    assert subprocess.check_output([standard_binary, 'extra'], text=True) == standard_expected

    hello = root / 'hello world.trb'
    hello.write_text('import trb/std/process\ndef main()\nputs(Process.argv()[0])\nend\n')
    assert run(hello, '--', 'spaces; $literal') == 'spaces; $literal\n'
    assert run('run', hello, '--', '--flag') == '--flag\n'
    run('check', hello)
    run('check', hello, '--mode=trb')
    run('check', hello, '--mode', 'go')
    assert 'run is not available for mode go' in run('run', hello, '--mode', 'go', success=False)
    assert 'mode must be go, ruby, typescript, or trb' in run('check', hello, '--mode', 'python', success=False)
    run('build', hello)
    assert (root / 'build/hello world.ssa').read_text().lstrip().startswith('data ')
    assert 'export function' in run('build', hello, '--stdout')
    executable = root / 'out dir/hello'
    run('build', hello, '--compile', '--outfile', executable)
    assert subprocess.check_output([executable, 'compiled'], text=True) == 'compiled\n'
    before = executable.read_bytes()
    hello.write_text('def main()\nputs(1 + true)\nend\n')
    run('build', hello, '--compile', '--outfile', executable, success=False)
    assert executable.read_bytes() == before, 'failed build replaced an existing executable'
    hello.write_text('def main()\nputs("UTF-8: é")\nend\n')
    assert run('run', hello) == 'UTF-8: é\n'
    hello.write_text('def main()\nputs("program ready")\nwhile true\nend\nend\n')
    child = subprocess.Popen([str(binary), 'run', str(hello)], cwd=root, env=env,
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        assert select.select([child.stdout], [], [], 20)[0], 'program did not start'
        assert child.stdout.readline() == 'program ready\n'
        child.send_signal(signal.SIGINT)
        stdout, stderr = child.communicate(timeout=10)
        assert child.returncode == 130, (child.returncode, stdout, stderr)
    finally:
        if child.poll() is None:
            child.kill()
            child.communicate()

    assert run('run', cwd=repository) == 'Hello from TypeRB Native!\n'
    project = root / 'project'
    (project / 'src/nested').mkdir(parents=True)
    (project / 'src/main.trb').write_text('def answer(): Integer\nreturn 42\nend\ndef main()\nputs("project")\nend\n')
    config = project / 'trbconfig.jsonc'
    config.write_text('{"name":"demo","sourceDir":"src"}')
    assert run('run', cwd=project / 'src/nested') == 'project\n'
    assert '42 : Integer' in run('repl', text='answer()\n:quit\n', cwd=project)
    (project / 'src/model.trb').write_text('record Box\nvalue: Integer\nend\n')
    records = run('repl', text='mut box := Box.new(value: 3)\nrecord Point\nx: Integer\ny: Integer\nend\nbox\n:type box\nbox.value\n:quit\n', cwd=project)
    assert records.count('Box(value: 3) : Box') == 2, records
    assert 'Box\n3 : Integer' in records, records

    # Generated project imports yield to authored declarations and imports.
    imports_project = root / 'repl-imports'
    (imports_project / 'src').mkdir(parents=True)
    (imports_project / 'trbconfig.jsonc').write_text('{"name":"imports","sourceDir":"src"}')
    (imports_project / 'src/helper.trb').write_text(
        'def value(): String\nreturn "project"\nend\n'
        'record Entry\nname: String\nend\n')
    assert run('repl', cwd=imports_project, text=(
        'import { value as label } from helper\nputs(label())\n'
        ':type label()\n:reload\nputs(label())\n:quit\n'
    )) == 'project\nString\nproject\nreloaded\nproject\n'
    assert run('repl', cwd=imports_project, text=(
        'import { value } from helper\nputs(value())\n:quit\n'
    )) == 'project\n'
    assert run('repl', cwd=imports_project, text=(
        'def value(): String\nreturn "local"\nend\nputs(value())\n:quit\n'
    )) == 'local\n'
    assert run('repl', cwd=imports_project, text=(
        'import {\n Entry as Item,\n value as label,\n} from helper\n'
        'def show()\nentry := Item.new(name: label())\nputs(entry.name)\nend\n'
        'show()\n:reload\nshow()\n:quit\n'
    )) == 'project\nproject\nreloaded\nproject\n'
    assert run('repl', cwd=imports_project, text=(
        'puts("import { value as label } from helper")\n'
        '# import { value as label } from helper\nputs(value())\n:quit\n'
    )) == 'import { value as label } from helper\nproject\n'
    duplicate = run('repl', cwd=imports_project, text=(
        'import { value as first } from helper\n'
        'import { value as second } from helper\nputs(first())\n:quit\n'))
    assert duplicate == ('project\n(trb):2: error[TRBN4003]: '
                         'declaration value from helper is already imported as first\n'), duplicate
    rejected_alias = run('repl', cwd=imports_project, text=(
        'import { missing as value } from helper\nputs(value())\n:quit\n'))
    assert rejected_alias == ('project\n(trb):1: error[TRBN4003]: '
                              'duplicate import binding value\n'), rejected_alias
    prior_use = run('repl', cwd=imports_project, text=(
        'puts(value())\nimport { value as label } from helper\n'
        'puts(value())\n:quit\n'))
    assert prior_use == ('project\nproject\n(trb):1: error[TRBN4003]: '
                         'unresolved function value\n'), prior_use
    for standard_import, expression in (
            ('import trb/std/math', 'Math.sqrt(9.0).to_i()'),
            ('import trb/std/math as Numbers', 'Numbers.sqrt(9.0).to_i()'),
            ('import trb/std/process as Command', 'Command.argv().size()')):
        expected = '0\n' if 'process' in standard_import else '3\n'
        assert run('repl', cwd=imports_project, text=(
            standard_import + '\nputs(' + expression + ')\n:quit\n')) == expected

    record_alias = run('repl', cwd=imports_project, text=(
        'import { Entry as Item } from helper\n'
        'entry := Item.new(name: "retained")\nentry.name\n:type entry\n'
        'entries := [entry]\nentries[0].name\n:reload\nentries[0].name\n:quit\n'))
    assert 'Entry(name: "retained") : Item\n' in record_alias, record_alias
    assert 'Array<Item>\n' in record_alias and 'Item\n' in record_alias, record_alias
    assert record_alias.count('"retained" : String\n') == 3, record_alias
    assert 'error' not in record_alias, record_alias

    # A later local record can reorder compiler IDs without changing retained
    # imported values, including aliases held by Arrays and Hashes.
    relocated = run('repl', cwd=imports_project, text=(
        'import { Entry as Item } from helper\n'
        'entry := Item.new(name: "retained")\nentries := [entry]\n'
        'table: Hash<String, Item> := {"saved" => entry}\n'
        'record Entry\ncount: Integer\nend\n'
        'puts(entry.name)\nputs(entries[0].name)\n'
        'puts(table["saved"].name)\n:quit\n'))
    assert relocated.endswith('retained\nretained\nretained\n'), relocated
    assert 'error' not in relocated, relocated

    # Imported record aliases retain declaration identity inside Array types.
    record_project = root / 'record-arrays'
    record_project.mkdir()
    (record_project / 'model.trb').write_text(
        'record Entry\nid: Integer\nend\ndef entries(): Array<Entry>\n'
        'return [Entry.new(id: 3)]\nend\n')
    record_entry = record_project / 'main.trb'
    record_entry.write_text(
        'import { Entry as Item, entries } from model\n'
        'def read(values: Array<Item>): Integer\nreturn values[0].id\nend\n'
        'def main()\nmut values: Array<Item> := entries()\n'
        'values.push(Item.new(id: 4))\nputs(read(values))\nputs(values[1].id)\nend\n')
    assert run('check', record_entry) == 'ok\n'
    assert run('run', record_entry) == '3\n4\n'
    record_entry.write_text(
        'import { entries } from model\nrecord Entry\nid: Integer\nend\n'
        'def consume(values: Array<Entry>)\nputs(values.size())\nend\n'
        'def main()\nconsume(entries())\nend\n')
    assert 'TRBN4004' in run('check', record_entry, success=False)
    record_entry.write_text(record_entry.read_text().replace(
        'record Entry', 'record OtherEntry').replace('Array<Entry>', 'Array<OtherEntry>'))
    assert 'TRBN4004' in run('check', record_entry, success=False)

    assert 'override' in run('check', '--mode', 'trb', cwd=project, success=False)
    config.write_text('{"name":"demo","mode":"go","sourceDir":"src","go":{"module":"example.com/demo"}}')
    run('check', cwd=project)
    assert 'run is not available for mode go' in run('run', cwd=project, success=False)
    assert '42 : Integer' in run('repl', '--mode=trb', text='answer()\n:quit\n', cwd=project)
    run('run', 'missing.trb', cwd=project, success=False)
    assert '"é" : String' in run('repl', text='"é"\n:quit\n')
    logical = run('repl', text='''def mark(value: Boolean): Boolean
puts("visited")
return value
end
true || mark(false)
false && mark(true)
false || mark(true) && !false
true || 1 / 0 == 0
false && (true || 1 / 0 == 0)
false == 1 < 2 || 3 > 2 == true
:exit
''')
    assert logical.count('visited') == 1, logical
    assert logical.count('true : Boolean') == 4, logical
    assert logical.count('false : Boolean') == 2, logical
    assert 'panic:' not in logical, logical
    logical_assignment = run('repl', text='''def mark(value: Boolean): Boolean
puts("visited")
return value
end
def index(): Integer
puts("index")
return 0
end
mut enabled := true
enabled ||= mark(false)
enabled &&= mark(false)
mut flags := [true]
flags[index()] ||= mark(true)
flags[index()] &&= mark(false)
flags[0]
:quit
''')
    assert logical_assignment.count('visited') == 2, logical_assignment
    assert logical_assignment.count('index') == 2, logical_assignment
    assert logical_assignment.endswith('false : Boolean\n'), logical_assignment
    unary = run('repl', text='-1 + 2\n-1 * 2 + 3\n-(1 + 2)\n!true && false || true\n:quit\n')
    assert unary == '1 : Integer\n1 : Integer\n-3 : Integer\ntrue : Boolean\n', unary
    required_rhs = run('repl', text='false || 1 / 0 == 0\n:q\n')
    assert 'division by zero' in required_rhs, required_rhs
    (root / 'helpers.trb').write_text('# A declaration file\ndef loaded(): Integer\nreturn 8\nend\n')
    assert '8 : Integer' in run('repl', text=':load helpers.trb\nloaded()\n:quit\n')
    replay = run('repl', text='mut n := 1\nn += 2\nputs("replay marker")\n:reload\nn\n:load helpers.trb\nn + loaded()\n:q\n')
    assert replay.count('replay marker') == 3, replay
    assert 'reloaded' in replay and '11 : Integer' in replay, replay
    (root / 'invalid.trb').write_text('1 + "bad"\n')
    rejected = run('repl', text='puts("do not replay")\n:load invalid.trb\n:q\n')
    assert rejected.count('do not replay') == 1, rejected


    output = run('repl', text='''mut total := 2
puts("once")
:type puts("must not print")
total += 3
total
:type total + 1
mut xs := [1, 2]
mut ys := xs
ys.push(3)
xs[0] = 9
ys
xs[99]
ys
1 + "bad"
1 + 2
"invalid".to_i()
"+12".to_i()
"9007199254740992".to_i()
[1, 2.5]
9007199254740991 + 1
10 / 0
def sum(xs: Array<Integer>): Integer
mut i := 0
mut n := 0
while i < xs.size()
n += xs[i]
i += 1
end
return n
end
sum(xs)
record Pair
left: Integer
right: Integer
end
mut pair := Pair.new(left: 2, right: 4)
pair.left = 7
pair
pair = Pair.new(left: 7, right: 4)
pair
:quit
''')
    for expected in ['5 : Integer', 'Integer\n', '[9, 2, 3]', 'out of bounds',
                     '3 : Integer', 'invalid Integer', '12 : Integer',
                     'outside the portable range', '[1, 2.5] : Array<Float>',
                     'division by zero', '14 : Integer', 'field is readonly',
                     'left: 2, right: 4', 'left: 7, right: 4']:
        assert expected in output, (expected, output)
    assert output.count('once') == 1, output
    assert 'must not print' not in output, output
    assert output.count('[9, 2, 3]') >= 2, output

    # A controlling terminal exercises native editing, tab completion, history and SIGINT.
    pid, descriptor = pty.fork()
    if pid == 0:
        os.chdir(root)
        os.execve(binary, [str(binary)], env)
    pending = b''

    def expect(needle, timeout=15):
        global pending
        end = time.monotonic() + timeout
        target = needle.encode()
        while target not in pending:
            assert time.monotonic() < end, (needle, pending.decode(errors='replace'))
            if select.select([descriptor], [], [], 0.2)[0]:
                try:
                    part = os.read(descriptor, 65536)
                except OSError as error:
                    if error.errno == errno.EIO:
                        part = b''
                    else:
                        raise
                assert part, ('terminal exited', pending)
                pending += part
        before, pending = pending.split(target, 1)
        return before.decode(errors='replace')

    def send(text):
        os.write(descriptor, text.encode())

    try:
        expect('trbn:trb> ')
        send('mut apples := 4\n')
        expect('4 : Integer')
        expect('trbn:trb> ')
        send('app\t\n')
        expect('4 : Integer')
        expect('trbn:trb> ')
        send('\x1b[A\n')
        expect('4 : Integer')
        expect('trbn:trb> ')
        send('import { Header } from trb/http\n')
        expect('trbn:trb> ')
        send('header := Header.new(name: "x", value: "kept")\n')
        expect('Header(name: "x", value: "kept") : Header')
        expect('trbn:trb> ')
        send('header.val\t\n')
        expect('"kept" : String')
        expect('trbn:trb> ')
        send('if true\n')
        expect('...> ')
        send('puts("loop started")\nwhile true\nend\nend\n')
        expect('loop started\r\n')
        send('\x03')
        expect('Interrupted')
        expect('trbn:trb> ')
        send('apples\n')
        expect('4 : Integer')
        expect('trbn:trb> ')
        send('apples +')
        expect('apples +')
        send('\x03')
        expect('trbn:trb> ')
        send('apples\n')
        expect('4 : Integer')
        expect('trbn:trb> ')
        send(':quit\n')
        deadline = time.monotonic() + 10
        while True:
            waited, status = os.waitpid(pid, os.WNOHANG)
            if waited:
                assert os.waitstatus_to_exitcode(status) == 0
                pid = 0
                break
            if select.select([descriptor], [], [], 0)[0]:
                try:
                    pending += os.read(descriptor, 65536)
                except OSError as error:
                    if error.errno != errno.EIO:
                        raise
            assert time.monotonic() < deadline, ('REPL did not exit', pending)
            time.sleep(0.02)
    finally:
        os.close(descriptor)
        if pid:
            os.kill(pid, signal.SIGKILL)
            os.waitpid(pid, 0)
    history = json.loads((root / 'history.json').read_text())
    assert 'mut apples := 4' in history
    assert 'if true\n  puts("loop started")\n  while true\n  end\nend' in history

print('Native CLI, project configuration, REPL and terminal tests passed')
