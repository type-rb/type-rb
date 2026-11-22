#!/usr/bin/env python3
"""Exercise the ordinary language server over the same stdio protocol as the editor."""

import argparse
import contextlib
import json
from pathlib import Path
import queue
import subprocess
import tempfile
import threading
import time


VALID = (
    'import { Greeting, greeting_text } from helper\n\n'
    'def main()\n\trow := Greeting.new(text: "hi")\n'
    '\tputs("🍋" + row.text)\n\tputs(greeting_text(row))\nend\n'
)
INVALID = VALID.replace('row.text)', '1)')


def messages(stream):
    while line := stream.readline():
        headers = {}
        while line != b"\r\n":
            if not line:
                raise EOFError("incomplete LSP headers")
            key, value = line.decode("ascii").split(":", 1)
            headers[key.lower()] = value.strip()
            line = stream.readline()
        size = int(headers["content-length"])
        body = stream.read(size)
        if len(body) != size:
            raise EOFError("incomplete LSP response")
        yield json.loads(body)


class Client:
    def __init__(self, proc):
        self.proc = proc
        self.inbox = queue.Queue()
        self.reader = threading.Thread(target=self.read, daemon=True)
        self.reader.start()

    def read(self):
        try:
            for packet in messages(self.proc.stdout):
                self.inbox.put(packet)
        except Exception as exc:
            self.inbox.put(exc)
        finally:
            self.inbox.put(None)

    def send(self, method, params, request_id=None):
        packet = {"jsonrpc": "2.0", "method": method, "params": params}
        if request_id is not None:
            packet["id"] = request_id
        payload = json.dumps(packet, ensure_ascii=False).encode()
        self.proc.stdin.write(f"Content-Length: {len(payload)}\r\n\r\n".encode() + payload)
        self.proc.stdin.flush()

    def receive(self, matches):
        deadline = time.monotonic() + 20
        while True:
            packet = self.inbox.get(timeout=max(0, deadline - time.monotonic()))
            if packet is None:
                raise EOFError(self.proc.stderr.read().decode())
            if isinstance(packet, Exception):
                raise packet
            if matches(packet):
                return packet

    def request(self, method, params, request_id):
        self.send(method, params, request_id)
        response = self.receive(lambda item: item.get("id") == request_id)
        assert "error" not in response, response
        return response["result"]

    def diagnostics(self, uri, version):
        def matches(packet):
            params = packet.get("params", {})
            return (packet.get("method") == "textDocument/publishDiagnostics"
                    and params.get("uri") == uri and params.get("version") == version)
        return self.receive(matches)["params"]["diagnostics"]


@contextlib.contextmanager
def server(command, root):
    with subprocess.Popen(command, cwd=root, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE) as proc:
        client = Client(proc)
        try:
            yield client
        finally:
            if proc.poll() is None:
                proc.kill()
            proc.wait()
            client.reader.join(timeout=2)


def fixture(root, configured):
    src = root / "src" if configured else root
    src.mkdir(exist_ok=True)
    main, helper = src / "main.trb", src / "helper.trb"
    main.write_text(VALID, encoding="utf-8")
    helper.write_text("record Greeting\n\ttext: String\nend\n\n"
                      "def greeting_text(row: Greeting): String\n\treturn row.text\nend\n",
                      encoding="utf-8")
    if not configured:
        return main, helper, ["--mode", "go", str(main)]
    config = root / "trbconfig.jsonc"
    config.write_text(json.dumps({"mode": "go", "name": "editor", "sourceDir": "src",
                                  "go": {"module": "example.com/editor"}}))
    return main, helper, ["--config", str(config)]


def expect_definition(client, document, helper, line, function=False):
    # A cleared diagnostic can arrive before the reference finishes debounced analysis.
    start, end = (4, 17) if function else (7, 15)
    expected = {"start": {"line": line, "character": start}, "end": {"line": line, "character": end}}
    target = None
    for request_id in range(10, 60):
        target = client.request("textDocument/definition", {
            "textDocument": document, "position": {"line": 5 if function else 3, "character": 10}},
            request_id)
        if isinstance(target, list):
            target = next(iter(target), None)
        if target and target["uri"] == helper.as_uri() and target["range"] == expected:
            return
        time.sleep(0.1)
    raise AssertionError(f"Expected definition at {helper.as_uri()} {expected}, received {target}")


def exercise(executable, configured):
    with tempfile.TemporaryDirectory(prefix="trb editor ") as tmp:
        root = Path(tmp).resolve()
        main, helper, args = fixture(root, configured)
        on_disk = [path.read_bytes() for path in (main, helper)]
        document = {"uri": main.as_uri()}
        with server([executable, "lsp", *args], root) as client:
            caps = client.request("initialize", {"capabilities": {}}, 1)["capabilities"]
            assert caps.get("definitionProvider"), caps
            client.send("initialized", {})
            client.send("textDocument/didOpen", {"textDocument": dict(
                document, version=1, languageId="trb", text=INVALID)})
            errors = client.diagnostics(main.as_uri(), 1)
            assert errors and errors[0]["range"]["start"]["line"] == 4, errors
            change = {"text": VALID}
            if caps["textDocumentSync"]["change"] == 2:
                # The emoji preceding the edited operand occupies two UTF-16 code units.
                before = INVALID.splitlines()[4].partition("1)")[0]
                offset = len(before.encode("utf-16-le")) // 2
                change = {"text": "row.text", "range": {
                    "start": {"line": 4, "character": offset},
                    "end": {"line": 4, "character": offset + 1}}}
            client.send("textDocument/didChange", {
                "textDocument": dict(document, version=2), "contentChanges": [change]})
            assert client.diagnostics(main.as_uri(), 2) == []
            expect_definition(client, document, helper, 0)
            expect_definition(client, document, helper, 4, function=True)
            imported = {"uri": helper.as_uri()}
            client.send("textDocument/didOpen", {"textDocument": dict(
                imported, version=1, languageId="trb", text="\n\n" + helper.read_text())})
            expect_definition(client, document, helper, 2)
            client.send("textDocument/didClose", {"textDocument": imported})
            expect_definition(client, document, helper, 0)
            client.send("textDocument/didClose", {"textDocument": document})
            client.request("shutdown", None, 3)
            client.send("exit", None)
            assert client.proc.wait(timeout=5) == 0
        assert on_disk == [path.read_bytes() for path in (main, helper)]
        print(f"{'configured' if configured else 'standalone'} editor session passed")


def extension_host(executable, vscode, extension):
    with tempfile.TemporaryDirectory(prefix="trb-editor-host-") as tmp:
        stage = Path(tmp).resolve()
        root = stage / "workspace"
        for name, configured in (("project", True), ("standalone", False)):
            folder = root / name
            folder.mkdir(parents=True)
            fixture(folder, configured)
        settings = root / ".vscode"
        settings.mkdir()
        (settings / "settings.json").write_text(json.dumps({
            "typerb.server.path": str(executable), "files.autoSave": "off"
        }))
        suite = Path(__file__).with_name("native-lsp-vscode-test.cjs").resolve()
        subprocess.run([vscode, "--new-window", "--disable-gpu", "--skip-welcome",
                        "--disable-workspace-trust", "--disable-updates",
                        "--user-data-dir", str(stage / "profile"),
                        "--extensions-dir", str(stage / "extensions"),
                        f"--extensionDevelopmentPath={extension}", f"--extensionTestsPath={suite}",
                        str(root)], check=True, timeout=120)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("executable", type=Path)
    parser.add_argument("--vscode", type=Path, help="VS Code application executable")
    parser.add_argument("--extension", type=Path, help="built official TypeRB extension directory")
    args = parser.parse_args()
    if bool(args.vscode) != bool(args.extension):
        parser.error("--vscode and --extension must be supplied together")
    for configured in (True, False):
        exercise(args.executable.resolve(), configured)
    if args.vscode:
        extension_host(args.executable.resolve(), args.vscode.resolve(), args.extension.resolve())
