package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
)

// Spec 076. A source's error goes into doctor's report and the readings JSON
// as it is, outside a reason: neither names the user's home.
func TestAPollErrorDoesNotNameTheUsersHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	failed := errors.New("reading the usage cache: open " + home + "/.cache/claude-usage-widget: permission denied")

	var report bytes.Buffer
	require.NoError(t, Report(&report, []Finding{{Key: "usage", Err: failed}}))
	assert.NotContains(t, report.String(), home)
	assert.Contains(t, report.String(), "~/.cache/claude-usage-widget")

	var readings bytes.Buffer
	src := panel.NewBandwidth([]string{"eth0"}, nil)
	_ = Readings(&readings, []panel.Source{src}, func(panel.Source) error { return failed })
	assert.NotContains(t, readings.String(), home)
	assert.True(t, strings.Contains(readings.String(), "~/.cache/claude-usage-widget"), "%s", readings.String())
}
