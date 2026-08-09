//go:build js && wasm

package orm

import "errors"

var errBrowserIntrospection = errors.New("trb/orm live database introspection is unavailable in the browser")

type sqliteIntrospector struct{}

func (sqliteIntrospector) Inspect(Config) (*Schema, error) {
	return nil, errBrowserIntrospection
}

type postgresqlIntrospector struct{}

func (postgresqlIntrospector) Inspect(Config) (*Schema, error) {
	return nil, errBrowserIntrospection
}

type mysqlIntrospector struct{}

func (mysqlIntrospector) Inspect(Config) (*Schema, error) {
	return nil, errBrowserIntrospection
}
