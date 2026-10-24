package releasecontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"regexp"
	"strings"
)

const ManifestPath = "contracts/v0.5/manifest.json"
const CatalogPath = "contracts/v0.5/surfaces.json"
const ViewPath = "docs/v0.5-feature-contract.md"

type Manifest struct {
	SchemaVersion    int       `json:"schemaVersion"`
	Release          string    `json:"release"`
	SourceRepository string    `json:"sourceRepository"`
	SourceRevision   string    `json:"sourceRevision"`
	CatalogSHA256    string    `json:"catalogSHA256"`
	Hosts            []string  `json:"hosts"`
	Commands         []Command `json:"commands"`
	Rows             []Row     `json:"rows"`
}

// Commands identify existing owning checks. QualificationReady stays false
// until an exact-candidate, per-output scenario plan is reviewed. Running a
// source implementation's unit tests is not proof about a different compiler.
type Command struct {
	ID         string   `json:"id"`
	Repository string   `json:"repository"`
	Argv       []string `json:"argv"`
	Purpose    string   `json:"purpose"`
}

type Row struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Contract           string   `json:"contract"`
	Failure            string   `json:"failure"`
	Surfaces           []string `json:"surfaces"`
	Implementations    []string `json:"implementations"`
	Outputs            []string `json:"outputs"`
	NativeExecutable   bool     `json:"nativeExecutable"`
	Obligations        []string `json:"obligations"`
	Commands           []string `json:"commands"`
	Baseline           string   `json:"baseline"`
	Remaining          string   `json:"remaining"`
	Owner              string   `json:"owner"`
	Related            []string `json:"related,omitempty"`
	QualificationReady bool     `json:"qualificationReady"`
}

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*$`)

func decode(data []byte, destination any) error {
	// Required fields cannot silently become Go zero values (for example a
	// missing exitCode or skipped count must not become a successful zero).
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := requiredFields(raw, reflect.TypeOf(destination).Elem(), "$"); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing JSON value: %v", err)
	}
	return nil
}

func requiredFields(value any, typ reflect.Type, path string) error {
	if typ.Kind() == reflect.Pointer {
		return requiredFields(value, typ.Elem(), path)
	}
	switch typ.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		for index := 0; index < typ.NumField(); index++ {
			field := typ.Field(index)
			tag := strings.Split(field.Tag.Get("json"), ",")
			if tag[0] == "" || tag[0] == "-" {
				continue
			}
			item, present := object[tag[0]]
			if !present {
				if len(tag) > 1 && tag[1] == "omitempty" {
					continue
				}
				return fmt.Errorf("missing required %s.%s", path, tag[0])
			}
			if err := requiredFields(item, field.Type, path+"."+tag[0]); err != nil {
				return err
			}
		}
	case reflect.Slice:
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		for index, item := range array {
			if err := requiredFields(item, typ.Elem(), fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case reflect.Map:
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("%s must be an object", path)
		}
	default:
		if value == nil {
			return fmt.Errorf("%s cannot be null", path)
		}
	}
	return nil
}

func Load(root string) (Manifest, []Surface, string, error) {
	var manifest Manifest
	var catalog []Surface
	data, err := read(root, ManifestPath)
	if err != nil {
		return manifest, nil, "", err
	}
	if err := decode(data, &manifest); err != nil {
		return manifest, nil, "", err
	}
	catalogData, err := read(root, CatalogPath)
	if err != nil {
		return manifest, nil, "", err
	}
	if err := decode(catalogData, &catalog); err != nil {
		return manifest, nil, "", err
	}
	if hashBytes(catalogData) != manifest.CatalogSHA256 {
		return manifest, nil, "", fmt.Errorf("catalog digest differs from reviewed manifest")
	}
	if err := ValidateManifest(manifest, catalog); err != nil {
		return manifest, nil, "", err
	}
	return manifest, catalog, hashBytes(data), nil
}

func publicIssue(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Host == "github.com" &&
		(strings.HasPrefix(u.Path, "/type-rb/type-rb-native/issues/") || strings.HasPrefix(u.Path, "/type-rb/type-rb/issues/"))
}

func unique(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return len(values) > 0
}

func ValidateManifest(manifest Manifest, catalog []Surface) error {
	if manifest.SchemaVersion != 1 || manifest.Release != "0.5.0" || manifest.SourceRepository != "type-rb/type-rb" || !revisionPattern.MatchString(manifest.SourceRevision) || !digestPattern.MatchString(manifest.CatalogSHA256) {
		return fmt.Errorf("invalid release/source/catalog identity")
	}
	if strings.Join(manifest.Hosts, ",") != "darwin-amd64,darwin-arm64,linux-amd64,linux-arm64" {
		return fmt.Errorf("the existing four-host distribution contract must be retained")
	}
	known := map[string]bool{}
	for _, surface := range catalog {
		if surface.ID == "" || surface.Source == "" || known[surface.ID] {
			return fmt.Errorf("invalid or duplicate catalog surface %s", surface.ID)
		}
		known[surface.ID] = true
	}
	if len(known) == 0 {
		return fmt.Errorf("empty release catalog")
	}
	commands := map[string]bool{}
	for _, command := range manifest.Commands {
		if !identifierPattern.MatchString(command.ID) || commands[command.ID] || len(command.Argv) == 0 || command.Purpose == "" || (command.Repository != "type-rb/type-rb" && command.Repository != "type-rb/type-rb-native") {
			return fmt.Errorf("invalid or duplicate owning command %s", command.ID)
		}
		commands[command.ID] = true
	}
	rows, owned := map[string]bool{}, map[string]string{}
	for _, row := range manifest.Rows {
		if !identifierPattern.MatchString(row.ID) || rows[row.ID] || row.Title == "" || row.Contract == "" || row.Failure == "" || row.Remaining == "" || !publicIssue(row.Owner) || !unique(row.Implementations) || !unique(row.Outputs) || !unique(row.Obligations) || !unique(row.Commands) {
			return fmt.Errorf("incomplete or duplicate release row %s", row.ID)
		}
		rows[row.ID] = true
		if row.ID == "native.qualification" {
			if strings.Join(row.Implementations, ",") != "native" {
				return fmt.Errorf("Native qualification must remain Native-only")
			}
		} else if strings.Join(row.Implementations, ",") != "go,native" {
			return fmt.Errorf("shared row %s must retain both implementations", row.ID)
		}
		for _, implementation := range row.Implementations {
			if implementation != "go" && implementation != "native" {
				return fmt.Errorf("unknown implementation %s", implementation)
			}
		}
		if row.Baseline != "unqualified" && row.Baseline != "contradictory" && row.Baseline != "blocked" {
			return fmt.Errorf("invalid baseline for %s", row.ID)
		}
		for _, output := range row.Outputs {
			if output != "go" && output != "ruby" && output != "typescript" && output != "service" {
				return fmt.Errorf("unknown shared output %s", output)
			}
		}
		for _, id := range row.Commands {
			if !commands[id] {
				return fmt.Errorf("unknown command %s in %s", id, row.ID)
			}
		}
		for _, id := range row.Surfaces {
			if !known[id] || owned[id] != "" {
				return fmt.Errorf("unknown or multiply owned surface %s in %s (previous %s)", id, row.ID, owned[id])
			}
			owned[id] = row.ID
		}
	}
	for id := range known {
		if owned[id] == "" {
			return fmt.Errorf("unowned public surface %s", id)
		}
	}
	return nil
}

// CheckDiscovery rejects additions, removals and declaration/mode drift. It
// cannot infer semantic changes inside bodies; those require contract review.
func CheckDiscovery(root string, expected []Surface) error {
	actual, err := Discover(root)
	if err != nil {
		return err
	}
	left, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	right, err := json.Marshal(actual)
	if err != nil {
		return err
	}
	if !bytes.Equal(left, right) {
		return fmt.Errorf("public surface drift: compare --discover with %s and review the contract; do not blindly refresh", CatalogPath)
	}
	return nil
}

func Render(manifest Manifest, digest string) []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "# v0.5.0 shared feature contract\n\nGenerated by `go run ./tools/release-contract --write-view`. Edit the reviewed\n[manifest](../%s), not this view. See the [maintenance and result contract](../contracts/v0.5/README.md).\n\nSource: [`%s`](https://github.com/type-rb/type-rb/tree/%s).\nManifest SHA-256: `%s`.\n\nThese are release obligations, **not a support percentage**. Every baseline is\nexplicitly unqualified, blocked or contradictory. Existing tests and Native\nfeature batches are evidence inputs; they do not qualify all release cells.\n\nBoth implementations require each listed shared output. `service` means a\nCLI/protocol/distribution contract independent of emitted program language.\nThe Native compiler additionally requires the executable path where indicated.\nAll four retained host targets apply: %s.\n\n", ManifestPath, manifest.SourceRevision, manifest.SourceRevision, digest, strings.Join(manifest.Hosts, ", "))
	for _, row := range manifest.Rows {
		fmt.Fprintf(&out, "## %s — %s\n\n%s\n\nFailure boundary: %s\n\n- Implementations: %s.\n- Shared outputs: %s; Native executable: %t.\n- Obligations: %s.\n- Baseline: **%s**; qualification plan ready: **%t**.\n- Owning commands: %s.\n- Remaining: %s\n- Next action owner: %s\n", row.ID, row.Title, row.Contract, row.Failure, strings.Join(row.Implementations, ", "), strings.Join(row.Outputs, ", "), row.NativeExecutable, strings.Join(row.Obligations, ", "), row.Baseline, row.QualificationReady, strings.Join(row.Commands, ", "), row.Remaining, row.Owner)
		if len(row.Related) > 0 {
			fmt.Fprintf(&out, "- Existing work/decisions: %s\n", strings.Join(row.Related, ", "))
		}
		fmt.Fprintf(&out, "- Explicitly owned surfaces: %d (full names and members in the manifest/catalog).\n\n", len(row.Surfaces))
	}
	return []byte(strings.TrimSpace(out.String()) + "\n")
}
