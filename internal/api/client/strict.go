package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

/*
ContractError is a response the contract does not allow (spec 113 R29): a
field missing or one it lacks, a value of the wrong type, null where the
contract has none, a value outside an enum, a time that does not parse, a
status it does not document. Path names the value, `$.reading.cooler.probes[0].temp_c`.
*/
type ContractError struct {
	Path   string
	Reason string
}

func (e *ContractError) Error() string {
	return "the service broke its contract at " + e.Path + ": " + e.Reason
}

var timeType = reflect.TypeFor[time.Time]()

/*
Decode reads data into v, a pointer to a contract type, and refuses anything
the contract does not allow rather than leaving a zero value where a field
was missing or wrong.

It needs no schema: the contract types are the schema in Go, held to
api/openapi.yaml field by field, enum by enum, by the contract's tests. Every
field is required; a pointer may be null, and a list only where its
`nullable` tag says so; an `enum` tag lists a string's values; a `format:"date"` tag is
a calendar date; a time.Time is RFC 3339. A client in another language does
the same with its own types, or with the schema.
*/
func Decode(data []byte, v any) error {
	t := reflect.TypeOf(v)
	if t == nil || t.Kind() != reflect.Pointer {
		return fmt.Errorf("decoding needs a pointer, not %T", v)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return &ContractError{Path: "$", Reason: "not JSON: " + err.Error()}
	}
	if dec.More() {
		return &ContractError{Path: "$", Reason: "more than one JSON value"}
	}
	if err := conform("$", raw, t.Elem(), field{}); err != nil {
		return err
	}
	strict := json.NewDecoder(bytes.NewReader(data))
	strict.DisallowUnknownFields()
	if err := strict.Decode(v); err != nil {
		return &ContractError{Path: "$", Reason: err.Error()}
	}
	return nil
}

// field is what a struct field's tags say of its value.
type field struct {
	enum     []string
	format   string
	nullable bool
}

func conform(path string, raw any, t reflect.Type, f field) error {
	if t.Kind() == reflect.Pointer {
		if raw == nil {
			return nil
		}
		return conform(path, raw, t.Elem(), f)
	}
	if raw == nil {
		if f.nullable {
			return nil
		}
		return &ContractError{Path: path, Reason: "null, where the contract has a " + kindName(t)}
	}
	wrong := func() error {
		return &ContractError{Path: path, Reason: jsonKind(raw) + " where a " + kindName(t) + " belongs"}
	}
	switch {
	case t == timeType:
		s, ok := raw.(string)
		if !ok {
			return wrong()
		}
		if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
			return &ContractError{Path: path, Reason: fmt.Sprintf("%q is not an RFC 3339 time", s)}
		}
		return nil
	case t.Kind() == reflect.Struct:
		obj, ok := raw.(map[string]any)
		if !ok {
			return wrong()
		}
		return conformStruct(path, obj, t)
	case t.Kind() == reflect.Slice:
		list, ok := raw.([]any)
		if !ok {
			return wrong()
		}
		for i, e := range list {
			if err := conform(fmt.Sprintf("%s[%d]", path, i), e, t.Elem(), field{}); err != nil {
				return err
			}
		}
		return nil
	case t.Kind() == reflect.Map:
		obj, ok := raw.(map[string]any)
		if !ok {
			return wrong()
		}
		for _, k := range sortedKeys(obj) {
			if err := conform(path+"."+k, obj[k], t.Elem(), field{}); err != nil {
				return err
			}
		}
		return nil
	case t.Kind() == reflect.String:
		s, ok := raw.(string)
		if !ok {
			return wrong()
		}
		return conformString(path, s, f)
	case t.Kind() == reflect.Bool:
		if _, ok := raw.(bool); !ok {
			return wrong()
		}
		return nil
	default:
		return conformNumber(path, raw, t, wrong)
	}
}

func conformStruct(path string, obj map[string]any, t reflect.Type) error {
	fields := structFields(t)
	names := make([]string, 0, len(fields))
	for _, sf := range fields {
		name, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
		names = append(names, name)
		value, ok := obj[name]
		if !ok {
			return &ContractError{Path: path + "." + name, Reason: "missing; the contract has every field, null where unknown"}
		}
		f := field{format: sf.Tag.Get("format"), nullable: sf.Tag.Get("nullable") == "true"}
		if e := sf.Tag.Get("enum"); e != "" {
			f.enum = strings.Split(e, ",")
		}
		if err := conform(path+"."+name, value, sf.Type, f); err != nil {
			return err
		}
	}
	for _, k := range sortedKeys(obj) {
		if !slices.Contains(names, k) {
			return &ContractError{Path: path + "." + k, Reason: "a field the contract lacks"}
		}
	}
	return nil
}

func conformString(path, s string, f field) error {
	if f.enum != nil && !slices.Contains(f.enum, s) {
		return &ContractError{Path: path, Reason: fmt.Sprintf("%q is not one of %s", s, strings.Join(f.enum, ", "))}
	}
	if f.format == "date" {
		if _, err := time.Parse(time.DateOnly, s); err != nil {
			return &ContractError{Path: path, Reason: fmt.Sprintf("%q is not a date", s)}
		}
	}
	return nil
}

func conformNumber(path string, raw any, t reflect.Type, wrong func() error) error {
	n, ok := raw.(json.Number)
	if !ok {
		return wrong()
	}
	var err error
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		_, err = strconv.ParseInt(n.String(), 10, t.Bits())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		_, err = strconv.ParseUint(n.String(), 10, t.Bits())
	case reflect.Float32, reflect.Float64:
		_, err = strconv.ParseFloat(n.String(), t.Bits())
	default:
		return &ContractError{Path: path, Reason: "the client has no rule for a " + t.String()}
	}
	if err != nil {
		return &ContractError{Path: path, Reason: fmt.Sprintf("%s is not a %s", n, kindName(t))}
	}
	return nil
}

// structFields are a struct's JSON fields, an embedded struct's included.
func structFields(t reflect.Type) []reflect.StructField {
	var out []reflect.StructField
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous {
			// An embedded struct's fields, with indexes from the outer struct,
			// so FieldByIndex reaches them through it.
			for _, inner := range structFields(f.Type) {
				inner.Index = append([]int{i}, inner.Index...)
				out = append(out, inner)
			}
			continue
		}
		if name, _, _ := strings.Cut(f.Tag.Get("json"), ","); name == "" || name == "-" || !f.IsExported() {
			continue
		}
		out = append(out, f)
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// kindName is what the contract calls a Go type's values.
func kindName(t reflect.Type) string {
	switch {
	case t == timeType:
		return "time"
	case t.Kind() == reflect.Struct, t.Kind() == reflect.Map:
		return "object"
	case t.Kind() == reflect.Slice:
		return "list"
	case t.Kind() == reflect.String:
		return "string"
	case t.Kind() == reflect.Bool:
		return "boolean"
	case t.Kind() == reflect.Float32, t.Kind() == reflect.Float64:
		return "number"
	default:
		return "whole number"
	}
}

// jsonKind is what a decoded JSON value is.
func jsonKind(v any) string {
	switch v.(type) {
	case map[string]any:
		return "an object"
	case []any:
		return "a list"
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case json.Number:
		return "a number"
	default:
		return "null"
	}
}
