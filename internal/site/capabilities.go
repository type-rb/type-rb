package site

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const capabilityPageSource = "capabilities.md"

var capabilityStatusOrder = []string{"available", "partial", "planned", "exploring"}

var capabilityStatusCatalog = map[string]capabilityStatusView{
	"available": {
		ID: "available", Label: "Available", Icon: "✓",
		Help: "The stated 1.0 capability is documented and usable now.",
	},
	"partial": {
		ID: "partial", Label: "Partial", Icon: "◐",
		Help: "A meaningful implementation exists, but the stated 1.0 scope is incomplete.",
	},
	"planned": {
		ID: "planned", Label: "Planned", Icon: "○",
		Help: "The capability belongs in the proposed 1.0 set but is not sufficiently implemented yet.",
	},
	"exploring": {
		ID: "exploring", Label: "Exploring", Icon: "?",
		Help: "The need is visible, but the 1.0 shape still requires design or inventory.",
	},
}

type capabilityCatalog struct {
	SchemaVersion int               `json:"schemaVersion"`
	UpdatedAt     string            `json:"updatedAt"`
	Target        string            `json:"target"`
	Scopes        []capabilityScope `json:"scopes"`
	Areas         []capabilityArea  `json:"areas"`
}

type capabilityScope struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Eyebrow     string `json:"eyebrow"`
	Description string `json:"description"`
}

type capabilityArea struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Items       []capabilityItem `json:"items"`
}

type capabilityItem struct {
	Title       string              `json:"title"`
	Status      string              `json:"status"`
	Scopes      []string            `json:"scopes"`
	Description string              `json:"description"`
	Evidence    *capabilityEvidence `json:"evidence"`
}

type capabilityEvidence struct {
	Label  string `json:"label"`
	Source string `json:"source"`
}

type capabilityCatalogView struct {
	Target    string
	UpdatedAt string
	Total     int
	Available int
	Partial   int
	Remaining int
	AreaCount int
	Statuses  []capabilityStatusView
	Scopes    []capabilityScopeView
	Areas     []capabilityAreaView
}

type capabilityStatusView struct {
	ID    string
	Label string
	Icon  string
	Help  string
}

type capabilityScopeView struct {
	ID             string
	Label          string
	Eyebrow        string
	Description    string
	Total          int
	Available      int
	Partial        int
	AvailableWidth template.CSS
	PartialWidth   template.CSS
}

type capabilityAreaView struct {
	ID          string
	Title       string
	Description string
	Total       int
	Available   int
	Icon        string
	Items       []capabilityItemView
}

type capabilityItemView struct {
	Title         string
	Status        string
	StatusLabel   string
	StatusIcon    string
	StatusHelp    string
	Scopes        string
	ScopeLabels   []string
	Description   string
	EvidenceLabel string
	EvidenceURL   string
	Search        string
}

func renderCapabilityCatalog(docsDir string, pageURLs map[string]string) (template.HTML, error) {
	catalog, err := loadCapabilityCatalog(docsDir, pageURLs)
	if err != nil {
		return "", err
	}
	view := capabilityView(catalog, pageURLs)
	parsed, err := template.ParseFS(assets, "assets/capabilities.html")
	if err != nil {
		return "", err
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, view); err != nil {
		return "", err
	}
	return template.HTML(output.String()), nil // All text came from validated, repository-owned JSON and html/template escaped it.
}

func loadCapabilityCatalog(docsDir string, pageURLs map[string]string) (capabilityCatalog, error) {
	data, err := os.ReadFile(filepath.Join(docsDir, "capabilities.json"))
	if err != nil {
		return capabilityCatalog{}, fmt.Errorf("read capability catalog: %w", err)
	}
	var catalog capabilityCatalog
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return capabilityCatalog{}, fmt.Errorf("parse capability catalog: %w", err)
	}
	if err := validateCapabilityCatalog(catalog, pageURLs); err != nil {
		return capabilityCatalog{}, fmt.Errorf("validate capability catalog: %w", err)
	}
	return catalog, nil
}

func validateCapabilityCatalog(catalog capabilityCatalog, pageURLs map[string]string) error {
	if catalog.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schemaVersion %d", catalog.SchemaVersion)
	}
	if strings.TrimSpace(catalog.Target) == "" {
		return errors.New("target is required")
	}
	if _, err := time.Parse("2006-01-02", catalog.UpdatedAt); err != nil {
		return errors.New("updatedAt must be an ISO date")
	}
	if len(catalog.Scopes) == 0 || len(catalog.Areas) == 0 {
		return errors.New("at least one scope and area are required")
	}

	scopeLabels := map[string]string{}
	for _, scope := range catalog.Scopes {
		if strings.TrimSpace(scope.ID) == "" || strings.TrimSpace(scope.Label) == "" || strings.TrimSpace(scope.Eyebrow) == "" || strings.TrimSpace(scope.Description) == "" {
			return errors.New("every scope needs an id, label, eyebrow, and description")
		}
		if _, exists := scopeLabels[scope.ID]; exists {
			return fmt.Errorf("duplicate scope %q", scope.ID)
		}
		scopeLabels[scope.ID] = scope.Label
	}

	areaIDs := map[string]bool{}
	itemTitles := map[string]bool{}
	statusCounts := map[string]int{}
	for _, area := range catalog.Areas {
		if strings.TrimSpace(area.ID) == "" || strings.TrimSpace(area.Title) == "" || strings.TrimSpace(area.Description) == "" || len(area.Items) == 0 {
			return errors.New("every area needs an id, title, description, and items")
		}
		if areaIDs[area.ID] {
			return fmt.Errorf("duplicate area %q", area.ID)
		}
		areaIDs[area.ID] = true
		for _, item := range area.Items {
			if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Description) == "" {
				return fmt.Errorf("area %q contains an item without a title or description", area.ID)
			}
			if itemTitles[item.Title] {
				return fmt.Errorf("duplicate capability title %q", item.Title)
			}
			itemTitles[item.Title] = true
			if _, exists := capabilityStatusCatalog[item.Status]; !exists {
				return fmt.Errorf("capability %q has unknown status %q", item.Title, item.Status)
			}
			statusCounts[item.Status]++
			if len(item.Scopes) == 0 {
				return fmt.Errorf("capability %q needs at least one scope", item.Title)
			}
			seenScopes := map[string]bool{}
			for _, scope := range item.Scopes {
				if _, exists := scopeLabels[scope]; !exists {
					return fmt.Errorf("capability %q references unknown scope %q", item.Title, scope)
				}
				if seenScopes[scope] {
					return fmt.Errorf("capability %q repeats scope %q", item.Title, scope)
				}
				seenScopes[scope] = true
			}
			if item.Evidence == nil || strings.TrimSpace(item.Evidence.Label) == "" || strings.TrimSpace(item.Evidence.Source) == "" {
				return fmt.Errorf("capability %q needs public evidence or planning context", item.Title)
			}
			if _, exists := pageURLs[item.Evidence.Source]; !exists {
				return fmt.Errorf("capability %q references unpublished documentation %q", item.Title, item.Evidence.Source)
			}
		}
	}
	for _, status := range capabilityStatusOrder {
		if statusCounts[status] == 0 {
			return fmt.Errorf("catalog needs at least one %s capability", status)
		}
	}
	return nil
}

func capabilityView(catalog capabilityCatalog, pageURLs map[string]string) capabilityCatalogView {
	statusCounts := map[string]int{}
	scopeLabels := map[string]string{}
	for _, scope := range catalog.Scopes {
		scopeLabels[scope.ID] = scope.Label
	}

	areas := make([]capabilityAreaView, 0, len(catalog.Areas))
	for _, area := range catalog.Areas {
		current := capabilityAreaView{ID: area.ID, Title: area.Title, Description: area.Description, Total: len(area.Items)}
		for _, item := range area.Items {
			status := capabilityStatusCatalog[item.Status]
			statusCounts[item.Status]++
			if item.Status == "available" {
				current.Available++
			}
			labels := make([]string, 0, len(item.Scopes))
			for _, scope := range item.Scopes {
				labels = append(labels, scopeLabels[scope])
			}
			current.Items = append(current.Items, capabilityItemView{
				Title: item.Title, Status: item.Status, StatusLabel: status.Label, StatusIcon: status.Icon, StatusHelp: status.Help,
				Scopes: strings.Join(item.Scopes, " "), ScopeLabels: labels, Description: item.Description,
				EvidenceLabel: item.Evidence.Label, EvidenceURL: pageURLs[item.Evidence.Source],
				Search: strings.ToLower(strings.Join([]string{area.Title, area.Description, item.Title, item.Description}, " ")),
			})
		}
		if current.Available == current.Total {
			current.Icon = "✓"
		} else {
			current.Icon = fmt.Sprintf("%d", current.Available)
		}
		areas = append(areas, current)
	}

	scopes := make([]capabilityScopeView, 0, len(catalog.Scopes))
	for _, scope := range catalog.Scopes {
		current := capabilityScopeView{ID: scope.ID, Label: scope.Label, Eyebrow: scope.Eyebrow, Description: scope.Description}
		for _, area := range catalog.Areas {
			for _, item := range area.Items {
				if !containsString(item.Scopes, scope.ID) {
					continue
				}
				current.Total++
				if item.Status == "available" {
					current.Available++
				} else if item.Status == "partial" {
					current.Partial++
				}
			}
		}
		current.AvailableWidth = percentageWidth(current.Available, current.Total)
		current.PartialWidth = percentageWidth(current.Partial, current.Total)
		scopes = append(scopes, current)
	}

	statuses := make([]capabilityStatusView, 0, len(capabilityStatusOrder))
	for _, status := range capabilityStatusOrder {
		statuses = append(statuses, capabilityStatusCatalog[status])
	}
	total := 0
	for _, count := range statusCounts {
		total += count
	}
	return capabilityCatalogView{
		Target: catalog.Target, UpdatedAt: catalog.UpdatedAt, Total: total,
		Available: statusCounts["available"], Partial: statusCounts["partial"],
		Remaining: statusCounts["planned"] + statusCounts["exploring"], AreaCount: len(catalog.Areas),
		Statuses: statuses, Scopes: scopes, Areas: areas,
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func percentageWidth(value, total int) template.CSS {
	if total == 0 {
		return "0%"
	}
	return template.CSS(fmt.Sprintf("%.3f%%", float64(value)*100/float64(total)))
}
