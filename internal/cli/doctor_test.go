package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku/hidraw"

	"github.com/ushineko/hayami/internal/cli"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
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
instead. A temporary home removes the credential stores, a temporary cache
removes the accounts, an empty PATH removes codex, and a bus address that does
not resolve removes BlueZ. Without this the test fetches somebody's real usage
over the network, which is not a unit test.

And an empty hidraw tree, which the environment cannot take away. Without it
these tests talked to the developer's actual mouse, headset and cooler: a unit
suite should not be writing to somebody's hardware at all, and `go test ./...`
runs packages as parallel processes, so the polls would race any other reader
of the same receiver. sanshoku's transports read the tree from two variables
for exactly this.

The hwmon tree is not among them: reading a temperature file writes nothing,
and a processor temperature that does or does not read is fine either way,
because what is asserted is that the section *says* which.
*/
func bare(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/hayami-test")
	noDevices(t)
}

// noDevices takes the desk away: an empty hidraw tree, a system bus that
// does not resolve and a PATH with no nvidia-smi on it, so a poll of the real
// sources opens no device, asks BlueZ nothing and runs no vendor tool. Any
// test that can reach the device sections calls it, and that includes a
// command run with default settings.
func noDevices(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path=/nonexistent/hayami-test")
	sys, dev := hidraw.SysRoot, hidraw.DevRoot
	hidraw.SysRoot, hidraw.DevRoot = t.TempDir(), t.TempDir()
	t.Cleanup(func() { hidraw.SysRoot, hidraw.DevRoot = sys, dev })
}

/*
readings prints every source, whatever the others did.

The fault this replaces: `readings` returned on the first failure, so on a
machine where the cooler's reader exited 1 it printed that one error and
nothing at all about the other three sections -- the one command meant for debugging a machine
you cannot see, made useless by the machine being unusual (issue #54).
*/
func TestReadingsPrintsEverySourceEvenWhenOneFails(t *testing.T) {
	sources := []panel.Source{
		stub{key: "bandwidth", drawn: true, data: []string{"eth0"}},
		stub{key: "cooler", err: errors.New("reading the cooler: no 7501 reply: no reply")},
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
	assert.Contains(t, got["cooler"].Error, "no 7501 reply")
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
			{Text: "the cooler would not answer", Status: view.Warn},
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
			{Label: "Coolant", Text: "no cooler", Detail: "no supported cooler detected", Status: view.Info},
			{Text: "the cooler would not answer", Status: view.Warn},
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
	assert.Contains(t, lines[2], "no supported cooler detected")
	assert.Contains(t, lines[3], "! the cooler would not answer",
		"a failure must be distinguishable from an absence at a glance")
}

// doctor is on both binaries. A person whose panel is missing a card is
// running the window, and telling them to install the other binary to find out
// why is asking them to do the diagnosis this command is for.
func TestDoctorIsOnBothBinaries(t *testing.T) {
	for _, tree := range []*cobra.Command{cli.TUI("1.2.3"), cli.GUI("1.2.3", nil, func(cli.Options) error { return nil })} {
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

/*
R4.1. A device the user may not open reads as a udev rule to install, not as a
device that is not there.

Through the real drivers and a hidraw tree written here: a Razer node whose
character device the user may not open, which is exactly what a machine that
never had OpenRazer's package looks like. Nothing is opened but a file this
test made.
*/
func TestDoctorSaysInstallTheUdevRuleForADeviceThatMayNotBeOpened(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root may open anything, so there is no permission to be refused")
	}
	bare(t)

	node := filepath.Join(hidraw.SysRoot, "hidraw4", "device")
	require.NoError(t, os.MkdirAll(node, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(node, "uevent"),
		[]byte("HID_ID=0003:00001532:000000A4\nHID_NAME=Razer Razer Mouse Dock Pro\n"), 0o600))
	// Usage Page (0xFF00), the Razer control interface's vendor page.
	require.NoError(t, os.WriteFile(filepath.Join(node, "report_descriptor"), []byte{0x06, 0x00, 0xFF}, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(hidraw.DevRoot, "hidraw4"), nil, 0o000))

	findings := cli.Diagnose(t.Context(), panel.Keys(), nil, counters)
	var out bytes.Buffer
	require.NoError(t, cli.Report(&out, findings))

	assert.Contains(t, out.String(), "! Razer Mouse Dock Pro is not permitted")
	assert.Contains(t, out.String(), "install the udev rule (60-sanshoku.rules)")
	assert.NotContains(t, out.String(), "no Razer device", "a device that is there was reported absent")
}

// Spec 022. The peripherals card always draws two cells, and a slot with no
// device is a placeholder. That is the card keeping its shape, not a reading:
// doctor on a desk with nothing on it says why, and does not report the
// placeholders as though something had been read.
func TestDoctorDoesNotReportThePlaceholders(t *testing.T) {
	bare(t)
	findings := cli.Diagnose(t.Context(), panel.Keys(), nil, counters)

	for _, f := range findings {
		if f.Key != "peripherals" {
			continue
		}
		assert.NotContains(t, f.Summary, view.NoMouse)
		assert.NotContains(t, f.Summary, view.NoDevice)
		for _, r := range f.Reasons {
			assert.NotContains(t, r.Text, view.NoDevice)
		}
		return
	}
	t.Fatal("doctor did not report the peripherals")
}

// sectionSource is a source that has already polled: it says a section.
type sectionSource struct{ sec view.Section }

func (s sectionSource) Key() string                      { return s.sec.Key }
func (sectionSource) Interval() time.Duration            { return time.Hour }
func (sectionSource) Poll(context.Context) (bool, error) { return true, nil }
func (s sectionSource) Section() view.Section            { return s.sec }
func (sectionSource) Data() any                          { return nil }

// A reason kept off the card (Aside) is not something missing from it. A
// machine with no graphics card this build can read has a cooler that is ok,
// and doctor still says why there is no GPU row (spec 026).
func TestAMachineWithNoGPUIsOKAndSaysSo(t *testing.T) {
	sec := view.Cooler(view.CoolerReading{CPU: 60, HasCPU: true, Coolant: 38.9, HasLiquid: true})
	sec.Reasons = []view.Reason{{Text: "no GPU sensor", Status: view.Info, Aside: true}}

	findings := cli.DiagnoseSources(t.Context(), []panel.Source{sectionSource{sec}})

	require.Len(t, findings, 1)
	assert.Equal(t, cli.StateOK, findings[0].State)

	var out bytes.Buffer
	require.NoError(t, cli.Report(&out, findings))
	assert.Contains(t, out.String(), "no GPU sensor", "doctor still lists it")

	// A reason the card does show still makes it partial.
	sec.Reasons = append(sec.Reasons, view.Reason{Label: "Coolant", Text: "no cooler", Status: view.Info})
	findings = cli.DiagnoseSources(t.Context(), []panel.Source{sectionSource{sec}})
	assert.Equal(t, cli.StatePartial, findings[0].State)
}
