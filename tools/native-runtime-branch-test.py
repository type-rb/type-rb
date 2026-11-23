#!/usr/bin/env python3
"""Check full-width runtime conditions and portable Integer exponentiation."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import tempfile


def literal_il(path):
    return '\n'.join(json.loads(match[0]) for match in
                     re.finditer(r'"(?:[^"\\]|\\.)*"', path.read_text()))


def functions(il):
    return re.finditer(r'(?m)^(?:export )?function[^\n]*\([^\n]*\) \{\n.*?^\}', il, re.S)


def check_authored_conditions(root):
    # Complete authored functions, including functions emitted in multiple
    # literal chunks. This guard does not parse or rewrite generated programs.
    paths = sorted((root / 'compiler/src/backend/qbe').rglob('*.trb'))
    paths.append(root / 'compiler/cli/host.trb')
    checked = 0
    for path in paths:
        if path.name.endswith('_test.trb'):
            continue
        for match in functions(literal_il(path)):
            code = match[0]
            types = {name: cls for cls, name in
                     re.findall(r'\b([lwds])\s+(%[\w.]+)', code.split('\n', 1)[0])}
            types.update({name: cls for name, cls in
                          re.findall(r'(%[\w.]+)\s*=([lwds])\b', code)})
            for value in re.findall(r'\bjnz (%[\w.]+),', code):
                assert types.get(value) != 'l', (path, code.split('\n')[0], value,
                    'jnz tests only a word; compare the complete long with cnel first')
                checked += 1
    assert checked, 'no authored QBE branch conditions found'
    print(f'{checked} authored QBE branch conditions checked')


CASES = [(0, 0, 1), (0, 4294967296, 0), (0, 8589934592, 0),
         (1, 9007199254740991, 1), (-1, 4294967296, 1),
         (-1, 4294967297, -1), (-1, 8589934592, 1),
         (-1, 8589934593, -1), (-1, 9007199254740991, -1),
         (2, 32, 4294967296), (2, 52, 4503599627370496),
         (-2, 51, -2251799813685248)]
CASES += [(2, exponent, 'Integer is outside the portable range') for exponent in
          (53, 4294967296, 4294967297, 8589934592, 8589934593, 9007199254740991)]
CASES += [(base, exponent, 'negative Integer exponent')
          for base in (0, 1, -1, 2) for exponent in (-1, -4294967296)]


def check_results(executable):
    for base, exponent, expected in CASES:
        result = subprocess.run([str(executable), str(base), str(exponent)],
                                capture_output=True, text=True, timeout=10)
        if isinstance(expected, int):
            assert result.returncode == 0 and result.stdout == f'{expected}\n', (
                base, exponent, expected, result)
        else:
            assert result.returncode != 0 and expected in result.stderr, (
                base, exponent, expected, result)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--qbe', type=Path)
    parser.add_argument('--check-source', action='store_true', help='Check authored branch widths without a toolchain')
    parser.add_argument('--compiler', type=Path)
    parser.add_argument('--source-root', type=Path, help='Recorded source for a regression control')
    args = parser.parse_args()
    root = args.source_root or Path(__file__).resolve().parent.parent
    if args.check_source:
        check_authored_conditions(root)
        return
    if not args.qbe:
        parser.error('--qbe is required for runtime execution')
    runtime = root / 'compiler/src/backend/qbe/runtime'
    system = literal_il(runtime / 'system.trb')
    multiply = next(match[0] for match in functions(system) if '$trbn_integer_mul(' in match[0].split('\n')[0])
    message = next(line for line in system.splitlines() if line.startswith('data $trbn_integer_range_error ='))
    power = literal_il(runtime / 'power.trb')
    il = re.sub(r'(?m)^function ', 'export function ', multiply + '\n' + message + '\n' + power)
    with tempfile.TemporaryDirectory(prefix='native-runtime-branches-') as temporary:
        work = Path(temporary)
        (work / 'runtime.ssa').write_text(il)
        (work / 'driver.c').write_text(r'''
#include <inttypes.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
extern int64_t trbn_integer_power(int64_t, int64_t);
void trbn_fail(const char *message, int64_t length) {
 if(write(2,message,(size_t)length)!=length)exit(71);
 exit(70);
}
int main(int argc, char **argv) {
 if(argc!=3)return 64;
 printf("%" PRId64 "\n",trbn_integer_power(strtoll(argv[1],NULL,10),strtoll(argv[2],NULL,10)));
 return 0;
}
''')
        subprocess.run([str(args.qbe.resolve()), '-o', str(work / 'runtime.s'),
                        str(work / 'runtime.ssa')], check=True)
        executable = work / 'power'
        subprocess.run(['cc', '-Wall', '-Wextra', '-Werror', str(work / 'driver.c'),
                        str(work / 'runtime.s'), '-o', str(executable)], check=True)
        check_results(executable)
        print(f'{len(CASES)} actual-runtime exponent boundary checks passed')
        if args.compiler:
            source = work / 'main.trb'
            source.write_text('import trb/std/process\n\ndef main()\n'
                              'arguments := Process.argv()\n'
                              'puts(arguments[0].to_i() ** arguments[1].to_i())\nend\n')
            subprocess.run([str(args.compiler.resolve()), 'build', '--compile', str(source), '--outfile',
                            str(work / 'ordinary')], check=True, cwd=work)
            check_results(work / 'ordinary')
            print(f'{len(CASES)} ordinary-CLI exponent boundary checks passed')
    check_authored_conditions(root)


if __name__ == '__main__':
    main()
