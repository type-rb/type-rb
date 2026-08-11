package compiler

import (
	"strings"
	"testing"
)

func TestPortableTimePackageLowersAcrossBackends(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Date, DateTime, Duration, Instant, TimeOfDay, TimeZone } from trb/std/time

def sample(): Instant
	date := Date.new(2026, 8, 11)
	clock := TimeOfDay.new(9, 30)
	local := date.at(clock)
	return local.to_instant(TimeZone.get("Asia/Tokyo")).add(Duration.minutes(30))
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: mode, GoModule: "example.com/time-app", RubyLoader: "require_relative", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			var consumer, runtime *Artifact
			for _, artifact := range artifacts {
				switch artifact.IR.ModulePath {
				case "main":
					consumer = artifact
				case "trb/std/time/index":
					runtime = artifact
				}
			}
			if consumer == nil || runtime == nil {
				t.Fatalf("%s did not emit the time consumer and runtime", mode)
			}
			consumerWants := map[string][]string{
				"go":         {`time.NewDate(2026, 8, 11)`, `time.TimeZoneGet("Asia/Tokyo")`, `time.DurationMinutes(30)`},
				"ruby":       {`Date.new(2026, 8, 11)`, `TimeZone.get("Asia/Tokyo")`, `Duration.minutes(30)`},
				"typescript": {`new Date(2026, 8, 11)`, `TimeZone.get("Asia/Tokyo")`, `Duration.minutes(30)`},
			}[mode]
			for _, want := range consumerWants {
				if !strings.Contains(string(consumer.Output), want) {
					t.Fatalf("generated %s consumer is missing %q:\n%s", mode, want, consumer.Output)
				}
			}
			runtimeWants := map[string][]string{
				"go":         {`type Instant struct`, `stdtime.LoadLocation`, `local DateTime is ambiguous in TimeZone`},
				"ruby":       {`class TrbTimeInstant`, `ENV["TZ"]`, `local DateTime is ambiguous in TimeZone`},
				"typescript": {`export class Instant`, `Intl.DateTimeFormat`, `local DateTime is ambiguous in TimeZone`},
			}[mode]
			for _, want := range runtimeWants {
				if !strings.Contains(string(runtime.Output), want) {
					t.Fatalf("generated %s time runtime is missing %q:\n%s", mode, want, runtime.Output)
				}
			}
		})
	}
}

func TestPortableTimeDiagnosticsAreModeIndependent(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "constructor", source: `import { Date } from trb/std/time
value := Date.new("2026", 8, 11)
`, want: "argument 1 to Date() has type String, expected Integer"},
		{name: "zone", source: `import { DateTime } from trb/std/time
value := DateTime.new(2026, 8, 11, 9, 30).to_instant("UTC")
`, want: "argument 1 to to_instant() has type String, expected TimeZone"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			if _, err := Compile("bad.trb", []byte(test.source), mode); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s %s: expected %q, got %v", mode, test.name, test.want, err)
			}
		}
	}
}
