package gui_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
parityWidth is the terminal width the text shell is rendered at for the
comparison: wide enough that no label or value is cut to fit, so a fact
missing from the text is missing, not truncated. Truncation at a narrow width
is the text arrangements' own rule and their own tests'.
*/
const parityWidth = 200

/*
parityAllowed names a fact one shell draws and the other does not, keyed by
the shell that lacks it and the fact, with the reason. Small on purpose: an
entry is a decision, not a difference that crept in.
*/
var parityAllowed = map[string]string{
	// In row the terminal draws a usage meter as the usage widget's --tui line
	// (spec 019): "10%" and "resets Oct 7", where the window's line keeps the
	// card's spelling, "5h: 10 %" and "in 2h 0m". The same figures, two
	// spellings; the pane's is the one a session manager shows.
	"terminal row: meter caption": "the terminal's row spells a meter the usage widget's way (spec 019)",
	"terminal row: meter reset":   "the terminal's row spells a meter the usage widget's way (spec 019)",
}

// paritySections are one section of every kind the registry knows, each built
// by its own builder from a reading with every part that section can show:
// rows with parts and detail lines, reasons, cells, meters.
func paritySections(t *testing.T) []view.Section {
	t.Helper()
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)

	bandwidth := view.Bandwidth([]view.BandwidthReading{
		{
			Name: "Wi-Fi 2", RxRate: 6400, TxRate: 46000, RxTotal: 33 << 30, TxTotal: 187 << 20,
			HasRate: true, HasTotal: true, Radio: true,
			Link: view.LinkReading{
				Connected: true, RSSI: -53, HasRSSI: true, Signal: 87, HasSignal: true,
				Band: "5 GHz", Channel: 149, HasChannel: true, Generation: "Wi-Fi 5",
				RxRate: 650, TxRate: 650, HasRx: true, HasTx: true,
			},
		},
		{Name: "Ethernet 2", HasRate: true, HasTotal: true},
	})
	bandwidth.Reasons = []view.Reason{{Label: "wlan9", Text: "not present", Status: view.Info}}

	cooler := view.Cooler(view.CoolerReading{Probes: []view.Probe{
		{ID: "cpu", Role: view.RoleCPU, Name: "AMD Ryzen 5 2600X", Load: view.Some(12.0), Temp: view.Some(51.5)},
		{ID: "gpu", Role: view.RoleGPU, Name: "NVIDIA GeForce RTX 3060 Ti", Load: view.Some(4.0), Temp: view.Some(43.0)},
		{ID: "coolant", Role: view.RoleCoolant, Name: "NZXT Kraken Elite V2", Temp: view.Some(31.4)},
		{ID: "pump", Role: view.RolePump, RPM: view.Some(2100)},
		{ID: "fan", Role: view.RoleFan, RPM: view.Some(900)},
	}})

	peripherals := view.Peripherals(view.PeripheralsReading{Devices: []view.PeripheralReading{
		{Name: "Basilisk Ultimate Dongle", Level: 48, Kind: view.KindMouse, Seen: now},
		{Name: "F75", Level: 100, Kind: view.KindKeyboard, Seen: now},
	}})

	usage := view.Usage(now, []view.UsageWindow{
		{Account: "CC max", Name: "5h", Fraction: 0.10, ResetsAt: now.Add(2 * time.Hour)},
		{Account: "CC max", Name: "7d", Fraction: 0.85, ResetsAt: now.Add(72 * time.Hour)},
		{Account: "CX", Name: "5h", Fraction: 0.20, ResetsAt: now.Add(2 * time.Hour)},
	}, now)

	out := []view.Section{bandwidth, cooler, peripherals, usage}
	keys := map[string]bool{}
	for _, s := range out {
		keys[s.Key] = true
	}
	for _, k := range panel.Keys() {
		require.True(t, keys[k], "section %q has no parity fixture: add one here", k)
	}
	return out
}

// fact is one thing a section says, and what kind of thing it is: the kind is
// what an exception names.
type fact struct{ kind, text string }

func (f fact) String() string { return fmt.Sprintf("%s %q", f.kind, strings.TrimSpace(f.text)) }

// facts are what a section says, each a short text either shell must draw:
// its title, every line's label, value and detail lines, every cell's name,
// value and note, every meter's label, caption, reset and stats.
func facts(s view.Section) []fact {
	out := []fact{{"title", s.Title}}
	add := func(kind, text string) {
		if strings.TrimSpace(text) != "" {
			out = append(out, fact{kind, text})
		}
	}
	for _, r := range s.Lines() {
		add("label", r.Label)
		add("value", strings.TrimSpace(r.Value+" "+r.Unit))
		for _, d := range r.DetailLines() {
			add("detail", d)
		}
	}
	for _, c := range s.Cells {
		add("cell name", c.Label)
		add("cell value", strings.TrimSpace(c.Value+" "+c.Unit))
		add("cell note", c.Note)
	}
	for _, m := range s.Meters {
		add("meter", m.Name())
		add("meter caption", m.Caption)
		add("meter reset", m.Reset)
		add("meter stats", m.StatsLeft)
		add("meter stats", m.StatsRight)
	}
	return out
}

// squeeze is text with every space taken out: the shells pad and align
// differently, and a part of a value can be its own text object in the window,
// so containment is judged on what is written, not on where.
func squeeze(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

/*
windowDraws is whether the window's text holds f.

A cell's name and note are drawn elided to the cell's width ("Basilisk
Ultimat…"): a cell is a fixed block in a glance card, and a name longer than
it is cut, by the library, where it can be read. That is drawn, not missing,
so an elided form counts -- the name's start up to the ellipsis, of at least
a few letters, so a lone "…" does not.
*/
func windowDraws(window string, f fact) bool {
	want := squeeze(f.text)
	if strings.Contains(window, want) {
		return true
	}
	if f.kind != "cell name" && f.kind != "cell note" {
		return false
	}
	runes := []rune(want)
	for n := len(runes) - 1; n >= 4; n-- {
		if strings.Contains(window, string(runes[:n])+"…") {
			return true
		}
	}
	return false
}

// arrangements are the three the view knows; each shell draws all three.
var arrangements = []view.Arrangement{view.ArrangeStack, view.ArrangeGrid, view.ArrangeRow}

/*
differences are the facts the two shells do not draw alike, as the keys an
exception names: the shell that lacks the fact, the arrangement and the kind
("terminal row: meter caption").

The window is drawn headlessly in each arrangement, by applying a setting as
the preferences would; the terminal is rendered in the same one.

In stack and grid every fact is drawn by both. Row is one line per reading in
both shells (spec 049): no headings, no detail lines, no stats. There a fact
both leave out is the arrangement's shape and not a difference; only a fact
one draws and the other does not is.

A plot is not a fact here: it is drawn, not written, and the window's has no
text. In row the window leaves its plots out (glance.Lines) where the terminal
draws a named trail line; spec 049 records it.
*/
func differences(t *testing.T) map[string][]string {
	t.Helper()
	sections := paritySections(t)

	a := test.NewTempApp(t)
	sources := make([]panel.Source, 0, len(sections))
	for _, s := range sections {
		sources = append(sources, &fixed{s})
	}
	p := gui.New(a, gui.Options{Sources: sources, Title: "hayami"})
	for _, s := range sections {
		p.Draw(s.Key, s, true)
	}

	out := map[string][]string{}
	for _, arr := range arrangements {
		c := config.Default()
		c.Arrangement = arr.String()
		p.Apply(c)
		for _, s := range sections {
			window := squeeze(strings.Join(gui.CardText(p, s.Key), ""))
			require.NotEmpty(t, window, "the window drew nothing for %s in %s", s.Key, arr)
			terminal := squeeze(strings.Join(view.Render([]view.Section{s}, arr, parityWidth), ""))
			for _, f := range facts(s) {
				w := windowDraws(window, f)
				tm := strings.Contains(terminal, squeeze(f.text))
				if w == tm && (w || arr == view.ArrangeRow) {
					continue
				}
				for shell, drew := range map[string]bool{"window": w, "terminal": tm} {
					if !drew {
						key := shell + " " + arr.String() + ": " + f.kind
						out[key] = append(out[key], s.Key+": "+f.String())
					}
				}
			}
		}
	}
	return out
}

/*
Specs 047 and 049. The window and the terminal draw the same thing for every
section in every arrangement: every label, value, detail line, cell and meter
a section holds is on the window's card and in the terminal's rendering, or in
neither where the arrangement leaves it out. A fact one shell draws and the
other does not is a failure unless parityAllowed says why.
*/
func TestTheWindowAndTheTerminalDrawTheSameFacts(t *testing.T) {
	for key, what := range differences(t) {
		if _, ok := parityAllowed[key]; !ok {
			assert.Failf(t, "the shells draw this differently", "%s: %v", key, what)
		}
	}
}

// Every exception is still needed: one that no longer excuses anything is a
// rule nobody is holding.
func TestEveryParityExceptionIsUsed(t *testing.T) {
	diff := differences(t)
	for entry := range parityAllowed {
		assert.Contains(t, diff, entry, "parity exception %q excuses nothing any more", entry)
	}
}
