#!/usr/bin/env python3
"""Pinned portable digest, HMAC and comparison contracts, including REPL."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time

from network_artifact_smoke import imports
from network_artifacts import artifact_spec, inspect, install, pack, target_name

PROBE = r'''
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
extern int OPENSSL_init_crypto(uint64_t, const void *);
extern int EVP_Q_digest(void *, const char *, const char *, const void *, size_t, unsigned char *, size_t *);
extern unsigned char *EVP_Q_mac(void *, const char *, const char *, const char *, const void *,
    const void *, size_t, const unsigned char *, size_t, unsigned char *, size_t, size_t *);
extern int CRYPTO_memcmp(const void *, const void *, size_t);
extern void OPENSSL_cleanse(void *, size_t);
static int digests, macs, comparisons, cleanses;
static size_t compared_bytes;
static int fault(const char *name) { const char *value = getenv("CRYPTO_TEST_FAULT"); return value && !strcmp(value, name); }
int crypto_test_init(uint64_t options, const void *settings) {
    if (options != 128 || settings) abort();
    return fault("initialize") ? 0 : OPENSSL_init_crypto(options, settings);
}
int crypto_test_digest(void *context, const char *name, const char *properties, const void *input,
    size_t length, unsigned char *output, size_t *used) {
    digests++;
    int result = EVP_Q_digest(context, name, properties, input, length, output, used);
    if (fault("length")) (*used)++;
    return fault("digest") ? 0 : result;
}
unsigned char *crypto_test_mac(void *context, const char *name, const char *properties,
    const char *algorithm, const void *parameters, const void *key, size_t key_length,
    const unsigned char *input, size_t length, unsigned char *output, size_t capacity, size_t *used) {
    macs++;
    unsigned char *result = EVP_Q_mac(context, name, properties, algorithm, parameters, key,
        key_length, input, length, output, capacity, used);
    return fault("hmac") ? NULL : result;
}
int crypto_test_compare(const void *left, const void *right, size_t length) {
    comparisons++; compared_bytes += length;
    return CRYPTO_memcmp(left, right, length);
}
void crypto_test_cleanse(void *buffer, size_t size) {
    if (size != 64) abort();
    OPENSSL_cleanse(buffer, size); cleanses++;
    for (size_t i = 0; i < size; i++) if (((unsigned char *)buffer)[i]) abort();
}
__attribute__((destructor)) static void report(void) {
    fprintf(stderr, "crypto-test,%d,%d,%d,%zu,%d\n", digests, macs, comparisons, compared_bytes, cleanses);
}
'''


def instrument(source):
    lines, owned, ordinary, hooks = [], False, False, 0
    replacements = {'OPENSSL_init_crypto': 'crypto_test_init', 'EVP_Q_digest': 'crypto_test_digest',
                    'EVP_Q_mac': 'crypto_test_mac', 'CRYPTO_memcmp': 'crypto_test_compare',
                    'OPENSSL_cleanse': 'crypto_test_cleanse'}
    for line in source.splitlines():
        if line.startswith('function '):
            owned = '$trbn_crypto_' in line
            ordinary = '$trbnf' in line
        if line == '}':
            owned = ordinary = False
        if (owned and 'call $trbn_string_alloc(' in line) or (ordinary and any(marker in line for marker in (
                'call $trbn_array_new(', 'call $trbn_array_push_', 'call $trbn_gc_alloc(', 'call $trbn_string_concat('))):
            lines.append(' call $trbn_gc_collect(w 0)')
            hooks += 1
        if owned:
            for original, replacement in replacements.items():
                line = line.replace('call $' + original + '(', 'call $' + replacement + '(')
        lines.append(line)
    assert hooks > 0
    return '\n'.join(lines) + '\n'


def probe_statistics(result):
    rows = [line for line in result.stderr.splitlines() if line.startswith('crypto-test,')]
    assert len(rows) == 1, result
    return list(map(int, rows[0].split(',')[1:]))


def byte_literal(hexadecimal):
    return '"' + ''.join('\\x' + hexadecimal[i:i + 2] for i in range(0, len(hexadecimal), 2)) + '"'


def source_for(corpus):
    lines = ['import trb/std/digest', 'import trb/std/hmac', 'import trb/std/secure_compare',
             'import trb/std/encoding/hex', 'def probe()']
    expected = []
    for i, row in enumerate(corpus['digests']):
        name = 'value' + str(i)
        lines += ['mut ' + name + ' := ""', 'mut count' + str(i) + ' := 0',
                  'while count' + str(i) + ' < ' + str(row['repetitions']),
                  name + ' += ' + byte_literal(row['unit_hex']), 'count' + str(i) + ' += 1', 'end']
        for algorithm, answer in row['expected'].items():
            prefix = 'digest:' + row['id'] + ':' + algorithm + ':'
            lines.append('puts(' + json.dumps(prefix) + ' + Hex.encode(Digest.' + algorithm + '(' + name + '.to_bytes())))')
            expected.append(prefix + answer)
    for row in corpus['hmac']:
        for algorithm, answer in row['expected'].items():
            prefix = 'hmac:' + row['id'] + ':' + algorithm + ':'
            lines.append('puts(' + json.dumps(prefix) + ' + Hex.encode(HMAC.' + algorithm + '(' +
                         byte_literal(row['key_hex']) + '.to_bytes(), ' + byte_literal(row['message_hex']) + '.to_bytes())))')
            expected.append(prefix + answer)
    for row in corpus['comparison']:
        for namespace in ('HMAC', 'SecureCompare'):
            prefix = 'compare:' + row['id'] + ':' + namespace + ':'
            lines += ['if ' + namespace + '.equal(' + byte_literal(row['left_hex']) + '.to_bytes(), ' +
                      byte_literal(row['right_hex']) + '.to_bytes())', 'puts(' + json.dumps(prefix + 'true') + ')',
                      'else', 'puts(' + json.dumps(prefix + 'false') + ')', 'end']
            expected.append(prefix + str(row['equal']).lower())
    lines += ['end', 'def main()', 'probe()', 'end', '']
    return '\n'.join(lines), '\n'.join(expected) + '\n'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    parser.add_argument('--reference', action='store_true')
    parser.add_argument('--libraries', type=Path)
    parser.add_argument('--core', action='store_true')
    parser.add_argument('--qbe', type=Path)
    parser.add_argument('--target')
    parser.add_argument('--measure', action='store_true')
    parser.add_argument('--receipt', type=Path, required=True)
    args = parser.parse_args()
    if not args.reference and not args.libraries:
        parser.error('Native requires a pinned crypto installation')
    if args.core and (not args.qbe or not args.target):
        parser.error('--core requires --qbe and --target')
    if args.reference and (args.core or args.measure):
        parser.error('reference cannot use Native qualification modes')
    repository = Path(__file__).resolve().parent.parent
    manifest = Path(__file__).with_name('crypto-cases.json')
    corpus = json.loads(manifest.read_text())
    assert corpus['reference_revision'] == (repository / 'TYPE_RB_REVISION').read_text().strip()
    source_text, expected = source_for(corpus)
    assert len(expected.splitlines()) == 82
    binary = args.binary.resolve()
    receipt = {'reference_revision': corpus['reference_revision'],
               'corpus_sha256': hashlib.sha256(manifest.read_bytes()).hexdigest(),
               'source_sha256': hashlib.sha256(source_text.encode()).hexdigest(),
               'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
               'mode': 'reference' if args.reference else 'native', 'observations': 82}
    with tempfile.TemporaryDirectory(prefix='crypto contracts ') as temporary:
        root = Path(temporary)
        source = root / 'src/main.trb'
        source.parent.mkdir()
        source.write_text(source_text)
        config = root / 'trbconfig.jsonc'
        config.write_text(json.dumps({'name': 'crypto-contracts', 'mode': 'go' if args.reference else 'trb',
                                     'sourceDir': 'src', 'go': {'module': 'example.com/crypto-contracts'}}))
        env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))
        for key in ('TRBN_CRYPTO_LIB_DIR', 'TRBN_QBE', 'TRBN_CC', 'LD_LIBRARY_PATH', 'DYLD_LIBRARY_PATH'):
            env.pop(key, None)

        def run(arguments, input=None, success=True, environment=env, timeout=180):
            result = subprocess.run(list(map(str, arguments)), input=input, text=True, capture_output=True,
                                    env=environment, cwd=root, timeout=timeout)
            assert (result.returncode == 0) == success, (arguments, result.stdout, result.stderr)
            return result

        def build(input_path, output, success=True):
            if args.core:
                command = [binary, 'build', input_path, '--output', output, '--qbe', binary.with_name('qbe'),
                           '--cc', '/usr/bin/cc', '--target', args.target]
            else:
                command = [binary, 'build', '--compile', *(['--config'] if input_path == config else []),
                           input_path, '--outfile', output]
            return run(command, success=success)

        program = root / 'application'
        if not args.reference:
            package = root / 'installed compiler'
            package.mkdir()
            shutil.copy2(binary, package / 'trbn')
            shutil.copy2(args.qbe.resolve() if args.qbe else binary.with_name('qbe'), package / 'qbe')
            binary = package / 'trbn'
            plain = root / 'plain.trb'
            plain.write_text('def compiler_set_crypto_link(_needed: Boolean)\nend\ndef main()\ncompiler_set_crypto_link(true)\nputs(7)\nend\n')
            # A standalone file must not inherit the configured crypto project.
            config.unlink()
            build(plain, program)
            assert artifact_spec('crypto')[2] not in program.read_bytes()
            assert run([program], environment={'PATH': ''}).stdout == '7\n'
            receipt['plain_bytes'] = program.stat().st_size
            config.write_text(json.dumps({'name': 'crypto-contracts', 'mode': 'trb', 'sourceDir': 'src'}))
            sentinel = b'previous executable\n'
            program.write_bytes(sentinel)
            rejected = build(config, program, success=False)
            assert 'pinned static cryptography library is missing' in rejected.stderr, rejected
            assert program.read_bytes() == sentinel and not list(root.glob('application.*'))
            archive = root / 'crypto.tar.gz'
            install(archive, package / 'crypto', pack(args.libraries, archive, 'crypto'), target_name(), 'crypto')
            corrupt = root / 'corrupt crypto'
            shutil.copytree(package / 'crypto', corrupt)
            (corrupt / 'libcrypto.a').write_bytes(b'invalid archive\n')
            env['TRBN_CRYPTO_LIB_DIR'] = str(corrupt)
            rejected = build(config, program, success=False)
            env.pop('TRBN_CRYPTO_LIB_DIR')
            assert 'compiler: cc failed' in rejected.stderr, rejected
            assert program.read_bytes() == sentinel and not list(root.glob('application.*'))
            shutil.rmtree(corrupt)
        run([binary, 'check', source])
        run([binary, 'check', *([] if args.core else ['--config']), config])
        if not args.reference:
            build(source, program)
            assert run([program]).stdout == expected
        build(config, program)
        result = run([program])
        assert result.stdout == expected and not result.stderr, result
        receipt['paths'] = ['source check', 'project check', 'compiled execution']
        if not args.core:
            result = run([binary, 'run', '--config', config])
            assert result.stdout == expected and not result.stderr, result
            result = run([binary, 'repl', '--config', config], 'import { probe } from main\nprobe()\n:quit\n')
            assert result.stdout == expected and not result.stderr, result
            receipt['paths'] += ['run', 'retained REPL']
        if not args.reference:
            receipt['identity'] = inspect(program, 'crypto')
            receipt['executable_bytes'] = program.stat().st_size
            receipt['imports'] = imports(program)
            assert not any('libcrypto' in item or 'libssl' in item for item in receipt['imports'])
            # Opening this configuration file would block. A successful bounded
            # run demonstrates that the provider ignores OPENSSL_CONF entirely.
            configuration = root / 'blocked configuration'
            os.mkfifo(configuration)
            isolated_env = {'PATH': '', 'OPENSSL_CONF': str(configuration),
                            'OPENSSL_MODULES': str(root / 'absent modules'),
                            'OPENSSL_ENGINES': str(root / 'absent engines')}
            result = run([program], environment=isolated_env, timeout=10)
            assert result.stdout == expected and not result.stderr, result
            receipt['configuration_isolation'] = True
            emitted = run([binary, *([] if args.core else ['--internal-driver']), 'emit-qbe', config]).stdout
            il, assembly, probe = root / 'stress.ssa', root / 'stress.s', root / 'probe.c'
            il.write_text(instrument(emitted))
            probe.write_text(PROBE)
            qbe_target = {'darwin-arm64': 'arm64_apple', 'darwin-amd64': 'amd64_apple',
                          'linux-arm64': 'arm64', 'linux-amd64': 'amd64_sysv'}[target_name()]
            run([binary.with_name('qbe'), '-t', qbe_target, '-o', assembly, il])
            flags = ['-fuse-ld=lld', '-pthread', '-ldl'] if target_name().startswith('linux-') else []
            stressed = root / 'stressed'
            run(['/usr/bin/cc', '-Werror', *flags, assembly, probe, package / 'crypto/libcrypto.a',
                 package / 'crypto/crypto-identity.o', '-lm', '-o', stressed])
            observed = run([stressed], environment=dict(isolated_env, TYPE_RB_NATIVE_RUNTIME_STATS='1'))
            assert observed.stdout == expected, observed
            equal_lengths = [row for row in corpus['comparison'] if len(row['left_hex']) == len(row['right_hex'])]
            compared_bytes = sum(len(row['left_hex']) // 2 for row in equal_lengths) * 2
            assert probe_statistics(observed) == [48, 14, len(equal_lengths) * 2, compared_bytes, 62], observed
            gc = {line.split(',')[1]: int(line.split(',')[2]) for line in observed.stderr.splitlines()
                  if line.startswith('type-rb-native-gc-stat-v1,')}
            assert gc['collections'] > 0 and gc['live-bytes'] == 0, gc
            receipt['forced_gc'] = gc
            receipt['provider_calls'] = probe_statistics(observed)
            for failure in ('initialize', 'digest', 'length', 'hmac'):
                observed = run([stressed], success=False, environment=dict(isolated_env, CRYPTO_TEST_FAULT=failure))
                assert observed.stderr.startswith('panic: cryptography provider failed\n') and '\x00' not in observed.stderr, observed
                counts = probe_statistics(observed)
                assert counts[-1] == (0 if failure == 'initialize' else 49 if failure == 'hmac' else 1), (failure, counts)
            receipt['failure_cleanup'] = ['initialize', 'digest', 'length', 'hmac']
            if args.measure:
                # One pre-registered diagnostic; correctness reruns omit it.
                candidates = {
                    'digest': 'import trb/std/digest\ndef main()\nputs(Digest.sha256("abc".to_bytes()).size())\nend\n',
                    'plain': 'def main()\nputs("01234567890123456789012345678901".to_bytes().size())\nend\n'}
                comparison = {role: {'build_seconds': [], 'startup_seconds': []} for role in candidates}
                for repetition in range(3):
                    for role in (['plain', 'digest'] if repetition % 2 == 0 else ['digest', 'plain']):
                        source.write_text(candidates[role])
                        executable = root / role
                        started = time.monotonic()
                        build(config, executable)
                        comparison[role]['build_seconds'].append(time.monotonic() - started)
                        started = time.monotonic()
                        result = run([executable], environment=isolated_env, timeout=10)
                        comparison[role]['startup_seconds'].append(time.monotonic() - started)
                        assert result.stdout == '32\n' and not result.stderr, result
                        comparison[role]['executable_bytes'] = executable.stat().st_size
                comparison['limit_bytes'] = 4 * 1024 * 1024
                comparison['increment_bytes'] = comparison['digest']['executable_bytes'] - comparison['plain']['executable_bytes']
                comparison['within_limit'] = comparison['increment_bytes'] <= comparison['limit_bytes']
                receipt['dependency_cost'] = comparison
                source.write_text(source_text)
            shutil.rmtree(package)
            result = run([program], environment=isolated_env, timeout=10)
            assert result.stdout == expected and not result.stderr, result
            receipt['isolated_executable'] = True
    args.receipt.write_text(json.dumps(receipt, indent=2) + '\n')
    print('82 digest/HMAC/comparison observations passed in compiled execution, run and REPL')


if __name__ == '__main__':
    main()
