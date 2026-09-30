package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/cli"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/peripherals"
	"github.com/ushineko/hayami/internal/view"
)

// stub is a source a test decides the answers for.
type stub struct {
	key     string
	drawn   bool
	err     error
	section view.Section
	data    any
}

func (s stub) Key() string                        { return s.key }
func (s stub) Poll(context.Context) (bool, error) { return s.drawn, s.err }
func (s stub) Interval() time.Duration            { return time.Second }
func (s stub) Section() view.Section              { return s.section }
func (s stub) Data() any                          { return s.data }

func counters() (map[string]core.Counters, error) {
	return map[string]core.Counters{"eth0": {Rx: 1, Tx: 1}}, nil
}

/*
bare is a machine with none of the things the sources look for.

Diagnose polls the real sources, which is the point of it -- a doctor built on
stubs would be testing the stubs -- so the suite has to take the machine away
instead. An empty PATH removes liquidctl and headsetcontrol, a temporary home
removes the credential stores, a temporary cache removes the accounts, and a
bus address that does not resolve removes BlueZ. Without this the test fetches
somebody's real usage over the network, which is not a unit test.

The hwmon tree is not among them: it has no environment override and a
processor temperature that does or does not read is fine either way, because
what is asserted is that the section *says* which.
*/
func bare(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path=/nonexistent/hayami-test")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/hayami-test")

	// And an empty hidraw tree, which the environment cannot take away.
	//
	// Without this these tests talked to the developer's actual mouse: the
	// environment above hides the programs and the credential stores, and
	// nothing hid the devices. Two consequences, and the second is the one
	// that bit. A unit suite should not be writing to somebody's hardware at
	// all; and `go test ./...` runs packages as parallel processes, so these
	// polls raced the live tests in internal/peripherals over one receiver.
	// HID++ gives a request four bits to say whose it is, so two processes
	// collide one time in fourteen -- and the live comparison duly failed with
	// a reading that belonged to the other test binary.
	sys, dev := peripherals.SysHidraw, peripherals.DevDir
	peripherals.SysHidraw, peripherals.DevDir = t.TempDir(), t.TempDir()
	t.Cleanup(func() { peripherals.SysHidraw, peripherals.DevDir = sys, dev })
}

/*
readings prints every source, whatever the others did.

The fault this replaces: `readings` returned on the first failure, so on a
machine where liquidctl exits 1 it printed that one error and nothing at all
about the other three sections -- the one command meant for debugging a machine
you cannot see, made useless by the machine being unusual (issue #54).
*/
func TestReadingsPrintsEverySourceEvenWhenOneFails(t *testing.T) {
	sources := []panel.Source{
		stub{key: "bandwidth", drawn: true, data: []string{"eth0"}},
		stub{key: "cooler", err: errors.New("asking liquidctl for the cooler: exit status 1")},
		stub{key: "usage", drawn: true, data: map[string]int{"5h": 5}},
	}

	var out bytes.Buffer
	err := cli.Readings(&out, sources, func(s panel.Source) error {
		_, err := s.Poll(context.Background())
		return err
	})

	require.NoError(t, err, "one source failing must not fail the command")

	var got map[string]cli.Reading
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Len(t, got, 3, "a source that failed was dropped from the report")
	assert.Contains(t, got["cooler"].Error, "exit status 1")
	assert.Empty(t, got["bandwidth"].Error)
	assert.NotNil(t, got["usage"].Data)
}

// A section's reasons reach the JSON with their status spelled out, because
// this output is read by people and a Status is an integer.
func TestReadingsCarriesTheReasonsWithTheirStatusNamed(t *testing.T) {
	sources := []panel.Source{stub{
		key:   "cooler",
		drawn: true,
		section: view.Section{Reasons: []view.Reason{
			{Label: "Coolant", Text: "no cooler", Detail: "matched nothing", Status: view.Info},
			{Text: "liquidctl failed", Status: view.Warn},
		}},
	}}

	var out bytes.Buffer
	require.NoError(t, cli.Readings(&out, sources, func(panel.Source) error { return nil }))

	var got map[string]cli.Reading
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Len(t, got["cooler"].Reasons, 2)
	assert.Equal(t, "info", got["cooler"].Reasons[0].Status)
	assert.Equal(t, "matched nothing", got["cooler"].Reasons[0].Detail)
	assert.Equal(t, "warn", got["cooler"].Reasons[1].Status)
}

// When nothing answered at all, the command does fail: a caller can still tell
// a machine with a problem from a machine with a quirk.
func TestReadingsFailsOnlyWhenNothingAnswered(t *testing.T) {
	sources := []panel.Source{stub{key: "cooler", err: errors.New("boom")}}

	var out bytes.Buffer
	err := cli.Readings(&out, sources, func(s panel.Source) error {
		_, e := s.Poll(context.Background())
		return e
	})

	require.Error(t, err)
	assert.Contains(t, out.String(), "cooler", "the JSON is written either way")
}

/*
doctor reports every section this build knows, not only the configured ones.

"I turned it off" is one of the answers somebody needs, and a section that is
off not appearing is the same mistake in miniature.
*/
func TestDoctorReportsEverySectionIncludingTheOnesThatAreOff(t *testing.T) {
	bare(t)
	findings := cli.Diagnose(t.Context(), []string{"bandwidth"}, []string{"eth0"}, counters)

	assert.Equal(t, []string{"bandwidth", "cooler", "peripherals", "usage"}, cli.Keys(findings))

	for _, f := range findings {
		if f.Key == "bandwidth" {
			continue
		}
		assert.Equal(t, cli.StateOff, f.State, "%s is not in the settings", f.Key)
	}
}

// Every section either reads something or says why not. A source that reports
// neither is the silence this command exists to make impossible.
func TestNoSectionIsSilent(t *testing.T) {
	bare(t)
	findings := cli.Diagnose(t.Context(), panel.Keys(), nil, counters)

	for _, f := range findings {
		assert.NotEqual(t, cli.StateSilent, f.State,
			"%s drew nothing and gave no reason for drawing nothing", f.Key)
	}
}

// The report is what a person pastes into an issue, so its shape is asserted:
// a key, a state, and each reason indented under them with its detail under it.
func TestTheReportPutsEachReasonUnderItsSection(t *testing.T) {
	findings := []cli.Finding{{
		Key:     "cooler",
		State:   cli.StatePartial,
		Summary: "CPU 38 °C",
		Reasons: []view.Reason{
			{Label: "Coolant", Text: "no cooler", Detail: `liquidctl --match "kraken" matched nothing`, Status: view.Info},
			{Text: "liquidctl failed", Status: view.Warn},
		},
	}}

	var out bytes.Buffer
	require.NoError(t, cli.Report(&out, findings))

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, 4)
	assert.Contains(t, lines[0], "cooler")
	assert.Contains(t, lines[0], "partial")
	assert.Contains(t, lines[0], "CPU 38 °C")
	assert.Contains(t, lines[1], "Coolant: no cooler")
	assert.Contains(t, lines[2], "kraken")
	assert.Contains(t, lines[3], "! liquidctl failed",
		"a failure must be distinguishable from an absence at a glance")
}

// doctor is on both binaries. A person whose panel is missing a card is
// running the window, and telling them to install the other binary to find out
// why is asking them to do the diagnosis this command is for.
func TestDoctorIsOnBothBinaries(t *testing.T) {
	for _, tree := range []*cobra.Command{cli.TUI("1.2.3"), cli.GUI("1.2.3", func(cli.Options) error { return nil })} {
		found := false
		for _, c := range tree.Commands() {
			if c.Name() == "doctor" {
				found = true
			}
		}
		assert.True(t, found, "%s has no doctor", tree.Name())
	}
}

// Nothing doctor prints could carry a credential.
func TestDoctorPrintsNoCredential(t *testing.T) {
	bare(t)
	findings := cli.Diagnose(t.Context(), panel.Keys(), nil, counters)

	var out bytes.Buffer
	require.NoError(t, cli.Report(&out, findings))

	for _, word := range []string{"token", "Token", "Bearer", "accessToken", "refreshToken", ".credentials"} {
		assert.NotContains(t, out.String(), word, "doctor's output is pasted into issues")
	}
}
