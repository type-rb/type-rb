#!/usr/bin/env python3
"""Check portable Integer parsing, error priority, bounded reads and retention."""
import argparse
import json
import os
from pathlib import Path
import re
import struct
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('binary', type=Path)
parser.add_argument('--reference', type=Path)
args = parser.parse_args()
binary = args.binary.resolve()
limit = 9007199254740991


def expected(text):
    if not re.fullmatch(rb'[+-]?[0-9]+', text):
        return 1, 73
    value = int(text)
    return (2, 73) if abs(value) > limit else (0, value)


cases = [b'', b'+', b'-', b'0', b'-0', b'+0001', b' 1', b'1 ', b'1_0',
         b'1.0', b'0x10', b'1e2', b'++1', b'--1', b'+-1']
for value in [limit - 1, limit, limit + 1, 2**63 - 1, 2**63, 2**64]:
    for sign in [b'', b'+', b'-']:
        cases += [sign + str(value).encode(), sign + b'0' * 128 + str(value).encode()]
for width in [8, 64, 1024]:
    cases += [b'0' * width, b'-' + b'0' * width, b'9' * width,
              b'9' * width + b'x', b'9' * width + b'\x00', b'1' + b'0' * width]
for byte in range(256):
    character = bytes([byte])
    cases += [character, b'12' + character + b'34', b'9007199254740992' + character,
              b'-999999999999999999999999' + character + b'1']
cases += [b'\xc0\xb1', b'\xed\xa0\x80', b'\xf4\x90\x80\x80',
          b'12\xf0\x9f\x98', '１２'.encode(), '−1'.encode()]
cases = list(dict.fromkeys(cases))

with tempfile.TemporaryDirectory(prefix='native-integer-parsing-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(command, *, data=None):
        result = subprocess.run([part if isinstance(part, bytes) else str(part) for part in command],
                                cwd=root, env=env, input=data, capture_output=True, timeout=180)
        assert result.returncode == 0, (command, result.stdout, result.stderr)
        return result

    source = '''import trb/std/process
import trb/std/result
import { NumberParseErrorKind } from trb/std/errors
def main()
Process.argv().each do |text|
case text.try_to_i()
when Result::Ok(value)
puts("ok")
puts(value)
when Result::Err(error)
case error.kind
when NumberParseErrorKind::InvalidFormat
puts("invalid")
when NumberParseErrorKind::OutOfRange
puts("range")
end
puts(error.input == text)
puts(error.message)
end
end
end
'''
    path = root / 'main.trb'
    path.write_text(source)
    il = run([binary, '--internal-driver', 'emit-qbe', path]).stdout
    # Export only the actual helper for an independent C observer. No runtime
    # replacement or allocation is needed. Failure must leave the output alone.
    helpers = re.findall(rb'^function l \$trbn_parse_integer\([^\n]*\) \{.*?^\}', il, re.M | re.S)
    assert len(helpers) == 1
    (root / 'parser.ssa').write_bytes(b'export ' + helpers[0] + b'\n')
    run([binary.with_name('qbe'), '-o', root / 'parser.s', root / 'parser.ssa'])
    (root / 'observer.c').write_text(r'''
#include <assert.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>
extern int64_t trbn_parse_integer(void *, int64_t *);
int main(void) {
    int64_t length, status, wanted;
    size_t page = (size_t)sysconf(_SC_PAGESIZE);
    unsigned char *memory = mmap(NULL, page * 2, PROT_READ | PROT_WRITE,
                                MAP_PRIVATE | MAP_ANON, -1, 0);
    assert(memory != MAP_FAILED);
    assert(mprotect(memory + page, page, PROT_NONE) == 0);
    while (fread(&length, sizeof length, 1, stdin) == 1) {
        assert(fread(&status, sizeof status, 1, stdin) == 1);
        assert(fread(&wanted, sizeof wanted, 1, stdin) == 1);
        assert(length >= 0 && (size_t)length + 32 < page);
        /* An aligned header followed by initialized bytes only. Inputs whose
           length is divisible by eight end exactly at the inaccessible page. */
        size_t padded = ((size_t)length + 7) & ~(size_t)7;
        unsigned char *text = memory + page - padded - 24;
        memset(text, 'x', padded + 24);
        memcpy(text + 8, &length, sizeof length);
        memcpy(text + 16, &length, sizeof length);
        assert(fread(text + 24, 1, (size_t)length, stdin) == (size_t)length);
        int64_t output = 73;
        assert(trbn_parse_integer(text, &output) == status);
        assert(output == wanted);
    }
    assert(!ferror(stdin));
    assert(munmap(memory, page * 2) == 0);
    return 0;
}
''')
    run(['/usr/bin/cc', root / 'parser.s', root / 'observer.c', '-o', root / 'observer'])
    payload = b''.join(struct.pack('=qqq', len(text), *expected(text)) + text for text in cases)
    raw_result = run([root / 'observer'], data=payload)
    assert not raw_result.stdout and not raw_result.stderr

    native = root / 'native'
    run([binary, 'build', '--compile', path, '--outfile', native])
    programs = [native]
    if args.reference:
        project = root / 'reference'
        (project / 'src').mkdir(parents=True)
        (project / 'src/main.trb').write_text(source)
        config = project / 'trbconfig.jsonc'
        config.write_text(json.dumps({
            'name': 'integer-parser-contract', 'version': '0.1.0', 'mode': 'go',
            'sourceDir': 'src', 'outDir': 'build', 'copyFiles': False,
            'packageManagement': 'managed',
            'go': {'module': 'example.org/integer-parser-contract', 'version': '1.27'},
        }))
        reference = root / 'reference-program'
        run([args.reference.resolve(), 'build', '--config', config, '--compile', '--outfile', reference])
        programs.append(reference)
    # Process arguments keep the public path dynamic, including malformed UTF-8.
    # Embedded NUL is covered by the bounded helper above and REPL below.
    dynamic = [text for text in cases if b'\x00' not in text]
    output = []
    for text in dynamic:
        status, value = expected(text)
        output.append(f'ok\n{value}\n' if status == 0 else
                      ('invalid\ntrue\ninvalid Integer\n' if status == 1 else
                       'range\ntrue\nInteger is outside the portable range\n'))
    wanted = ''.join(output).encode()
    for program in programs:
        actual = run([program, *dynamic])
        assert actual.stdout == wanted and not actual.stderr, (program, actual.stdout, actual.stderr)

    read_retained = b'''case saved
when Result::Ok(value)
puts(value)
when Result::Err(error)
puts(error.message)
end
case invalid
when Result::Ok(value)
puts(value)
when Result::Err(error)
puts(error.input == "9007199254740992x")
puts(error.message)
end
case nul
when Result::Ok(value)
puts(value)
when Result::Err(error)
puts(error.input == "999999999999999999999\\x00")
puts(error.message)
end
'''
    retained = b'''saved := "9007199254740991".try_to_i()
invalid := "9007199254740992x".try_to_i()
nul := "999999999999999999999\\x00".try_to_i()
record Later
value: Integer
end
"999999999999999999999x".to_i()
"9007199254740992".to_i()
''' + read_retained + b':reload\n' + read_retained + b':quit\n'
    retained_output = b'9007199254740991\ntrue\ninvalid Integer\ntrue\ninvalid Integer\n'
    repls = [binary] + ([args.reference.resolve()] if args.reference else [])
    for repl in repls:
        result = run([repl, 'repl'], data=retained)
        assert result.stdout.count(retained_output) >= 2, result.stdout
        assert b'NumberParseErrorKind::OutOfRange' not in result.stdout, result.stdout
        assert len(result.stderr.splitlines()) == 2, result.stderr
        assert b'invalid Integer' in result.stderr, result.stderr
        assert b'Integer is outside the portable range' in result.stderr, result.stderr
        assert b'reloaded' in result.stdout, result.stdout

print(f'PASS Integer parsing: {len(cases)} bounded/sentinel cases, {len(dynamic)} dynamic cases, error priority and retained REPL')
