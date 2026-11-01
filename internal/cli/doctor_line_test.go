package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Spec 076. A line of the doctor's report carries no escape sequence from
// what it reports: a device's name, a reason a source gave.
func TestADoctorLineCarriesNoEscape(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, line(&b, "usage\x1b]52;c;aGk=\x07", "ok", "detail\x1b]0;title\x07"))
	assert.NotContains(t, b.String(), "\x1b")
	assert.NotContains(t, b.String(), "\x07")
	assert.Contains(t, b.String(), "detail]0;title")
}
