package panel_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// uptimes answers each poll with the next of answers, an error where it is
// zero, and the last for every poll after.
func uptimes(answers ...time.Duration) func(context.Context) (time.Duration, error) {
	call := 0
	return func(context.Context) (time.Duration, error) {
		d := answers[min(call, len(answers)-1)]
		call++
		if d == 0 {
			return 0, errors.New("unreadable")
		}
		return d, nil
	}
}

func lastRow(s view.Section) view.Row { return s.Rows[len(s.Rows)-1] }

// Spec 099 R1, R4, R6. The host's uptime is the card's last row and is in
// the card's JSON.
func TestTheCoolerReadsTheUptime(t *testing.T) {
	h := detailHost(stepping(nil), &gpuReader{}, "")
	h.Uptime = uptimes(3*24*time.Hour + 4*time.Hour + 12*time.Minute)
	c := panel.NewCoolerOn(h, noScan)

	drawn, _ := c.Poll(t.Context())
	require.True(t, drawn)

	row := lastRow(c.Section())
	assert.Equal(t, view.UptimeID, row.ID)
	assert.Equal(t, " 3d 04h 12m", row.Value)

	body, err := json.Marshal(c.Data())
	require.NoError(t, err)
	assert.Contains(t, string(body), `"uptime":{"v":274320000000000,"ok":true}`)
}

// Spec 099 R5. An uptime read once and then not is kept, so the row does not
// leave and come back; the failure is an aside.
func TestAnUptimeThatStopsAnsweringKeepsItsRow(t *testing.T) {
	h := detailHost(stepping(nil), &gpuReader{}, "")
	h.Uptime = uptimes(time.Hour, 0)
	c := panel.NewCoolerOn(h, noScan)

	_, _ = c.Poll(t.Context())
	_, _ = c.Poll(t.Context())
	s := c.Section()

	assert.Equal(t, view.UptimeID, lastRow(s).ID)
	assert.Equal(t, "    01h 00m", lastRow(s).Value)
	var found bool
	for _, r := range s.Reasons {
		found = found || (r.Label == "Uptime" && r.Aside)
	}
	assert.True(t, found, "the failed read is an aside reason")
}

// Spec 099 R4, R5. An uptime never read is no row.
func TestAnUptimeNeverReadIsNoRow(t *testing.T) {
	h := detailHost(stepping(nil), &gpuReader{}, "")
	h.Uptime = uptimes(0)
	c := panel.NewCoolerOn(h, noScan)

	_, _ = c.Poll(t.Context())

	for _, r := range c.Section().Rows {
		assert.NotEqual(t, view.UptimeID, r.ID)
	}
}

// Spec 099 R7. The uptime alone does not make a Hardware card.
func TestAnUptimeAloneIsNoCard(t *testing.T) {
	h := fakeHost()
	h.Uptime = uptimes(time.Hour)
	c := panel.NewCoolerOn(h, noScan)

	drawn, _ := c.Poll(t.Context())

	assert.False(t, drawn)
}
