#!/usr/bin/env python3
"""Verify selective links, failure cleanup, packaged paths and relocatable HTTP apps."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time

from network_artifacts import MARKER, identity, inspect, install, pack, target_name, verify
from network_artifact_smoke import imports

spec = importlib.util.spec_from_file_location('network_test', Path(__file__).with_name('native-web-network-test.py'))
network = importlib.util.module_from_spec(spec)
spec.loader.exec_module(network)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    parser.add_argument('--libraries', type=Path, required=True)
    parser.add_argument('--receipt', type=Path, required=True)
    parser.add_argument('--core', action='store_true')
    parser.add_argument('--target')
    parser.add_argument('--qbe', type=Path)
    parser.add_argument('--baseline-binary', type=Path)
    parser.add_argument('--baseline-libraries', type=Path)
    args = parser.parse_args()
    if bool(args.baseline_binary) != bool(args.baseline_libraries):
        parser.error('baseline requires both its binary and libraries')
    binary = args.binary.resolve()
    qbe = args.qbe.resolve() if args.qbe else binary.with_name('qbe')
    library_receipt = verify(args.libraries, target_name())
    observations = {'target': target_name(), 'build_seconds': [], 'startup_seconds': []}
    with tempfile.TemporaryDirectory(prefix='network distribution ') as temporary:
        root = Path(temporary)
        package = root / 'relocated compiler'
        package.mkdir()
        shutil.copy2(binary, package / 'trbn')
        shutil.copy2(qbe, package / 'qbe')
        archive = root / 'network.tar.gz'
        digest = pack(args.libraries, archive)
        install(archive, package / 'network', digest, target_name())
        environment = dict(os.environ)
        for key in ('TRBN_NETWORK_LIB_DIR', 'TRBN_QBE', 'TRBN_CC', 'LD_LIBRARY_PATH', 'DYLD_LIBRARY_PATH'):
            environment.pop(key, None)

        def build(source, output, env=environment, check=True):
            if args.core:
                command = [package / 'trbn', 'build', source, '--output', output,
                           '--qbe', package / 'qbe', '--cc', '/usr/bin/cc', '--target', args.target]
            else:
                source_args = ['--config', source] if source.name == 'trbconfig.jsonc' else [source]
                command = [package / 'trbn', 'build', *source_args, '--compile', '--outfile', output]
            result = subprocess.run(list(map(str, command)), env=env, capture_output=True, timeout=180)
            if check:
                assert result.returncode == 0, (result.stdout, result.stderr)
            return result

        plain = root / 'plain.trb'
        plain.write_text('def compiler_set_network_link(_needed: Boolean)\nend\ndef main()\ncompiler_set_network_link(true)\nputs("plain")\nend\n')
        simple = root / 'plain'
        missing = dict(environment, TRBN_NETWORK_LIB_DIR=str(root / 'missing'))
        build(plain, simple, missing)
        assert MARKER not in simple.read_bytes()
        assert subprocess.check_output([str(simple)], env={'PATH': ''}) == b'plain\n'
        observations['plain_bytes'] = simple.stat().st_size
        observations['plain_imports'] = imports(simple)

        port = network.free_port()
        for name, source in network.FILES.items():
            path = root / 'src' / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(source.replace('PORT', str(port)))
        config = root / 'trbconfig.jsonc'
        config.write_text(json.dumps({'name': 'distribution-test', 'mode': 'trb', 'sourceDir': 'src', 'outDir': 'out'}))
        program = root / 'server'
        sentinel = b'previous output survives failed links\n'
        program.write_bytes(sentinel)
        failed = build(config, program, missing, check=False)
        assert failed.returncode != 0 and b'pinned static HTTP libraries are missing' in failed.stderr, failed
        assert program.read_bytes() == sentinel
        assert not list(root.glob('server.*'))

        corrupt = root / 'corrupt'
        shutil.copytree(package / 'network', corrupt)
        (corrupt / 'libuv.a').write_bytes(b'invalid archive\n')
        failed = build(config, program, dict(environment, TRBN_NETWORK_LIB_DIR=str(corrupt)), check=False)
        assert failed.returncode != 0 and b'compiler: cc failed' in failed.stderr, failed
        assert program.read_bytes() == sentinel
        assert not list(root.glob('server.*'))
        for _ in range(3):
            start = time.monotonic()
            build(config, program)
            observations['build_seconds'].append(time.monotonic() - start)
        if args.baseline_binary:
            baseline = root / 'baseline-server'
            baseline_env = dict(environment, TRBN_NETWORK_LIB_DIR=str(args.baseline_libraries.resolve()),
                                TRBN_QBE=str(qbe))
            control = {'build_seconds': [], 'startup_seconds': []}
            for _ in range(3):
                start = time.monotonic()
                built = subprocess.run([str(args.baseline_binary.resolve()), 'build', '--config', str(config),
                                        '--compile', '--outfile', str(baseline)], env=baseline_env,
                                       capture_output=True, timeout=180)
                assert built.returncode == 0, (built.stdout, built.stderr)
                control['build_seconds'].append(time.monotonic() - start)
            control['http_bytes'] = baseline.stat().st_size
            control['sidecar_bytes'] = sum(path.stat().st_size for path in args.baseline_libraries.iterdir()
                                           if path.suffix in ('.so', '.dylib'))
            control['imports'] = imports(baseline)
            for _ in range(3):
                start = time.monotonic()
                child = network.run_server(baseline, port, baseline_env)
                control['startup_seconds'].append(time.monotonic() - start)
                child.terminate()
                stdout, stderr = child.communicate(timeout=5)
                assert child.returncode == 0, (stdout, stderr)
            observations['dynamic_baseline'] = control
        assert inspect(program) == identity(library_receipt)
        observations['http_bytes'] = program.stat().st_size
        observations['http_imports'] = imports(program)
        assert not any('libuv' in name or 'llhttp' in name for name in observations['http_imports'])
        isolated = root / 'runtime only'
        isolated.mkdir()
        program.rename(isolated / 'application')
        program = isolated / 'application'
        shutil.rmtree(package)
        shutil.rmtree(corrupt)
        runtime_environment = {'PATH': '', 'TRBN_NETWORK_LIB_DIR': str(root / 'missing')}
        for _ in range(3):
            start = time.monotonic()
            child = network.run_server(program, port, runtime_environment)
            observations['startup_seconds'].append(time.monotonic() - start)
            child.terminate()
            stdout, stderr = child.communicate(timeout=5)
            assert child.returncode == 0, (stdout, stderr)
        network.exercise(program, port, runtime_environment)
        observations['identity'] = inspect(program)
    args.receipt.write_text(json.dumps(observations, indent=2) + '\n')
    print('selective static links, preserved outputs, installed paths and isolated HTTP execution passed')


if __name__ == '__main__':
    main()
