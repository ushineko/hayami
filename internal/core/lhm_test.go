package core_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// The fixture has the shape of a LibreHardwareMonitor 0.9.6 answer, with
// invented hardware and figures.
func fixture(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/lhm.json")
	require.NoError(t, err)
	return body
}

func tree(t *testing.T, body []byte) core.LHMNode {
	t.Helper()
	var root core.LHMNode
	require.NoError(t, json.Unmarshal(body, &root))
	return root
}

// serve answers every request with body and status.
func serve(t *testing.T, status int, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/data.json" {
			t.Errorf("asked %s %s; only GET /data.json is ever asked", r.Method, r.URL)
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func reader(url string, host core.LHMHost) *core.LHM {
	l := core.NewLHM(url)
	l.Host = func() core.LHMHost { return host }
	return l
}

func absence(t *testing.T, err error) string {
	t.Helper()
	var a *core.SensorAbsence
	require.ErrorAs(t, err, &a)
	return a.Detail
}

// R1. The processor's temperature is the CPU hardware's, not the card's,
// though both are temperatures.
func TestTheProcessorsTemperatureIsReadFromTheTree(t *testing.T) {
	v, label, ok := core.LHMCPUTemperature(tree(t, fixture(t)))
	require.True(t, ok)
	assert.InDelta(t, 47.3, v, 0.001)
	assert.Equal(t, "Core (Tctl/Tdie)", label)
}

// R1. By label in hwmon's order, never by the sensor's number: Tdie wins over
// the one named for both, which wins over Tctl, and Intel's package last.
func TestTheProcessorsLabelsArePreferredInOrder(t *testing.T) {
	sensor := func(id, text, value string) core.LHMNode {
		return core.LHMNode{SensorID: id, Text: text, Type: "Temperature", RawValue: core.LHMText(value)}
	}
	cases := []struct {
		name    string
		sensors []core.LHMNode
		want    float64
		label   string
	}{
		{"Tdie over Tctl", []core.LHMNode{
			sensor("/amdcpu/0/temperature/0", "Core (Tctl)", "70.0 °C"),
			sensor("/amdcpu/0/temperature/1", "Core (Tdie)", "60.0 °C"),
		}, 60, "Core (Tdie)"},
		{"both names over Tctl", []core.LHMNode{
			sensor("/amdcpu/0/temperature/0", "Core (Tctl)", "70.0 °C"),
			sensor("/amdcpu/0/temperature/9", "Core (Tctl/Tdie)", "65.0 °C"),
		}, 65, "Core (Tctl/Tdie)"},
		{"Intel package", []core.LHMNode{
			sensor("/intelcpu/0/temperature/0", "CPU Core #1", "55.0 °C"),
			sensor("/intelcpu/0/temperature/8", "CPU Package", "58.0 °C"),
		}, 58, "CPU Package"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, label, ok := core.LHMCPUTemperature(core.LHMNode{Children: c.sensors})
			require.True(t, ok)
			assert.InDelta(t, c.want, v, 0.001)
			assert.Equal(t, c.label, label)
		})
	}
}

// R1. A card's "GPU Core" or a drive's temperature is never the processor's,
// and a tree with no processor temperature has none.
func TestNoProcessorTemperatureIsNone(t *testing.T) {
	root := core.LHMNode{Children: []core.LHMNode{
		{SensorID: "/gpu-nvidia/0/temperature/0", Text: "Core (Tctl/Tdie)", Type: "Temperature", RawValue: "39.0 °C"},
		{SensorID: "/amdcpu/0/load/0", Text: "Core (Tctl/Tdie)", Type: "Load", RawValue: "12.0 %"},
	}}
	_, _, ok := core.LHMCPUTemperature(root)
	assert.False(t, ok)
}

// R1. The values are text, formatted in the server's locale.
func TestAValueIsReadWithEitherDecimalMark(t *testing.T) {
	for in, want := range map[string]float64{
		"51.5 °C": 51.5, "51,5 °C": 51.5, " 47 °C": 47, "-3.5 °C": -3.5, "100": 100,
	} {
		v, ok := core.ParseLHMValue(in)
		require.True(t, ok, in)
		assert.InDelta(t, want, v, 0.001, in)
	}
	for _, in := range []string{"", "°C", "n/a"} {
		_, ok := core.ParseLHMValue(in)
		assert.False(t, ok, in)
	}
}

// R1. A number where text was expected is read too, and a SensorId that
// appears twice -- the fixture's "/gpu-nvidia/0/load/3" -- costs nothing.
func TestTheTreeIsReadWhateverItsValuesAre(t *testing.T) {
	root := tree(t, []byte(`{"Children":[{"Text":"Core (Tdie)","SensorId":"/amdcpu/0/temperature/1","Type":"Temperature","RawValue":48.5,"Value":null}]}`))
	v, _, ok := core.LHMCPUTemperature(root)
	require.True(t, ok)
	assert.InDelta(t, 48.5, v, 0.001)

	assert.Equal(t, 2, strings.Count(string(fixture(t)), `"/gpu-nvidia/0/load/3"`), "the fixture keeps the duplicate")
	_, _, ok = core.LHMCPUTemperature(tree(t, fixture(t)))
	assert.True(t, ok)
}

// R2. Over HTTP, a server that answers gives its temperature.
func TestTheServersTemperatureIsTheProcessors(t *testing.T) {
	srv := serve(t, http.StatusOK, fixture(t))
	v, err := reader(srv.URL+"/data.json", core.LHMHost{}).CPUTemperature(t.Context())
	require.NoError(t, err)
	assert.InDelta(t, 47.3, v, 0.001)
}

// R2 (c). A server with no processor temperature is LibreHardwareMonitor
// without PawnIO.
func TestAServerWithNoProcessorTemperatureAsksAboutPawnIO(t *testing.T) {
	srv := serve(t, http.StatusOK, []byte(`{"Text":"Sensor","Children":[]}`))
	_, err := reader(srv.URL+"/data.json", core.LHMHost{PawnIO: true, Running: true}).CPUTemperature(t.Context())
	require.ErrorIs(t, err, core.ErrLHMNoSensor)
	assert.Contains(t, absence(t, err), "is PawnIO installed?")
}

// R2 (d). A server that wants a password says so.
func TestAServerThatWantsAPasswordSaysSo(t *testing.T) {
	srv := serve(t, http.StatusUnauthorized, nil)
	_, err := reader(srv.URL+"/data.json", core.LHMHost{Running: true}).CPUTemperature(t.Context())
	require.ErrorIs(t, err, core.ErrLHMAuth)
	assert.Contains(t, absence(t, err), "turn its authentication off")
}

// R2 (a), (b). Nothing answering is told apart by what the machine has.
func TestNothingAnsweringIsToldApartByWhatTheMachineHas(t *testing.T) {
	srv := serve(t, http.StatusOK, nil)
	url := srv.URL + "/data.json"
	srv.Close() // nothing listens there now

	cases := []struct {
		host core.LHMHost
		want string
	}{
		{core.LHMHost{}, "Windows needs LibreHardwareMonitor and its PawnIO driver"},
		{core.LHMHost{PawnIO: true, Running: true}, "its web server is off: Options → Remote Web Server → Run"},
		{core.LHMHost{Running: true}, "its web server is off"},
		{core.LHMHost{PawnIO: true}, "LibreHardwareMonitor is not running"},
	}
	for _, c := range cases {
		_, err := reader(url, c.host).CPUTemperature(t.Context())
		require.ErrorIs(t, err, core.ErrLHMUnreachable)
		assert.Contains(t, absence(t, err), c.want, "%+v", c.host)
	}
}

// R1. A server that does not answer in time is not waited on.
func TestASlowServerIsNotWaitedOn(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(func() { close(release); srv.Close() })

	start := time.Now()
	_, err := reader(srv.URL+"/data.json", core.LHMHost{Running: true}).CPUTemperature(t.Context())
	require.ErrorIs(t, err, core.ErrLHMUnreachable)
	assert.Less(t, time.Since(start), core.LHMTimeout+time.Second)
}

// R1. A body far past any sensor tree is refused rather than held.
func TestAnOversizedAnswerIsRefused(t *testing.T) {
	srv := serve(t, http.StatusOK, []byte(`{"Text":"`+strings.Repeat("x", 9<<20)+`"}`))
	_, err := reader(srv.URL+"/data.json", core.LHMHost{}).CPUTemperature(t.Context())
	assert.Contains(t, absence(t, err), "not a sensor tree")
}

// R1. A cancelled poll is the caller's, not an absence to explain.
func TestACancelledPollIsTheCallers(t *testing.T) {
	srv := serve(t, http.StatusOK, fixture(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := reader(srv.URL+"/data.json", core.LHMHost{}).CPUTemperature(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

// R1. Empty is the default address.
func TestEmptyIsTheDefaultAddress(t *testing.T) {
	assert.Equal(t, core.LHMURL, core.NewLHM("  ").URL)
	assert.Equal(t, "http://127.0.0.1:9999/data.json", core.NewLHM("http://127.0.0.1:9999/data.json").URL)
}
