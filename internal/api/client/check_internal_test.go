package client

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api/contract"
)

/*
A field of an embedded struct is read through the outer one: a VPN's name in
a VPNDetail is checked as that name, at its own path, not whichever field of
VPNDetail has the same position (found by the adversarial review: it held
only while the two structs had as many fields).
*/
func TestFullValuesReadsAnEmbeddedStructsFields(t *testing.T) {
	d := contract.Detail{VPNs: []contract.VPNDetail{{VPN: contract.VPN{Name: "work  "}, Addresses: []string{}}}}
	err := fullValues("$", d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "$.vpns[0].name")

	v := reflect.ValueOf(contract.VPNDetail{})
	for _, f := range structFields(v.Type()) {
		field, ok := v.Type().FieldByName(f.Name)
		require.True(t, ok)
		assert.Equal(t, field.Index, f.Index, "%s is reached by its own index", f.Name)
	}
}

// Load takes counts and a time, and says so rather than failing inside.
func TestLoadRefusesOptionsThatAreNotCounts(t *testing.T) {
	_, err := Load(context.Background(), LoadOptions{Clients: 1, Streams: -1, Duration: 1})
	require.Error(t, err)
	_, err = Load(context.Background(), LoadOptions{Clients: 1, Streams: 1})
	require.Error(t, err)
}
