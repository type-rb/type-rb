package runtimeoperation

import "testing"

func TestDescribeSeparatesSharedEffectsFromPackageSemantics(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		want      Descriptor
	}{
		{
			name:      "Jobs native enqueue",
			operation: "trb.jobs.sql.enqueue",
			want:      Descriptor{MaySuspend: true, PropagatesExecutionScope: true},
		},
		{
			name:      "ORM terminal",
			operation: "trb.orm.query.count",
			want:      Descriptor{MaySuspend: true, PropagatesExecutionScope: true},
		},
		{
			name:      "ORM transaction lifecycle",
			operation: "trb.orm.transaction",
			want:      Descriptor{MaySuspend: true, PropagatesExecutionScope: true},
		},
		{name: "ORM query construction", operation: "trb.orm.query.where"},
		{name: "ORM inspection", operation: "trb.orm.query.to_sql"},
		{
			name:      "asynchronous operation without cancellation scope",
			operation: "trb.internal.auth.oidc.verify_bearer",
			want:      Descriptor{MaySuspend: true},
		},
		{
			name:      "synchronous integration forwarding cancellation scope",
			operation: "trb.cli.run",
			want:      Descriptor{PropagatesExecutionScope: true},
		},
		{name: "unknown operation", operation: "example.runtime.read"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Describe(test.operation); got != test.want {
				t.Fatalf("Describe(%q) = %#v, want %#v", test.operation, got, test.want)
			}
		})
	}
}

func TestORMExecutionExcludesQueryConstruction(t *testing.T) {
	if !ORMExecution("trb.orm.query.count") || !ORMExecution("trb.orm.transaction") {
		t.Fatal("database execution operations were not recognized")
	}
	if ORMExecution("trb.orm.query.where") || ORMExecution("trb.orm.query.to_sql") {
		t.Fatal("pure ORM operations were classified as database execution")
	}
}
