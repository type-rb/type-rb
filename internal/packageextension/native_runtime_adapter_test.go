package packageextension

import (
	"strings"
	"testing"
)

func TestValidateNativeRuntimeAdapterCatalogAcceptsMinimalFunctionBinding(t *testing.T) {
	catalog := NativeRuntimeAdapterCatalog{
		ProtocolVersion: NativeRuntimeAdapterProtocolVersion,
		Bindings: map[string]NativeRuntimeAdapterBinding{
			"github.com/acme/aws-s3/native#head_object": {
				Dependency: "github.com/acme/aws-s3-wire", Module: "github.com/acme/aws-s3-wire",
				Symbol: "HeadObject", CallConvention: "function", MaySuspend: true, PropagatesExecutionScope: true,
			},
		},
	}
	if err := ValidateNativeRuntimeAdapterCatalog(catalog); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNativeRuntimeAdapterCatalogRejectsIncompleteOrExpandedBindings(t *testing.T) {
	valid := NativeRuntimeAdapterBinding{Dependency: "wire", Module: "wire", Symbol: "invoke", CallConvention: "function"}
	tests := []struct {
		name    string
		binding NativeRuntimeAdapterBinding
		want    string
	}{
		{name: "dependency", binding: NativeRuntimeAdapterBinding{Module: "wire", Symbol: "invoke", CallConvention: "function"}, want: "has no dependency"},
		{name: "module", binding: NativeRuntimeAdapterBinding{Dependency: "wire", Symbol: "invoke", CallConvention: "function"}, want: "has no module"},
		{name: "symbol", binding: NativeRuntimeAdapterBinding{Dependency: "wire", Module: "wire", CallConvention: "function"}, want: "has no symbol"},
		{name: "convention", binding: func() NativeRuntimeAdapterBinding { value := valid; value.CallConvention = "method"; return value }(), want: "unsupported callConvention"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateNativeRuntimeAdapterCatalog(NativeRuntimeAdapterCatalog{
				ProtocolVersion: NativeRuntimeAdapterProtocolVersion,
				Bindings:        map[string]NativeRuntimeAdapterBinding{"provider/native#invoke": test.binding},
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestValidateNativeRuntimeAdapterCatalogRejectsEmptyBindings(t *testing.T) {
	err := ValidateNativeRuntimeAdapterCatalog(NativeRuntimeAdapterCatalog{
		ProtocolVersion: NativeRuntimeAdapterProtocolVersion,
		Bindings:        map[string]NativeRuntimeAdapterBinding{},
	})
	if err == nil || !strings.Contains(err.Error(), "at least one binding") {
		t.Fatalf("expected empty binding diagnostic, got %v", err)
	}
}
