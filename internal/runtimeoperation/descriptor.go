// Package runtimeoperation describes compiler-known execution behavior for
// opaque runtime operations. It deliberately does not describe target-native
// lowering, typed signatures, or Result error conversion: those remain owned
// by package declarations and their backend integrations.
package runtimeoperation

// Descriptor contains only the call-graph effects shared by otherwise
// package-specific runtime operations.
type Descriptor struct {
	MaySuspend               bool
	PropagatesExecutionScope bool
}

// Describe returns the compiler-known effects of an opaque runtime operation.
// Unknown and purely representational operations have no effects.
func Describe(name string) Descriptor {
	if ormExecutionOperation(name) {
		return Descriptor{MaySuspend: true, PropagatesExecutionScope: true}
	}

	switch name {
	case "trb.cli.run":
		return Descriptor{PropagatesExecutionScope: true}
	case "trb.jobs.perform_later", "trb.jobs.perform_in", "trb.jobs.perform_at",
		"trb.jobs.sql.enqueue", "trb.jobs.sql.enqueue_at",
		"trb.web.testing.dispatch",
		"trb.web.middleware.logger.call", "trb.web.middleware.timeout.call",
		"trb.platform.typescript.browser.request",
		"trb.platform.typescript.browser.file_read", "trb.platform.typescript.browser.file_read_text":
		return Descriptor{MaySuspend: true, PropagatesExecutionScope: true}
	case "trb.std.test.describe", "trb.std.test.test",
		"trb.internal.auth.oidc.verify_bearer":
		return Descriptor{MaySuspend: true}
	default:
		return Descriptor{}
	}
}

// ORMExecution reports whether an ORM operation may execute database work.
// Query construction and inspection remain ordinary synchronous operations.
func ORMExecution(name string) bool {
	return ormExecutionOperation(name)
}

func ormExecutionOperation(name string) bool {
	switch name {
	case "trb.orm.transaction",
		"trb.orm.all", "trb.orm.first", "trb.orm.count", "trb.orm.explain", "trb.orm.find_by", "trb.orm.exists",
		"trb.orm.pluck", "trb.orm.pick", "trb.orm.ids", "trb.orm.sum", "trb.orm.average", "trb.orm.minimum", "trb.orm.maximum",
		"trb.orm.find", "trb.orm.create", "trb.orm.scope.find", "trb.orm.scope.create", "trb.orm.draft.save",
		"trb.orm.insert_all", "trb.orm.insert_if_absent", "trb.orm.draft.upsert", "trb.orm.upsert_all", "trb.orm.update",
		"trb.orm.changes.save", "trb.orm.delete", "trb.orm.destroy", "trb.orm.destroy_all", "trb.orm.update_all", "trb.orm.delete_all",
		"trb.orm.query.find_by", "trb.orm.query.exists", "trb.orm.query.update_all", "trb.orm.query.delete_all", "trb.orm.query.destroy_all",
		"trb.orm.query.pluck", "trb.orm.query.pick", "trb.orm.query.ids", "trb.orm.query.sum", "trb.orm.query.average", "trb.orm.query.minimum", "trb.orm.query.maximum",
		"trb.orm.query.all", "trb.orm.query.first", "trb.orm.query.count", "trb.orm.query.explain", "trb.orm.query.find_each", "trb.orm.query.find_in_batches",
		"trb.orm.group.count", "trb.orm.group.sum", "trb.orm.group.average", "trb.orm.group.minimum", "trb.orm.group.maximum",
		"trb.orm.association.value.belongs_to", "trb.orm.association.value.has_many", "trb.orm.association.value.has_one",
		"trb.orm.association.load.belongs_to", "trb.orm.association.load.has_many", "trb.orm.association.load.has_one",
		"trb.orm.association.reload.belongs_to", "trb.orm.association.reload.has_many", "trb.orm.association.reload.has_one":
		return true
	default:
		return false
	}
}
