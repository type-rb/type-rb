package web

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/type-rb/type-rb/internal/packageextension"
)

func TestEndpointCatalogIsVersionedAndSerializable(t *testing.T) {
	catalog := EndpointCatalog{
		ProtocolVersion: EndpointCatalogProtocolVersion,
		Package:         PackageName,
		Endpoints: []EndpointContract{{
			Name: "CreateReportEndpoint", ModulePath: "routes/reports", Handler: "post", Method: "POST", Path: "/reports",
			Input: &packageextension.ProjectTypeUse{Authored: packageextension.Type{Kind: "named", Name: "CreateReportInput"}, Resolved: packageextension.Type{Kind: "named", Name: "CreateReportInput"}},
			Responses: []EndpointResponse{{
				Status: 202,
				Type:   packageextension.ProjectTypeUse{Authored: packageextension.Type{Kind: "named", Name: "CreateReportResponse"}, Resolved: packageextension.Type{Kind: "named", Name: "CreateReportResponse"}},
			}},
		}},
	}
	if err := ValidateEndpointCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	var decoded EndpointCatalog
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEndpointCatalog(decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, catalog) {
		t.Fatalf("endpoint catalog changed across JSON round trip:\noriginal: %#v\ndecoded: %#v", catalog, decoded)
	}
}

func TestEndpointCatalogValidationRejectsDuplicateRouteBindings(t *testing.T) {
	response := []EndpointResponse{{
		Status: 200,
		Type:   packageextension.ProjectTypeUse{Authored: packageextension.Type{Kind: "named", Name: "Payload"}, Resolved: packageextension.Type{Kind: "named", Name: "Payload"}},
	}}
	catalog := EndpointCatalog{
		ProtocolVersion: EndpointCatalogProtocolVersion,
		Package:         PackageName,
		Endpoints: []EndpointContract{
			{Name: "First", ModulePath: "routes/index", Handler: "get", Method: "GET", Path: "/", Responses: response},
			{Name: "Second", ModulePath: "routes/index", Handler: "get", Method: "GET", Path: "/", Responses: response},
		},
	}
	if err := ValidateEndpointCatalog(catalog); err == nil {
		t.Fatal("duplicate route binding was accepted")
	}
}
