package mysqlsource

import (
	"errors"
	"net/url"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// Parse accepts both go-sql-driver DSNs and TypeRB's portable mysql:// URL.
func Parse(source string) (*mysqldriver.Config, error) {
	if !strings.Contains(source, "://") {
		configuration, err := mysqldriver.ParseDSN(source)
		if err != nil {
			return nil, err
		}
		return removePortableOptions(configuration)
	}

	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme != "mysql" {
		return nil, errors.New("expected a mysql:// URL or Go driver DSN")
	}
	if parsed.Hostname() == "" {
		return nil, errors.New("MySQL database URL must contain a host")
	}
	databaseName := strings.TrimPrefix(parsed.EscapedPath(), "/")
	databaseName, err = url.PathUnescape(databaseName)
	if err != nil || databaseName == "" || strings.Contains(databaseName, "/") {
		return nil, errors.New("MySQL database URL must contain one database name")
	}

	configuration := mysqldriver.NewConfig()
	configuration.Net = "tcp"
	configuration.Addr = parsed.Host
	configuration.DBName = databaseName
	if parsed.User != nil {
		configuration.User = parsed.User.Username()
		if password, ok := parsed.User.Password(); ok {
			configuration.Passwd = password
		}
	}
	if parsed.RawQuery != "" {
		configuration, err = mysqldriver.ParseDSN(configuration.FormatDSN() + "?" + parsed.RawQuery)
		if err != nil {
			return nil, err
		}
	}
	return removePortableOptions(configuration)
}

func removePortableOptions(configuration *mysqldriver.Config) (*mysqldriver.Config, error) {
	if setting, ok := configuration.Params["allowPublicKeyRetrieval"]; ok {
		if setting != "true" && setting != "false" {
			return nil, errors.New("MySQL allowPublicKeyRetrieval must be true or false")
		}
		delete(configuration.Params, "allowPublicKeyRetrieval")
	}
	return configuration, nil
}
