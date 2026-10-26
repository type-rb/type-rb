package releasecontract

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Results is the v2 evidence interchange schema. ValidateResults distinguishes
// a structurally valid progress report from complete release qualification.
type Results struct {
	SchemaVersion  int         `json:"schemaVersion"`
	Release        string      `json:"release"`
	ManifestSHA256 string      `json:"manifestSHA256"`
	Candidates     []Candidate `json:"candidates"`
	Cells          []Cell      `json:"cells"`
}

type Candidate struct {
	Implementation string            `json:"implementation"`
	Host           string            `json:"host"`
	SourceRevision string            `json:"sourceRevision"`
	CompilerSHA256 string            `json:"compilerSHA256"`
	Toolchains     map[string]string `json:"toolchains"`
}

type Cell struct {
	Row            string        `json:"row"`
	Implementation string        `json:"implementation"`
	Output         string        `json:"output"`
	Host           string        `json:"host"`
	Status         string        `json:"status"`
	Reason         string        `json:"reason,omitempty"`
	Observations   []Observation `json:"observations"`
	Deferrals      []string      `json:"deferrals,omitempty"`
}

type Observation struct {
	Obligation   string   `json:"obligation"`
	CommandID    string   `json:"commandID"`
	ExecutedArgv []string `json:"executedArgv"`
	Cases        int      `json:"cases"`
	Skipped      int      `json:"skipped"`
	Failures     int      `json:"failures"`
	ExitCode     int      `json:"exitCode"`
	Evidence     string   `json:"evidence"`
	SHA256       string   `json:"sha256"`
}

func cellKey(cell Cell) string {
	return cell.Row + "/" + cell.Implementation + "/" + cell.Output + "/" + cell.Host
}
func candidateKey(implementation, host string) string { return implementation + "/" + host }

// deferredObligations is derived only from the hashed, reviewed manifest.
func deferredObligations(row Row, implementation, output string) (map[string]bool, []string) {
	obligations, ids := map[string]bool{}, []string{}
	for _, d := range row.Deferrals {
		if contains(d.Implementations, implementation) && contains(d.Outputs, output) {
			ids = append(ids, d.ID)
			for _, obligation := range d.Obligations {
				obligations[obligation] = true
			}
		}
	}
	return obligations, ids
}

func ResultsTemplate(manifest Manifest, digest string) Results {
	result := Results{SchemaVersion: 2, Release: manifest.Release, ManifestSHA256: digest, Candidates: []Candidate{}, Cells: []Cell{}}
	for _, row := range manifest.Rows {
		for _, implementation := range row.Implementations {
			outputs := append([]string{}, row.Outputs...)
			if implementation == "native" && row.NativeExecutable {
				outputs = append(outputs, "native")
			}
			for _, output := range outputs {
				for _, host := range manifest.Hosts {
					_, deferralIDs := deferredObligations(row, implementation, output)
					result.Cells = append(result.Cells, Cell{Row: row.ID, Implementation: implementation, Output: output, Host: host, Status: row.Baseline, Reason: row.Remaining, Observations: []Observation{}, Deferrals: deferralIDs})
				}
			}
		}
	}
	return result
}

func LoadResults(path string) (Results, error) {
	var result Results
	data, err := os.ReadFile(path)
	if err == nil {
		err = decode(data, &result)
	}
	return result, err
}

func evidenceHash(directory, name string) (string, error) {
	if name == "" || filepath.IsAbs(name) || filepath.Clean(name) != name || name == "." || strings.HasPrefix(name, "..") {
		return "", fmt.Errorf("evidence must be a relative bundle file")
	}
	root, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(directory, name))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("evidence escapes its bundle")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func ValidateResults(manifest Manifest, digest string, result Results, directory string, qualify bool) error {
	if result.SchemaVersion != 2 || result.Release != manifest.Release || result.ManifestSHA256 != digest {
		return fmt.Errorf("stale or incompatible result identity")
	}
	expected := map[string]bool{}
	for _, cell := range ResultsTemplate(manifest, digest).Cells {
		expected[cellKey(cell)] = true
	}
	rows := map[string]Row{}
	for _, row := range manifest.Rows {
		rows[row.ID] = row
	}
	candidates := map[string]bool{}
	for _, candidate := range result.Candidates {
		key := candidateKey(candidate.Implementation, candidate.Host)
		if candidates[key] || (candidate.Implementation != "go" && candidate.Implementation != "native") || !contains(manifest.Hosts, candidate.Host) || !revisionPattern.MatchString(candidate.SourceRevision) || !digestPattern.MatchString(candidate.CompilerSHA256) || len(candidate.Toolchains) == 0 {
			return fmt.Errorf("incomplete or duplicate candidate %s", key)
		}
		for name, version := range candidate.Toolchains {
			if name == "" || version == "" {
				return fmt.Errorf("empty toolchain identity")
			}
		}
		candidates[key] = true
	}
	seen, unqualified := map[string]bool{}, 0
	for _, cell := range result.Cells {
		key := cellKey(cell)
		if !expected[key] || seen[key] {
			return fmt.Errorf("unexpected or duplicate result %s", key)
		}
		seen[key] = true
		row := rows[cell.Row]
		deferred, ids := deferredObligations(row, cell.Implementation, cell.Output)
		if !slices.Equal(cell.Deferrals, ids) {
			return fmt.Errorf("deferrals differ from the reviewed manifest for %s", key)
		}
		switch cell.Status {
		case "unqualified", "blocked", "contradictory", "failed":
			if strings.TrimSpace(cell.Reason) == "" {
				return fmt.Errorf("missing non-passing reason for %s", key)
			}
			unqualified++
		case "passed", "qualified-with-deferrals":
			if (cell.Status == "qualified-with-deferrals") != (len(ids) > 0) {
				return fmt.Errorf("deferred obligations cannot count as passed for %s", key)
			}
			if !row.QualificationReady {
				return fmt.Errorf("%s has no reviewed exact-candidate qualification plan", key)
			}
			if !candidates[candidateKey(cell.Implementation, cell.Host)] {
				return fmt.Errorf("missing candidate identity for %s", key)
			}
			observed := map[string]bool{}
			for _, observation := range cell.Observations {
				if deferred[observation.Obligation] || observed[observation.Obligation] || !contains(row.Obligations, observation.Obligation) || !contains(row.Commands, observation.CommandID) || len(observation.ExecutedArgv) == 0 {
					return fmt.Errorf("unknown or duplicate observation for %s", key)
				}
				observed[observation.Obligation] = true
				if observation.Cases <= 0 || observation.Skipped != 0 || observation.Failures != 0 || observation.ExitCode != 0 {
					return fmt.Errorf("empty, skipped or unsuccessful observation for %s", key)
				}
				actual, err := evidenceHash(directory, observation.Evidence)
				if err != nil {
					return fmt.Errorf("%s: %w", key, err)
				}
				if !digestPattern.MatchString(observation.SHA256) || actual != observation.SHA256 {
					return fmt.Errorf("evidence digest mismatch for %s", key)
				}
			}
			for _, obligation := range row.Obligations {
				if !deferred[obligation] && !observed[obligation] {
					return fmt.Errorf("missing %s observation for %s", obligation, key)
				}
			}
		default:
			return fmt.Errorf("unknown result status for %s", key)
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("incomplete results: %d of %d required cells", len(seen), len(expected))
	}
	if qualify && unqualified != 0 {
		return fmt.Errorf("release not qualified: %d of %d cells do not pass", unqualified, len(expected))
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
