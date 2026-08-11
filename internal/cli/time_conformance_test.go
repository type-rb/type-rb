package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestPortableTimeAcrossAvailableBackendsAndREPL(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runtime := map[string]string{"go": "go", "ruby": "ruby", "typescript": "node"}[mode]
			if _, err := exec.LookPath(runtime); err != nil {
				t.Skip(runtime + " is not installed")
			}
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/time-conformance"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(timeConformanceSource), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("run status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != timeConformanceOutput {
				t.Fatalf("unexpected %s time output\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, timeConformanceOutput, stdout.String(), stderr.String())
			}

			stdout.Reset()
			stderr.Reset()
			input := "import { Date, DateTime, Duration, Instant, TimeZone } from trb/std/time\n" +
				"import { decode, encode } from trb/std/json\n" +
				"Date.parse(\"2024-02-29\").add_days(1).to_s()\n" +
				"Instant.parse(\"2026-08-11T10:00:00+09:00\").to_datetime(TimeZone.utc()).to_s()\n" +
				"Duration.milliseconds(-1500).to_s()\n" +
				"decode<Date>(\"\\\"2026-08-11\\\"\")\n" +
				"encode(Duration.parse(\"PT1.25S\"))\n" +
				":quit\n"
			command = &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
				t.Fatalf("REPL status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			for _, want := range []string{`"2024-03-01" : String`, `"2026-08-11T01:00:00" : String`, `"-PT1.5S" : String`, `Result::Ok(value: #<Date _day: 11, _month: 8, _year: 2026>)`, `Result::Ok(value: "\"PT1.25S\"")`} {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("%s REPL output is missing %q: %s", mode, want, stdout.String())
				}
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected %s REPL stderr: %s", mode, stderr.String())
			}
		})
	}
}

const timeConformanceSource = `import { Date, DateTime, DateTimeErrorKind, Duration, Instant, TimeOfDay, TimeZone } from trb/std/time
import { decode, encode } from trb/std/json
import { Result } from trb/std/result

record TemporalPayload
	date: Date
	clock: TimeOfDay
	local: DateTime
	instant: Instant
	duration: Duration
	zone: TimeZone
end

def main()
	puts(Date.parse("2024-02-29").add_days(1).to_s())
	puts(TimeOfDay.parse("09:30").to_s())
	puts(DateTime.parse("2026-08-11T09:30:00.1234").to_s())
	puts(Instant.parse("2026-08-11T10:00:00+09:00").to_s())
	new_york := TimeZone.get("America/New_York")
	before_gap := Instant.parse("2024-03-10T06:30:00Z")
	puts(before_gap.add(Duration.hours(2)).to_datetime(new_york).to_s())
	puts(Duration.milliseconds(-1500).to_s())
	case Date.try_new(2023, 2, 29)
	when Result::Ok(_value)
		puts("unexpected")
	when Result::Err(error)
		puts(error.kind == DateTimeErrorKind::InvalidDate)
	end
	gap := DateTime.new(2024, 3, 10, 2, 30)
	case gap.try_to_instant(new_york)
	when Result::Ok(_value)
		puts("unexpected")
	when Result::Err(error)
		puts(error.kind == DateTimeErrorKind::NonexistentLocalTime)
	end
	overlap := DateTime.new(2024, 11, 3, 1, 30)
	case overlap.try_to_instant(new_york)
	when Result::Ok(_value)
		puts("unexpected")
	when Result::Err(error)
		puts(error.kind == DateTimeErrorKind::AmbiguousLocalTime)
	end
	payload := TemporalPayload.new(
		date: Date.new(2026, 8, 11),
		clock: TimeOfDay.new(9, 30, 0, 123000000),
		local: DateTime.new(2026, 8, 11, 9, 30),
		instant: Instant.parse("2026-08-11T00:30:00Z"),
		duration: Duration.parse("PT90.5S"),
		zone: TimeZone.get("Asia/Tokyo")
	)
	case encode(payload)
	when Result::Ok(encoded)
		case decode<TemporalPayload>(encoded)
		when Result::Ok(copy)
			puts(copy.date.to_s())
			puts(copy.clock.to_s())
			puts(copy.local.to_s())
			puts(copy.instant.to_s())
			puts(copy.duration.to_s())
			puts(copy.zone.identifier())
		when Result::Err(error)
			puts(error.path)
		end
	when Result::Err(error)
		puts(error.path)
	end
	case decode<TemporalPayload>("{\"date\":\"bad\",\"clock\":\"09:30:00\",\"local\":\"2026-08-11T09:30:00\",\"instant\":\"2026-08-11T00:30:00Z\",\"duration\":\"PT1S\",\"zone\":\"UTC\"}")
	when Result::Ok(_copy)
		puts("unexpected")
	when Result::Err(error)
		puts(error.path)
	end
end
`

const timeConformanceOutput = `2024-03-01
09:30:00
2026-08-11T09:30:00.1234
2026-08-11T01:00:00Z
2024-03-10T04:30:00
-PT1.5S
true
true
true
2026-08-11
09:30:00.123
2026-08-11T09:30:00
2026-08-11T00:30:00Z
PT90.5S
Asia/Tokyo
/date
`
