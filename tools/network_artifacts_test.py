#!/usr/bin/env python3
import hashlib
import json
from pathlib import Path
import tarfile
import tempfile
import unittest

import network_artifacts as artifacts


class ArtifactsTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.directory = self.root / 'libraries'
        self.directory.mkdir()
        self.receipt = {'schema': 2, 'linkage': 'static', 'target': artifacts.target_name(),
                        'pins': json.loads(artifacts.PINS.read_text()), 'libraries': {},
                        'configuration': {}, 'cc': 'synthetic compiler', 'cmake': 'synthetic cmake',
                        'builder_sha256': '0' * 64, 'artifact_tool_sha256': '1' * 64}
        for name in artifacts.FILES - {'network-identity.o'}:
            (self.directory / name).write_bytes(name.encode())
        for name in ('libuv', 'libllhttp'):
            self.receipt['libraries'][name] = hashlib.sha256((self.directory / (name + '.a')).read_bytes()).hexdigest()
        (self.directory / 'network-identity.o').write_bytes(
            artifacts.MARKER + json.dumps(artifacts.identity(self.receipt)).encode() + b'\0')
        self.receipt['files'] = {p.name: {'sha256': hashlib.sha256(p.read_bytes()).hexdigest(), 'bytes': p.stat().st_size}
                                 for p in self.directory.iterdir()}
        self.save()

    def save(self):
        (self.directory / 'network-build.json').write_text(json.dumps(self.receipt))

    def test_pack_install_and_inspect_without_build_tools(self):
        archive = self.root / 'network.tar.gz'
        digest = artifacts.pack(self.directory, archive)
        output = self.root / 'a directory with spaces' / 'network'
        artifacts.install(archive, output, digest, artifacts.target_name())
        self.assertEqual(artifacts.verify(output, artifacts.target_name()), self.receipt)
        self.assertEqual(artifacts.inspect(output / 'network-identity.o'), artifacts.identity(self.receipt))
        with self.assertRaisesRegex(ValueError, 'already exists'):
            artifacts.install(archive, output, digest, artifacts.target_name())

    def test_rejects_wrong_host_and_source_pins(self):
        with self.assertRaisesRegex(ValueError, 'target differs'):
            artifacts.verify(self.directory, 'invalid-host')
        self.receipt['pins']['libuv']['commit'] = '1' * 40
        self.save()
        with self.assertRaisesRegex(ValueError, 'pins differ'):
            artifacts.verify(self.directory, artifacts.target_name())

    def test_rejects_corrupt_library_and_stale_inventory(self):
        (self.directory / 'libuv.a').write_bytes(b'corrupt')
        with self.assertRaisesRegex(ValueError, 'size differs|checksum differs'):
            artifacts.verify(self.directory, artifacts.target_name())
        (self.directory / 'untracked.so').write_bytes(b'old installation')
        with self.assertRaisesRegex(ValueError, 'unexpected'):
            artifacts.verify(self.directory, artifacts.target_name())

    def test_rejects_disagreeing_embedded_inventory(self):
        self.receipt['cc'] = 'different compiler'
        self.save()
        with self.assertRaisesRegex(ValueError, 'embedded dependency build differs'):
            artifacts.verify(self.directory, artifacts.target_name())

    def test_rejects_wrong_artifact_digest_before_publication(self):
        archive = self.root / 'network.tar.gz'
        artifacts.pack(self.directory, archive)
        output = self.root / 'output'
        with self.assertRaisesRegex(ValueError, 'checksum'):
            artifacts.install(archive, output, '0' * 64, artifacts.target_name())
        self.assertFalse(output.exists())

    def test_rejects_archive_traversal_links_duplicates_and_bombs(self):
        for kind in ('traversal', 'symlink', 'duplicate', 'oversized'):
            with self.subTest(kind=kind):
                archive = self.root / (kind + '.tar')
                with tarfile.open(archive, 'w:gz') as tar:
                    member = tarfile.TarInfo('../escape' if kind == 'traversal' else 'libuv.a')
                    if kind == 'symlink':
                        member.type = tarfile.SYMTYPE
                        member.linkname = '/etc/passwd'
                    if kind == 'oversized':
                        member.size = artifacts.MAX_BYTES + 1
                        # A sparse-looking header suffices: reject before extracting.
                    if kind == 'oversized':
                        tar.fileobj.write(member.tobuf())
                        tar.offset += 512
                    else:
                        tar.addfile(member)
                    if kind == 'duplicate':
                        tar.addfile(member)
                output = self.root / ('output-' + kind)
                with self.assertRaises((ValueError, tarfile.ReadError)):
                    artifacts.install(archive, output, hashlib.sha256(archive.read_bytes()).hexdigest(), artifacts.target_name())
                self.assertFalse(output.exists())
                self.assertFalse((self.root / 'escape').exists())


if __name__ == '__main__':
    unittest.main()
