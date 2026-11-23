#!/usr/bin/env python3
"""Build a pinned static gzip archive; repository runtime stays in TypeRB."""
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

OPTIONS = ['-DLIBDEFLATE_BUILD_SHARED_LIB=OFF', '-DLIBDEFLATE_BUILD_GZIP=OFF',
           '-DLIBDEFLATE_BUILD_TESTS=OFF', '-DLIBDEFLATE_INSTALL=OFF',
           '-DLIBDEFLATE_DECOMPRESSION_SUPPORT=OFF', '-DLIBDEFLATE_ZLIB_SUPPORT=OFF']


def run(*args):
    subprocess.run(list(map(str, args)), check=True, timeout=180,
                   env=dict(os.environ, ZERO_AR_DATE='1'))


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
    pins = json.loads(Path(__file__).with_name('compression-dependencies.json').read_text())
    pin = pins['libdeflate']
    archive = args.archives / ('libdeflate-' + pin['version'] + '.tar.gz')
    if archive.exists():
        if archive.stat().st_size > 8 * 1024 * 1024:
            raise ValueError('oversized source archive')
        data = archive.read_bytes()
    else:
        with urllib.request.urlopen(pin['url'], timeout=60) as response:
            data = response.read(8 * 1024 * 1024 + 1)
    if len(data) > 8 * 1024 * 1024 or hashlib.sha256(data).hexdigest() != pin['sha256']:
        raise ValueError('source archive checksum or size differs')
    if not archive.exists():
        archive.write_bytes(data)
    cc = os.environ.get('CC', 'cc')
    started = time.monotonic()
    with tempfile.TemporaryDirectory(prefix='.compression-build-', dir=output.parent) as temporary:
        root = Path(temporary)
        with tarfile.open(archive) as source:
            total = 0
            for member in source.getmembers():
                name = Path(member.name)
                total += member.size
                if name.is_absolute() or '..' in name.parts or not (member.isdir() or member.isfile()) or total > 32 * 1024 * 1024:
                    raise ValueError('unsafe source archive entry')
            source.extractall(root, filter='data')
        source = root / pin['root']
        build = root / 'build'
        probe = root / 'abi.c'
        probe.write_text('#include <stddef.h>\n#include <libdeflate.h>\n'
                         '_Static_assert(sizeof(void*) == 8, "LP64 pointers");\n'
                         '_Static_assert(sizeof(size_t) == 8, "LP64 size_t");\n'
                         '_Static_assert(sizeof(int) == 4, "C int");\n')
        run(cc, '-std=c11', '-I', source, '-c', probe, '-o', root / 'abi.o')
        run('cmake', '-S', source, '-B', build, '-DCMAKE_BUILD_TYPE=Release',
            '-DCMAKE_C_COMPILER=' + cc,
            '-DCMAKE_C_FLAGS=-fPIC -ffile-prefix-map=' + str(root) + '=.', *OPTIONS)
        run('cmake', '--build', build, '--parallel', '2')
        publication = root / 'publication'
        publication.mkdir()
        shutil.copy2(build / 'libdeflate.a', publication / 'libdeflate.a')
        shutil.copy2(source / 'COPYING', publication / 'libdeflate-LICENSE')
        receipt = {'schema': 2, 'linkage': 'static', 'target': target, 'pins': pins,
                   'configuration': {'cmake': OPTIONS, 'level': 'library default (-1)',
                                     'formats': ['gzip'], 'streaming': False,
                                     'position_independent': True, 'file_prefix_map': True},
                   'cc': subprocess.check_output([cc, '--version'], text=True).splitlines()[0],
                   'cmake': subprocess.check_output(['cmake', '--version'], text=True).splitlines()[0],
                   'builder_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                   'artifact_tool_sha256': hashlib.sha256(Path(__file__).with_name('network_artifacts.py').read_bytes()).hexdigest(),
                   'libraries': {'libdeflate': hashlib.sha256((publication / 'libdeflate.a').read_bytes()).hexdigest()}}
        embed_identity(receipt, root / 'identity.s', 'compression')
        run(cc, '-c', root / 'identity.s', '-o', publication / 'compression-identity.o')
        receipt['files'] = {p.name: {'bytes': p.stat().st_size, 'sha256': hashlib.sha256(p.read_bytes()).hexdigest()}
                            for p in sorted(publication.iterdir())}
        receipt['build_seconds'] = time.monotonic() - started
        (publication / 'compression-build.json').write_text(json.dumps(receipt, indent=2) + '\n')
        verify(publication, target, 'compression')
        publication.rename(output)
    print(output)


if __name__ == '__main__':
    main()
