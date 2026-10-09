package gui_test

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// usageAt is the usage section as read at fetched, drawn at now.
type usageAt struct{ now, fetched time.Time }

func (u *usageAt) Key() string                        { return "usage" }
func (u *usageAt) Interval() time.Duration            { return time.Hour }
func (u *usageAt) Poll(context.Context) (bool, error) { return true, nil }
func (u *usageAt) Data() any                          { return nil }
func (u *usageAt) Section() view.Section {
	return view.Usage(u.now, []view.UsageWindow{
		{Account: "CC max", Name: "5h", Fraction: 0.10, ResetsAt: u.now.Add(2 * time.Hour)},
		{Account: "CC max", Name: "7d", Fraction: 0.08, ResetsAt: u.now.Add(72 * time.Hour)},
	}, u.fetched)
}

/*
Spec 054. A usage reading older than UsageStale is drawn dim, in the lines a
fresh one takes, with its age in the hover note. It used to add a "read …
ago" row above the meter, which came and went with the age and moved the
panel (#172). It is dim as a last-known card is, and not called unavailable:
it is this run's reading, only an old one.
*/
func TestAStaleUsageReadingIsDimInPlaceWithItsAgeInTheTip(t *testing.T) {
	a := test.NewTempApp(t)
	now := time.Now()
	src := &usageAt{now: now, fetched: now.Add(-time.Minute)}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})

	p.Draw("usage", src.Section(), true)
	fresh := gui.CardRows(p, "usage")
	assert.False(t, gui.CardDim(p, "usage"), "a fresh reading is drawn dim")

	src.fetched = now.Add(-12 * time.Minute)
	p.Draw("usage", src.Section(), true)
	assert.True(t, gui.CardDim(p, "usage"), "a stale reading is drawn as though it were current")
	assert.False(t, gui.CardMarked(p, "usage"), "a stale reading is called unavailable")
	assert.Equal(t, fresh, gui.CardRows(p, "usage"), "a stale reading changed the card's rows")
	assert.Contains(t, gui.CardTip(p, "usage"), "read 12m ago", "the age is not a hover away")

	src.fetched = now
	p.Draw("usage", src.Section(), true)
	assert.False(t, gui.CardDim(p, "usage"), "a refreshed reading stayed dim")
}
