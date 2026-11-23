import base64
import gzip
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from tools import evidence_bundle as bundle


class EvidenceBundleTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root / 'source'
        (self.source / 'evidence').mkdir(parents=True)
        (self.source / 'evidence/empty.stderr').write_bytes(b'')
        (self.source / 'evidence/program').write_bytes(b'\0\xff\n')
        (self.source / 'evidence/program').chmod(0o755)
        (self.source / 'evidence/raw.tsv').write_text('round\tstatus\n1\t137\n')
        self.output = self.root / 'evidence.jsonl.gz'

    def pack(self):
        return bundle.pack(self.source, self.output, ['evidence', 'missing'])

    def rewrite(self, change):
        with gzip.open(self.output, 'rt') as stream:
            lines = [json.loads(line) for line in stream]
        change(lines)
        with gzip.open(self.output, 'wb') as stream:
            stream.write(b''.join(bundle.line(line) for line in lines))

    def test_complete_roundtrip_empty_binary_failure_and_modes(self):
        result = self.pack()
        self.assertEqual(result['files'], 3)
        dest = self.root / 'restored'
        self.assertEqual(bundle.extract(self.output, dest), result)
        for source in self.source.rglob('*'):
            if source.is_file():
                target = dest / source.relative_to(self.source)
                self.assertEqual(target.read_bytes(), source.read_bytes())
                self.assertEqual(target.stat().st_mode, source.stat().st_mode)
        with gzip.open(self.output, 'rt') as stream:
            self.assertEqual(json.loads(next(stream))['missing'], ['missing'])

    def test_deterministic_and_no_overwrite(self):
        self.pack()
        other = self.root / 'other.gz'
        bundle.pack(self.source, other, ['missing', 'evidence'])
        self.assertEqual(other.read_bytes(), self.output.read_bytes())
        with self.assertRaises(FileExistsError):
            self.pack()
        existing = self.root / 'existing'
        existing.mkdir()
        with self.assertRaises(FileExistsError):
            bundle.extract(self.output, existing)

    def test_symlink_and_overlapping_selections(self):
        (self.source / 'evidence/link').symlink_to(self.source / 'evidence/program')
        with self.assertRaises(ValueError):
            self.pack()
        self.assertFalse(self.output.exists())
        (self.source / 'evidence/link').unlink()
        with self.assertRaises(ValueError):
            bundle.pack(self.source, self.output, ['evidence', 'evidence/program'])
        with self.assertRaises(ValueError):
            bundle.pack(self.source, self.output, ['../source'])

    def test_bad_cohorts_do_not_create_extraction_directories(self):
        mutations = [
            lambda rows: rows[-1].update(base64=base64.b64encode(b'changed').decode()),
            lambda rows: rows.pop(),
            lambda rows: rows.append(rows[-1]),
            lambda rows: rows[-1].update(path='../escape'),
            lambda rows: rows[-1].update(path='/absolute'),
            lambda rows: rows[-1].update(path='evidence/program/child'),
            lambda rows: rows[0].update(missing=['evidence']),
            lambda rows: rows[-1].update(mode=0o4755),
        ]
        for change in mutations:
            with self.subTest(change=change):
                self.output.unlink(missing_ok=True)
                self.pack()
                self.rewrite(change)
                dest = self.root / 'must-not-exist'
                with self.assertRaises(ValueError):
                    bundle.extract(self.output, dest)
                self.assertFalse(dest.exists())

    def test_truncated_gzip_fails_before_extract(self):
        self.pack()
        self.output.write_bytes(self.output.read_bytes()[:-8])
        dest = self.root / 'must-not-exist'
        with self.assertRaises(EOFError):
            bundle.extract(self.output, dest)
        self.assertFalse(dest.exists())

    def test_hidden_inputs_do_not_widen_the_existing_upload_scope(self):
        (self.source / 'evidence/.hidden').write_text('private fixture')
        with self.assertRaises(ValueError):
            self.pack()
        self.assertFalse(self.output.exists())

    def test_write_failure_preserves_sources_and_removes_partial_bundle(self):
        original = bundle.line
        def fail_on_record(record):
            if 'path' in record:
                raise OSError('synthetic write failure')
            return original(record)
        with patch.object(bundle, 'line', side_effect=fail_on_record):
            with self.assertRaises(OSError):
                self.pack()
        self.assertFalse(self.output.exists())
        self.assertEqual((self.source / 'evidence/raw.tsv').read_text(),
                         'round\tstatus\n1\t137\n')

    def test_nothing_available_is_not_a_successful_cohort(self):
        with self.assertRaises(ValueError):
            bundle.pack(self.source, self.output, ['unavailable'])
        self.assertFalse(self.output.exists())


if __name__ == '__main__':
    unittest.main()
