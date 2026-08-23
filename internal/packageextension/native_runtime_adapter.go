package packageextension

import (
	"fmt"
	"strings"
)

const NativeRuntimeAdapterProtocolVersion = 1

// NativeRuntimeAdapterCatalog maps stable semantic declaration identities to
// target-native runtime functions. It deliberately contains no TypeRB
// signatures; those remain owned by the declaration adapter catalog.
type NativeRuntimeAdapterCatalog struct {
	ProtocolVersion int                                    `json:"protocolVersion"`
	Bindings        map[string]NativeRuntimeAdapterBinding `json:"bindings"`
}

// NativeRuntimeAdapterBinding is the target-specific half of one runtime
// declaration. The initial call convention supports only a top-level native
// function. When execution scope propagation is enabled, the generated caller
// passes the target's hidden scope as the first argument.
type NativeRuntimeAdapterBinding struct {
	Dependency               string `json:"dependency"`
	Module                   string `json:"module"`
	Symbol                   string `json:"symbol"`
	CallConvention           string `json:"callConvention"`
	MaySuspend               bool   `json:"maySuspend,omitempty"`
	PropagatesExecutionScope bool   `json:"propagatesExecutionScope,omitempty"`
}

func ValidateNativeRuntimeAdapterCatalog(catalog NativeRuntimeAdapterCatalog) error {
	if catalog.ProtocolVersion != NativeRuntimeAdapterProtocolVersion {
		return fmt.Errorf("unsupported native runtime adapter protocol version %d", catalog.ProtocolVersion)
	}
	if catalog.Bindings == nil {
		return fmt.Errorf("native runtime adapter bindings are required")
	}
	if len(catalog.Bindings) == 0 {
		return fmt.Errorf("native runtime adapter must contain at least one binding")
	}
	for identity, binding := range catalog.Bindings {
		if strings.TrimSpace(identity) == "" {
			return fmt.Errorf("native runtime adapter contains an empty binding identity")
		}
		if strings.TrimSpace(binding.Dependency) == "" {
			return fmt.Errorf("native runtime adapter binding %s has no dependency", identity)
		}
		if strings.TrimSpace(binding.Module) == "" {
			return fmt.Errorf("native runtime adapter binding %s has no module", identity)
		}
		if strings.TrimSpace(binding.Symbol) == "" {
			return fmt.Errorf("native runtime adapter binding %s has no symbol", identity)
		}
		if binding.CallConvention != "function" {
			return fmt.Errorf("native runtime adapter binding %s has unsupported callConvention %q", identity, binding.CallConvention)
		}
	}
	return nil
}
