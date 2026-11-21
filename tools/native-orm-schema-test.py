"""Build a lock-derived SQLite model offline and compare its runtime query results."""
import argparse
import json
import os
import shutil
import sqlite3
from pathlib import Path
import subprocess
import tempfile


def run(command, root, environment, error=None):
    result = subprocess.run(command, cwd=root, env=environment, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=60)
    if error is None:
        if result.returncode:
            raise AssertionError(f"{command}: {result.stderr}")
    elif result.returncode == 0 or error not in result.stderr:
        raise AssertionError(f"expected {error!r} from {command}, got {result.returncode}: {result.stderr}")
    return result.stdout


def expect_output(binary, root, environment, expected):
    actual = run([str(binary)], root, environment)
    if actual != expected:
        raise AssertionError(f"{binary}: expected {expected!r}, got {actual!r}")


def check_model_rows(cli, reference, root, config, environment):
    src = root / "src"
    fixture = Path(__file__).resolve().parents[1] / "fixtures/projects/sqlite-model-operations"
    for module in (fixture / "src").glob("*.trb"):
        shutil.copyfile(module, src / module.name)
    shutil.copyfile(fixture / "application.schema.json", root / "application.schema.json")
    source = (src / "main.trb").read_text()
    entry = src / "main.trb"
    database = root / "lookup.sqlite"
    environment = dict(environment, DATABASE_URL=str(database))
    environment.pop("ORM_TEST_OPERATION", None)
    binaries = []
    for mode, tool in [("trb", cli), ("go", reference)]:
        if not tool:
            continue
        cfg = dict(config, mode=mode)
        if mode == "go":
            cfg["go"] = {"module": "example.com/orm-lookup"}
        (root / "trbconfig.jsonc").write_text(json.dumps(cfg))
        for original, invalid in [
                ('Report.create(title: "first")', 'Report.create(title: "first", id: nil)'),
                ('Report.create(title: "first")', 'Report.create()'),
                ('original.with(title: "ready")', 'original.with(id: 7)'),
                ('Tag.find("reader\'s copy")', 'Tag.find(1)'),
                ('report := Report.new()', 'report := Report.new(1, "title")')]:
            entry.write_text(source.replace(original, invalid))
            run([str(tool), "check"], root, environment, "error[")
        entry.write_text(source)
        queries = src / "queries.trb"
        query_source = queries.read_text()
        for invalid in ['base.where("missing", ">", 1)', 'base.where("id", ">", "one")',
                        'base.where("title", "unknown", "x")', 'base.where("id", "LIKE", "1")',
                        'base.where("note", ">", nil)', 'base.where(id: "x")',
                        'base.order(missing: :asc)', 'base.order(id: :sideways)']:
            queries.write_text(query_source.replace('base.where("id", ">", 1)', invalid))
            run([str(tool), "check"], root, environment, "error[")
        queries.write_text(query_source)
        run([str(tool), "check"], root, environment)
        app = root / ("lookup-" + mode)
        run([str(tool), "build", "--compile", "--outfile", str(app)], root, environment)
        binaries.append(app)
    assert not database.exists()
    for app in binaries:
        expect_output(app, root, dict(environment, ORM_TEST_OPERATION="empty"), "0\n\n")
        assert not database.exists()
        pages = root / (app.name + "-pages.sqlite")
        query_env = dict(environment, DATABASE_URL=str(pages), ORM_TEST_OPERATION="queries")
        with sqlite3.connect(pages) as connection:
            connection.execute("CREATE TABLE reports(id INTEGER PRIMARY KEY, title TEXT NOT NULL, note TEXT)")
            connection.executemany("INSERT INTO reports VALUES (?, ?, ?)", [
                (1, "reader's copy", None), (2, "else", None), (3, "reader's copy", "ok"),
                (4, "reader's copy", None), (5, "reader's copy", "ok")])
        expect_output(app, root, query_env,
                      "5\n4\n1\n1\ntrue\n4\n3\n4\n0\ntrue\n0\ntrue\n0\n2\n4\n5\n4\n"
                      "4\n2\n1\n5\n3\n2\n2\n2\n1\n0\ndatabase insert failed\n0\n"
                      "Query\ndatabase query failed\nQuery\ndatabase query failed\nQuery\ndatabase count failed\n")
        with sqlite3.connect(pages) as connection:
            assert connection.execute("SELECT count(*) FROM reports").fetchone() == (5,)
            connection.execute("DROP TABLE reports")
        expect_output(app, root, query_env, "Query\ndatabase query failed\n")
        scan = root / (app.name + "-scan.sqlite")
        scan_env = dict(environment, DATABASE_URL=str(scan), ORM_TEST_OPERATION="scan")
        payloads = [None if i % 4 == 0 else bytes([i % 251]) * (0 if i % 4 == 1 else 2048)
                    for i in range(512)]
        with sqlite3.connect(scan) as connection:
            connection.execute("CREATE TABLE metrics(id INTEGER PRIMARY KEY, passed BOOLEAN, rating REAL, payload BLOB)")
            connection.executemany("INSERT INTO metrics VALUES(?, 1, 1.25, ?)", enumerate(payloads, 1))
        checksum = sum(len(payload) + payload[0] for payload in payloads if payload)
        expect_output(app, root, scan_env, f"512\n{checksum}\n")
        times = root / (app.name + "-times.sqlite")
        with sqlite3.connect(times) as connection:
            connection.execute("CREATE TABLE events(id INTEGER PRIMARY KEY, on_date DATE NOT NULL, at_time TIME NOT NULL,"
                               " local_at DATETIME NOT NULL, exact_at TIMESTAMPTZ NOT NULL, optional_at DATETIME)")
        expect_output(app, root, dict(environment, DATABASE_URL=str(times), ORM_TEST_OPERATION="times"),
                      "true\n2025-03-08\n12:34:56.123456\n2025-03-08T12:34:56.123456\n"
                      "2025-03-08T03:34:56.123456789Z\n2025-03-10T04:05:06.000007\ntrue\n1\n")
        with sqlite3.connect(times) as connection:
            assert connection.execute("SELECT on_date, at_time, local_at, exact_at, optional_at FROM events").fetchone() == (
                "2025-03-08", "12:34:56.123456", "2025-03-08 12:34:56.123456", "2025-03-08 03:34:56.123456789 +0000 UTC", None)
        metrics = root / (app.name + "-metrics.sqlite")
        with sqlite3.connect(metrics) as connection:
            connection.execute("CREATE TABLE metrics(id INTEGER PRIMARY KEY, passed BOOLEAN DEFAULT 1, rating REAL, payload BLOB)")
            connection.execute("INSERT INTO metrics VALUES(42, 'True', 4, X'FF0080')")
        expect_output(app, root, dict(environment, DATABASE_URL=str(metrics), ORM_TEST_OPERATION="metrics"),
                      "true\ntrue\nfalse\n2.75\n4\n0\n255\ntrue\ntrue\ntrue\n43\n")
        with sqlite3.connect(metrics) as connection:
            assert connection.execute("SELECT passed, rating, payload FROM metrics ORDER BY id").fetchall() == [
                ("True", 4.0, None), (0, 2.75, b"A\x00" + "β".encode())]
        transactions = root / (app.name + "-transactions.sqlite")
        with sqlite3.connect(transactions) as connection:
            connection.execute("CREATE TABLE reports(id INTEGER PRIMARY KEY, title TEXT NOT NULL UNIQUE, note TEXT)")
        expect_output(app, root, dict(environment, DATABASE_URL=str(transactions), ORM_TEST_OPERATION="transactions"),
                      "0\n1\ndatabase insert failed\n0\n1\ntrue\n2\n2\ndatabase insert failed\n0\n")
        with sqlite3.connect(transactions) as connection:
            assert connection.execute("SELECT id, title, note FROM reports ORDER BY id").fetchall() == [
                (1, "root", "saved"), (2, "kept", None)]
        blocked = dict(environment, DATABASE_URL=str(transactions), ORM_TEST_OPERATION="blocked_commit")
        with sqlite3.connect(transactions) as reader:
            reader.execute("BEGIN")
            assert reader.execute("SELECT count(*) FROM reports").fetchone() == (2,)
            expect_output(app, root, blocked, "database transaction failed to commit\n")
            reader.rollback()
            assert reader.execute("SELECT count(*) FROM reports").fetchone() == (2,)
        expect_output(app, root, blocked, "3\n")
        inserts = root / (app.name + "-creates.sqlite")
        with sqlite3.connect(inserts) as connection:
            connection.execute("CREATE TABLE reports(id INTEGER PRIMARY KEY, title TEXT NOT NULL, note TEXT DEFAULT 'from database')")
        expect_output(app, root, dict(environment, DATABASE_URL=str(inserts), ORM_TEST_OPERATION="insert"),
                      "1\nfirst\nfrom database\n42\ntrue\nConstraint\ndatabase insert failed\n")
        with sqlite3.connect(inserts) as connection:
            assert connection.execute("SELECT id, title, note FROM reports ORDER BY id").fetchall() == [
                (1, "first", "from database"), (42, "second", None)]
        expect_output(app, root, dict(environment, DATABASE_URL=str(inserts), ORM_TEST_OPERATION="update"),
                      "first\nready\nseparate edit\n1\ntrue\nfirst\ndatabase update requires at least one value\n")
        with sqlite3.connect(inserts) as connection:
            assert connection.execute("SELECT title, note FROM reports WHERE id = 1").fetchone() == ("ready", None)
        expect_output(app, root, dict(environment, DATABASE_URL=str(inserts), ORM_TEST_OPERATION="save_missing"),
                      "InvalidData\ndatabase update target was not found\n")
        with sqlite3.connect(inserts) as connection:
            assert connection.execute("SELECT id FROM reports").fetchall() == [(42,)]
        path = root / (app.name + "-tags.sqlite")
        with sqlite3.connect(path) as connection:
            connection.executescript("CREATE TABLE tags(code TEXT PRIMARY KEY);"
                                     "CREATE TRIGGER protect_tag BEFORE DELETE ON tags BEGIN SELECT RAISE(ABORT, 'protected'); END;")
            connection.executemany("INSERT INTO tags VALUES (?)", [("reader's copy",), ("spare",)])
        remove = dict(environment, DATABASE_URL=str(path), ORM_TEST_OPERATION="delete")
        expect_output(app, root, remove, "reader's copy\nConstraint\ndatabase delete failed\n")
        with sqlite3.connect(path) as connection:
            assert connection.execute("SELECT COUNT(*) FROM tags").fetchone() == (2,)
            connection.execute("DROP TRIGGER protect_tag")
        expect_output(app, root, remove, "reader's copy\ntrue\nfalse\n")
        with sqlite3.connect(path) as connection:
            assert connection.execute("SELECT code FROM tags").fetchall() == [("spare",)]
        expect_output(app, root, remove, "missing tag\n")
    with sqlite3.connect(database) as connection:
        connection.executescript("CREATE TABLE reports(id INTEGER PRIMARY KEY, title TEXT, note TEXT);"
                                 "INSERT INTO reports VALUES (1, 'alpha', NULL), (2, 'beta', '日本語')")
    for app in binaries:
        expect_output(app, root, environment, "1\nalpha\n2\nbeta\n日本語\nmissing\n")
    with sqlite3.connect(database) as connection:
        connection.execute("UPDATE reports SET title = NULL WHERE id = 1")
    for app in binaries:
        expect_output(app, root, environment, "InvalidData\ndatabase row was invalid\n2\nbeta\n日本語\nmissing\n")
    with sqlite3.connect(database) as connection:
        connection.execute("DROP TABLE reports")
    for app in binaries:
        expect_output(app, root, environment, "Query\ndatabase query failed\n" * 3)
        no_url = dict(environment)
        no_url.pop("DATABASE_URL")
        expect_output(app, root, no_url, "Connection\ndatabase connection failed\n" * 3)


def check_model_worker(cli, reference, root):
    src = root / "src"
    (src / "config").mkdir(parents=True)
    (src / "models.trb").write_text("import { Model } from trb/orm\nclass Report < Model\nend\n")
    (src / "config/jobs.trb").write_text("""import { JobAdapter } from trb/jobs
import { SQLAdapter } from trb/jobs/sql
JOBS_ADAPTER: JobAdapter := SQLAdapter.new(source_environment: "QUEUE_DB")
""")
    (src / "publish.trb").write_text("""import { Report } from models
import { Job, JobResult, JobError } from trb/jobs
import trb/std/unit

class PublishJob < Job
    def perform(id: Integer): JobResult
        report := Report.find(id) catch |error|
            return JobResult::Err(JobError.new(message: error.message))
        end
        if report == nil
            return JobResult::Err(JobError.new(message: "report missing"))
        end
        _ready := report.with(status: "ready").save() catch |error|
            return JobResult::Err(JobError.new(message: error.message))
        end
        return JobResult::Ok(Unit.new())
    end
end
""")
    (src / "main.trb").write_text("""import { Report } from models
import { PublishJob } from publish

def main()
    report := Report.create(status: "pending") catch |error|
        puts(error.message)
        return
    end
    receipt := PublishJob.perform_later(report.id) catch |error|
        puts(error.message)
        return
    end
    puts(receipt.id)
end
""")
    schema = {"formatVersion": 1, "adapter": "sqlite", "tables": {"reports": {
        "columns": {"id": {"type": "Integer", "primaryKey": True, "generated": True},
                    "status": {"type": "String"}},
        "uniqueConstraints": {"primary:id": {"primary": True, "columns": ["id"]}}}}}
    (root / "lock.json").write_text(json.dumps(schema))
    for mode, tool in [("trb", cli), ("go", reference)]:
        if not tool:
            continue
        queue = root / (mode + "-queue.sqlite")
        data = root / (mode + "-data.sqlite")
        environment = dict(os.environ, QUEUE_DB=str(queue), DATABASE_URL=str(data))
        config = {"name": "worker-model", "mode": mode, "sourceDir": "src",
                  "db": {"adapter": "sqlite"}, "jobs": {"configuration": "config/jobs"},
                  "packageOptions": {"trb/orm": {"adapter": "sqlite", "schemaLock": "lock.json",
                                                 "database": {"environment": "DATABASE_URL"}}}}
        if mode == "go":
            config["go"] = {"module": "example.com/worker-model"}
        (root / "trbconfig.jsonc").write_text(json.dumps(config))
        binary = root / ("submit-" + mode)
        run([str(tool), "check"], root, environment)
        run([str(tool), "build", "--compile", "--outfile", str(binary)], root, environment)
        assert not data.exists() and not queue.exists()
        with sqlite3.connect(data) as connection:
            connection.execute("CREATE TABLE reports(id INTEGER PRIMARY KEY, status TEXT NOT NULL)")
        job = run([str(binary)], root, environment).strip()
        with sqlite3.connect(queue) as connection:
            assert connection.execute("SELECT state FROM trb_jobs WHERE id = ?", (job,)).fetchone() == ("ready",)
        with sqlite3.connect(data) as connection:
            assert connection.execute("SELECT id, status FROM reports").fetchall() == [(1, "pending")]
        run([str(tool), "jobs", "start", "--once"], root, environment)
        with sqlite3.connect(data) as connection:
            assert connection.execute("SELECT id, status FROM reports").fetchall() == [(1, "ready")]
        with sqlite3.connect(queue) as connection:
            assert connection.execute("SELECT state FROM trb_jobs WHERE id = ?", (job,)).fetchone() is None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--native", required=True, type=Path)
    parser.add_argument("--reference", type=Path)
    args = parser.parse_args()
    cli = str(args.native.resolve())
    reference = str(args.reference.resolve()) if args.reference else None
    with tempfile.TemporaryDirectory(prefix="trbn-orm-lock-") as directory:
        root = Path(directory)
        src = root / "src"
        src.mkdir()
        lock = root / "application.schema.json"
        database = root / "absent.sqlite"
        environment = dict(os.environ, DATABASE_URL=str(database))
        config = {"name": "orm-lock", "mode": "trb", "sourceDir": "src",
                  "db": {"adapter": "sqlite"},
                  "packageOptions": {"trb/orm": {"adapter": "sqlite", "schemaLock": lock.name,
                                                 "database": {"environment": "DATABASE_URL"}}}}
        (root / "trbconfig.jsonc").write_text(json.dumps(config))
        columns = {"id": {"type": "Integer", "primaryKey": True, "generated": True},
                   "name": {"type": "String"}, "price": {"type": "Float", "nullable": True},
                   "updated_at": {"type": "DateTime", "nullable": True}}
        schema = {"formatVersion": 1, "adapter": "sqlite", "tables": {"categories": {
            "columns": columns, "uniqueConstraints": {"primary:id": {"primary": True, "columns": ["id"]}}}}}
        lock.write_text(json.dumps(schema))
        source = """import { Model } from trb/orm
import { DateTime } from trb/std/time

class Category < Model
end

def read_updated_at(category: Category): DateTime?
    return category.updated_at
end

def read_price(category: Category): Float?
    return category.price
end

def read_name(category: Category): String
    return category.name
end

def main()
    puts("locked")
end
"""
        entry = src / "main.trb"
        entry.write_text(source)
        check = [cli, "check", "--config", "trbconfig.jsonc"]
        run(check, root, environment)
        unconfigured = dict(environment)
        unconfigured.pop("DATABASE_URL")
        run(check, root, unconfigured)
        binary = root / "sample"
        run([cli, "build", "--compile", "--config", "trbconfig.jsonc", "--outfile", str(binary)], root, environment)
        expect_output(binary, root, environment, "locked\n")
        entry.write_text(source.replace("return category.name", "return category.unknown_column"))
        run(check, root, environment, "unknown class field")
        entry.write_text("import { OrmSql } from trb/internal/orm/sql\n" + source)
        run(check, root, environment, "reserved for the verified model provider")
        other_model = src / "other.trb"
        other_model.write_text("import { Model } from trb/orm\nclass Category < Model\nend\n")
        entry.write_text("import { Category as OtherCategory } from other\n" + source +
                         "\ndef other_name(item: OtherCategory): String\n    return item.name\nend\n")
        run(check, root, environment, "is declared more than once")
        other_model.unlink()
        entry.write_text(source)
        options = config["packageOptions"]["trb/orm"]
        default_lock = root / "db/schema.lock.json"
        default_lock.parent.mkdir()
        lock.rename(default_lock)
        del options["schemaLock"]
        (root / "trbconfig.jsonc").write_text(json.dumps(config))
        run(check, root, unconfigured)
        options["schemaLock"] = lock.name
        (root / "trbconfig.jsonc").write_text(json.dumps(config))
        default_lock.rename(lock)
        schema["tables"]["others"] = schema["tables"].pop("categories")
        lock.write_text(json.dumps(schema))
        run(check, root, environment, "expects table categories")
        lock.unlink()
        run(check, root, environment, "schema lock")
        assert not database.exists()
        schema["tables"]["cé_categories"] = schema["tables"].pop("others")
        lock.write_text(json.dumps(schema, ensure_ascii=False))
        entry.write_text(source.replace("Category", "CÉCategory").replace(
            "import { Model }", "import { DbErrorKind, DbResult, Model }").replace(
            '    puts("locked")',
            '    case CÉCategory.count()\n    when DbResult::Ok(value)\n        puts(value)\n'
            '    when DbResult::Err(error)\n        case error.kind\n'
            '        when DbErrorKind::Connection\n            puts("Connection")\n'
            '        when DbErrorKind::Query\n            puts("Query")\n'
            '        else\n            puts("unexpected error kind")\n'
            '        end\n        puts(error.message)\n    end'))
        run(check, root, environment)
        run([cli, "build", "--compile", "--config", "trbconfig.jsonc", "--outfile", str(binary)], root, environment)
        assert not database.exists()
        assert b"TRBN-SQLITE-V1\0" in binary.read_bytes(), "missing pinned SQLite build identity"
        binaries = [binary]
        if reference:
            reference_config = dict(config, mode="go", go={"module": "example.com/orm-lock"})
            (root / "trbconfig.jsonc").write_text(json.dumps(reference_config))
            go_binary = root / "go-sample"
            run([reference, "check", "--config", "trbconfig.jsonc"], root, environment)
            run([reference, "build", "--compile", "--outfile", str(go_binary)], root, environment)
            assert not database.exists()
            binaries.append(go_binary)
        with sqlite3.connect(database) as connection:
            connection.executescript("CREATE TABLE cé_categories(id INTEGER PRIMARY KEY, name TEXT, price REAL, updated_at DATETIME);"
                                     "INSERT INTO cé_categories(name) VALUES ('one'), ('two')")
        for app in binaries:
            expect_output(app, root, environment, "2\n")
        with sqlite3.connect(database) as connection:
            connection.execute("DELETE FROM cé_categories")
        for app in binaries:
            expect_output(app, root, environment, "0\n")
        with sqlite3.connect(database) as connection:
            connection.execute("DROP TABLE cé_categories")
        for app in binaries:
            expect_output(app, root, environment, "Query\ndatabase count failed\n")
        for app in binaries:
            expect_output(app, root, unconfigured, "Connection\ndatabase connection failed\n")
            fresh = root / (app.name + "-fresh.sqlite")
            expect_output(app, root, dict(environment, DATABASE_URL=str(fresh)),
                          "Query\ndatabase count failed\n")
            assert fresh.is_file(), "the runtime should open or create the configured database"
            unavailable = root / "missing-directory" / "database.sqlite"
            expect_output(app, root, dict(environment, DATABASE_URL=str(unavailable)),
                          "Connection\ndatabase connection failed\n")
        check_model_rows(cli, reference, root, config, environment)
        check_model_worker(cli, reference, root / "worker-project")
    print("SQLite ORM lock-only builds, typed counts, create/find/update/delete, nine column types, deferred saves, transaction isolation, nested rollback, failed commit cleanup and worker updates passed")


if __name__ == "__main__":
    main()
