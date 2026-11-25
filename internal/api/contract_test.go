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

/*
propertyRule is what a property's schema says of its values that a client
checks without the schema (spec 113 R29): its enum, without null, joined by
commas, followed through a $ref; and its string format, where it is "date"
(a time.Time field is date-time by its type, so needs no tag).
*/
func propertyRule(schemas map[string]any, p map[string]any) (enum, format string) {
	if ref, ok := p["$ref"].(string); ok {
		p = schemas[strings.TrimPrefix(ref, "#/components/schemas/")].(map[string]any)
	}
	if values, ok := p["enum"].([]any); ok {
		var out []string
		for _, e := range values {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		enum = strings.Join(out, ",")
	}
	if f, _ := p["format"].(string); f == "date" {
		format = f
	}
	return enum, format
}

// allowsNull reports whether a property's schema takes null: by its type, or
// by a oneOf that has it.
func allowsNull(schemas, p map[string]any) bool {
	if ref, ok := p["$ref"].(string); ok {
		p = schemas[strings.TrimPrefix(ref, "#/components/schemas/")].(map[string]any)
	}
	switch typ := p["type"].(type) {
	case string:
		if typ == "null" {
			return true
		}
	case []any:
		if slices.Contains(typ, any("null")) {
			return true
		}
	}
	if one, ok := p["oneOf"].([]any); ok {
		for _, o := range one {
			if allowsNull(schemas, o.(map[string]any)) {
				return true
			}
		}
	}
	return false
}

// arrayItems is the items schema of a property that is a list, directly or
// as one of a oneOf; nil for any other.
func arrayItems(p map[string]any) map[string]any {
	if items, ok := p["items"].(map[string]any); ok {
		return items
	}
	if one, ok := p["oneOf"].([]any); ok {
		for _, o := range one {
			if items := arrayItems(o.(map[string]any)); items != nil {
				return items
			}
		}
	}
	return nil
}

// jsonField is a struct's field by its JSON name, an embedded struct's
// included.
func jsonField(t reflect.Type, name string) (reflect.StructField, bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous {
			if inner, ok := jsonField(f.Type, name); ok {
				return inner, true
			}
			continue
		}
		if n, _, _ := strings.Cut(f.Tag.Get("json"), ","); n == name {
			return f, true
		}
	}
	return reflect.StructField{}, false
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
		for p, ps := range obj["properties"].(map[string]any) {
			enum, format := propertyRule(schemas, ps.(map[string]any))
			f, ok := jsonField(reflect.TypeOf(v), p)
			if !ok {
				continue
			}
			assert.Equal(t, enum, f.Tag.Get("enum"), "%s.%s: the Go type's enum tag is the schema's enum", name, p)
			assert.Equal(t, format, f.Tag.Get("format"), "%s.%s: the Go type's format tag is the schema's string format", name, p)
			goNull := f.Type.Kind() == reflect.Pointer || f.Tag.Get("nullable") == "true"
			assert.Equal(t, allowsNull(schemas, ps.(map[string]any)), goNull, "%s.%s: null in the schema is a pointer or a nullable tag in Go", name, p)
			if items := arrayItems(ps.(map[string]any)); items != nil && f.Type.Kind() == reflect.Slice {
				assert.Equal(t, allowsNull(schemas, items), f.Type.Elem().Kind() == reflect.Pointer, "%s.%s: a null item is a pointer item in Go", name, p)
			}
		}
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

/*
R26. The contract is an OpenAPI 3.1 document: it keeps the OpenAPI
Initiative's own schema for one (vendored in testdata/openapi, with its
source), so a generator in another language reads it. Its schemas are each
checked against JSON Schema 2020-12 when the tests compile them.
*/
func TestTheContractIsAnOpenAPIDocument(t *testing.T) {
	f, err := os.Open("testdata/openapi/schema-2022-10-07.json")
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	meta, err := jsonschema.UnmarshalJSON(f)
	require.NoError(t, err)
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	require.NoError(t, c.AddResource("https://spec.openapis.org/oas/3.1/schema/2022-10-07", meta))
	oas, err := c.Compile("https://spec.openapis.org/oas/3.1/schema/2022-10-07")
	require.NoError(t, err)

	doc, err := openAPI()
	require.NoError(t, err)
	js, err := json.Marshal(doc)
	require.NoError(t, err)
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(js))
	require.NoError(t, err)
	require.NoError(t, oas.Validate(v), "api/openapi.yaml is not an OpenAPI 3.1 document")

	// And it fails one that is not, falsified a way at a time: an info with
	// no title, a parameter in a place there is none, a 3.0 document.
	faults := map[string]func(doc map[string]any){
		"no title": func(doc map[string]any) { delete(doc["info"].(map[string]any), "title") },
		"a parameter in the body": func(doc map[string]any) {
			doc["components"].(map[string]any)["parameters"].(map[string]any)["Key"].(map[string]any)["in"] = "body"
		},
		"3.0": func(doc map[string]any) { doc["openapi"] = "3.0.3" },
	}
	for name, fault := range faults {
		broken := map[string]any{}
		require.NoError(t, json.Unmarshal(js, &broken))
		fault(broken)
		bj, err := json.Marshal(broken)
		require.NoError(t, err)
		bv, err := jsonschema.UnmarshalJSON(bytes.NewReader(bj))
		require.NoError(t, err)
		assert.Error(t, oas.Validate(bv), "%s: not an OpenAPI 3.1 document", name)
	}
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
