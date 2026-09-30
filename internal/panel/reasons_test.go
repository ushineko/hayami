package panel_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/cooler"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/peripherals"
	"github.com/ushineko/hayami/internal/view"
)

// reasonTexts is what a section says it could not read, as plain strings.
func reasonTexts(s view.Section) []string {
	out := make([]string, 0, len(s.Reasons))
	for _, r := range s.Reasons {
		out = append(out, r.Text)
	}
	return out
}

// find returns the reason with a given text.
func find(t *testing.T, s view.Section, text string) view.Reason {
	t.Helper()
	for _, r := range s.Reasons {
		if r.Text == text {
			return r
		}
	}
	require.FailNowf(t, "no such reason", "%q is not among %v", text, reasonTexts(s))
	return view.Reason{}
}

/*
A cooler on a machine with no cooler draws the processor and says why the
coolant is missing.

The shape the panel had on the machine that prompted this: the processor was
being read perfectly well and the whole card was hidden, because liquidctl's
exit status was treated as a fault (issue #54).
*/
func TestACoolerWithNoLiquidDrawsTheProcessorAndSaysWhy(t *testing.T) {
	c := panel.NewCooler()
	panel.SetCoolerSources(c,
		degrees(38),
		func(context.Context) (cooler.Liquid, error) { return cooler.Liquid{}, cooler.ErrNoCooler },
	)

	drawn, err := c.Poll(t.Context())
	require.NoError(t, err, "a machine with no cooler is not a machine with a problem")
	assert.True(t, drawn)

	sec := c.Section()
	r := find(t, sec, "no cooler")
	assert.Equal(t, view.Info, r.Status, "absent hardware must not be marked as a failure")
	assert.Equal(t, "Coolant", r.Label)
	assert.NotEmpty(t, r.Detail, "the reason should say what was looked for")
}

// A liquidctl that ran and failed is marked, and keeps its error where a
// reader can reach it.
func TestALiquidctlThatFailedIsMarkedAndKeepsItsError(t *testing.T) {
	boom := errors.New("asking liquidctl for the cooler: exit status 2: OSError")
	c := panel.NewCooler()
	panel.SetCoolerSources(c,
		degrees(38),
		func(context.Context) (cooler.Liquid, error) { return cooler.Liquid{}, boom },
	)

	drawn, err := c.Poll(t.Context())
	require.Error(t, err, "a failure is still reported to the caller that logs it")
	assert.True(t, drawn, "the processor is still worth drawing")

	sec := c.Section()
	r := find(t, sec, "liquidctl failed")
	assert.Equal(t, view.Warn, r.Status, "a source that failed is not the same as hardware that is absent")
	assert.Contains(t, r.Detail, "exit status 2")
	assert.NotContains(t, sec.Lines()[len(sec.Lines())-1].Value, "exit status",
		"an exit status is not a glance; it belongs in the hover")
}

// A cooler with both readings says nothing extra. A card that explained itself
// while showing its numbers would be a panel talking about itself.
func TestACoolerThatReadsEverythingSaysNothingExtra(t *testing.T) {
	c := panel.NewCooler()
	panel.SetCoolerSources(c, degrees(38), liquid(cooling(30, 2000)))

	_, err := c.Poll(t.Context())

	require.NoError(t, err)
	assert.Empty(t, c.Section().Reasons)
}

// A reason is a statement about now. A poll that recovers must take its reason
// away rather than leaving it under a live reading.
func TestARecoveredSourceDropsItsReason(t *testing.T) {
	c := panel.NewCooler()
	panel.SetCoolerSources(c,
		degrees(38),
		func(context.Context) (cooler.Liquid, error) { return cooler.Liquid{}, cooler.ErrNoCooler },
	)
	_, err := c.Poll(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, c.Section().Reasons)

	panel.SetCoolerSources(c, degrees(38), liquid(cooling(30, 2000)))
	_, err = c.Poll(t.Context())

	require.NoError(t, err)
	assert.Empty(t, c.Section().Reasons, "a reason outlived the thing it was about")
}

/*
A peripherals section on a machine with nothing on the desk says what did not
answer, one source at a time.

One per source rather than one for the section, because they are different
things to go and do something about: a receiver that is not plugged in, a
program that is not installed, hardware of a given make that is not there, and
a radio the machine does not have.
*/
func TestPeripheralsNamesEachSourceThatFoundNothing(t *testing.T) {
	p := panel.NewPeripherals()
	panel.SetPeripheralSources(p,
		func() ([]peripherals.Battery, error) { return nil, nil },
		func(context.Context) ([]peripherals.Battery, error) {
			return nil, peripherals.ErrNoHeadsetcontrol
		},
		time.Now,
	)

	drawn, err := p.Poll(t.Context())
	require.NoError(t, err, "absent hardware is not an error")
	assert.False(t, drawn, "there is nothing to draw")

	sec := p.Section()
	assert.Equal(t, []string{
		"no Logitech receiver",
		"headsetcontrol is not installed",
		"no Razer device",
		"no SteelSeries device",
		"no Bluetooth adapter",
	}, reasonTexts(sec))
	for _, r := range sec.Reasons {
		assert.Equal(t, view.Info, r.Status, "%q was marked as a failure", r.Text)
	}
}

// A BlueZ that cannot be activated -- the bus name failing to start because
// the machine has no adapter at all -- is absence, not failure.
func TestBluezThatWillNotActivateIsAnAbsentAdapter(t *testing.T) {
	p := panel.NewPeripherals()
	panel.SetPeripheralSources(p,
		func() ([]peripherals.Battery, error) { return nil, nil },
		func(context.Context) ([]peripherals.Battery, error) { return nil, peripherals.ErrNoHeadsetcontrol },
		time.Now,
	)
	panel.SetPeripheralBluetooth(p, func() ([]peripherals.Battery, error) {
		return nil, errors.New("bluez is not answering: Could not activate remote peer 'org.bluez': unit failed")
	})

	_, err := p.Poll(t.Context())

	require.Error(t, err, "an unrecognised BlueZ failure is still reported")
	assert.Contains(t, reasonTexts(p.Section()), "a Bluetooth device would not answer")
}

// A card that is showing hardware does not also explain what it is not
// showing. That belongs in doctor.
func TestPeripheralsWithADeviceSaysNothingExtra(t *testing.T) {
	p := panel.NewPeripherals()
	panel.SetPeripheralSources(p,
		func() ([]peripherals.Battery, error) {
			return []peripherals.Battery{{Name: "MX Master", Level: 70, HasLevel: true}}, nil
		},
		func(context.Context) ([]peripherals.Battery, error) { return nil, peripherals.ErrNoHeadsetcontrol },
		time.Now,
	)

	drawn, err := p.Poll(t.Context())

	require.NoError(t, err)
	require.True(t, drawn)
	assert.Empty(t, p.Section().Reasons)
}

// A usage section with no accounts says so, rather than being absent.
func TestUsageWithNoAccountsSaysSo(t *testing.T) {
	u := panel.NewUsage()
	panel.SetUsageRead(u, func(context.Context) ([]view.UsageWindow, time.Time, []view.Reason, error) {
		return nil, time.Time{}, []view.Reason{{
			Text: "no Claude or Codex account", Status: view.Info,
		}}, nil
	})

	drawn, err := u.Poll(t.Context())

	require.NoError(t, err)
	assert.False(t, drawn)
	assert.Equal(t, []string{"no Claude or Codex account"}, reasonTexts(u.Section()))
}

// A gather that fails outright still produces a section that says something.
func TestUsageThatCannotBeGatheredSaysWhy(t *testing.T) {
	u := panel.NewUsage()
	panel.SetUsageRead(u, func(context.Context) ([]view.UsageWindow, time.Time, []view.Reason, error) {
		return nil, time.Time{}, nil, errors.New("listing the usage cache: permission denied")
	})

	_, err := u.Poll(t.Context())

	require.Error(t, err)
	sec := u.Section()
	require.Len(t, sec.Reasons, 1)
	assert.Equal(t, view.Warn, sec.Reasons[0].Status)
	assert.Contains(t, sec.Reasons[0].Detail, "permission denied")
}

// liquid adapts a test's cooler reading to the source's signature.
func liquid(f func() (cooler.Liquid, error)) func(context.Context) (cooler.Liquid, error) {
	return func(context.Context) (cooler.Liquid, error) { return f() }
}

/*
A bandwidth section with no interfaces chosen says so, and one whose interface
the kernel does not list names it.

A renamed interface after a hardware change is the ordinary way the second
happens, and the row it leaves behind is blanks of the right width -- correct
for the column, and silent about the name being wrong.
*/
func TestBandwidthSaysWhenThereIsNothingToWatch(t *testing.T) {
	b := panel.NewBandwidth(nil, fakeCounters)

	_, err := b.Poll(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{"no interfaces chosen"}, reasonTexts(b.Section()))
}

func TestBandwidthNamesAnInterfaceTheKernelDoesNotHave(t *testing.T) {
	b := panel.NewBandwidth([]string{"eth0", "wlan9"}, fakeCounters)

	_, err := b.Poll(t.Context())

	require.NoError(t, err)
	sec := b.Section()
	require.Len(t, sec.Reasons, 1)
	assert.Equal(t, "wlan9", sec.Reasons[0].Label)
	assert.Equal(t, "not present", sec.Reasons[0].Text)
	assert.Equal(t, view.Info, sec.Reasons[0].Status)

	// And it takes that interface's row rather than sitting under it. A blank
	// row and a reason both about wlan9 is one line too many.
	for _, r := range sec.Rows {
		assert.NotEqual(t, "wlan9", r.Label, "the absent interface kept a row of blanks as well")
	}
}

func fakeCounters() (map[string]core.Counters, error) {
	return map[string]core.Counters{"eth0": {Rx: 1000, Tx: 500}}, nil
}

/*
An account waiting out a backoff with nothing cached says so, and says until
when.

The exact state the machine this spec came from was in: `usage-max.json` held
`"data": null` behind a gate thirty-five minutes out, and the section drew
nothing with no explanation anywhere. The gate is not shortened -- it is
written into a file three programs read -- so saying what is happening is the
whole of the remedy.

Driven through the real gather, with the cache and the home directory taken
away, because the decision is made from a cache file's contents and a stub
would be asserting the stub.
*/
func TestAUsageAccountWaitingOutABackoffSaysSo(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", t.TempDir()) // no credential store, so nothing fetches
	t.Setenv("PATH", t.TempDir()) // and no codex either

	// A gate an hour out, and nothing behind it.
	dir := filepath.Join(cache, "claude-usage-widget")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	gate := float64(time.Now().Add(time.Hour).UnixNano()) / float64(time.Second)
	body := fmt.Sprintf(`{"next_attempt_at":%f,"fetched_at":null,"data":null}`, gate)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "usage-max.json"), []byte(body), 0o600))

	u := panel.NewUsage()
	drawn, err := u.Poll(t.Context())

	require.NoError(t, err)
	assert.False(t, drawn, "there is no reading behind the gate")

	sec := u.Section()
	r := find(t, sec, "waiting to retry")
	assert.Equal(t, "max", r.Label)
	assert.Equal(t, view.Warn, r.Status, "a backoff a person may want to understand is marked")
	assert.Contains(t, r.Detail, "gate opens at",
		"a panel that is waiting should say until when")
}

// An account that has simply never fetched, with its gate open, is stated
// rather than marked: nothing has gone wrong yet.
func TestAnAccountThatHasNeverFetchedIsNotMarkedAsAFailure(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	dir := filepath.Join(cache, "claude-usage-widget")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "usage-max.json"),
		[]byte(`{"next_attempt_at":1,"fetched_at":null,"data":null}`), 0o600))

	u := panel.NewUsage()
	_, err := u.Poll(t.Context())

	require.NoError(t, err)
	r := find(t, u.Section(), "nothing fetched yet")
	assert.Equal(t, view.Info, r.Status)
}

/*
A device that is present and unreadable is named even when others are drawing.

The suppression rule is right for absences — "no Logitech receiver" is noise
beside a mouse that is showing — and wrong for a device that is on the desk and
being deliberately left alone. Without this, a build that had quietly stopped
recognising a device looked exactly like one that never met it (spec 017).
*/
func TestAnUnreadableDeviceIsNamedEvenWhenOthersAreDrawing(t *testing.T) {
	p := panel.NewPeripherals()
	panel.SetPeripheralSources(p,
		func() ([]peripherals.Battery, error) {
			return []peripherals.Battery{{Name: "MX Master", Level: 70, HasLevel: true}}, nil
		},
		func(context.Context) ([]peripherals.Battery, error) { return nil, peripherals.ErrNoHeadsetcontrol },
		time.Now,
	)
	panel.SetPeripheralUnsupported(p, func() []string { return []string{"Arctis Nova Pro Wireless"} })

	drawn, err := p.Poll(t.Context())

	require.NoError(t, err)
	require.True(t, drawn)

	texts := reasonTexts(p.Section())
	assert.Len(t, texts, 1, "the absences should still be suppressed: %v", texts)
	assert.Contains(t, texts[0], "Arctis Nova Pro Wireless")
}
