package dbtool

import (
	"reflect"
	"strings"
	"testing"
)

func TestPostgreSQLConnectionKeepsPasswordOutOfArguments(t *testing.T) {
	arguments, environment, err := connection("postgresql", "postgres://app:secret@db.example:5433/catalog?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	wantArguments := []string{"--user", "app", "--host", "db.example", "--port", "5433", "catalog"}
	if !reflect.DeepEqual(arguments, wantArguments) {
		t.Fatalf("arguments=%#v, want %#v", arguments, wantArguments)
	}
	if !reflect.DeepEqual(environment, []string{"PGPASSWORD=secret", "PGSSLMODE=require"}) {
		t.Fatalf("environment=%#v", environment)
	}
	if strings.Contains(strings.Join(arguments, " "), "secret") {
		t.Fatal("password leaked into process arguments")
	}
}

func TestMySQLConnectionKeepsPasswordOutOfArguments(t *testing.T) {
	arguments, environment, err := connection("mysql", "app:secret@tcp(db.example:3307)/catalog")
	if err != nil {
		t.Fatal(err)
	}
	wantArguments := []string{"--user", "app", "--host", "db.example", "--port", "3307", "catalog"}
	if !reflect.DeepEqual(arguments, wantArguments) {
		t.Fatalf("arguments=%#v, want %#v", arguments, wantArguments)
	}
	if !reflect.DeepEqual(environment, []string{"MYSQL_PWD=secret"}) {
		t.Fatalf("environment=%#v", environment)
	}
	if strings.Contains(strings.Join(arguments, " "), "secret") {
		t.Fatal("password leaked into process arguments")
	}
}

func TestVersionExtractsSqldefSemanticVersion(t *testing.T) {
	for input, want := range map[string]string{
		"v3.11.19\n":                  "3.11.19",
		"3.11.19 (abcdef 2026-08-01)": "3.11.19",
	} {
		if got := version(input); got != want {
			t.Fatalf("version(%q)=%q, want %q", input, got, want)
		}
	}
}
