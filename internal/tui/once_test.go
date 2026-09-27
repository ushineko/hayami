package tui_test

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/tui"
	"github.com/ushineko/hayami/internal/view"
)

// slowSource is a section that takes a while to answer, as a real one does:
// a subprocess, a device that has to be woken, a cache read.
type slowSource struct {
	key   string
	takes time.Duration
	says  bool
}

func (s *slowSource) Key() string { return s.key }

func (s *slowSource) Poll(context.Context) (bool, error) {
	time.Sleep(s.takes)
	return s.says, nil
}

// Interval is long but not unbounded. Nothing in these tests waits for a
// reschedule, and a source that asked to be polled again in an hour would make
// any test that did run for an hour.
func (s *slowSource) Interval() time.Duration { return time.Minute }
func (s *slowSource) Section() view.Section {
	return view.Section{Key: s.key, Title: s.key, Rows: []view.Row{{Label: s.key, Value: "1"}}}
}
func (s *slowSource) Data() any { return nil }

// run drives the model the way bubbletea does: Init hands back a batch of
// commands, each command produces a message, and each message goes to Update.
//
// It is written this way rather than by constructing the messages directly
// because the messages are the package's own business. What the program does
// is run the commands Init gave it, and that is what this reproduces.
func run(t *testing.T, m tui.Model, within time.Duration) tui.Model {
	t.Helper()

	cmds := expand(m.Init())
	msgs := make(chan tea.Msg, len(cmds))
	for _, c := range cmds {
		go func(c tea.Cmd) { msgs <- c() }(c)
	}

	deadline := time.After(within)
	for i := 0; i < len(cmds); i++ {
		select {
		case msg := <-msgs:
			updated, cmd := m.Update(msg)
			m = updated.(tui.Model)
			if quits(cmd) {
				return m
			}
		case <-deadline:
			t.Fatal("the frame never finished")
		}
	}
	return m
}

// expand flattens the batch Init returns into the commands it holds.
func expand(cmd tea.Cmd) []tea.Cmd {
	if cmd == nil {
		return nil
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		return batch
	}
	return []tea.Cmd{cmd}
}

// quits reports whether a command ends the program.
//
// It runs the command to find out, which is safe only where the command can
// only be nil or a quit. Under --once that is all Update returns; the
// reschedule it returns otherwise is a timer, and running one of those waits
// out its interval.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// The bug. --once batches a poll per source and they answer in whatever order
// they finish; quitting on the first message drew whichever won the race and
// dropped every other section.
//
// With the default sections that was bandwidth answering "nothing yet", so the
// frame had no sections in it at all — which is what anybody trying --once for
// the first time saw.
func TestOnceWaitsForEverySectionAndNotJustTheFirst(t *testing.T) {
	sources := []panel.Source{
		// Answers immediately and has nothing to say, as bandwidth does
		// before an interface is named. This is the one that used to win.
		&slowSource{key: "bandwidth", takes: 0, says: false},
		&slowSource{key: "cooler", takes: 40 * time.Millisecond, says: true},
		&slowSource{key: "peripherals", takes: 80 * time.Millisecond, says: true},
	}

	m := run(t, tui.New(tui.Options{Sources: sources, Once: true}), 2*time.Second)

	var keys []string
	for _, s := range m.Sections() {
		keys = append(keys, s.Key)
	}
	assert.Equal(t, []string{"cooler", "peripherals"}, keys,
		"the frame drew only the section that answered first")
}

// A source with nothing to say is still a source that answered. The frame
// waits for it and then leaves it out, which is what the panel does for a
// machine without the hardware.
func TestASourceWithNothingToSayStillEndsTheWait(t *testing.T) {
	sources := []panel.Source{
		&slowSource{key: "cooler", takes: 0, says: true},
		&slowSource{key: "peripherals", takes: 0, says: false},
	}

	m := run(t, tui.New(tui.Options{Sources: sources, Once: true}), 2*time.Second)

	require.Len(t, m.Sections(), 1)
	assert.Equal(t, "cooler", m.Sections()[0].Key)
}

// A prompt that hangs is worse than a prompt missing a reading. A source that
// never answers is left out rather than allowed to hold the frame.
func TestOnceIsBoundedWhenASourceNeverAnswers(t *testing.T) {
	sources := []panel.Source{
		&slowSource{key: "cooler", takes: 0, says: true},
		&slowSource{key: "wedged", takes: time.Hour, says: true},
	}

	started := time.Now()
	m := run(t, tui.New(tui.Options{Sources: sources, Once: true}),
		tui.OnceDeadline+5*time.Second)

	assert.Less(t, time.Since(started), tui.OnceDeadline+2*time.Second)

	require.Len(t, m.Sections(), 1)
	assert.Equal(t, "cooler", m.Sections()[0].Key,
		"the frame drew what had arrived by the deadline")
}

// Without --once nothing waits and nothing quits: each source keeps its own
// cadence and reschedules itself.
func TestWithoutOnceEachSourceReschedulesItself(t *testing.T) {
	sources := []panel.Source{&slowSource{key: "cooler", takes: 0, says: true}}

	m := tui.New(tui.Options{Sources: sources, Once: false})
	for _, c := range expand(m.Init()) {
		updated, cmd := m.Update(c())
		m = updated.(tui.Model)

		// The command is the next poll's timer and is deliberately not run:
		// running it would wait out the interval. That it exists at all is the
		// claim — a panel that is not --once reschedules rather than quitting.
		require.NotNil(t, cmd, "a panel that is not --once did not reschedule")
	}
	require.Len(t, m.Sections(), 1)
}
