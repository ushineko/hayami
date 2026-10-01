package desktop_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hayami/internal/desktop"
)

// The restore rides the watch's bus connection, so asking for it first is
// refused rather than dereferenced. The compositor itself is not in these
// tests; what it does with the scripts is recorded on the desk in spec 029.
func TestRestoringBeforeWatchingIsRefused(t *testing.T) {
	pos := desktop.NewPosition(appID)

	err := pos.Restore(10, 20)
	assert.ErrorIs(t, err, desktop.ErrNoKWin)
}

// Stopping what was never started is nothing, so a panel that failed to
// watch can still call what rememberPosition handed back.
func TestStoppingAnUnwatchedPositionIsNothing(t *testing.T) {
	pos := desktop.NewPosition(appID)
	assert.NotPanics(t, pos.Stop)
}
