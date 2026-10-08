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
	// The row arrangement is one line per reading (view.renderRow), the shape a
	// session manager's pane wants (spec 019): no section headings, and no
	// second line under a reading for its totals or its link.
	"row: title":  "the row arrangement draws no section headings",
	"row: detail": "the row arrangement is one line per reading; detail lines are the stacked shapes'",
	// A usage meter in a row is the usage widget's line (spec 019): the same
	// figures, spelled "10%" and "resets Oct 7" where the stacked shapes say
	// "5h: 10 %" and "in 2h 0m".
	"row: meter caption": "the row arrangement spells a meter the usage widget's way (spec 019)",
	"row: meter reset":   "the row arrangement spells a meter the usage widget's way (spec 019)",
	"row: meter stats":   "the row arrangement spells a meter the usage widget's way (spec 019)",
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
Spec 047. The window and the terminal draw the same thing for every section:
every label, value, detail line, cell and meter a section holds is on the
window's card and in the terminal's rendering, in each arrangement. A fact one
of them leaves out is a failure unless parityAllowed says why.

Before this the parity test checked that every section key built a source
under that key, which was true by construction and said nothing about what
either shell drew.

The window is drawn headlessly. Its two arrangements, stack and grid, lay the
same cards out differently and draw the same cards, so one drawing stands for
both; row is the terminal's (see the allow-list and spec 047).
*/
func TestTheWindowAndTheTerminalDrawTheSameFacts(t *testing.T) {
	sections := paritySections(t)

	a := test.NewTempApp(t)
	sources := make([]panel.Source, 0, len(sections))
	for _, s := range sections {
		sources = append(sources, &fixed{s})
	}
	p := gui.New(a, gui.Options{Sources: sources, Title: "hayami"})

	for _, s := range sections {
		t.Run(s.Key, func(t *testing.T) {
			p.Draw(s.Key, s, true)
			window := squeeze(strings.Join(gui.CardText(p, s.Key), ""))
			require.NotEmpty(t, window, "the window drew nothing for %s", s.Key)

			terminal := map[string]string{}
			for _, arr := range []view.Arrangement{view.ArrangeStack, view.ArrangeGrid, view.ArrangeRow} {
				terminal[arr.String()] = squeeze(strings.Join(view.Render([]view.Section{s}, arr, parityWidth), ""))
			}

			for _, f := range facts(s) {
				if !windowDraws(window, f) {
					if _, ok := parityAllowed["window: "+f.kind]; !ok {
						assert.Failf(t, "the window does not draw it", "%s: %s", s.Key, f)
					}
				}
				for arr, drawn := range terminal {
					if strings.Contains(drawn, squeeze(f.text)) {
						continue
					}
					if _, ok := parityAllowed[arr+": "+f.kind]; !ok {
						assert.Failf(t, "the terminal does not draw it", "%s in %s: %s", s.Key, arr, f)
					}
				}
			}
		})
	}
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

// Every exception is still needed: one that no longer excuses anything is a
// rule nobody is holding. The terminal's are checked here against its
// rendering; the window has none.
func TestEveryParityExceptionIsUsed(t *testing.T) {
	used := map[string]bool{}
	for _, s := range paritySections(t) {
		for _, arr := range []view.Arrangement{view.ArrangeStack, view.ArrangeGrid, view.ArrangeRow} {
			drawn := squeeze(strings.Join(view.Render([]view.Section{s}, arr, parityWidth), ""))
			for _, f := range facts(s) {
				if !strings.Contains(drawn, squeeze(f.text)) {
					used[arr.String()+": "+f.kind] = true
				}
			}
		}
	}
	for entry := range parityAllowed {
		assert.True(t, used[entry], "parity exception %q excuses nothing any more", entry)
	}
}
