package api_test

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/ushineko/hayami/internal/api/contract"
)

/*
Spec 113 R26-R28. api/openapi.yaml is the contract. Every response a test
gets is checked against its schema, and every Go contract type against its
schema field by field, so neither can move without the other.
*/

const openAPIPath = "../../api/openapi.yaml"

// openAPI is the document, as JSON values.
var openAPI = sync.OnceValues(func() (map[string]any, error) {
	b, err := os.ReadFile(openAPIPath)
	if err != nil {
		return nil, err
	}
	var doc any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	js, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	return out, json.Unmarshal(js, &out)
})

// compiler holds the document as a schema resource.
var compiler = sync.OnceValues(func() (*jsonschema.Compiler, error) {
	doc, err := openAPI()
	if err != nil {
		return nil, err
	}
	js, _ := json.Marshal(doc)
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(js))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	return c, c.AddResource("file:///openapi.json", v)
})

// schema is one of the document's schemas, by name.
func schema(t testing.TB, name string) *jsonschema.Schema {
	t.Helper()
	c, err := compiler()
	require.NoError(t, err)
	s, err := c.Compile("file:///openapi.json#/components/schemas/" + name)
	require.NoError(t, err, "the contract has no schema %s", name)
	return s
}

// conforms checks body against the named schema.
func conforms(t testing.TB, name string, body []byte) {
	t.Helper()
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	require.NoError(t, err, "not JSON: %s", body)
	if err := schema(t, name).Validate(v); err != nil {
		t.Errorf("the response does not keep the %s contract: %v\n%s", name, err, body)
	}
}

// contractTypes are the Go type of each object schema.
var contractTypes = map[string]any{
	"Index": contract.Index{}, "Health": contract.Health{}, "SectionList": contract.SectionList{},
	"SectionInfo": contract.SectionInfo{}, "Absence": contract.Absence{}, "DetailRef": contract.DetailRef{},
	"Section": contract.Section{}, "Reading": contract.Reading{}, "Readings": contract.Readings{},
	"Error": contract.Error{}, "ErrorBody": contract.ErrorBody{},
	"Clock": contract.Clock{}, "Bandwidth": contract.Bandwidth{}, "Interface": contract.Interface{},
	"RateTrail": contract.RateTrail{}, "Wireless": contract.Wireless{}, "VPN": contract.VPN{},
	"Usage": contract.Usage{}, "Account": contract.Account{}, "UsageWindow": contract.UsageWindow{},
	"Cooler": contract.Cooler{}, "Probe": contract.Probe{}, "Peripherals": contract.Peripherals{}, "Device": contract.Device{},
	"Detail": contract.Detail{}, "ProcessList": contract.ProcessList{}, "ProcessUse": contract.ProcessUse{},
	"ProcessSummary": contract.ProcessSummary{}, "CPUSummary": contract.CPUSummary{}, "GPUSummary": contract.GPUSummary{},
	"MemorySummary": contract.MemorySummary{}, "IOSummary": contract.IOSummary{}, "ProcessPage": contract.ProcessPage{},
	"Process": contract.Process{}, "ProcessRef": contract.ProcessRef{}, "Network": contract.Network{},
	"NetProcess": contract.NetProcess{}, "NetPeer": contract.NetPeer{}, "NetSocket": contract.NetSocket{},
	"Forwarded": contract.Forwarded{}, "InterfaceRate": contract.InterfaceRate{}, "VPNDetail": contract.VPNDetail{},
	"VPNStats": contract.VPNStats{}, "VPNCheck": contract.VPNCheck{}, "VPNSample": contract.VPNSample{},
	"AccountDetail": contract.AccountDetail{}, "UsageLimit": contract.UsageLimit{}, "UsageShare": contract.UsageShare{},
	"UsageFact": contract.UsageFact{}, "Overview": contract.Overview{}, "OverviewAccount": contract.OverviewAccount{},
	"Forge": contract.Forge{}, "ForgeCount": contract.ForgeCount{}, "ForgeItem": contract.ForgeItem{},
	"ForgeDay": contract.ForgeDay{}, "ChangePage": contract.ChangePage{}, "Change": contract.Change{},
	"Commit": contract.Commit{}, "ChangedFile": contract.ChangedFile{},
}

// jsonFields are a struct's JSON field names, an embedded struct's included.
func jsonFields(t reflect.Type) []string {
	var out []string
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous {
			out = append(out, jsonFields(f.Type)...)
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// R28. A Go contract type and its schema have the same fields, every one of
// them required (a value unknown is null, never left out), and no field the
// other lacks.
func TestEveryContractTypeIsItsSchema(t *testing.T) {
	doc, err := openAPI()
	require.NoError(t, err)
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)

	for name, s := range schemas {
		obj := s.(map[string]any)
		if obj["type"] != "object" {
			continue
		}
		v, ok := contractTypes[name]
		if !assert.True(t, ok, "the contract's %s has no Go type", name) {
			continue
		}
		var props, required []string
		for p := range obj["properties"].(map[string]any) {
			props = append(props, p)
		}
		for _, r := range obj["required"].([]any) {
			required = append(required, r.(string))
		}
		sort.Strings(props)
		sort.Strings(required)
		assert.Equal(t, props, jsonFields(reflect.TypeOf(v)), "%s: the schema's fields and the Go type's", name)
		assert.Equal(t, props, required, "%s: every field is required; an unknown value is null", name)
		assert.Equal(t, false, obj["additionalProperties"], "%s: a response has no field the contract lacks", name)
	}
	for name := range contractTypes {
		_, ok := schemas[name]
		assert.True(t, ok, "the Go type %s has no schema", name)
	}
}

// R27. The contract's version is the one the service says.
func TestTheContractsVersionIsTheServices(t *testing.T) {
	doc, err := openAPI()
	require.NoError(t, err)
	assert.Equal(t, contract.Version, doc["info"].(map[string]any)["version"])
	assert.Equal(t, "3.1.0", doc["openapi"])
}

// Every path the server routes is in the contract, and the reverse.
func TestEveryPathIsInTheContract(t *testing.T) {
	doc, err := openAPI()
	require.NoError(t, err)
	var paths []string
	for p := range doc["paths"].(map[string]any) {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	assert.Equal(t, []string{"/", "/events", "/health", "/readings", "/sections", "/sections/{key}", "/sections/{key}/details/{id}"}, paths)
}
