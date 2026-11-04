package cli_test

import (
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/cli"
	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// A tunnel's addresses are for a person who opened the drawer or named the
// detail. `readings` is what gets pasted into a bug report, and must not carry
// them (spec 085 R9). The loopback interface stands in for a tunnel: its address
// is real and known.
func TestReadingsCarryNoVPNAddress(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("LOCALAPPDATA", dir)
	loopback := loopbackName(t)
	source := panel.NewBandwidth(nil, func() (map[string]core.Counters, error) { return nil, nil })
	source.SetVPNSystems([]core.VPNBackend{{Name: "fake", Active: func(context.Context) ([]core.VPN, error) {
		return []core.VPN{{Name: "a", State: core.VPNUp, Interface: loopback}}, nil
	}}}, nil, config.BandwidthSettings{VPNs: []config.VPNItem{{Name: "a"}}})

	var out bytes.Buffer
	require.NoError(t, cli.Readings(&out, []panel.Source{source}, func(s panel.Source) error {
		_, err := s.Poll(context.Background())
		return err
	}))
	assert.NotContains(t, out.String(), "127.0.0.1")
	assert.Contains(t, out.String(), `"up"`, "the reading itself is there")

	asked, err := source.PollDetail(context.Background(), view.DetailVPN)
	require.NoError(t, err)
	assert.Contains(t, asked.Text(), "127.0.0.1", "and whoever asks for the detail gets the address")
}

func loopbackName(t *testing.T) string {
	t.Helper()
	nics, err := net.Interfaces()
	require.NoError(t, err)
	for _, n := range nics {
		if n.Flags&net.FlagLoopback == 0 {
			continue
		}
		addrs, err := n.Addrs()
		require.NoError(t, err)
		for _, a := range addrs {
			if ip, ok := a.(*net.IPNet); ok && ip.IP.String() == "127.0.0.1" {
				return n.Name
			}
		}
	}
	t.Skip("no loopback interface with 127.0.0.1")
	return ""
}
