package releasecontract

import "testing"

func TestScopedDeferralsNeverBecomePassesOrResultWaivers(t *testing.T) {
	for _, mutation := range []string{"none", "pass", "invent", "omit", "go-waiver", "missing-required", "observe-deferred", "skip-required", "unreviewed", "empty-id"} {
		t.Run(mutation, func(t *testing.T) {
			m, d, r, dir := passingFixture(t)
			m.Rows[0].Deferrals = []Deferral{{ID: "defer-reject", Implementations: []string{"native"}, Outputs: []string{"native"}, Obligations: []string{"reject"}}}
			c := &r.Cells[2]
			if c.Implementation != "native" || c.Output != "native" {
				t.Fatal("fixture changed")
			}
			c.Status = "qualified-with-deferrals"
			c.Deferrals = []string{"defer-reject"}
			rejection := c.Observations[1]
			c.Observations = c.Observations[:1]
			switch mutation {
			case "pass":
				c.Status = "passed"
			case "invent":
				c.Deferrals = []string{"invented"}
			case "omit":
				c.Deferrals = nil
			case "go-waiver":
				r.Cells[0].Status = "qualified-with-deferrals"
				r.Cells[0].Deferrals = c.Deferrals
			case "missing-required":
				c.Observations = nil
			case "observe-deferred":
				c.Observations = append(c.Observations, rejection)
			case "skip-required":
				c.Observations[0].Skipped = 1
			case "empty-id":
				r.Cells[0].Deferrals = []string{""}
			case "unreviewed":
				m.Rows[0].QualificationReady = false
			}
			err := ValidateResults(m, d, r, dir, true)
			if mutation == "none" && err != nil {
				t.Fatal(err)
			}
			if mutation != "none" && err == nil {
				t.Fatal("unreviewed or false-green exemption accepted")
			}
		})
	}
}

func TestManifestDeferralsRejectUnboundedOrStaleScopes(t *testing.T) {
	for _, mutation := range []string{"release", "owner", "decision", "trigger", "scope", "unknown-obligation", "implementation", "output", "overlap", "duplicate"} {
		t.Run(mutation, func(t *testing.T) {
			m, c, _, err := Load("../..")
			if err != nil {
				t.Fatal(err)
			}
			var row *Row
			for i := range m.Rows {
				if m.Rows[i].ID == "cli.test" {
					row = &m.Rows[i]
				}
			}
			d := &row.Deferrals[0]
			switch mutation {
			case "release":
				d.Release = "0.6.0"
			case "owner":
				d.Owner = ""
			case "decision":
				d.Decision = ""
			case "trigger":
				d.ReviewTrigger = ""
			case "scope":
				d.Scope = ""
			case "unknown-obligation":
				d.Obligations = []string{"everything"}
			case "implementation":
				d.Implementations = []string{"other"}
			case "output":
				d.Outputs = []string{"native"}
			case "overlap":
				other := *d
				other.ID = "overlap"
				row.Deferrals = append(row.Deferrals, other)
			case "duplicate":
				row.Deferrals = append(row.Deferrals, *d)
			}
			if ValidateManifest(m, c) == nil {
				t.Fatal("unbounded deferral accepted")
			}
		})
	}
}
