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
    '\tputs("🍋" + row.text)\n\tputs("🍋🍋🍋" + greeting_text(row))\nend\n'
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

    def diagnostics(self, uri, version, nonempty=None):
        def matches(packet):
            params = packet.get("params", {})
            return (packet.get("method") == "textDocument/publishDiagnostics"
                    and params.get("uri") == uri and params.get("version") == version
                    and (nonempty is None or bool(params["diagnostics"]) == nonempty))
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
    position = {"line": 3, "character": 10}
    if function:
        # Use the very last letter. Counting Unicode scalars would miss the identifier.
        prefix = VALID.splitlines()[5].split("greeting_text")[0] + "greeting_tex"
        position = {"line": 5, "character": len(prefix.encode("utf-16-le")) // 2}
    target = None
    for request_id in range(10, 60):
        target = client.request("textDocument/definition", {
            "textDocument": document, "position": position}, request_id)
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
            authored = helper.read_text()
            client.send("textDocument/didOpen", {"textDocument": dict(
                imported, version=1, languageId="trb", text="\n\n" + authored)})
            expect_definition(client, document, helper, 2)
            options = {"textDocument": imported, "options": {"tabSize": 4, "insertSpaces": False}}
            assert caps.get("documentFormattingProvider")
            client.send("textDocument/didChange", {"textDocument": dict(imported, version=2),
                        "contentChanges": [{"text": authored.replace("\ttext: String", "text:String")}]})
            edits = client.request("textDocument/formatting", options, 71)
            assert len(edits) == 1 and edits[0]["newText"] == authored, edits
            client.send("textDocument/didSave", {"textDocument": imported, "text": authored})
            assert client.request("textDocument/formatting", options, 72) == []
            client.send("textDocument/didChange", {"textDocument": dict(imported, version=3),
                        "contentChanges": [{"text": "record\n"}]})
            assert client.request("textDocument/formatting", options, 73) == []
            client.send("textDocument/didClose", {"textDocument": imported})
            expect_definition(client, document, helper, 0)
            # A separate writer modifies a closed dependency while the main buffer is open.
            watched = {"changes": [{"uri": helper.as_uri(), "type": 2}]}
            helper.write_text(authored.replace("text: String", "text: Integer"))
            client.send("workspace/didChangeWatchedFiles", watched)
            client.diagnostics(main.as_uri(), 2, nonempty=True)
            helper.write_text(authored)
            client.send("workspace/didChangeWatchedFiles", watched)
            client.diagnostics(main.as_uri(), 2, nonempty=False)
            client.send("textDocument/didClose", {"textDocument": document})
            client.request("shutdown", None, 3)
            client.send("exit", None)
            assert client.proc.wait(timeout=5) == 0
        assert on_disk == [path.read_bytes() for path in (main, helper)]
        print(f"{'configured' if configured else 'standalone'} editor session passed")


def discovery_session(executable):
    # Compare complete items, including declaration IDs (UTF-8 bytes) and
    # editor ranges (UTF-16). The emoji occurs both before and inside a case.
    sample = ("# 🍋\nimport { describe, expect, test } from trb/std/test\n"
              'describe("Totals") do\n\ttest("same") { expect("🍋").to_equal("🍋") }\nend\n')

    def expected(path, source, name):
        def point(index):
            before = source[:index]
            return {"line": before.count("\n"),
                    "character": len(before.rsplit("\n", 1)[-1].encode("utf-16-le")) // 2}

        suite_start = source.index('describe("')
        case_start = source.index('test("')
        suite_id = f"{path}:{len(source[:suite_start].encode())}"
        return [{"id": suite_id, "kind": "suite", "name": "Totals", "fullName": "Totals",
                 "uri": path.as_uri(), "range": {"start": point(suite_start),
                                                 "end": point(source.rindex("end") + 3)}},
                {"id": f"{path}:{len(source[:case_start].encode())}", "parentId": suite_id,
                 "kind": "test", "name": name, "fullName": "Totals / " + name,
                 "uri": path.as_uri(), "range": {"start": point(case_start),
                                                 "end": point(source.rindex("}") + 1)}}]

    with tempfile.TemporaryDirectory(prefix="trb-test-discovery-") as tmp:
        root = Path(tmp).resolve()
        project, other = root / "first", root / "other"
        project.mkdir()
        other.mkdir()
        main, _, args = fixture(project, True)
        sibling, _, _ = fixture(other, True)
        (sibling.parent / "foreign_test.trb").write_text(sample)
        first, second = main.parent / "one_test.trb", main.parent / "two_test.trb"
        first.write_text(sample)
        (main.parent / "decoy_test.trb").write_text(sample.replace(
            "import { describe, expect, test } from trb/std/test", "# names without test imports"))
        with server([executable, "lsp", *args], project) as client:
            client.request("initialize", {"capabilities": {}}, 1)
            client.send("initialized", {})
            request_id = 2

            def verify(want):
                nonlocal request_id
                deadline = time.monotonic() + 5
                while True:
                    request_id += 1
                    rows = client.request("typerb/discoverTests", {}, request_id)
                    if sorted(rows, key=lambda item: item["id"]) == sorted(want, key=lambda item: item["id"]):
                        return
                    if time.monotonic() >= deadline:
                        raise AssertionError((want, rows))
                    time.sleep(0.1)

            def watched(kind):
                client.send("workspace/didChangeWatchedFiles", {
                    "changes": [{"uri": second.as_uri(), "type": kind}]})

            one = expected(first, sample, "same")
            verify(one)
            second.write_text(sample)
            watched(1)
            verify(one + expected(second, sample, "same"))
            changed = sample.replace('"same"', '"updated"')
            second.write_text(changed)
            watched(2)
            verify(one + expected(second, changed, "updated"))
            edited = changed.replace('"updated"', '"unsaved"')
            client.send("textDocument/didOpen", {"textDocument": {
                "uri": second.as_uri(), "languageId": "trb", "version": 1, "text": edited}})
            verify(one + expected(second, edited, "unsaved"))
            client.send("textDocument/didClose", {"textDocument": {"uri": second.as_uri()}})
            verify(one + expected(second, changed, "updated"))
            second.unlink()
            watched(3)
            verify(one)
            nested = sample.replace('describe("Totals") do', 'describe("Outer") do\n\tdescribe("Inner") do') + "end\n"
            second.write_text(nested)
            watched(1)
            deadline = time.monotonic() + 5
            while True:
                request_id += 1
                rows = [row for row in client.request("typerb/discoverTests", {}, request_id)
                        if row["uri"] == second.as_uri()]
                if [row["fullName"] for row in rows] == ["Outer", "Outer / Inner", "Outer / Inner / same"]:
                    break
                if time.monotonic() >= deadline:
                    raise AssertionError(rows)
                time.sleep(0.1)
            assert rows[2]["parentId"] == rows[1]["id"] and rows[1]["parentId"] == rows[0]["id"]
            client.request("shutdown", None, 100000)
            client.send("exit", None)
            assert client.proc.wait(timeout=5) == 0
        print("configured editor test discovery, refresh and project isolation passed")


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
    discovery_session(args.executable.resolve())
    if args.vscode:
        extension_host(args.executable.resolve(), args.vscode.resolve(), args.extension.resolve())
