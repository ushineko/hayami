package gui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/view"
)

// Spec 112 R1, R3. A change's description in the drawer is flowed to the
// drawer's width: each row fits it, the words are all there in order, and
// a row is not cut short of the room by a fixed count of characters.
func TestADescriptionFillsTheDrawersWidth(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	para := strings.TrimSpace(strings.Repeat("Content keeps the existing release status tag and adds another, ", 6))
	c := &view.DevChange{Kind: "pr", Ref: "#1", Title: "a change", State: "open", Body: para}
	page := view.DevChangePage("GitHub", view.DetailAccounts, view.DevOpenPR, "octo/widget", "#1", "", c, false, nil)
	section := view.Usage(now, []view.UsageWindow{{Account: "CC desk", Name: "5h", Fraction: 0.1, ResetsAt: now.Add(time.Hour)}}, now)
	src := newDetailSource("usage", section, map[string]view.Detail{view.DetailAccounts: page})
	p, await := drawerPanel(t, src)

	gui.Tap(p, "usage", "", 0)
	await("the page", func() bool { return gui.DrawerOpen(p) && len(gui.DrawerPartRows(p)) == 2 })
	rows := gui.DrawerPartRows(p)[1]
	require.Greater(t, len(rows), 1, "the paragraph was not flowed")
	room := gui.DrawerRoom(p)
	longest := float32(0)
	for _, r := range rows {
		w := gui.TextWidth(p, r)
		assert.LessOrEqual(t, w, room, "%q is wider than the drawer", r)
		longest = max(longest, w)
	}
	assert.Equal(t, para, strings.Join(rows, " "), "a word was lost or moved")
	assert.Greater(t, longest, room*0.8, "rows stop well short of the drawer's width")
	assert.Greater(t, len([]rune(rows[0])), view.DevWideNameWidth, "still cut at the fixed width")
}
