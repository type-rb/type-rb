#!/usr/bin/env python3
"""Qualify the frozen application payload/time slice, including process inputs."""
import argparse
import datetime
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import signal
import struct
import subprocess
import sys
import tempfile
import zipfile
from native_network_link import static_network_flags

ROOT = Path(__file__).resolve().parent.parent
MANIFEST = ROOT / 'tools/native-time-services.json'
FROZEN_SHA256 = '78f231332848b8983e5a20d55f697702f51cbc5b65e38e895f6dd3f5d9b04565'


def tzif(offset):
    return (b'TZif\0' + bytes(15) + struct.pack('>6I', 0, 0, 0, 0, 1, 5)
            + struct.pack('>iBB', offset, 0, 0) + b'TEST\0')


def fixtures(root):
    for directory, content in [('zones', tzif(20700)), ('malformed', b'not TZif'),
                               ('v4', tzif(0)[:4] + b'4' + tzif(0)[5:])]:
        for name in ['Example/Native', 'Asia/Tokyo']:
            path = root / directory / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(content)
    for name, method in [('stored.zip', zipfile.ZIP_STORED), ('compressed.zip', zipfile.ZIP_DEFLATED)]:
        with zipfile.ZipFile(root / name, 'w', compression=method) as archive:
            for member in ['Example/Native', 'Asia/Tokyo']:
                entry = zipfile.ZipInfo(member, date_time=(2024, 1, 1, 0, 0, 0))
                entry.compress_type = method
                archive.writestr(entry, tzif(20700))
    (root / 'invalid.zip').write_bytes(b'not a zip')
    (root / 'truncated.zip').write_bytes((root / 'stored.zip').read_bytes()[:-12])
    (root / 'local.tzif').write_bytes(tzif(3600))


def invoke(binary, arguments, root, environment, source=None, normalize=True):
    child = subprocess.Popen([str(binary), *map(str, arguments)], cwd=root, env=environment,
                             stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                             start_new_session=True)
    try:
        stdout, stderr = child.communicate(source.encode() if source is not None else None, timeout=120)
        code = child.returncode
    except subprocess.TimeoutExpired:
        os.killpg(child.pid, signal.SIGKILL)
        stdout, stderr = child.communicate()
        code = 124
    except BaseException:
        if child.poll() is None:
            os.killpg(child.pid, signal.SIGKILL)
        child.communicate()
        raise
    result = {'code': code, 'stdout': stdout.decode('utf-8', 'backslashreplace'),
              'stderr': stderr.decode('utf-8', 'backslashreplace')}
    if normalize:
        for stream in ['stdout', 'stderr']:
            result[stream] = result[stream].replace(str(root), '<fixture>')
    return result


def local_expected(expected, environment):
    # Local-unset intentionally uses host data. Qualify that row against the
    # same-host reference when supplied; a Native-only lane records this
    # independent OS localtime probe instead of claiming a reference replay.
    script = ('import datetime,time; time.tzset(); '
              'print(datetime.datetime.fromtimestamp(1704132000).isoformat())')
    value = subprocess.check_output([sys.executable, '-c', script], env=environment,
                                    text=True, timeout=10).strip()
    result = json.loads(json.dumps(expected))
    for outcome in result.values():
        lines = outcome['stdout'].splitlines(keepends=True)
        lines[0] = 'ok|Local|' + value + '\n'
        outcome['stdout'] = ''.join(lines)
    return result


def stress(native, program, expected, root, environment, report, save):
    target = root / 'gc-stress'
    target.mkdir()
    main = target / 'main.trb'
    main.write_text(program['declarations'] + '\ndef main()\nmut iteration := 0\nwhile iteration < 8\n'
                    + program['body'] + '\niteration += 1\nend\nend\n')
    receipt = {'sourceSha256': hashlib.sha256(main.read_bytes()).hexdigest(), 'steps': [], 'complete': False}
    report['gcStress'] = receipt
    emitted = invoke(native, ['--internal-driver', 'emit-qbe', main], target, environment, normalize=False)
    receipt['steps'].append({'phase': 'emit', 'code': emitted['code'], 'stderr': emitted['stderr'],
                             'qbeSha256': hashlib.sha256(emitted['stdout'].encode()).hexdigest()})
    save()
    if emitted['code']:
        raise ValueError('time service GC program did not compile')
    forced = []
    ordinary = host = False
    hooks = {'ordinary': 0, 'host': 0}
    for line in emitted['stdout'].splitlines():
        if line.startswith('function ') or line.startswith('export function '):
            ordinary = '$trbnf' in line
            host = any('$' + name + '(' in line for name in ('trbn_time_zone_data', 'trbn_time_zone_environment'))
        if line == '}':
            ordinary = host = False
        allocation = (ordinary and any(marker in line for marker in
                      ('call $trbn_array_new(', 'call $trbn_array_new_reserved(', 'call $trbn_array_push_',
                       'call $trbn_array_grow_paced(', 'call $trbn_gc_alloc(', 'call $trbn_string_concat(')))
        host_allocation = host and any(marker in line for marker in ('call $trbn_string_alloc(', 'call $trbn_string_from_c('))
        if allocation or host_allocation:
            forced.append(' call $trbn_gc_collect(w 0)')
            hooks['host' if host_allocation else 'ordinary'] += 1
        forced.append(line)
    receipt['hooks'] = hooks
    save()
    if hooks['host'] < 4 or hooks['ordinary'] < 8:
        raise ValueError('GC qualification missed the actual time/managed boundaries')
    il, assembly, executable = target / 'time.ssa', target / 'time.s', target / 'time'
    il.write_text('\n'.join(forced) + '\n')
    qbe = native.with_name('qbe')
    commands = [('qbe', qbe, ['-o', assembly, il]),
                ('link', Path(os.environ.get('CC', '/usr/bin/cc')),
                 [assembly, *static_network_flags(qbe), '-lm', '-o', executable])]
    for phase, binary, arguments in commands:
        result = invoke(binary, arguments, target, environment)
        receipt['steps'].append({'phase': phase, **result})
        save()
        if result['code']:
            raise ValueError('GC fixture ' + phase + ' failed')
    executed = invoke(executable, [], root / 'fixtures', dict(environment, TYPE_RB_NATIVE_RUNTIME_STATS='1'))
    receipt['execute'] = executed
    save()
    if executed['code'] != 0 or executed['stdout'] != expected['execute']['stdout'] * 8:
        raise ValueError('time service values changed under forced collection')
    statistics = {}
    for line in executed['stderr'].splitlines():
        prefix, key, value = line.split(',')
        if prefix != 'type-rb-native-gc-stat-v1' or key in statistics:
            raise ValueError('unexpected GC diagnostic')
        statistics[key] = int(value)
    receipt['statistics'] = statistics
    if (statistics.get('collections', 0) < 8 or statistics.get('live-bytes') != 0
            or statistics.get('allocated-bytes') != statistics.get('reclaimed-bytes')):
        raise ValueError('time service roots or final reclamation differ')
    receipt['complete'] = True
    save()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--native', type=Path)
    parser.add_argument('--reference', type=Path)
    parser.add_argument('--report', required=True, type=Path)
    parser.add_argument('--gc-stress', action='store_true')
    args = parser.parse_args()
    if args.gc_stress and not args.native:
        parser.error('--gc-stress requires --native')
    if not args.native and not args.reference:
        parser.error('at least one compiler is required')
    compilers = {name: path.resolve() for name, path in [('reference', args.reference), ('native', args.native)] if path}
    manifest_bytes = MANIFEST.read_bytes()
    if hashlib.sha256(manifest_bytes).hexdigest() != FROZEN_SHA256:
        parser.error('service manifest differs from the reviewed reference observations')
    manifest = json.loads(manifest_bytes)
    # Keep source-era expectations immutable when the current oracle advances.
    # Every supplied compiler still has to match the unchanged outcomes below;
    # the reference lane qualifies the new pin rather than relabeling old proof.
    current_reference = (ROOT / 'TYPE_RB_REVISION').read_text().strip()
    spec = importlib.util.spec_from_file_location('language', ROOT / 'tools/native-language-coverage.py')
    language = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(language)
    registry = json.loads((ROOT / 'tools/native-language-cases.json').read_text())
    report = {'schemaVersion': 1, 'scope': manifest['scope'],
              'referenceRevision': current_reference,
              'expectedReferenceRevision': manifest['referenceRevision'],
              'manifestSha256': language.digest(MANIFEST),
              'compilers': {name: {'sha256': language.digest(binary)} for name, binary in compilers.items()},
              'registry': [], 'environment': [], 'builds': [], 'complete': False}

    def save():
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')

    failures = []
    try:
        with tempfile.TemporaryDirectory(prefix='native-time-services-') as temporary:
            root = Path(temporary).resolve()
            for entry in manifest['registry']:
                case = next(c for c in registry['cases'] if c['id'] == entry['id'])
                digest = hashlib.sha256(json.dumps(case, sort_keys=True).encode()).hexdigest()
                if digest != entry['caseSha256'] or language.parity_gaps(case):
                    raise ValueError('frozen registry case changed: ' + entry['id'])
                target = root / entry['id']
                target.mkdir()
                observed = language.collect(case, compilers.get('native'), compilers.get('reference'), target)
                matches = {role: language.outcomes_match(result, case[role]) for role, result in observed.items()}
                report['registry'].append({'id': entry['id'], 'matches': matches, 'observations': observed})
                save()
                if not all(matches.values()):
                    failures.append(entry['id'])
                print(entry['id'], matches, flush=True)

            data = root / 'fixtures'
            data.mkdir()
            fixtures(data)
            base_env = dict(os.environ, TZ='UTC', NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history.json'))
            base_env.pop('ZONEINFO', None)
            built = {}
            for kind, program in manifest['programs'].items():
                target = root / ('program-' + kind)
                target.mkdir()
                (target / 'main.trb').write_text(program['declarations'] + '\ndef main()\n' + program['body'] + '\nend\n')
                for role, compiler in compilers.items():
                    executable = target / role
                    checked = invoke(compiler, ['check', 'main.trb'], target, base_env)
                    result = invoke(compiler, ['build', '--compile', '--outfile', executable, 'main.trb'], target, base_env)
                    report['builds'].append({'program': kind, 'role': role, 'check': checked, 'build': result})
                    save()
                    if checked['code'] != 0 or result['code'] != 0 or not executable.is_file():
                        raise ValueError('service program did not check/build: ' + kind + '/' + role)
                    built[kind, role] = executable
            for group in manifest['environment']:
                kind = 'cache' if group['id'] == 'local-lifetime' else 'invalid' if group['id'] == 'zone-invalid-name' else 'zones'
                row = {'id': group['id'], 'variants': []}
                report['environment'].append(row)
                for variant in group['variants']:
                    env = dict(base_env)
                    for key, value in variant['environment'].items():
                        if value is None:
                            env.pop(key, None)
                        else:
                            env[key] = value.replace('<fixture>', str(data))
                    expected = variant['reference']
                    if group['id'] == 'local-unset':
                        expected = local_expected(expected, env)
                    result = {'environment': variant['environment'], 'expected': expected, 'observations': {}, 'matches': {}}
                    if group['id'] == 'local-unset':
                        result['hostOracle'] = 'same-host reference and OS localtime' if args.reference else 'OS localtime; no reference replay'
                    row['variants'].append(result)
                    for role, compiler in compilers.items():
                        (data / 'local.tzif').write_bytes(tzif(3600))
                        executed = invoke(built[kind, role], [], data, env)
                        (data / 'local.tzif').write_bytes(tzif(3600))
                        program = manifest['programs'][kind]
                        source = program['declarations'] + '\ndef probe()\n' + program['body'] + '\nend\nprobe()\n:quit\n'
                        repl = invoke(compiler, ['repl'], data, env, source)
                        observed = {'execute': executed, 'repl': repl}
                        result['observations'][role] = observed
                        result['matches'][role] = observed == expected
                        if observed != expected:
                            failures.append(group['id'] + '/' + role)
                        save()
                print(group['id'], [v['matches'] for v in row['variants']], flush=True)
            if args.gc_stress and not failures:
                env = dict(base_env, ZONEINFO=str(data / 'stored.zip'))
                expected = next(g for g in manifest['environment'] if g['id'] == 'zone-stored-zip')['variants'][0]['reference']
                stress(compilers['native'], manifest['programs']['zones'], expected, root, env, report, save)
            report['failures'] = failures
            report['complete'] = not failures
            save()
    except BaseException as error:
        report['error'] = type(error).__name__ + ': ' + str(error)
        raise
    finally:
        save()
    if failures:
        raise SystemExit('service contract differences: ' + ', '.join(failures))
    print('20 application service groups qualified for ' + ', '.join(compilers))


if __name__ == '__main__':
    main()
