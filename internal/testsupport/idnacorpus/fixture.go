// Package idnacorpus exercises UTS #46 providers against Unicode's fixed data.
package idnacorpus

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

//go:embed IdnaTestV2.txt
var corpus string

var escaped = regexp.MustCompile(`\\u([0-9A-Fa-f]{4})|\\x\{([0-9A-Fa-f]+)\}`)

func decode(value string) string {
	if value == `""` {
		return ""
	}
	return escaped.ReplaceAllStringFunc(value, func(s string) string {
		digits := s[2:]
		if s[1] == 'x' {
			digits = s[3 : len(s)-1]
		}
		number, err := strconv.ParseUint(digits, 16, 32)
		if err != nil {
			panic(err)
		}
		return string(rune(number))
	})
}

// Check compares strict, non-transitional ToASCII output and rejection. Failed
// conversions need not return Unicode's partial output or exact error wording.
func Check(t *testing.T, directory string, command ...string) {
	t.Helper()
	type fixture struct {
		line            int
		input, expected string
		valid           bool
	}
	var rows []fixture
	var input bytes.Buffer
	for number, line := range strings.Split(corpus, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line == "" {
			continue
		}
		fields := strings.Split(line, ";")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		source := decode(fields[0])
		unicode := source
		if fields[1] != "" {
			unicode = decode(fields[1])
		}
		ascii := unicode
		if fields[3] != "" {
			ascii = decode(fields[3])
		}
		status := fields[4]
		if status == "" {
			status = fields[2]
		}
		rows = append(rows, fixture{number + 1, source, ascii, status == "" || status == "[]"})
		if err := json.NewEncoder(&input).Encode(source); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, command[0], command[1:]...)
	process.Dir = directory
	process.Stdin = &input
	var output, stderr bytes.Buffer
	process.Stdout = &output
	process.Stderr = &stderr
	if err := process.Run(); err != nil {
		t.Fatalf("provider: %v\n%s", err, stderr.String())
	}
	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if len(lines) != len(rows) {
		t.Fatalf("provider returned %d results for %d cases: %s", len(lines), len(rows), stderr.String())
	}
	for i, line := range lines {
		var result map[string]string
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			t.Fatal(err)
		}
		actual, valid := result["output"]
		row := rows[i]
		if valid != row.valid || valid && actual != row.expected {
			t.Errorf("IdnaTestV2:%d input=%q expected=%q valid=%v actual=%v", row.line, row.input, row.expected, row.valid, result)
		}
	}
	t.Logf("Unicode 17.0.0: %d official non-transitional ToASCII cases", len(rows))
}
