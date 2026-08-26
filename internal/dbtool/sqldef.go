// Package dbtool adapts external schema management tools to TypeRB's stable
// database commands. It does not own schema semantics or ORM behavior.
package dbtool

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/mysqlsource"
	"github.com/type-rb/type-rb/internal/project"
)

type Operation string

const (
	Plan   Operation = "plan"
	Apply  Operation = "apply"
	Export Operation = "export"
)

type Sqldef struct {
	Root   string
	Config *project.DatabaseConfig
	Stdout io.Writer
	Stderr io.Writer
}

func (s Sqldef) Run(operation Operation, desired []byte, allowDestructive bool) ([]byte, error) {
	if s.Config == nil || s.Config.Sqldef == nil {
		return nil, errors.New("db.sqldef configuration is required")
	}
	if err := s.verifyVersion(); err != nil {
		return nil, err
	}
	if s.Config.Database == nil {
		return nil, errors.New("db.database is required for this command")
	}
	database, err := s.Config.Database.Resolve(s.Root, s.Config.Adapter)
	if err != nil {
		return nil, err
	}
	connectionArgs, connectionEnv, err := connection(s.Config.Adapter, database)
	if err != nil {
		return nil, err
	}
	arguments := append([]string(nil), s.Config.Sqldef.Arguments...)
	switch operation {
	case Plan:
		arguments = append(arguments, "--dry-run")
	case Apply:
		arguments = append(arguments, "--apply")
		if allowDestructive {
			arguments = append(arguments, "--enable-drop")
		}
	case Export:
		arguments = append(arguments, "--export")
	default:
		return nil, fmt.Errorf("unsupported sqldef operation %q", operation)
	}
	arguments = append(arguments, connectionArgs...)
	command := exec.Command(s.Config.Sqldef.Command, arguments...)
	command.Dir = s.Root
	command.Env = append(os.Environ(), connectionEnv...)
	if operation != Export {
		command.Stdin = bytes.NewReader(desired)
	}
	var captured bytes.Buffer
	if operation == Export {
		command.Stdout = &captured
	} else if s.Stdout != nil {
		command.Stdout = s.Stdout
	}
	command.Stderr = s.Stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("%s %s: %w", filepath.Base(s.Config.Sqldef.Command), operation, err)
	}
	return captured.Bytes(), nil
}

func (s Sqldef) verifyVersion() error {
	command := exec.Command(s.Config.Sqldef.Command, "--version")
	command.Dir = s.Root
	output, err := command.CombinedOutput()
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s is not installed or is not on PATH; install sqldef %s or set db.sqldef.command", s.Config.Sqldef.Command, s.Config.Sqldef.Version)
	}
	if err != nil {
		return fmt.Errorf("check %s version: %w: %s", filepath.Base(s.Config.Sqldef.Command), err, strings.TrimSpace(string(output)))
	}
	actual := version(string(output))
	wanted := strings.TrimPrefix(strings.TrimSpace(s.Config.Sqldef.Version), "v")
	if actual != wanted {
		return fmt.Errorf("%s version %s is required, found %s; update sqldef or set db.sqldef.version explicitly", filepath.Base(s.Config.Sqldef.Command), wanted, actual)
	}
	return nil
}

func version(output string) string {
	for _, field := range strings.Fields(output) {
		candidate := strings.TrimPrefix(strings.TrimSpace(field), "v")
		parts := strings.Split(candidate, ".")
		if len(parts) != 3 {
			continue
		}
		valid := true
		for _, part := range parts {
			if _, err := strconv.Atoi(part); err != nil {
				valid = false
				break
			}
		}
		if valid {
			return candidate
		}
	}
	return strings.TrimSpace(output)
}

func connection(adapter, database string) ([]string, []string, error) {
	switch adapter {
	case "sqlite":
		return []string{database}, nil, nil
	case "postgresql":
		return postgresqlConnection(database)
	case "mysql":
		return mysqlConnection(database)
	default:
		return nil, nil, fmt.Errorf("unsupported database adapter %q", adapter)
	}
}

func postgresqlConnection(database string) ([]string, []string, error) {
	if !strings.Contains(database, "://") {
		return []string{database}, nil, nil
	}
	parsed, err := url.Parse(database)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return nil, nil, fmt.Errorf("invalid PostgreSQL database URL %q", database)
	}
	name := strings.TrimPrefix(parsed.EscapedPath(), "/")
	name, err = url.PathUnescape(name)
	if err != nil || name == "" || strings.Contains(name, "/") {
		return nil, nil, fmt.Errorf("PostgreSQL database URL must contain one database name")
	}
	arguments := []string{}
	environment := []string{}
	if parsed.User != nil {
		if user := parsed.User.Username(); user != "" {
			arguments = append(arguments, "--user", user)
		}
		if password, ok := parsed.User.Password(); ok {
			environment = append(environment, "PGPASSWORD="+password)
		}
	}
	if host := parsed.Hostname(); host != "" {
		arguments = append(arguments, "--host", host)
	}
	if port := parsed.Port(); port != "" {
		arguments = append(arguments, "--port", port)
	}
	if sslMode := parsed.Query().Get("sslmode"); sslMode != "" {
		environment = append(environment, "PGSSLMODE="+sslMode)
	}
	return append(arguments, name), environment, nil
}

func mysqlConnection(database string) ([]string, []string, error) {
	configuration, err := mysqlsource.Parse(database)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid MySQL database source: %w", err)
	}
	if strings.TrimSpace(configuration.DBName) == "" {
		return nil, nil, errors.New("MySQL database source must contain a database name")
	}
	arguments := []string{}
	if configuration.User != "" {
		arguments = append(arguments, "--user", configuration.User)
	}
	if configuration.Net == "unix" {
		arguments = append(arguments, "--socket", configuration.Addr)
	} else if configuration.Addr != "" {
		host, port, splitErr := net.SplitHostPort(configuration.Addr)
		if splitErr != nil {
			host = configuration.Addr
		}
		if host != "" {
			arguments = append(arguments, "--host", host)
		}
		if port != "" {
			arguments = append(arguments, "--port", port)
		}
	}
	environment := []string{}
	if configuration.Passwd != "" {
		environment = append(environment, "MYSQL_PWD="+configuration.Passwd)
	}
	return append(arguments, configuration.DBName), environment, nil
}
