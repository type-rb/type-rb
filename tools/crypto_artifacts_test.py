#!/usr/bin/env python3
"""Crypto bundle identities use the same bounded installer as HTTP archives."""
import hashlib
import json
from pathlib import Path
import tempfile
import unittest

import network_artifacts as artifacts


class CryptoArtifactsTest(unittest.TestCase):
    def test_round_trip_and_family_separation(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            bundle = root / 'bundle'
            bundle.mkdir()
            pins, files, marker, libraries = artifacts.artifact_spec('crypto')
            receipt = {'schema': 2, 'target': artifacts.target_name(), 'linkage': 'static',
                       'pins': json.loads(pins.read_text()), 'libraries': {}, 'configuration': {},
                       'cc': 'fixture', 'cmake': 'fixture', 'builder_sha256': 'a' * 64,
                       'artifact_tool_sha256': 'b' * 64}
            for name in files - {'crypto-identity.o'}:
                (bundle / name).write_bytes(name.encode())
            receipt['libraries'] = {name: hashlib.sha256((bundle / (name + '.a')).read_bytes()).hexdigest() for name in libraries}
            (bundle / 'crypto-identity.o').write_bytes(marker + json.dumps(artifacts.identity(receipt)).encode() + b'\0')
            receipt['files'] = {name: {'bytes': (bundle / name).stat().st_size,
                                       'sha256': hashlib.sha256((bundle / name).read_bytes()).hexdigest()} for name in files}
            (bundle / 'crypto-build.json').write_text(json.dumps(receipt))
            archive = root / 'crypto.tar.gz'
            digest = artifacts.pack(bundle, archive, 'crypto')
            installed = root / 'installed'
            artifacts.install(archive, installed, digest, artifacts.target_name(), 'crypto')
            self.assertEqual(artifacts.verify(installed, artifacts.target_name(), 'crypto'), receipt)
            self.assertEqual(artifacts.inspect(installed / 'crypto-identity.o', 'crypto'), artifacts.identity(receipt))
            with self.assertRaises(ValueError):
                artifacts.inspect(installed / 'crypto-identity.o')
            with self.assertRaises(ValueError):
                artifacts.install(archive, root / 'wrong-family', digest, artifacts.target_name())
            self.assertFalse((root / 'wrong-family').exists())
            with self.assertRaises(ValueError):
                artifacts.verify(installed, 'wrong-target', 'crypto')
            (installed / 'libcrypto.a').write_bytes(b'corrupt')
            with self.assertRaises(ValueError):
                artifacts.verify(installed, artifacts.target_name(), 'crypto')


if __name__ == '__main__':
    unittest.main()
