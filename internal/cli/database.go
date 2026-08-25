package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/dbtool"
	"github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/project"
	"github.com/type-rb/type-rb/internal/schemalock"
)

func (c *CLI) runDatabase(args []string) error {
	if len(args) == 0 {
		return errors.New("db requires plan, apply, export, lock, or check")
	}
	switch args[0] {
	case "plan":
		return c.runDatabasePlan(args[1:])
	case "apply":
		return c.runDatabaseApply(args[1:])
	case "export":
		return c.runDatabaseExport(args[1:])
	case "lock":
		return c.runDatabaseLock(args[1:])
	case "check":
		return c.runDatabaseCheck(args[1:])
	default:
		return fmt.Errorf("unknown db command %q; expected plan, apply, export, lock, or check", args[0])
	}
}

func (c *CLI) runDatabasePlan(args []string) error {
	flags := databaseFlags("db plan", c.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("db plan does not accept positional arguments")
	}
	config, err := loadDatabaseConfig(databaseConfigPath(flags))
	if err != nil {
		return err
	}
	schema, err := readDatabaseSchema(config)
	if err != nil {
		return err
	}
	_, err = newSqldef(c, config).Run(dbtool.Plan, schema, false)
	return err
}

func (c *CLI) runDatabaseApply(args []string) error {
	flags := databaseFlags("db apply", c.Stderr)
	allowDestructive := flags.Bool("allow-destructive", false, "allow DROP operations")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("db apply does not accept positional arguments")
	}
	config, err := loadDatabaseConfig(databaseConfigPath(flags))
	if err != nil {
		return err
	}
	schema, err := readDatabaseSchema(config)
	if err != nil {
		return err
	}
	if _, err := newSqldef(c, config).Run(dbtool.Apply, schema, *allowDestructive); err != nil {
		return err
	}
	lock, err := inspectDatabaseLock(config)
	if err != nil {
		return fmt.Errorf("schema was applied, but updating %s failed: %w", config.Database.Lock, err)
	}
	if err := writeDatabaseLock(config, lock); err != nil {
		return fmt.Errorf("schema was applied, but updating %s failed: %w", config.Database.Lock, err)
	}
	fmt.Fprintln(c.Stdout, filepath.Join(config.Root, config.Database.Lock))
	return nil
}

func (c *CLI) runDatabaseExport(args []string) error {
	flags := databaseFlags("db export", c.Stderr)
	output := flags.String("output", "", "write exported SQL below the project root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("db export does not accept positional arguments")
	}
	config, err := loadDatabaseConfig(databaseConfigPath(flags))
	if err != nil {
		return err
	}
	exported, err := newSqldef(c, config).Run(dbtool.Export, nil, false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*output) == "" {
		_, err = c.Stdout.Write(exported)
		return err
	}
	path, err := projectOutputPath(config.Root, *output)
	if err != nil {
		return fmt.Errorf("db export --output: %w", err)
	}
	if err := atomicOutputWrite(path, exported, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(c.Stdout, path)
	return nil
}

func (c *CLI) runDatabaseLock(args []string) error {
	flags := databaseFlags("db lock", c.Stderr)
	fromDatabase := flags.Bool("from-db", false, "generate the lock from the configured database")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("db lock does not accept positional arguments")
	}
	config, err := loadDatabaseConfig(databaseConfigPath(flags))
	if err != nil {
		return err
	}
	var lock *schemalock.Lock
	if *fromDatabase {
		lock, err = inspectDatabaseLock(config)
	} else {
		lock, err = parseDatabaseSchema(config)
	}
	if err != nil {
		return err
	}
	if err := writeDatabaseLock(config, lock); err != nil {
		return err
	}
	fmt.Fprintln(c.Stdout, filepath.Join(config.Root, config.Database.Lock))
	return nil
}

func (c *CLI) runDatabaseCheck(args []string) error {
	flags := databaseFlags("db check", c.Stderr)
	fromDatabase := flags.Bool("from-db", false, "compare the lock with the configured database")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("db check does not accept positional arguments")
	}
	config, err := loadDatabaseConfig(databaseConfigPath(flags))
	if err != nil {
		return err
	}
	committed, err := schemalock.Read(filepath.Join(config.Root, config.Database.Lock))
	if err != nil {
		return fmt.Errorf("read schema lock: %w", err)
	}
	var current *schemalock.Lock
	if *fromDatabase {
		current, err = inspectDatabaseLock(config)
	} else {
		current, err = parseDatabaseSchema(config)
	}
	if err != nil {
		return err
	}
	equal, err := schemalock.Equal(committed, current)
	if err != nil {
		return err
	}
	if !equal {
		source := config.Database.Schema
		suffix := ""
		if *fromDatabase {
			source = "the configured database"
			suffix = " --from-db"
		}
		return fmt.Errorf("%s differs from %s; run trb db lock%s", config.Database.Lock, source, suffix)
	}
	fmt.Fprintln(c.Stdout, "Database schema lock is current.")
	return nil
}

func databaseFlags(name string, output io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(output)
	flags.String("config", "", "path to trbconfig.jsonc")
	return flags
}

func databaseConfigPath(flags *flag.FlagSet) string {
	value := flags.Lookup("config")
	if value == nil {
		return ""
	}
	return value.Value.String()
}

func loadDatabaseConfig(path string) (*project.Config, error) {
	config, err := loadConfig(path, ".")
	if err != nil {
		return nil, err
	}
	if config.Database == nil {
		return nil, errors.New("trbconfig.jsonc requires a db section")
	}
	return config, nil
}

func newSqldef(c *CLI, config *project.Config) dbtool.Sqldef {
	return dbtool.Sqldef{Root: config.Root, Config: config.Database, Stdout: c.Stdout, Stderr: c.Stderr}
}

func readDatabaseSchema(config *project.Config) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(config.Root, config.Database.Schema))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", config.Database.Schema, err)
	}
	return data, nil
}

func parseDatabaseSchema(config *project.Config) (*schemalock.Lock, error) {
	schema, err := readDatabaseSchema(config)
	if err != nil {
		return nil, err
	}
	lock, err := schemalock.ParseSQL(config.Database.Adapter, schema)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", config.Database.Schema, err)
	}
	return lock, nil
}

func inspectDatabaseLock(config *project.Config) (*schemalock.Lock, error) {
	if config.Database.Database == nil {
		return nil, errors.New("db.database is required for live database access")
	}
	database, err := config.Database.Database.Resolve(config.Root, config.Database.Adapter)
	if err != nil {
		return nil, err
	}
	schema, err := orm.InspectSchema(orm.Config{Adapter: config.Database.Adapter, Database: database})
	if err != nil {
		return nil, err
	}
	return lockFromORMSchema(schema), nil
}

func lockFromORMSchema(schema *orm.Schema) *schemalock.Lock {
	lock := schemalock.New(schema.Adapter)
	for _, sourceTable := range schema.Tables {
		table := schemalock.Table{
			Columns: map[string]schemalock.Column{}, ForeignKeys: map[string]schemalock.ForeignKey{},
			UniqueConstraints: map[string]schemalock.UniqueConstraint{},
		}
		for _, sourceColumn := range sourceTable.Columns {
			portableType := sourceColumn.Type
			portableType.Nullable = false
			table.Columns[sourceColumn.Name] = schemalock.Column{
				Type: portableType.String(), Nullable: sourceColumn.Nullable, PrimaryKey: sourceColumn.PrimaryKey,
				HasDefault: sourceColumn.HasDefault, Generated: sourceColumn.Generated,
			}
		}
		for _, sourceKey := range sourceTable.ForeignKeys {
			key := schemalock.ForeignKeyKey(sourceKey.Column, sourceKey.ReferencedTable, sourceKey.ReferencedColumn)
			table.ForeignKeys[key] = schemalock.ForeignKey{
				Column: sourceKey.Column, ReferencedTable: sourceKey.ReferencedTable, ReferencedColumn: sourceKey.ReferencedColumn,
			}
		}
		for _, sourceConstraint := range sourceTable.UniqueConstraints {
			columns := append([]string(nil), sourceConstraint.Columns...)
			sort.Strings(columns)
			key := schemalock.ConstraintKey(sourceConstraint.Primary, columns)
			table.UniqueConstraints[key] = schemalock.UniqueConstraint{Columns: columns, Primary: sourceConstraint.Primary}
		}
		lock.Tables[sourceTable.Name] = table
	}
	return lock
}

func writeDatabaseLock(config *project.Config, lock *schemalock.Lock) error {
	if lock.Adapter != config.Database.Adapter {
		return fmt.Errorf("schema lock adapter is %s, expected %s", lock.Adapter, config.Database.Adapter)
	}
	return lock.Write(filepath.Join(config.Root, config.Database.Lock))
}
