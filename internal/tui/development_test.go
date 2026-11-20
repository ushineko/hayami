package tui_test

import (
	"context"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// devSource is the Development section with one forge, a fortnight read,
// counting the days opened and the refreshes asked for.
type devSource struct {
	mu        sync.Mutex
	opened    []string
	refreshes int
}

var devNow = time.Date(2026, 3, 14, 12, 0, 0, 0, time.Local)

func devReading() view.DevReading {
	r := view.DevReading{Provider: "github", Title: "GitHub", Unit: "contributions", ReadAt: devNow}
	for i := range 14 {
		r.Days = append(r.Days, view.DevDay{Date: time.Date(2026, 3, 1+i, 0, 0, 0, 0, time.Local).Format(time.DateOnly), Count: i + 1})
	}
	r.Items = []view.DevItem{{Date: "2026-03-11", Repo: "octo/widget", Kind: "pr", Ref: "#3", Title: "a gadget", Action: "merged", Count: 1}}
	return r
}

func (s *devSource) Key() string                        { return "usage" }
func (s *devSource) Poll(context.Context) (bool, error) { return true, nil }
func (s *devSource) Interval() time.Duration            { return time.Hour }
func (s *devSource) Data() any                          { return nil }
func (s *devSource) Section() view.Section {
	return view.Section{Key: "usage", Title: "Development", Opens: view.DetailAccounts,
		Rows: []view.Row{{Label: "CC", Value: "1 %"}}}
}
func (s *devSource) Details() []view.DetailInfo {
	return []view.DetailInfo{{ID: view.DetailAccounts, Title: view.DevTitle}}
}
func (s *devSource) PollDetail(_ context.Context, id string) (view.Detail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, date, ok := view.ParseDevDayID(id); ok {
		s.opened = append(s.opened, date)
		return view.DevDayPage(devReading(), devNow, date), nil
	}
	return view.Development(nil, []view.Detail{view.DevPart(devReading(), devNow, -1)}), nil
}
func (s *devSource) DetailInterval(string) time.Duration { return time.Hour }
func (s *devSource) CloseDetail(string)                  {}
func (s *devSource) Drills(id string) bool {
	_, _, ok := view.ParseDevDayID(id)
	return ok
}
func (s *devSource) DrillUp(string) string { return view.DetailAccounts }
func (s *devSource) Refresh(ask string) bool {
	if _, ok := view.ParseRefresh(ask); !ok {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshes++
	return true
}

var _ panel.Refresher = (*devSource)(nil)

// Spec 102 R13, AC11. In the terminal the strip is a line: ↓ marks it,
// ←/→ move the day, Enter opens that day, Esc goes back; Enter on the
// heading reads the forge again.
func TestTheTerminalWalksTheDays(t *testing.T) {
	src := &devSource{}
	m := pane(t, view.ArrangeStack, 80, 40, src)
	m, _ = press(t, m, keyList("tab", "enter")...)
	require.Contains(t, text(m), "GitHub")

	m, _ = press(t, m, key("down"), key("enter"))
	assert.Equal(t, 1, src.refreshes, "Enter on the heading did not read again")

	m, _ = press(t, m, key("down"))
	assert.Contains(t, text(m), "[█]", "the strip's line marks today")
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyLeft}, tea.KeyMsg{Type: tea.KeyLeft}, tea.KeyMsg{Type: tea.KeyLeft})
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyRight}, key("enter"))
	require.NotEmpty(t, src.opened)
	assert.Equal(t, "2026-03-12", src.opened[len(src.opened)-1])
	assert.Contains(t, text(m), "Thursday 12 March")

	m, _ = press(t, m, key("esc"))
	assert.NotContains(t, text(m), "Thursday 12 March")
	assert.Contains(t, text(m), "contributions, most 14")
}
