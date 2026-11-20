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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--native", required=True, type=Path)
    parser.add_argument("--reference", type=Path)
    args = parser.parse_args()
    cli = str(args.native.resolve())
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
        schema["tables"]["é_categories"] = schema["tables"].pop("others")
        lock.write_text(json.dumps(schema, ensure_ascii=False))
        entry.write_text(source.replace("Category", "ÉCategory").replace(
            "import { Model }", "import { DbErrorKind, DbResult, Model }").replace(
            '    puts("locked")',
            '    case ÉCategory.count()\n    when DbResult::Ok(value)\n        puts(value)\n'
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
        if args.reference:
            reference = str(args.reference.resolve())
            reference_config = dict(config, mode="go", go={"module": "example.com/orm-lock"})
            (root / "trbconfig.jsonc").write_text(json.dumps(reference_config))
            go_binary = root / "go-sample"
            run([reference, "check", "--config", "trbconfig.jsonc"], root, environment)
            run([reference, "build", "--compile", "--outfile", str(go_binary)], root, environment)
            assert not database.exists()
            binaries.append(go_binary)
        with sqlite3.connect(database) as connection:
            connection.executescript("CREATE TABLE é_categories(id INTEGER PRIMARY KEY, name TEXT, price REAL, updated_at DATETIME);"
                                     "INSERT INTO é_categories(name) VALUES ('one'), ('two')")
        for app in binaries:
            expect_output(app, root, environment, "2\n")
        with sqlite3.connect(database) as connection:
            connection.execute("DELETE FROM é_categories")
        for app in binaries:
            expect_output(app, root, environment, "0\n")
        with sqlite3.connect(database) as connection:
            connection.execute("DROP TABLE é_categories")
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
    print("SQLite ORM lock-only build, field diagnostics, typed counts and runtime database opening passed")


if __name__ == "__main__":
    main()
