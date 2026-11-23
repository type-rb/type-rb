#!/usr/bin/env python3
"""Build the qualified static OpenSSL profile; language adapters stay in TypeRB."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import time
import urllib.request

from network_artifacts import embed_identity, target_name, verify

OPTIONS = ['no-shared', 'no-module', 'no-dso', 'no-engine', 'no-legacy',
           'no-tests', 'no-apps', 'no-docs', 'no-autoload-config']
TARGETS = {'darwin-arm64': 'darwin64-arm64-cc', 'darwin-amd64': 'darwin64-x86_64-cc',
           'linux-arm64': 'linux-aarch64', 'linux-amd64': 'linux-x86_64'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--archives', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    output = args.output.resolve()
    if output.exists():
        raise ValueError('use a new installation directory; existing archives are not replaced')
    target = target_name()
    output.parent.mkdir(parents=True, exist_ok=True)
    args.archives.mkdir(parents=True, exist_ok=True)
    pins = json.loads(Path(__file__).with_name('crypto-dependencies.json').read_text())
    pin = pins['openssl']
    archive = args.archives / (pin['root'] + '.tar.gz')
    if archive.exists():
        if archive.stat().st_size > 64 * 1024 * 1024:
            raise ValueError('oversized source archive')
        data = archive.read_bytes()
    else:
        with urllib.request.urlopen(pin['url'], timeout=60) as response:
            data = response.read(64 * 1024 * 1024 + 1)
    if len(data) > 64 * 1024 * 1024 or hashlib.sha256(data).hexdigest() != pin['sha256']:
        raise ValueError('source archive checksum or size differs')
    if not archive.exists():
        archive.write_bytes(data)
    cc = os.environ.get('CC', 'cc')
    started = time.monotonic()
    with tempfile.TemporaryDirectory(prefix='.crypto-build-', dir=output.parent) as temporary:
        root = Path(temporary)
        with tarfile.open(archive) as source:
            total = 0
            for member in source.getmembers():
                name = Path(member.name)
                total += member.size
                if name.is_absolute() or '..' in name.parts or not (member.isdir() or member.isfile()) or total > 512 * 1024 * 1024:
                    raise ValueError('unsafe source archive entry')
            source.extractall(root, filter='data')
        source = root / pin['root']
        flags = '-O2 -fPIC -ffunction-sections -fdata-sections -ffile-prefix-map=' + str(root) + '=.'
        environment = dict(os.environ, CC=cc, CFLAGS=flags, ZERO_AR_DATE='1', SOURCE_DATE_EPOCH=str(pin['source_date_epoch']))
        for key in ('OPENSSL_CONF', 'OPENSSL_MODULES', 'OPENSSL_ENGINES'):
            environment.pop(key, None)

        def run(*arguments, cwd=source):
            subprocess.run(list(map(str, arguments)), cwd=cwd, env=environment, check=True, timeout=600)

        run('perl', 'Configure', TARGETS[target], *OPTIONS, '--prefix=/trbn/openssl',
            '--openssldir=/trbn/openssl/no-configuration')
        run('make', '-j2', 'build_libs')
        # ABI and configuration probe only; no repository runtime implementation in C.
        probe = root / 'abi.c'
        probe.write_text('#include <stddef.h>\n#include <openssl/crypto.h>\n#include <openssl/evp.h>\n'
                         '_Static_assert(sizeof(void*) == 8 && sizeof(size_t) == 8, "LP64");\n'
                         '_Static_assert(sizeof(int) == 4, "C int");\n'
                         '_Static_assert(OPENSSL_INIT_NO_LOAD_CONFIG == 128, "initialization flag");\n'
                         'int main(void) { unsigned char out[64]; size_t size = 0;\n'
                         'if (!OPENSSL_init_crypto(OPENSSL_INIT_NO_LOAD_CONFIG, NULL)) return 1;\n'
                         'if (!EVP_Q_digest(NULL,"SHA256",NULL,"abc",3,out,&size) || size != 32) return 2;\n'
                         'if (!EVP_Q_mac(NULL,"HMAC",NULL,"SHA256",NULL,"",0,(const unsigned char *)"",0,out,sizeof(out),&size) || size != 32) return 3;\n'
                         'return CRYPTO_memcmp(out,out,size); }\n')
        system_flags = ['-pthread', '-ldl'] if target.startswith('linux-') else []
        run(cc, '-std=c11', '-Werror', '-I', source / 'include', probe, source / 'libcrypto.a', *system_flags, '-o', root / 'abi')
        run(root / 'abi')
        publication = root / 'publication'
        publication.mkdir()
        shutil.copy2(source / 'libcrypto.a', publication / 'libcrypto.a')
        shutil.copy2(source / 'LICENSE.txt', publication / 'openssl-LICENSE')
        receipt = {'schema': 2, 'linkage': 'static', 'target': target, 'pins': pins,
                   'configuration': {'configure': OPTIONS, 'target': TARGETS[target],
                                     'cflags': '-O2 -fPIC -ffunction-sections -fdata-sections -ffile-prefix-map=<build>=.',
                                     'prefix': '/trbn/openssl', 'openssldir': '/trbn/openssl/no-configuration',
                                     'initialization': 'OPENSSL_INIT_NO_LOAD_CONFIG', 'assembly': True,
                                     'source_date_epoch': pin['source_date_epoch']},
                   'cc': subprocess.check_output([cc, '--version'], text=True).splitlines()[0],
                   'cmake': 'not used; OpenSSL Configure and make',
                   'builder_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                   'artifact_tool_sha256': hashlib.sha256(Path(__file__).with_name('network_artifacts.py').read_bytes()).hexdigest(),
                   'libraries': {'libcrypto': hashlib.sha256((publication / 'libcrypto.a').read_bytes()).hexdigest()}}
        embed_identity(receipt, root / 'identity.s', 'crypto')
        run(cc, '-c', root / 'identity.s', '-o', publication / 'crypto-identity.o')
        receipt['files'] = {p.name: {'bytes': p.stat().st_size, 'sha256': hashlib.sha256(p.read_bytes()).hexdigest()}
                            for p in sorted(publication.iterdir())}
        receipt['build_seconds'] = time.monotonic() - started
        (publication / 'crypto-build.json').write_text(json.dumps(receipt, indent=2) + '\n')
        verify(publication, target, 'crypto')
        publication.rename(output)
    print(output)


if __name__ == '__main__':
    main()
