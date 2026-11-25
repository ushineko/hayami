package api

import (
	"math"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/ushineko/hayami/internal/api/contract"
	"github.com/ushineko/hayami/internal/safetext"
)

/*
cleanText makes every string in a response safe and bounded (spec 113 R13),
and reports whether any had to be cut.

It walks the value rather than trusting each mapping to remember: a string
from outside hayami -- a device's name, a VPN's, a pull request's title --
reaches a response only through here. Control and bidirectional-override
characters are removed; nothing is padded or ellipsised; a string longer
than contract.MaxText bytes is cut to it on a character's boundary. A field
tagged `text:"lines"` keeps its line breaks: a change's description.

v must be a pointer.
*/
func cleanText(v any) bool {
	return cleanValue(reflect.ValueOf(v), false)
}

func cleanValue(v reflect.Value, lines bool) bool {
	cut := false
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			break
		}
		// A figure that is not a number is not known: null, which is what
		// the contract says of a figure not known. JSON has no NaN.
		if e := v.Elem(); v.Kind() == reflect.Pointer && e.CanFloat() &&
			(math.IsNaN(e.Float()) || math.IsInf(e.Float(), 0)) && v.CanSet() {
			v.Set(reflect.Zero(v.Type()))
			break
		}
		cut = cleanValue(v.Elem(), lines)
	case reflect.Struct:
		t := v.Type()
		for i := range v.NumField() {
			if !t.Field(i).IsExported() {
				continue
			}
			if cleanValue(v.Field(i), t.Field(i).Tag.Get("text") == "lines") {
				cut = true
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if cleanValue(v.Index(i), lines) {
				cut = true
			}
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			e := reflect.New(v.Type().Elem()).Elem()
			e.Set(v.MapIndex(k))
			if cleanValue(e, lines) {
				cut = true
			}
			v.SetMapIndex(k, e)
		}
	case reflect.String:
		if v.CanSet() {
			s, c := cleanString(v.String(), lines)
			v.SetString(s)
			cut = c
		}
	default:
	}
	return cut
}

// cleanString is s made printable and cut to contract.MaxText bytes.
func cleanString(s string, lines bool) (string, bool) {
	if lines {
		parts := strings.Split(s, "\n")
		for i, p := range parts {
			parts[i] = safetext.Printable(p, 0)
		}
		s = strings.Join(parts, "\n")
	} else {
		s = safetext.Printable(s, 0)
	}
	if len(s) <= contract.MaxText {
		return s, false
	}
	end := contract.MaxText
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end], true
}
