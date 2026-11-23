#!/usr/bin/env python3
"""Hash behavior, stateful REPL and bucket reclamation through native code."""
import argparse
from compiler_sources import stage_sources
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('binary', type=Path)
    parser.add_argument('--reference', type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve()
    repository = Path(__file__).resolve().parent.parent
    with tempfile.TemporaryDirectory(prefix='native hash ') as temporary:
        root = Path(temporary)
        env = dict(os.environ, NO_COLOR='1', TRBN_HISTORY=str(root / 'history'))

        def run(*arguments, text=None, success=True):
            result = subprocess.run([str(binary), *map(str, arguments)], input=text,
                                    text=True, capture_output=True, cwd=root, env=env, timeout=60)
            assert (result.returncode == 0) == success, result
            return result

        cases = []
        for name in ('hash-values', 'hash-managed', 'hash-cycles',
                     'hash-lookup-reuse', 'hash-lookup-alias-barriers'):
            fixture = repository / 'compiler/conformance/valid' / (name + '.trb')
            cases.append((name, fixture.read_text(), fixture.with_suffix('.out').read_text(),
                          name in ('hash-managed', 'hash-cycles')))
        # Keep legal unformatted newlines: formatting a fixture would remove
        # the token boundaries that previously broke only REPL evaluation.
        cases.append(('hash-newlines', '''def main()
 mut h := {1 => 2
 , 3 => 4}
 puts(h[3])
 puts(h.fetch(1
 ))
 puts(h.size(
 ))
 puts(h.key?(3
 ))
 puts(h.dup(
 ).size())
 puts(h.merge({5 => 6}
 ).fetch(5))
 h.update({7 => 8}
 )
 puts(h.delete(7
 ))
 puts(h.values(
 ).size())
 puts(h.keys(
 ).size())
 puts(h.empty?(
 ))
end
''', '4\n2\n2\ntrue\n2\n6\n8\n2\n2\nfalse\n', False))
        for name, contents, expected, collects in cases:
            source = root / (name + '.trb')
            source.write_text(contents)
            assert run('check', source).stdout == 'ok\n'
            output = root / name
            run('build', '--compile', '--outfile', output, source)
            executed = subprocess.run([output], capture_output=True, text=True,
                                      env=dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1'), timeout=60)
            assert executed.returncode == 0 and executed.stdout == expected, executed
            assert 'type-rb-native-gc-stat-v1,live-bytes,0\n' in executed.stderr, executed
            if collects:
                assert 'type-rb-native-gc-stat-v1,automatic-collections,0\n' not in executed.stderr
            submission = source.read_text().replace('def main()', 'def exercise_hash()')
            result = run('repl', text=submission + '\nexercise_hash()\n:quit\n')
            assert result.stdout == expected and result.stderr == '', result
            if args.reference:
                checked = subprocess.run([str(args.reference.resolve()), 'check', str(source)],
                                         cwd=root, capture_output=True, text=True, timeout=60)
                assert checked.returncode == 0, checked
                reference = subprocess.run([str(args.reference.resolve()), 'repl', '--mode', 'go'],
                                           cwd=root, input=submission + '\nexercise_hash()\n:quit\n',
                                           capture_output=True, text=True, timeout=60)
                # The reference acknowledges declarations before invocation.
                assert reference.returncode == 0 and not reference.stderr, reference
                assert reference.stdout.endswith(expected), reference

        # The ordinary byte-preserving adapter must exercise the selected MIR
        # path, including embedded NUL and Unicode. Collect only at operations
        # whose existing MIR effects already require roots.
        for name in ('hash-lookup-reuse', 'hash-lookup-alias-barriers'):
            source = root / (name + '.trb')
            emitted = run('--internal-driver', 'emit-qbe', source)
            assert 'call $trbn_hash_existing_slot_string(' in emitted.stdout, name
            forced, ordinary, hooks = [], False, 0
            for line in emitted.stdout.splitlines():
                if line.startswith('function '):
                    ordinary = '$trbnf' in line
                if line == '}':
                    ordinary = False
                if ordinary and any(marker in line for marker in (
                        'call $trbn_array_new(', 'call $trbn_array_new_reserved(',
                        'call $trbn_array_push_', 'call $trbn_array_grow_paced(',
                        'call $trbn_gc_alloc(', 'call $trbn_string_concat',
                        'call $trbn_integer_to_string(', 'call $trbn_hash_new(',
                        'call $trbn_hash_set(', 'call $trbn_hash_delete(',
                        'call $trbn_hash_update(', 'call $trbnf')):
                    forced.append('\tcall $trbn_gc_collect(w 0)')
                    hooks += 1
                forced.append(line)
            assert hooks >= 4, (name, hooks)
            il, assembly, program = [root / (name + suffix) for suffix in ('.ssa', '.s', '-forced')]
            il.write_text('\n'.join(forced) + '\n')
            subprocess.run([binary.with_name('qbe'), '-o', assembly, il], check=True, timeout=60)
            subprocess.run(['/usr/bin/cc', assembly, '-lm', '-o', program], check=True, timeout=60)
            result = subprocess.run([program], capture_output=True, text=True,
                                    env=dict(env, TYPE_RB_NATIVE_RUNTIME_STATS='1'), timeout=60)
            expected = (repository / 'compiler/conformance/valid' / (name + '.out')).read_text()
            assert result.returncode == 0 and result.stdout == expected, result
            statistics = {}
            for line in result.stderr.splitlines():
                prefix, key, value = line.split(',')
                assert prefix == 'type-rb-native-gc-stat-v1' and key not in statistics, line
                statistics[key] = int(value)
            assert statistics['collections'] >= hooks, (name, statistics)
            assert statistics['live-bytes'] == 0, statistics
            assert statistics['allocated-bytes'] == statistics['reclaimed-bytes'], statistics

        result = run('repl', text='mut h := {}\nh.size()\nh[1] = 2\nmut shared := h\n'
                     'h.delete(1)\nshared.empty?()\nshared[3] = 4\nh[3]\n'
                     'h[999]\nh[3]\n:quit\n')
        assert '{} : Hash [mut]\n0 : Integer\n' in result.stdout, result
        assert 'true : Boolean\n4 : Integer\n4 : Integer\n4 : Integer\n' in result.stdout, result
        assert 'Hash key is missing' in result.stderr, result
        assert 'panic: index is out of bounds' not in result.stderr, result

        for expression in ('values[3]', 'values.fetch(3)', 'values.delete(3)'):
            source = root / 'missing.trb'
            source.write_text('def main()\nmut values := {1 => 2}\nputs(' + expression + ')\nend\n')
            output = root / 'missing'
            run('build', '--compile', '--outfile', output, source)
            failed = subprocess.run([output], capture_output=True, text=True, timeout=60)
            assert failed.returncode != 0 and failed.stdout == '', failed
            assert failed.stderr == 'panic: Hash key is missing\n', failed

        invalid = ('values := {1 => 2}\nvalues[1] = 3',
                   'values := {1 => [2]}\nvalues.fetch(1)[0] = 3',
                   'values := {1 => 2}\nvalues.size() = 3',
                   '{1 => 2}[1] = 3',
                   '{1 => 2}.delete(1)',
                   'values := {1 => 2}\nvalues.delete(1)',
                   'values := {1 => 2}\nvalues.update({2 => 3})',
                   'mut values := {1 => 2}\nvalues[1] = "bad"',
                   'values := {1 => 2}\nputs(values["bad"])')
        for body in invalid:
            source = root / 'invalid.trb'
            source.write_text('def main()\n' + body + '\nend\n')
            assert 'TRBN4004' in run('check', source, success=False).stderr
            assert 'TRBN4004' in run('build', '--compile', source, success=False).stderr
            if args.reference:
                checked = subprocess.run([str(args.reference.resolve()), 'check', str(source)],
                                         cwd=root, capture_output=True, text=True, timeout=60)
                assert checked.returncode != 0, checked

        # Compile the TypeRB-authored bucket implementation directly. This proves
        # deletion clears references and releases capacity independently of RSS.
        # REPL values retain checked program/type owners. Keep the internal
        # consumer's imports complete as that representation evolves.
        stage_sources([repository / 'compiler/src', repository / 'compiler/cli'], root)
        source = root / 'hash_storage_probe.trb'
        source.write_text('''import { repl_store, repl_integer } from repl/model
import { repl_hash_new, repl_hash_set, repl_hash_delete } from repl/hash

def main()
 mut store := repl_store()
 h := repl_hash_new(store, "Hash<Integer,Integer>", 0)
 mut i := 0
 while i < 1000
  repl_hash_set(store, h, repl_integer(store, i), repl_integer(store, i))
  i += 1
 end
 before := store.hashes[store.integers[h]].keys.size()
 i = 0
 while i < 990
  repl_hash_delete(store, h, repl_integer(store, i))
  i += 1
 end
 table := store.hashes[store.integers[h]]
 puts(table.keys.size() < before)
 puts(table.sizes[0])
 mut slot := 0
 while slot < table.keys.size()
  if table.keys[slot] <= 0 && table.values[slot] != 0
   puts("retained deleted payload")
  end
  slot += 1
 end
 while i < 1000
  repl_hash_delete(store, h, repl_integer(store, i))
  i += 1
 end
 puts(store.hashes[store.integers[h]].keys.size())
 puts(store.hashes[store.integers[h]].values.size())
end
''')
        output = root / 'storage'
        run('build', '--compile', '--outfile', output, source)
        assert subprocess.check_output([output], text=True, timeout=60) == 'true\n10\n0\n0\n'
    print('Native Hash behavior, GC, REPL and bucket reclamation passed')


if __name__ == '__main__':
    main()
