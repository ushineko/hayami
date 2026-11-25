package client_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api/client"
	"github.com/ushineko/hayami/internal/api/contract"
)

/*
R29. A response the contract does not allow is an error naming the value,
never a zero value: each way a response can break the contract, against the
type it should have been.
*/
func TestDecodeRefusesWhatTheContractDoesNotAllow(t *testing.T) {
	const device = `{"name":"Mouse","kind":"mouse","battery_pct":80,"band":null,"segments":null,` +
		`"charge":"draining","stale":false,"last_seen":"2026-11-25T10:00:00Z"}`
	cases := []struct {
		name, body, path, reason string
	}{
		{"a field missing", `{"name":"Mouse","kind":"mouse","battery_pct":80,"band":null,"segments":null,"charge":"draining","stale":false}`,
			"$.last_seen", "missing"},
		{"a field the contract lacks", `{"name":"Mouse","kind":"mouse","battery_pct":80,"band":null,"segments":null,` +
			`"charge":"draining","stale":false,"last_seen":null,"serial":"x"}`, "$.serial", "a field the contract lacks"},
		{"a string where a number belongs", `{"name":"Mouse","kind":"mouse","battery_pct":"80","band":null,"segments":null,` +
			`"charge":"draining","stale":false,"last_seen":null}`, "$.battery_pct", "a string where a whole number belongs"},
		{"a fraction where a whole number belongs", `{"name":"Mouse","kind":"mouse","battery_pct":80.5,"band":null,"segments":null,` +
			`"charge":"draining","stale":false,"last_seen":null}`, "$.battery_pct", "80.5 is not a whole number"},
		{"null where there is none", `{"name":null,"kind":"mouse","battery_pct":80,"band":null,"segments":null,` +
			`"charge":"draining","stale":false,"last_seen":null}`, "$.name", "null, where the contract has a string"},
		{"a value outside the enum", `{"name":"Mouse","kind":"trackball","battery_pct":80,"band":null,"segments":null,` +
			`"charge":"draining","stale":false,"last_seen":null}`, "$.kind", `"trackball" is not one of mouse, keyboard, headset, gamepad, other`},
		{"a time that does not parse", `{"name":"Mouse","kind":"mouse","battery_pct":80,"band":null,"segments":null,` +
			`"charge":"draining","stale":false,"last_seen":"yesterday"}`, "$.last_seen", `"yesterday" is not an RFC 3339 time`},
		{"not JSON", `{"name":`, "$", "not JSON"},
		{"two values", device + device, "$", "more than one JSON value"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var d contract.Device
			err := client.Decode([]byte(c.body), &d)
			var ce *client.ContractError
			require.True(t, errors.As(err, &ce), "want a contract error, got %v", err)
			assert.Equal(t, c.path, ce.Path)
			assert.Contains(t, ce.Reason, c.reason)
		})
	}

	var d contract.Device
	require.NoError(t, client.Decode([]byte(device), &d))
	assert.Equal(t, "Mouse", d.Name)
	require.NotNil(t, d.BatteryPct)
	assert.Equal(t, 80, *d.BatteryPct)
}

// Deep inside a response, the path still names the value.
func TestDecodeNamesAValueDeepInside(t *testing.T) {
	body := `{"list":"cpu","name":null,"note":null,"processes":[{"pid":1,"name":"init","cpu_pct":"high",` +
		`"gpu_pct":null,"memory_bytes":null,"memory_pct":null,"read_bps":null,"write_bps":null}],"summary":null}`
	var l contract.ProcessList
	err := client.Decode([]byte(body), &l)
	var ce *client.ContractError
	require.True(t, errors.As(err, &ce), "%v", err)
	assert.Equal(t, "$.processes[0].cpu_pct", ce.Path)

	// A list is never null.
	err = client.Decode([]byte(`{"list":"cpu","name":null,"note":null,"processes":null,"summary":null}`), &l)
	require.True(t, errors.As(err, &ce), "%v", err)
	assert.Equal(t, "$.processes", ce.Path)

	// A date is a calendar date.
	err = client.Decode([]byte(`{"date":"25/11/2026","count":1}`), &contract.ForgeCount{})
	require.True(t, errors.As(err, &ce), "%v", err)
	assert.Equal(t, "$.date", ce.Path)
}
