#!/usr/bin/env python3
"""Exercise embedded official sources through ordinary file/project/REPL imports."""
import argparse
from native_network_link import static_network_flags
from compiler_sources import stage_sources
import json
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
parser.add_argument('--reference', type=Path)
args = parser.parse_args()
binary = args.binary.resolve()
reference = args.reference.resolve() if args.reference else None
repository = Path(__file__).resolve().parent.parent
source = '''import { Headers as HeaderMap, HttpMethod, Body } from trb/http
def probe()
puts(HeaderMap.new().size())
puts(HttpMethod.get().to_s())
puts(Body.new("A😀".to_bytes()).size())
puts(Body.new("body".to_bytes()).bytes().to_s())
headers := HeaderMap.new().add("X-Test", "first").add("x-test", "second")
puts(headers.values("X-TEST").size())
puts(headers.with("X-Test", "kept").size())
puts(headers.without("X-Test").size())
end
def main()
probe()
end
'''
expected = '0\nGET\n5\nbody\n2\n1\n0\n'

with tempfile.TemporaryDirectory(prefix='native-official-import-') as temporary:
    root = Path(temporary)
    environment = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None, success=True):
        result = subprocess.run(list(map(str, command)), cwd=root, env=environment,
                                input=data, capture_output=True, text=True, timeout=90)
        assert (result.returncode == 0) == success, (command, result.stdout, result.stderr)
        return result

    file_root = root / 'file root'
    file_root.mkdir()
    main = file_root / 'main.trb'
    main.write_text(source)
    # A filesystem spelling cannot replace the compiler-owned official source.
    shadow = file_root / 'trb/http/index.trb'
    shadow.parent.mkdir(parents=True)
    shadow.write_text('class Headers\ndef invalid()\nunknown()\nend\nend\n')
    assert run([binary, 'check', main]).stdout == 'ok\n'
    program = file_root / 'program'
    run([binary, 'build', '--compile', main, '--outfile', program])
    assert run([program]).stdout == expected

    project = root / 'project'
    (project / 'src').mkdir(parents=True)
    config = project / 'trbconfig.jsonc'
    config.write_text(json.dumps({'name': 'official-import', 'mode': 'trb', 'sourceDir': 'src'}))
    (project / 'src/main.trb').write_text(source)
    assert run([binary, 'check', '--config', config]).stdout == 'ok\n'
    run([binary, 'build', '--compile', '--config', config, '--outfile', program])
    assert run([program]).stdout == expected

    declarations = source.split('def main()')[0]
    retained = run([binary, 'repl'], data=declarations + 'probe()\nclass Later\nend\nprobe()\n:quit\n')
    assert not retained.stderr and retained.stdout == expected * 2, retained

    # The complete Web source and the internal failure service are canonical,
    # even when authored files attempt to shadow their package spelling.
    web = root / 'web values'
    web.mkdir()
    (web / 'trb/web').mkdir(parents=True)
    (web / 'trb/web/index.trb').write_text('this is not a valid replacement\n')
    (web / 'producer.trb').write_text('import { Response, ContextKey } from trb/web\n'
        'def make_response(): Response\nreturn Response.text("kept", 202)\nend\n'
        'def make_key(): ContextKey<String>\nreturn ContextKey<String>.new("held")\nend\n')
    web_source = ('import { Response as Reply, ContextKey } from trb/web\n'
        'import { make_response, make_key } from producer\n'
        'def probe_web()\nresponse: Reply := make_response()\n'
        'key: ContextKey<String> := make_key()\nputs(response.status)\n'
        'puts(response.body.to_s())\nputs(key.name)\nend\n'
        'def main()\nprobe_web()\nend\n')
    web_main = web / 'main.trb'
    web_main.write_text(web_source)
    assert run([binary, 'check', web_main]).stdout == 'ok\n'
    run([binary, 'build', '--compile', web_main, '--outfile', program])
    assert run([program]).stdout == '202\nkept\nheld\n'
    # Retained values must keep the official owner across later homonyms and
    # another checked submission, without replaying their construction.
    retained_web = ('import { make_response, make_key } from producer\n'
        'response := make_response()\nkey := make_key()\n'
        'class Response\ndef status(): Integer\nreturn 99\nend\nend\n'
        'class ContextKey<T>\nreadonly @name: String\n'
        'def initialize(name: String)\n@name = name\nend\nend\n'
        'puts(response.status)\nputs(response.body.to_s())\nputs(key.name)\n'
        'class Later\nend\nputs(response.status)\nputs(key.name)\n:quit\n')
    web_main.unlink()
    retained_config = web / 'trbconfig.jsonc'
    retained_config.write_text(json.dumps({'name': 'web-values', 'mode': 'trb', 'sourceDir': '.'}))
    # The authored shadow is unreachable in file mode. Do not include it in a
    # project-wide declaration scan, which intentionally checks every module.
    (web / 'trb/web/index.trb').unlink()
    observed_web = run([binary, 'repl', '--config', retained_config], data=retained_web)
    assert not observed_web.stderr, observed_web
    assert observed_web.stdout.endswith('202\nkept\nheld\n202\nheld\n'), observed_web
    web_main.write_text(web_source)
    if reference:
        reference_web_config = web / 'reference.trbconfig.jsonc'
        reference_web_config.write_text(json.dumps({'name': 'web-values', 'mode': 'go',
            'sourceDir': '.', 'go': {'module': 'example.com/web-values'}}))
        run([reference, 'check', '--config', reference_web_config])
        run([reference, 'build', '--compile', '--config', reference_web_config, '--outfile', program])
        assert run([program]).stdout == '202\nkept\nheld\n'

    # Collect at managed allocation boundaries while Request/Response updates
    # retain prior bodies, headers and cookie records across ordinary calls.
    lifetime = web / 'lifetime.trb'
    lifetime.write_text('''import { Request, Response, ResponseCookie } from trb/web
import { Body, Header, Headers, HttpMethod } from trb/http
def exercise(index: Integer): String
value := "payload-" + index.to_s()
request := Request.new(method: HttpMethod.post(), path: "/probe", query_string: "", headers: Headers.new([Header.new(name: "x-id", value: index.to_s())]), body: Body.new(value.to_bytes()))
changed := request.with_header("x-id", "kept").add_header("extra", "value")
response := Response.text(changed.bytes().to_s()).set_cookie(ResponseCookie.new(name: "id", value: index.to_s())).vary("Origin")
return response.body.to_s() + ":" + request.header_values("x-id").join(",") + ":" + changed.header_values("x-id").join(",")
end
def main()
(0...30).each do |index|
puts(exercise(index))
end
end
''')
    emitted = run([binary, '--internal-driver', 'emit-qbe', lifetime])
    assert not emitted.stderr, emitted
    forced, ordinary, hooks = [], False, 0
    for line in emitted.stdout.splitlines():
        if line.startswith('function '):
            ordinary = '$trbnf' in line
        if line == '}':
            ordinary = False
        if ordinary and any(marker in line for marker in (
                'call $trbn_array_new(', 'call $trbn_array_new_reserved(',
                'call $trbn_array_push_', 'call $trbn_array_grow_paced(',
                'call $trbn_gc_alloc(', 'call $trbn_string_concat(')):
            forced.append('\tcall $trbn_gc_collect(w 0)')
            hooks += 1
        forced.append(line)
    assert hooks >= 4, hooks
    il, assembly = web / 'lifetime.ssa', web / 'lifetime.s'
    il.write_text('\n'.join(forced) + '\n')
    run([binary.with_name('qbe'), '-o', assembly, il])
    run(['/usr/bin/cc', assembly, *static_network_flags(binary.with_name('qbe')), '-lm', '-o', program])
    collected = subprocess.run([str(program)], cwd=root, text=True, capture_output=True,
        env=dict(environment, TYPE_RB_NATIVE_RUNTIME_STATS='1'), timeout=90)
    assert collected.returncode == 0, collected
    assert collected.stdout == ''.join(f'payload-{index}:{index}:kept\n' for index in range(30)), collected
    statistics = {}
    for line in collected.stderr.splitlines():
        prefix, key, value = line.split(',')
        assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
        statistics[key] = int(value)
    assert statistics['collections'] >= 30, statistics
    assert statistics['live-bytes'] == 0, statistics
    assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics

    # Exercise the checked intrinsic in the Native-built REPL evaluator with
    # messages unavailable through the pinned public package's fixed failures.
    # The fixture grants loader ownership explicitly; authored imports above
    # remain forbidden, including after a trusted source has been loaded.
    # Forward the normal driver entries so byte and host adapters retain the
    # same canonical compiler owner as the ordinary CLI.
    probe_root = root / 'runtime probe'
    probe_root.mkdir()
    stage_sources([repository / 'compiler/src', repository / 'compiler/cli'], probe_root)
    probe = probe_root / 'runtime_probe.trb'
    probe.write_text('''import { compiler_main as core_main, compiler_file_main as core_file_main, check, emit_qbe } from compiler
import { compiler_new } from state/state
import { lex } from frontend/syntax/lexer
import { parse_module } from frontend/syntax/parser
import { resolve } from frontend/checking/program/resolution
import { official_source_kind } from project/official_catalog
import { standard_runtime_fail } from frontend/resolution/standard_intrinsics
import { load_string, store_integer } from support/storage
import { repl_store, repl_string, repl_failed } from repl/model
import { repl_call } from repl/eval

def compiler_main(source: String, mode: String, file_mode: Boolean): Integer
return core_main(source, mode, file_mode)
end

def compiler_file_main(source: String, path: String, mode: String, file_mode: Boolean): Integer
return core_file_main(source, path, mode, file_mode)
end

def native_cli_main(): Integer
source := "import trb/internal/runtime\\ndef main()\\nRuntime.fail<Integer>(\\"\\")\\nend\\n"
mut state := compiler_new(1048576)
lex(state, source)
store_integer(state.standard_modules, state.module_graph.entry_module[0], official_source_kind("trb/web"))
parse_module(state, 0, state.module_graph.entry_module[0])
if !resolve(state) || !check(state)
puts("fixture check failed")
puts(load_string(state.diagnostics.messages, 0))
return 1
end
mut function := -1
state.scalar_mir.signatures.each do |signature|
if standard_runtime_fail(state, signature.id)
function = signature.id
end
end
if function < 0
puts("fixture has no intrinsic")
return 1
end
mut store := repl_store()
repl_call(state, function, [repl_string(store, "")], store)
puts(repl_failed(store))
puts(store.error[0].size())
store.error[0] = ""
store.failed[0] = false
message := "message-with-" + "NUL\\x00-tail"
repl_call(state, function, [repl_string(store, message)], store)
puts(repl_failed(store))
puts(store.error[0] == message)
repl_call(state, function, [repl_string(store, "replacement")], store)
puts(store.error[0] == message)
source_with_message := "import trb/internal/runtime\\ndef message(): String\\nputs(\\"before\\")\\nreturn \\"NUL\\\\x00tail\\" + 7.to_s()\\nend\\ndef main()\\nRuntime.fail<Float>(message())\\nputs(\\"after\\")\\nend\\n"
mut ordinary := compiler_new(1048576)
lex(ordinary, source_with_message)
store_integer(ordinary.standard_modules, ordinary.module_graph.entry_module[0], official_source_kind("trb/web"))
parse_module(ordinary, 0, ordinary.module_graph.entry_module[0])
if !resolve(ordinary) || !check(ordinary)
puts("ordinary fixture check failed")
puts(load_string(ordinary.diagnostics.messages, 0))
return 1
end
puts("runtime-qbe-begin")
output := emit_qbe(ordinary)
(0...output.lines.used[0]).each do |index|
puts(load_string(output.lines, index))
end
return 0
end

def main()
return
end
''')
    run([binary, 'build', '--compile', probe, '--outfile', program])
    probe_output = run([program]).stdout
    assert 'runtime-qbe-begin\n' in probe_output, probe_output
    checks, runtime_qbe = probe_output.split('runtime-qbe-begin\n')
    assert checks == 'true\n0\ntrue\ntrue\ntrue\n', checks
    runtime_il, runtime_assembly = probe_root / 'runtime.ssa', probe_root / 'runtime.s'
    runtime_il.write_text(runtime_qbe)
    run([binary.with_name('qbe'), '-o', runtime_assembly, runtime_il])
    run(['/usr/bin/cc', runtime_assembly, '-lm', '-o', program])
    failure = run([program], success=False)
    assert failure.returncode == 2 and failure.stdout == 'before\n', failure
    assert failure.stderr == 'panic: NUL\x00tail7\n', failure

    # Independent importers keep the same nominal identity.
    (file_root / 'helper.trb').write_text('import { Headers } from trb/http\n'
        'def identity(value: Headers): Headers\nreturn value\nend\n')
    main.write_text('import { Headers as HeaderMap } from trb/http\nimport { identity } from helper\n'
                    'def main()\nputs(identity(HeaderMap.new()).size())\nend\n')
    run([binary, 'build', '--compile', main, '--outfile', program])
    assert run([program]).stdout == '0\n'

    for imports, marker in [
        ('import { Headers, Headers as Other } from trb/http', 'already imported'),
        ('import { Missing } from trb/http', 'Missing'),
        ('import trb/not_a_package', 'package imports'),
        ('import trb/internal/runtime', 'package imports'),
        ('import trb/platform/typescript/browser', 'unimplemented platform or semantic provider'),
        ('import trb/platform/go/cli', 'unimplemented platform or semantic provider'),
        # The provider exports named dispatch; it has no default root value.
        ('import trb/web/testing', 'no record root declaration matching testing'),
    ]:
        main.write_text(imports + '\ndef main()\nreturn\nend\n')
        rejected = run([binary, 'check', main], success=False)
        assert marker in rejected.stderr, rejected
    internal = run([binary, 'repl'], data='import trb/internal/runtime\n:quit\n')
    assert internal.stderr, internal

    # Values returned by another module retain their owner even when the REPL
    # has no import for their type, or a later declaration reuses its name.
    retained_project = root / 'retained'
    (retained_project / 'src').mkdir(parents=True)
    retained_config = retained_project / 'trbconfig.jsonc'
    retained_config.write_text(json.dumps({'name': 'retained', 'mode': 'trb', 'sourceDir': 'src'}))
    (retained_project / 'src/producer.trb').write_text('''import { Body, Header, Headers } from trb/http
def make_headers(): Headers
puts("headers factory")
return Headers.new([Header.new(name: "X-Id", value: "kept")])
end
def make_entries(): Array<Header>
return [Header.new(name: "X-Entry", value: "entry")]
end
def make_reader(): () -> Body
puts("reader factory")
body := Body.new("body".to_bytes())
return fn(): Body
return body
end
end
''')
    retained_input = '''headers := make_headers()
entries := make_entries()
read := make_reader()
:type headers
class Headers
def size(): Integer
return 99
end
end
record Header
number: Integer
end
puts(headers.first("x-id"))
puts(entries[0].value)
puts(read().to_s())
puts(Headers.new().size())
:reload
puts(headers.first("x-id"))
puts(entries[0].value)
puts(read().to_s())
:quit
'''
    retained = run([binary, 'repl', '--config', retained_config], data=retained_input)
    assert not retained.stderr, retained
    assert retained.stdout.count('headers factory\n') == 2, retained
    assert retained.stdout.count('reader factory\n') == 2, retained
    assert '\nHeaders\n' in retained.stdout, retained
    assert retained.stdout.endswith('kept\nentry\nbody\n99\nreloaded\nkept\nentry\nbody\n'), retained

    if reference:
        main.write_text(source)
        run([reference, 'check', main])
        run([reference, 'build', '--compile', '--outfile', program, main])
        assert run([program]).stdout == expected
        observed = run([reference, 'repl'], data=declarations + 'probe()\nprobe()\n:quit\n')
        assert not observed.stderr and observed.stdout == expected * 2, observed
        # Compare the same source project in each compiler's execution mode.
        reference_config = retained_project / 'reference.trbconfig.jsonc'
        reference_config.write_text(json.dumps({'name': 'retained', 'mode': 'go',
            'sourceDir': 'src', 'go': {'module': 'example.com/retained'}}))
        observed = run([reference, 'repl', '--config', reference_config], data=retained_input)
        # Native announces a successful reload after replay; the reference
        # announces it before replay. Keep both complete effect transcripts
        # exact while accounting for this existing status-line placement.
        before, after = retained.stdout.split('reloaded\n')
        replay_effects = 'headers factory\nreader factory\nkept\nentry\nbody\n99\n'
        assert before.endswith(replay_effects), retained
        expected_reference = before[:-len(replay_effects)] + 'reloaded\n' + replay_effects + after
        assert not observed.stderr and observed.stdout == expected_reference, observed

print('PASS exact bundled HTTP/Web imports, canonical identity, GC, Runtime failures, file/project/REPL and rejected boundaries')
