#!/usr/bin/env python3
"""Selective OIDC linking, failed-build preservation and relocated applications."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time

from network_artifact_smoke import imports
from network_artifacts import artifact_spec, inspect, install, pack, target_name

COMMON = '''import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
'''
REQUEST = '''request := Request.new(method: HttpMethod.get(), path: "/", query_string: "", headers: Headers.new(), body: Body.empty())
'''
PLAIN = COMMON + 'def main()\n' + REQUEST.replace('request :=', '_request :=') + 'puts("OIDC bearer configuration is invalid")\nend\n'
OIDC = COMMON + '''import { OidcBearerOptions } from trb/auth/oidc
import trb/web/auth/bearer
import trb/std/result
def main()
''' + REQUEST + '''case Bearer.verify(request, OidcBearerOptions.new(issuer: "", audience: "api", jwks_uri: nil, roles_claim: "roles", clock_skew_seconds: 0))
when Result::Err(error)
puts(error.message)
when Result::Ok(_principal)
puts("unexpected success")
end
end
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    parser.add_argument('--libraries', type=Path, required=True)
    parser.add_argument('--receipt', type=Path, required=True)
    parser.add_argument('--measure', action='store_true', help='one separately registered three-pair size/build/startup observation')
    args = parser.parse_args()
    binary = args.binary.resolve()
    receipt = {'target': target_name(), 'compiler_bytes': binary.stat().st_size}
    environment = dict(os.environ)
    with tempfile.TemporaryDirectory(prefix='oidc distribution ') as temporary:
        root = Path(temporary)
        package = root / 'installed oidc'
        archive = root / 'oidc.tar.gz'
        install(archive, package, pack(args.libraries, archive, 'oidc'), target_name(), 'oidc')
        source, output = root / 'main.trb', root / 'application'

        def build(success=True):
            result = subprocess.run([str(binary), 'build', '--compile', str(source), '--outfile', str(output)],
                                    cwd=root, env=environment, capture_output=True, text=True, timeout=180)
            assert (result.returncode == 0) == success, result
            return result

        def run(expected):
            result = subprocess.run([str(output)], cwd=root, env={'PATH': '', 'OPENSSL_CONF': str(root / 'absent')},
                                    capture_output=True, text=True, timeout=10)
            assert result.returncode == 0 and result.stdout == expected and not result.stderr, result

        environment['TRBN_OIDC_LIB_DIR'] = str(root / 'absent')
        source.write_text('def compiler_set_oidc_link(_needed: Boolean)\nend\ndef main()\ncompiler_set_oidc_link(true)\nputs(7)\nend\n')
        build()
        run('7\n')
        assert artifact_spec('oidc')[2] not in output.read_bytes()
        receipt['authored_setter_has_no_effect'] = True
        source.write_text(OIDC)
        sentinel = b'previous executable\n'
        output.write_bytes(sentinel)
        result = build(False)
        assert 'pinned static OIDC libraries are missing' in result.stderr, result
        assert output.read_bytes() == sentinel and not list(root.glob('application.*'))
        receipt['missing_preserves_output'] = True
        original = (package / 'libcurl.a').read_bytes()
        (package / 'libcurl.a').write_bytes(b'invalid archive\n')
        environment['TRBN_OIDC_LIB_DIR'] = str(package)
        result = build(False)
        assert 'compiler: cc failed' in result.stderr, result
        assert output.read_bytes() == sentinel and not list(root.glob('application.*'))
        receipt['corruption_preserves_output'] = True
        (package / 'libcurl.a').write_bytes(original)
        build()
        receipt['identity'] = inspect(output, 'oidc')
        receipt['crypto_identity'] = inspect(output, 'crypto')
        receipt['executable_bytes'] = output.stat().st_size
        receipt['imports'] = imports(output)
        assert not any(part in name for name in receipt['imports'] for part in ('libcurl', 'libssl', 'libcrypto', 'libz.', 'libnghttp2', 'libicu'))
        run('OIDC bearer configuration is invalid\n')
        if args.measure:
            costs = {role: {'build_seconds': [], 'startup_seconds': []} for role in ('web', 'oidc')}
            for iteration in range(3):
                for role in (['web', 'oidc'] if iteration % 2 == 0 else ['oidc', 'web']):
                    source.write_text(PLAIN if role == 'web' else OIDC)
                    started = time.monotonic()
                    build()
                    costs[role]['build_seconds'].append(time.monotonic() - started)
                    started = time.monotonic()
                    run('OIDC bearer configuration is invalid\n')
                    costs[role]['startup_seconds'].append(time.monotonic() - started)
                    costs[role]['executable_bytes'] = output.stat().st_size
            costs['increment_bytes'] = costs['oidc']['executable_bytes'] - costs['web']['executable_bytes']
            receipt['dependency_cost'] = costs
            source.write_text(OIDC)
            build()
        shutil.rmtree(package)
        run('OIDC bearer configuration is invalid\n')
        receipt['relocated_without_library_directory'] = True
    args.receipt.write_text(json.dumps(receipt, indent=2) + '\n')
    print('OIDC selective linking, preservation and relocation passed')


if __name__ == '__main__':
    main()
