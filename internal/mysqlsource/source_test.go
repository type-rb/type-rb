package mysqlsource

import (
	"testing"
	"time"
)

func TestParsePortableURL(t *testing.T) {
	configuration, err := Parse("mysql://app:p%40ss@db.example:3307/reporting?allowPublicKeyRetrieval=true&timeout=2s")
	if err != nil {
		t.Fatal(err)
	}
	if configuration.User != "app" || configuration.Passwd != "p@ss" {
		t.Fatalf("unexpected credentials: user=%q password=%q", configuration.User, configuration.Passwd)
	}
	if configuration.Net != "tcp" || configuration.Addr != "db.example:3307" || configuration.DBName != "reporting" {
		t.Fatalf("unexpected connection target: %#v", configuration)
	}
	if configuration.Timeout != 2*time.Second {
		t.Fatalf("timeout=%s, want 2s", configuration.Timeout)
	}
	if _, ok := configuration.Params["allowPublicKeyRetrieval"]; ok {
		t.Fatal("Bun-only allowPublicKeyRetrieval option remained in the Go driver configuration")
	}
}

func TestParseDriverDSN(t *testing.T) {
	configuration, err := Parse("app:secret@tcp(db.example:3307)/reporting?allowPublicKeyRetrieval=false&timeout=3s")
	if err != nil {
		t.Fatal(err)
	}
	if configuration.User != "app" || configuration.Addr != "db.example:3307" || configuration.DBName != "reporting" || configuration.Timeout != 3*time.Second {
		t.Fatalf("unexpected driver configuration: %#v", configuration)
	}
	if _, ok := configuration.Params["allowPublicKeyRetrieval"]; ok {
		t.Fatal("Bun-only allowPublicKeyRetrieval option remained in the Go driver configuration")
	}
}

func TestParseRejectsInvalidPortableURL(t *testing.T) {
	for _, source := range []string{
		"postgres://app@db.example/reporting",
		"mysql:///reporting",
		"mysql://db.example",
		"mysql://db.example/reporting/extra",
		"mysql://db.example/reporting?allowPublicKeyRetrieval=maybe",
	} {
		if _, err := Parse(source); err == nil {
			t.Fatalf("Parse accepted invalid MySQL source %q", source)
		}
	}
}
