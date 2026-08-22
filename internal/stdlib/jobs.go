package stdlib

import "github.com/type-rb/type-rb/internal/types"

func jobsSQLIntrinsicSymbols() map[string]Symbol {
	result := types.Type{
		Kind: types.Named,
		Name: "Result",
		Args: []types.Type{types.FromName("JobReference"), types.FromName("EnqueueError")},
	}
	adapter := Parameter{Name: "adapter", Type: types.FromName("SQLAdapter")}
	request := Parameter{Name: "request", Type: types.FromName("EnqueueRequest")}
	return map[string]Symbol{
		"enqueue": {
			Name:               "enqueue",
			Intrinsic:          "trb.jobs.sql.enqueue",
			RuntimeIndependent: true,
			Parameters:         []Parameter{adapter, request},
			Return:             result,
		},
		"enqueue_at": {
			Name:               "enqueue_at",
			Intrinsic:          "trb.jobs.sql.enqueue_at",
			RuntimeIndependent: true,
			Parameters: []Parameter{
				adapter,
				request,
				{Name: "scheduled_at", Type: types.FromName("Instant")},
			},
			Return: result,
		},
	}
}
