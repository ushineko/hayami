package gui_test

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/readings"
	"github.com/ushineko/hayami/internal/view"
)

// reasonSource is a section that has nothing to report and says why.
type reasonSource struct {
	key     string
	rows    []view.Row
	reasons []view.Reason
	note    string
}

func (r *reasonSource) Key() string             { return r.key }
func (r *reasonSource) Title() string           { return r.key }
func (r *reasonSource) Interval() time.Duration { return time.Hour }
func (r *reasonSource) Poll(context.Context) (bool, error) {
	return len(r.rows) > 0, nil
}
func (r *reasonSource) Section() view.Section {
	return view.Section{Key: r.key, Title: r.key, Rows: r.rows, Reasons: r.reasons, Note: r.note}
}
func (r *reasonSource) Data() any { return nil }

/*
A section with nothing but a reason is still drawn.

The fault this spec is about: a card that is hidden and hardware that is absent
look identical, so there is no way to tell a machine with no liquid cooler from
a build that is failing to read one (issue #54).
*/
func TestASectionWithOnlyAReasonIsStillDrawn(t *testing.T) {
	a := test.NewTempApp(t)
	src := &reasonSource{key: "peripherals", reasons: []view.Reason{
		{Text: "no Bluetooth adapter", Status: view.Info},
	}}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})
	require.NotNil(t, p)

	p.Draw("peripherals", src.Section(), false)

	assert.True(t, gui.CardDrawn(p, "peripherals"),
		"a card that could explain itself was hidden instead")
	assert.Contains(t, gui.CardRows(p, "peripherals"), "no Bluetooth adapter")
}

// And a section with nothing at all still is not.
func TestASectionWithNothingAtAllIsNotDrawn(t *testing.T) {
	a := test.NewTempApp(t)
	src := &reasonSource{key: "cooler"}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})

	p.Draw("cooler", src.Section(), false)

	assert.False(t, gui.CardDrawn(p, "cooler"))
}

// A reason's detail is the hover, not the card. A panel that spent two lines
// on an exit status would be a panel about itself.
func TestAReasonsDetailIsTheHoverAndNotTheCard(t *testing.T) {
	a := test.NewTempApp(t)
	src := &reasonSource{key: "cooler",
		rows: []view.Row{{Label: "CPU", Value: "38", Unit: "°C"}},
		reasons: []view.Reason{
			{Label: "Coolant", Text: "no cooler", Detail: `--match "kraken" matched nothing`, Status: view.Info},
		},
	}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})

	p.Draw("cooler", src.Section(), true)

	assert.Contains(t, gui.CardRows(p, "cooler"), "no cooler")
	for _, row := range gui.CardRows(p, "cooler") {
		assert.NotContains(t, row, "kraken")
	}
	assert.Contains(t, gui.CardTip(p, "cooler"), "kraken")
}

/*
A restored reading keeps this poll's reasons.

Both are true at once -- "the coolant is the last one heard" and "the cooler
would not answer" -- and the card should say both. The readings come from the
cache; a reason never does, because it is a statement about now.
*/
func TestARestoredSectionKeepsThisPollsReasons(t *testing.T) {
	a := test.NewTempApp(t)
	src := &reasonSource{key: "cooler", reasons: []view.Reason{
		{Text: "the cooler would not answer", Status: view.Warn},
	}}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})
	gui.Seed(p, readings.Cache{"cooler": {At: time.Now(), Section: view.Section{
		Key: "cooler", Title: "cooler", Rows: []view.Row{{Label: "CPU", Value: "38", Unit: "°C"}},
	}}})

	p.Draw("cooler", src.Section(), false)

	rows := gui.CardRows(p, "cooler")
	assert.Contains(t, rows, "38 °C", "the cached reading was dropped")
	assert.Contains(t, rows, "the cooler would not answer", "this poll's reason was dropped")
}
