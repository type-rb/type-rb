"""Independent Go HTTP/2 test server; never used by application compilation."""
import hashlib
import json
import os
from pathlib import Path
import select
import signal
import subprocess


class Http2Provider:
    def __init__(self, root, keys):
        self.root = root
        self.root.mkdir()
        (root / 'keys.json').write_text(json.dumps(keys))
        source = Path(__file__).with_name('fixtures') / 'oidc_http2.go'
        self.identity = {'source_sha256': hashlib.sha256(source.read_bytes()).hexdigest(),
                         'go': subprocess.check_output(['go', 'version'], text=True).strip()}
        executable = root / 'provider'
        built = subprocess.run(['go', 'build', '-o', str(executable), str(source)],
                               capture_output=True, text=True, timeout=180)
        assert built.returncode == 0, built.stderr
        self.identity['executable_sha256'] = hashlib.sha256(executable.read_bytes()).hexdigest()
        self.executable = executable
        self.process = None

    def __enter__(self):
        self.errors = (self.root / 'server.stderr').open('w')
        self.process = subprocess.Popen([str(self.executable), str(self.root)],
                                        stdout=subprocess.PIPE, stderr=self.errors,
                                        text=True, start_new_session=True)
        try:
            ready, _, _ = select.select([self.process.stdout], [], [], 10)
            assert ready, 'HTTP/2 test server did not become ready'
            self.endpoints = json.loads(self.process.stdout.readline())
            return self
        except BaseException:
            self.__exit__(None, None, None)
            raise

    def requests(self):
        return [json.loads(line) for line in (self.root / 'requests.jsonl').read_text().splitlines()]

    def __exit__(self, *_):
        try:
            if self.process.poll() is None:
                os.killpg(self.process.pid, signal.SIGTERM)
            try:
                self.process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                os.killpg(self.process.pid, signal.SIGKILL)
                self.process.wait(timeout=5)
            self.process.stdout.close()
        finally:
            self.errors.close()
