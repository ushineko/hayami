package core

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sensorLines(t *testing.T, lines ...SensorHelperLine) string {
	t.Helper()
	var b strings.Builder
	for _, l := range lines {
		j, err := json.Marshal(l)
		require.NoError(t, err)
		b.Write(j)
		b.WriteByte('\n')
	}
	return b.String()
}

// Spec 103. The panel keeps the helper's latest reading, and a reading that
// has gone stale is no reading.
func TestTheSensorStreamKeepsTheLatestReading(t *testing.T) {
	now := time.Unix(1000, 0)
	s := newSensorStream(func() time.Time { return now })
	hello := SensorHelperLine{V: SensorHelperProtocol, Version: "1.0.0"}
	// A pipe that ends is no reading, whatever came before the end: the
	// helper has stopped.
	s.run(strings.NewReader(sensorLines(t, hello, SensorHelperReading(45, "Core (Tctl/Tdie)"))))
	<-s.got
	_, _, ended, err := s.latest()
	assert.True(t, ended)
	require.Error(t, err)

	s = newSensorStream(func() time.Time { return now })
	require.NoError(t, s.read(strings.NewReader(sensorLines(t, hello, SensorHelperReading(47.5, "Core (Tctl/Tdie)")))))
	c, label, ended, err := s.latest()
	require.NoError(t, err)
	assert.False(t, ended)
	assert.InDelta(t, 47.5, c, 1e-9)
	assert.Equal(t, "Core (Tctl/Tdie)", label)

	now = now.Add(sensorStale + time.Second)
	_, _, _, err = s.latest()
	require.Error(t, err, "a stale reading")
}

// A helper of another protocol, and one that says why it stopped, are
// errors that end the stream.
func TestTheSensorStreamRefusesAnotherVersionAndPassesAnError(t *testing.T) {
	s := newSensorStream(time.Now)
	err := s.read(strings.NewReader(sensorLines(t, SensorHelperLine{V: 9})))
	require.ErrorContains(t, err, "version 9")

	s = newSensorStream(time.Now)
	err = s.read(strings.NewReader(sensorLines(t, SensorHelperLine{V: SensorHelperProtocol}, SensorHelperLine{Error: "PawnIO is not installed"})))
	require.ErrorContains(t, err, "PawnIO is not installed")
}

// The panel bounds what a helper sends: its error reaches a terminal as
// words, and a reading no processor gives is not kept.
func TestTheSensorStreamBoundsWhatTheHelperSends(t *testing.T) {
	s := newSensorStream(time.Now)
	err := s.read(strings.NewReader(sensorLines(t, SensorHelperLine{V: SensorHelperProtocol},
		SensorHelperLine{Error: "boom\x1b]52;c;aGVsbG8=\x07\x1b[2J done"})))
	require.Error(t, err)
	for _, r := range err.Error() {
		assert.False(t, unicode.IsControl(r), "control character %U in %q", r, err.Error())
	}

	s = newSensorStream(time.Now)
	require.NoError(t, s.read(strings.NewReader(sensorLines(t, SensorHelperLine{V: SensorHelperProtocol},
		SensorHelperReading(400, "Core (Tctl/Tdie)"), SensorHelperReading(-300, "Core (Tctl/Tdie)")))))
	_, _, _, err = s.latest()
	require.Error(t, err, "an impossible reading was kept")
}
