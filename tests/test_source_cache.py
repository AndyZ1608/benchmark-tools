"""Source preparation must preserve tracked inputs, including on failure."""
import hashlib
import io
import json
import pathlib
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest

REPO = pathlib.Path(__file__).resolve().parents[1]


class SourceCache(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='vmbench source cache ')
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        (self.root / 'scripts').mkdir()
        (self.root / 'third-party/runtime/library/notices').mkdir(parents=True)
        for name in ['fetch-engines.py', 'fetch-runtime-sources.py']:
            shutil.copy2(REPO / 'scripts' / name, self.root / 'scripts' / name)
        archive = self.root / 'payload.tar.gz'
        with tarfile.open(archive, 'w:gz') as tar:
            notice = b'fixture license\n'
            info = tarfile.TarInfo('library/COPYING')
            info.size = len(notice)
            tar.addfile(info, io.BytesIO(notice))
        self.data = archive.read_bytes()
        entry = {'name': 'fio', 'url': archive.as_uri(), 'sha256': hashlib.sha256(self.data).hexdigest()}
        (self.root / 'scripts/engines.lock.json').write_text(json.dumps([entry]))
        self.runtime = {'build_target': 'fixture', 'files': [dict(entry, path='library/source.tar.gz')]}
        (self.root / 'scripts/runtime-sources.lock.json').write_text(json.dumps(self.runtime))

    def run_fetch(self, name, offline=True, success=True):
        result = subprocess.run([sys.executable, str(self.root / 'scripts' / name)] + (['--offline'] if offline else []), text=True, capture_output=True, timeout=30)
        self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
        return result

    def snapshot(self):
        return {p.relative_to(self.root): (p.read_bytes(), p.stat().st_mtime_ns, p.stat().st_mode)
                for p in (self.root / 'third-party').rglob('*') if p.is_file()}

    def test_offline_inputs_and_notices_are_read_only(self):
        (self.root / 'third-party/fio.tar.gz').write_bytes(self.data)
        (self.root / 'third-party/runtime/library/source.tar.gz').write_bytes(self.data)
        (self.root / 'third-party/runtime/library/notices/COPYING').write_bytes(b'original tracked notice\n')
        (self.root / 'third-party/runtime/SOURCE-INDEX.json').write_bytes(b'original tracked index\n')
        before = self.snapshot()
        for _ in range(2):
            self.run_fetch('fetch-engines.py')
            self.run_fetch('fetch-runtime-sources.py')
        self.assertEqual(self.snapshot(), before)
        cache = self.root / '.vmbench/cache'
        self.assertEqual((cache / 'engine-sources/fio.tar.gz').read_bytes(), self.data)
        self.assertEqual((cache / 'runtime-sources/library/notices/COPYING').read_bytes(), b'fixture license\n')
        self.assertEqual(json.loads((cache / 'runtime-sources/SOURCE-INDEX.json').read_text()), self.runtime)

    def test_downloads_only_populate_cache_and_work_offline_afterwards(self):
        before = self.snapshot()
        for script in ['fetch-engines.py', 'fetch-runtime-sources.py']:
            self.run_fetch(script, success=False)
            self.run_fetch(script, offline=False)  # local file URL, no network
        (self.root / 'payload.tar.gz').unlink()
        for script in ['fetch-engines.py', 'fetch-runtime-sources.py']:
            self.run_fetch(script)
        self.assertEqual(self.snapshot(), before)

    def test_bad_downloads_fail_without_publishing(self):
        (self.root / 'payload.tar.gz').write_bytes(b'bad archive')
        before = self.snapshot()
        for script in ['fetch-engines.py', 'fetch-runtime-sources.py']:
            self.assertIn('checksum mismatch', self.run_fetch(script, offline=False, success=False).stderr.lower())
        self.assertEqual(self.snapshot(), before)
        self.assertFalse((self.root / '.vmbench').exists())

    def test_bad_tracked_sources_are_never_repaired_or_hidden_by_cache(self):
        for script in ['fetch-engines.py', 'fetch-runtime-sources.py']:
            self.run_fetch(script, offline=False)
        (self.root / 'third-party/fio.tar.gz').write_bytes(b'local change')
        (self.root / 'third-party/runtime/library/source.tar.gz').write_bytes(b'local change')
        before = self.snapshot()
        for script in ['fetch-engines.py', 'fetch-runtime-sources.py']:
            self.assertIn('checksum mismatch', self.run_fetch(script, success=False).stderr.lower())
        self.assertEqual(self.snapshot(), before)


if __name__ == '__main__':
    unittest.main()
