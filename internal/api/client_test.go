package api_test

import (
	"bytes"
	"context"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api"
	"github.com/ushineko/hayami/internal/api/client"
)

/*
R29. The reference client against the real server over the fixtures, on a
real socket: every endpoint decodes into its contract type, a drill whose ID
holds "/" and a detail whose ID holds a space resolve, what is not there is
an *APIError that lists what is, and a stream gives typed events.
*/
func TestTheReferenceClientReadsEveryEndpoint(t *testing.T) {
	h := start(t, sources(fixtures()), options{})
	c := client.New(h.sock)
	t.Cleanup(c.Close)
	ctx := context.Background()

	health, err := c.Health(ctx)
	require.NoError(t, err)
	assert.True(t, health.OK)
	index, err := c.Index(ctx)
	require.NoError(t, err)
	assert.Equal(t, "1.1", index.API)
	list, err := c.Sections(ctx)
	require.NoError(t, err)
	require.Len(t, list.Sections, len(index.Sections))

	cooler, err := c.Section(ctx, "cooler")
	require.NoError(t, err)
	require.NotNil(t, cooler.Reading.Cooler)
	assert.Nil(t, cooler.Reading.Usage)

	drill, err := c.Detail(ctx, "cooler", "cpu/"+longProcess)
	require.NoError(t, err)
	assert.Equal(t, "cpu/"+longProcess, drill.ID)
	require.NotNil(t, drill.Processes)
	require.NotNil(t, drill.Processes.Name)
	assert.Equal(t, longProcess, *drill.Processes.Name, "the name whole")

	account, err := c.Detail(ctx, "usage", longAccount)
	require.NoError(t, err, "an ID with spaces")
	assert.Equal(t, longAccount, account.ID)

	readings, err := c.Readings(ctx, "clock", "cooler")
	require.NoError(t, err)
	assert.Len(t, readings.Readings, 2)

	_, err = c.Section(ctx, "nope")
	var e *client.APIError
	require.ErrorAs(t, err, &e)
	assert.Equal(t, http.StatusNotFound, e.Status)
	assert.ElementsMatch(t, index.Sections, e.Body.Known)

	s, err := c.Events(ctx, []string{"clock"}, []string{"cooler/cpu"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	seen := map[string]bool{}
	for !seen[client.EventSection] || !seen[client.EventDetail] {
		ev, err := s.Next()
		require.NoError(t, err)
		seen[ev.Name] = true
		switch ev.Name {
		case client.EventSection:
			assert.Equal(t, "clock", ev.Section.Key)
		case client.EventDetail:
			assert.Equal(t, "cpu", ev.Detail.ID)
		}
	}
}

// R30. `api check` passes against the server over the fixtures, every detail
// and drill followed.
func TestCheckPassesAgainstTheService(t *testing.T) {
	h := start(t, sources(fixtures()), options{})
	var out bytes.Buffer
	r, err := client.Check(context.Background(), client.New(h.sock), client.CheckOptions{Depth: 3, Out: &out})
	require.NoError(t, err, out.String())
	assert.Equal(t, 5, r.Sections)
	assert.Positive(t, r.Details)
	assert.Equal(t, 5, r.Events)
	assert.Contains(t, out.String(), "cpu/"+longProcess, "a drill was followed")
}

// R32. `api load` against the server: every answer one the contract
// documents, and every stream receiving.
func TestLoadRunsCleanAgainstTheService(t *testing.T) {
	fakes := fixtures()
	for _, f := range fakes {
		f.interval = 100 * time.Millisecond
	}
	h := start(t, sources(fakes), options{})
	r, err := client.Load(context.Background(), client.LoadOptions{Clients: 8, Streams: 4, Duration: 2 * time.Second, Address: h.sock})
	require.NoError(t, err)
	var out strings.Builder
	r.Write(&out)
	t.Log("\n" + out.String())
	assert.True(t, r.OK(), out.String())
	assert.Positive(t, r.Requests)
	assert.Positive(t, r.Statuses[http.StatusOK])
}

// R3. The client finds the service where the service says it is.
func TestTheClientsAddressIsTheServices(t *testing.T) {
	if !api.Supported() {
		t.Skip("the service does not run on this system yet")
	}
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	want, err := api.Address()
	require.NoError(t, err)
	got, err := client.Address()
	require.NoError(t, err)
	assert.Equal(t, want, got)

	if runtime.GOOS != "linux" {
		return // a pipe's name has no sun_path to outgrow; macOS's is listen_darwin_test.go's
	}
	t.Setenv("XDG_RUNTIME_DIR", "/"+strings.Repeat("d", 100))
	_, err = api.Address()
	require.Error(t, err)
	_, err = client.Address()
	require.Error(t, err, "a path over the system's limit is refused by both")
}

// Spec 115 R15. The reference client reads the TCP port with its token: the
// whole check passes over it, and a wrong token is a 401 the client types.
func TestTheClientReadsThePort(t *testing.T) {
	h := start(t, sources(fixtures()), options{tcp: true})
	var out bytes.Buffer
	r, err := client.Check(context.Background(), client.NewTCP(h.tcpAddr, h.key.Token), client.CheckOptions{Depth: 3, Out: &out})
	require.NoError(t, err, out.String())
	assert.Equal(t, 5, r.Sections)
	assert.Equal(t, 5, r.Events, "the stream was read over the port")

	_, err = client.NewTCP(h.tcpAddr, "wrong").Health(context.Background())
	require.Error(t, err)
	assert.True(t, client.IsStatus(err, http.StatusUnauthorized), "got %v", err)
	assert.NotContains(t, err.Error(), h.key.Token)

	lr, err := client.Load(context.Background(), client.LoadOptions{Clients: 2, Streams: 1, Duration: time.Second, Address: h.tcpAddr, Token: h.key.Token})
	require.NoError(t, err)
	assert.True(t, lr.OK(), "load over the port saw errors")
}
