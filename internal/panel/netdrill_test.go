package panel_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
The peer breakdown's pages (spec 065), pinned through PollDetail, which is
what the shells call: a name's processes and peers, one process, one peer,
and a name gone quiet. Spec 113 R14 moved how these pages are read and
drawn; these say what they draw did not move.

Peers are documentation addresses (RFC 5737), which have no name to find,
so a line names a peer by its address whatever the resolver does.
*/

// netDrillUse is a browser of two processes on eth0, one talking to two
// peers and the other to one of them, and a process of another name.
func netDrillUse(pid int) core.NetUse {
	return core.NetUse{HasRate: true, Counts: "TCP only", Processes: []core.NetProcess{
		{PID: pid, Name: "browser", Iface: "eth0", Rx: 3000, Tx: 300, Peers: []core.NetPeer{
			{Addr: "192.0.2.10", Rx: 2000, Tx: 200, Sockets: []core.NetSocket{
				{LocalPort: 51000, PeerPort: 443, Rx: 2000, Tx: 200},
				{LocalPort: 51001, PeerPort: 443},
			}},
			{Addr: "192.0.2.20", Rx: 1000, Tx: 100, Served: true},
		}},
		{PID: 8, Name: "browser", Iface: "eth0", Rx: 500, Tx: 50, Peers: []core.NetPeer{
			{Addr: "192.0.2.10", Rx: 500, Tx: 50, Sockets: []core.NetSocket{{LocalPort: 52000, PeerPort: 443, Rx: 500, Tx: 50}}},
		}},
		{PID: 9, Name: "mail", Iface: "eth0", Rx: 10, Tx: 10},
	}}
}

// page is a drill's lines as name, value and what each opens, names trimmed
// of the indent that shows their depth.
type pageLine struct{ name, value, open string }

func page(d view.Detail) []pageLine {
	var out []pageLine
	for _, l := range panel.DrillLines(d) {
		out = append(out, pageLine{strings.TrimSpace(l.Name), strings.TrimSpace(l.Value), l.Open})
	}
	return out
}

func names(lines []pageLine) []string {
	var out []string
	for _, l := range lines {
		out = append(out, l.name)
	}
	return out
}

func TestANamesPageIsItsProcessesThenItsPeers(t *testing.T) {
	b := bandwidthWith(t, 8000, 2000, &netReader{use: netDrillUse(7)})

	d, err := b.PollDetail(t.Context(), "peers/eth0/browser")

	require.NoError(t, err)
	require.Len(t, d.Parts, 1, "a drill is one page")
	assert.Equal(t, "Bandwidth > eth0 > browser", d.Parts[0].Title, "the trail heads the page")
	lines := page(d)
	assert.Equal(t, []string{"all", "pid 7", "pid 8", "peers", "192.0.2.10", "← 192.0.2.20"}, names(lines),
		"the sum, the processes greatest first, then the peers greatest first; a client of ours is marked")
	assert.Equal(t, "peers/eth0/browser/7", lines[1].open)
	assert.Equal(t, "peers/eth0/browser/8", lines[2].open)
	assert.Equal(t, "peers/eth0/browser/@192.0.2.10", lines[4].open)
	assert.Empty(t, lines[0].open, "the sum opens nothing")
	assert.Equal(t, "TCP only", d.Note)
}

func TestAProcesssPageIsItsFigureAndItself(t *testing.T) {
	self := os.Getpid()
	b := bandwidthWith(t, 8000, 2000, &netReader{use: netDrillUse(self)})
	id := "peers/eth0/browser/" + strconv.Itoa(self)

	d, err := b.PollDetail(t.Context(), id)

	require.NoError(t, err)
	lines := page(d)
	require.GreaterOrEqual(t, len(lines), 7)
	assert.Equal(t, []string{"now", "pid", "parent", "state", "threads", "sockets"}, names(lines[:6]))
	assert.Equal(t, strconv.Itoa(self), lines[1].value)
	assert.Contains(t, lines[2].value, "("+strconv.Itoa(os.Getppid())+")", "the parent is named with its PID")
	last := lines[len(lines)-1]
	assert.Equal(t, "kill", last.name)
	assert.Equal(t, view.ActKill, last.open)
}

func TestAPageForAProcessThatHasGoneSaysNothingOfIt(t *testing.T) {
	b := bandwidthWith(t, 8000, 2000, &netReader{use: netDrillUse(7)})

	d, err := b.PollDetail(t.Context(), "peers/eth0/browser/2147483646")

	require.NoError(t, err)
	assert.Empty(t, page(d), "no figure and no offer to end it")
}

func TestAPeersPageIsItsConnections(t *testing.T) {
	b := bandwidthWith(t, 8000, 2000, &netReader{use: netDrillUse(7)})

	d, err := b.PollDetail(t.Context(), "peers/eth0/browser/@192.0.2.10")

	require.NoError(t, err)
	assert.Equal(t, "Bandwidth > eth0 > browser > 192.0.2.10", d.Parts[0].Title, "the trail drops the peer's @")
	assert.Equal(t, []pageLine{
		{"all", "↓  2.4 KiB/s ↑  250 B/s", ""},
		{"3 connections, 2 moving", "", ""},
		{"51000 → 443", "↓  2.0 KiB/s ↑  200 B/s", ""},
		{"52000 → 443", "↓  500 B/s   ↑   50 B/s", ""},
	}, page(d), "every connection counted, only the moving ones drawn, greatest first")
}

func TestANameGoneQuietIsIdleAndNotGone(t *testing.T) {
	b := bandwidthWith(t, 8000, 2000, &netReader{use: netDrillUse(7)})

	d, err := b.PollDetail(t.Context(), "peers/eth0/editor")

	require.NoError(t, err)
	assert.Equal(t, []string{"idle", "all"}, names(page(d)))
}
