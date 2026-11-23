#!/usr/bin/env python3
"""Gzip bundle identities use the same bounded installer as HTTP archives."""
import hashlib
import json
from pathlib import Path
import tempfile
import unittest

import network_artifacts as artifacts


class CompressionArtifactsTest(unittest.TestCase):
    def test_round_trip_and_family_separation(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            bundle = root / 'bundle'
            bundle.mkdir()
            pins, files, marker, libraries = artifacts.artifact_spec('compression')
            receipt = {'schema': 2, 'target': artifacts.target_name(), 'linkage': 'static',
                       'pins': json.loads(pins.read_text()), 'libraries': {}, 'configuration': {},
                       'cc': 'fixture', 'cmake': 'fixture', 'builder_sha256': 'a' * 64,
                       'artifact_tool_sha256': 'b' * 64}
            for name in files - {'compression-identity.o'}:
                (bundle / name).write_bytes(name.encode())
            receipt['libraries'] = {name: hashlib.sha256((bundle / (name + '.a')).read_bytes()).hexdigest() for name in libraries}
            (bundle / 'compression-identity.o').write_bytes(marker + json.dumps(artifacts.identity(receipt)).encode() + b'\0')
            receipt['files'] = {name: {'bytes': (bundle / name).stat().st_size,
                                       'sha256': hashlib.sha256((bundle / name).read_bytes()).hexdigest()} for name in files}
            (bundle / 'compression-build.json').write_text(json.dumps(receipt))
            archive = root / 'gzip.tar.gz'
            digest = artifacts.pack(bundle, archive, 'compression')
            installed = root / 'installed'
            artifacts.install(archive, installed, digest, artifacts.target_name(), 'compression')
            self.assertEqual(artifacts.verify(installed, artifacts.target_name(), 'compression'), receipt)
            self.assertEqual(artifacts.inspect(installed / 'compression-identity.o', 'compression'), artifacts.identity(receipt))
            with self.assertRaises(ValueError):
                artifacts.inspect(installed / 'compression-identity.o')
            with self.assertRaises(ValueError):
                artifacts.install(archive, root / 'wrong-family', digest, artifacts.target_name())
            self.assertFalse((root / 'wrong-family').exists())
            with self.assertRaises(ValueError):
                artifacts.verify(installed, 'wrong-target', 'compression')
            (installed / 'libdeflate.a').write_bytes(b'corrupt')
            with self.assertRaises(ValueError):
                artifacts.verify(installed, artifacts.target_name(), 'compression')


if __name__ == '__main__':
    unittest.main()
