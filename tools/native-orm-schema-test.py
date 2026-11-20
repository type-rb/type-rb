"""Build a lock-derived SQLite model offline and compare its runtime query results."""
import argparse
import json
import os
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
    (src / "models.trb").write_text("import { Model } from trb/orm\nclass Report < Model\nend\nclass Tag < Model\nend\n")
    (root / "application.schema.json").write_text(json.dumps({
        "formatVersion": 1, "adapter": "sqlite", "tables": {
            "tags": {"columns": {"code": {"type": "String", "primaryKey": True}},
                     "uniqueConstraints": {"primary:code": {"primary": True, "columns": ["code"]}}},
            "reports": {
                "columns": {"id": {"type": "Integer", "primaryKey": True, "generated": True},
                            "note": {"type": "String", "nullable": True, "hasDefault": True},
                            "title": {"type": "String"}},
                "uniqueConstraints": {"primary:id": {"primary": True, "columns": ["id"]}}}}}))
    (src / "transactions.trb").write_text("""import { Report } from models
import { Database, DbResult } from trb/orm

def committed(label: String): DbResult<Integer>
    return Database.transaction() do |tx|
        reports := Report.using(tx)
        first := try reports.create(title: label)
        puts(try Report.count())
        puts(try reports.count())
        inner_value := tx.transaction() do |inner|
            nested := Report.using(inner)
            _created := try nested.create(title: "discard")
            _duplicate := try nested.create(title: "discard")
            1
        end catch |error|
            puts(error.message)
            0
        end
        puts(inner_value)
        puts(try reports.count())
        nested_count := try tx.transaction() do |inner|
            row := try Report.using(inner).create(title: "kept")
            temporary := try Report.using(inner).create(title: "temporary")
            puts(try temporary.delete())
            row.id
        end
        puts(nested_count)
        _changed := try first.update(note: "saved")
        try reports.count()
    end
end

def rolled_back(): DbResult<Integer>
    return Database.transaction() do |tx|
        scoped := Report.using(tx)
        _discard := try scoped.create(title: "outer discard")
        inner := try tx.transaction() do |nested|
            _created := try Report.using(nested).create(title: "kept")
            7
        end
        inner
    end
end

def exercise_transactions()
    count := committed("root") catch |error|
        puts(error.message)
        return
    end
    puts(count)
    observed := rolled_back() catch |error|
        puts(error.message)
        0
    end
    puts(observed)
end
""")
    source = """import { exercise_transactions } from transactions
import { Report, Tag } from models
import { DbErrorKind, DbResult } from trb/orm
import trb/std/process

def change_report()
    original := Report.find(1) catch |error|
        puts(error.message)
        return
    end
    if original == nil
        return
    end
    pending := original.with(title: "ready")
    changed := original.update(note: "separate edit") catch |error|
        puts(error.message)
        return
    end
    puts(changed.title)
    saved := pending.save() catch |error|
        puts(error.message)
        return
    end
    puts(saved.title)
    note := saved.note
    if note != nil
        puts(note)
    end
    again := pending.save() catch |error|
        puts(error.message)
        return
    end
    puts(again.id)
    cleared := saved.update(note: nil) catch |error|
        puts(error.message)
        return
    end
    puts(cleared.note == nil)
    puts(original.title)
    case original.update()
    when DbResult::Err(error)
        puts(error.message)
    when DbResult::Ok(_unexpected)
        puts("unexpected update")
    end
end

def save_missing_report()
    original := Report.find(1) catch |error|
        puts(error.message)
        return
    end
    if original == nil
        return
    end
    pending := original.with(note: "after deletion")
    _deleted := original.delete() catch |error|
        puts(error.message)
        return
    end
    case pending.save()
    when DbResult::Err(error)
        case error.kind
        when DbErrorKind::InvalidData
            puts("InvalidData")
        else
            puts("unexpected")
        end
        puts(error.message)
    when DbResult::Ok(_unexpected)
        puts("unexpected update")
    end
end

def create_reports()
    first := Report.create(title: "first") catch |error|
        puts(error.message)
        return
    end
    puts(first.id)
    puts(first.title)
    note := first.note
    if note != nil
        puts(note)
    end
    second := Report.create(note: nil, title: "second", id: 42) catch |error|
        puts(error.message)
        return
    end
    puts(second.id)
    puts(second.note == nil)
    case Report.create(id: 42, title: "duplicate", note: "text")
    when DbResult::Ok(_unexpected)
        puts("unexpected insert")
    when DbResult::Err(error)
        case error.kind
        when DbErrorKind::Constraint
            puts("Constraint")
        else
            puts("unexpected error")
        end
        puts(error.message)
    end
end

def remove_tag()
    tag := Tag.find("reader's copy") catch |error|
        puts(error.message)
        return
    end
    if tag == nil
        puts("missing tag")
        return
    end
    puts(tag.code)
    first := tag.delete() catch |error|
        case error.kind
        when DbErrorKind::Constraint
            puts("Constraint")
        else
            puts("unexpected")
        end
        puts(error.message)
        return
    end
    puts(first)
    again := tag.delete() catch |error|
        puts(error.message)
        return
    end
    puts(again)
end

def show(id: Integer)
    case Report.find(id)
    when DbResult::Ok(item)
        if item == nil
            puts("missing")
            return
        end
        puts(item.id)
        puts(item.title)
        note := item.note
        if note != nil
            puts(note)
        end
    when DbResult::Err(error)
        case error.kind
        when DbErrorKind::Connection
            puts("Connection")
        when DbErrorKind::InvalidData
            puts("InvalidData")
        when DbErrorKind::Query
            puts("Query")
        else
            puts("unexpected")
        end
        puts(error.message)
    end
end

def main()
    operation := Process.environment("ORM_TEST_OPERATION")
    if operation != nil
        if operation == "transactions"
            exercise_transactions()
            return
        end
        if operation == "update"
            change_report()
            return
        end
        if operation == "save_missing"
            save_missing_report()
            return
        end
        if operation == "insert"
            create_reports()
            return
        end
        if operation == "delete"
            remove_tag()
            return
        end
        if operation == "empty"
            report := Report.new()
            puts(report.id)
            puts(report.title)
            return
        end
    end
    show(1)
    show(2)
    show(3)
end
"""
    entry = src / "main.trb"
    entry.write_text(source)
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
        run([str(tool), "check"], root, environment)
        app = root / ("lookup-" + mode)
        run([str(tool), "build", "--compile", "--outfile", str(app)], root, environment)
        binaries.append(app)
    assert not database.exists()
    for app in binaries:
        expect_output(app, root, dict(environment, ORM_TEST_OPERATION="empty"), "0\n\n")
        assert not database.exists()
        transactions = root / (app.name + "-transactions.sqlite")
        with sqlite3.connect(transactions) as connection:
            connection.execute("CREATE TABLE reports(id INTEGER PRIMARY KEY, title TEXT NOT NULL UNIQUE, note TEXT)")
        expect_output(app, root, dict(environment, DATABASE_URL=str(transactions), ORM_TEST_OPERATION="transactions"),
                      "0\n1\ndatabase insert failed\n0\n1\ntrue\n2\n2\ndatabase insert failed\n0\n")
        with sqlite3.connect(transactions) as connection:
            assert connection.execute("SELECT id, title, note FROM reports ORDER BY id").fetchall() == [
                (1, "root", "saved"), (2, "kept", None)]
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
    print("SQLite ORM lock-only builds, typed counts, create/find/update/delete, deferred saves, transaction isolation, nested rollback, runtime errors and worker updates passed")


if __name__ == "__main__":
    main()
