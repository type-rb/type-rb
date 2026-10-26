package releasecontract

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewedInventoryOwnsEveryCurrentSurfaceAndView(t *testing.T) {
	manifest, catalog, digest, err := Load("../..")
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckDiscovery("../..", catalog); err != nil {
		t.Fatal(err)
	}
	view, err := read("../..", ViewPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(view, Render(manifest, digest)) {
		t.Fatal("generated feature view is stale")
	}
	ids := map[string]bool{}
	for _, row := range manifest.Rows {
		ids[row.ID] = true
	}
	for _, id := range []string{"output.go", "output.ruby", "output.typescript", "application.composed", "native.qualification", "distribution", "editor.integrations", "ecosystem.protocols"} {
		if !ids[id] {
			t.Errorf("missing whole release obligation %s", id)
		}
	}
	results := ResultsTemplate(manifest, digest)
	if err := ValidateResults(manifest, digest, results, t.TempDir(), false); err != nil {
		t.Fatal(err)
	}
	if err := ValidateResults(manifest, digest, results, t.TempDir(), true); err == nil {
		t.Fatal("unqualified inventory claimed release qualification")
	}
	for _, cell := range results.Cells {
		if cell.Implementation == "go" && cell.Output == "native" {
			t.Fatal("Go acquired a Native backend obligation")
		}
	}
}

func TestInventoryRejectsDisappearingOrUnownedContracts(t *testing.T) {
	for _, change := range []string{"new-surface", "duplicate-owner", "host-removed", "native-removed", "unknown-command", "missing-owner"} {
		t.Run(change, func(t *testing.T) {
			manifest, catalog, _, err := Load("../..")
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "new-surface":
				catalog = append(catalog, Surface{ID: "package:trb/std/future", Source: "future.go"})
			case "duplicate-owner":
				manifest.Rows[1].Surfaces = append(manifest.Rows[1].Surfaces, manifest.Rows[0].Surfaces[0])
			case "host-removed":
				manifest.Hosts = manifest.Hosts[1:]
			case "native-removed":
				manifest.Rows[0].Implementations = []string{"go"}
			case "unknown-command":
				manifest.Rows[0].Commands = []string{"invented"}
			case "missing-owner":
				manifest.Rows[0].Owner = ""
			}
			if err := ValidateManifest(manifest, catalog); err == nil {
				t.Fatal("weakened inventory was accepted")
			}
		})
	}
}

func passingFixture(t *testing.T) (Manifest, string, Results, string) {
	t.Helper()
	directory := t.TempDir()
	data := []byte("candidate-bound scenario: expected acceptance and rejection verified\n")
	if err := os.WriteFile(filepath.Join(directory, "receipt.log"), data, 0600); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{Release: "0.5.0", Hosts: []string{"linux-arm64"}, Rows: []Row{{ID: "example", Implementations: []string{"go", "native"}, Outputs: []string{"go"}, NativeExecutable: true, QualificationReady: true, Obligations: []string{"accept", "reject"}, Commands: []string{"scenario"}}}}
	digest := strings.Repeat("a", 64)
	result := ResultsTemplate(manifest, digest)
	for _, implementation := range []string{"go", "native"} {
		result.Candidates = append(result.Candidates, Candidate{Implementation: implementation, Host: "linux-arm64", SourceRevision: strings.Repeat("b", 40), CompilerSHA256: strings.Repeat("c", 64), Toolchains: map[string]string{"compiler": "exact-fixture-tool"}})
	}
	for index := range result.Cells {
		result.Cells[index].Status = "passed"
		for _, obligation := range []string{"accept", "reject"} {
			result.Cells[index].Observations = append(result.Cells[index].Observations, Observation{Obligation: obligation, CommandID: "scenario", ExecutedArgv: []string{"fixture-runner", "--candidate", "candidate"}, Cases: 1, Evidence: "receipt.log", SHA256: hashBytes(data)})
		}
	}
	return manifest, digest, result, directory
}

func TestResultQualificationRejectsFalseGreenReceipts(t *testing.T) {
	manifest, digest, result, directory := passingFixture(t)
	if err := ValidateResults(manifest, digest, result, directory, true); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"stale", "missing", "duplicate", "unexpected", "empty", "skip", "failure", "exit", "candidate", "toolchain", "plan", "obligation", "hash", "escape", "symlink", "blocked"} {
		t.Run(change, func(t *testing.T) {
			m, d, r, dir := passingFixture(t)
			switch change {
			case "stale":
				r.ManifestSHA256 = strings.Repeat("d", 64)
			case "missing":
				r.Cells = r.Cells[1:]
			case "duplicate":
				r.Cells = append(r.Cells, r.Cells[0])
			case "unexpected":
				r.Cells[0].Output = "native"
			case "empty":
				r.Cells[0].Observations[0].Cases = 0
			case "skip":
				r.Cells[0].Observations[0].Skipped = 1
			case "failure":
				r.Cells[0].Observations[0].Failures = 1
			case "exit":
				r.Cells[0].Observations[0].ExitCode = 1
			case "candidate":
				r.Candidates = r.Candidates[1:]
			case "toolchain":
				r.Candidates[0].Toolchains = map[string]string{}
			case "plan":
				m.Rows[0].QualificationReady = false
			case "obligation":
				r.Cells[0].Observations = r.Cells[0].Observations[1:]
			case "hash":
				r.Cells[0].Observations[0].SHA256 = strings.Repeat("d", 64)
			case "escape":
				r.Cells[0].Observations[0].Evidence = "../receipt.log"
			case "symlink":
				outside := filepath.Join(t.TempDir(), "receipt.log")
				if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(dir, "outside.log")); err != nil {
					t.Fatal(err)
				}
				r.Cells[0].Observations[0].Evidence = "outside.log"
			case "blocked":
				r.Cells[0].Status = "blocked"
				r.Cells[0].Reason = "missing runtime provider"
			}
			if err := ValidateResults(m, d, r, dir, true); err == nil {
				t.Fatal("false green result accepted")
			}
		})
	}
}

func TestResultDecoderRejectsMissingAndUnknownFields(t *testing.T) {
	_, _, result, _ := passingFixture(t)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range [][]byte{
		bytes.Replace(data, []byte(`"exitCode":0,`), nil, 1),
		bytes.Replace(data, []byte(`"skipped":0,`), nil, 1),
		bytes.Replace(data, []byte(`"cases":1`), []byte(`"cases":null`), 1),
		bytes.Replace(data, []byte(`"schemaVersion":2`), []byte(`"schemaVersion":2,"invented":true`), 1),
		append(append([]byte{}, data...), []byte(` {}`)...),
	} {
		var decoded Results
		if err := decode(changed, &decoded); err == nil {
			t.Fatal("incomplete or unknown receipt field accepted")
		}
	}
}
