package panel

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/view"
)

// Bandwidth is the network interfaces as a Source.
type Bandwidth struct {
	*core.BandwidthSection

	// net builds the reader of the network by process, and reader is the one
	// in use while the list is shown (spec 060). Nil for a source with no
	// list: a test's counters.
	net    func() core.NetReader
	netMu  sync.Mutex
	reader core.NetReader
}

// NewBandwidth builds the bandwidth source over the named interfaces. read is
// the source of counters; nil means the kernel's own table.
func NewBandwidth(names []string, read func() (map[string]core.Counters, error)) *Bandwidth {
	return &Bandwidth{BandwidthSection: core.NewBandwidthSection(names, read)}
}

// SetNetProcesses gives the source a reader of the network by process, which
// is what makes its card open a list.
func (b *Bandwidth) SetNetProcesses(build func() core.NetReader) { b.net = build }

// Section turns the last sample into rows, and the trails into a plot.
//
// HasRate is false until the second poll, because a rate is a difference. The
// distinction survives to here rather than being flattened to zero: a blank of
// the right width says "not yet", and a nought says "nothing is happening",
// and they are not the same claim.
func (b *Bandwidth) Section() view.Section {
	readings := b.Readings()
	out := make([]view.BandwidthReading, 0, len(readings))
	var reasons []view.Reason
	for _, r := range readings {
		if !r.Present {
			// An interface the user named and the kernel does not list. It
			// used to draw a row of blanks -- correct for the column widths
			// and silent about the name being wrong, which after a hardware
			// rename is the ordinary way this happens. The reason takes that
			// row's place rather than sitting under it: two lines about one
			// absent interface is one line too many.
			reasons = append(reasons, view.Reason{
				Label: r.Name, Text: "not present", Status: view.Info,
			})
			continue
		}
		trail := b.Trail(r.Name)
		w, radio := b.Wireless(r.Name)
		out = append(out, view.BandwidthReading{
			Name:     r.Name,
			RxRate:   r.RxRate,
			TxRate:   r.TxRate,
			RxTotal:  r.RxTotal,
			TxTotal:  r.TxTotal,
			HasRate:  r.HasRate,
			HasTotal: r.Present,
			RxTrail:  trail.Rx,
			TxTrail:  trail.Tx,
			Radio:    radio,
			Link:     link(w),
		})
	}
	sec := view.Bandwidth(out)
	if len(readings) == 0 {
		reasons = append(reasons, view.Reason{
			Text: "no interfaces chosen", Status: view.Info,
			Detail: "pick one in the preferences, or pass --sections",
		})
	}
	if err := b.WirelessErr(); errors.Is(err, core.ErrWirelessDenied) {
		// Aside: the row already shows what it could read, blank where it
		// could not, and a line on the card about a privacy setting would be
		// there every day on every machine that keeps it. The pointer and
		// doctor say it; the words are the source's (spec 040).
		reasons = append(reasons, reason(view.Reason{Label: "Wi-Fi", Status: view.Info, Aside: true}, err))
	}
	sec.Reasons = reasons
	if b.net != nil {
		sec.Opens = view.DetailProcesses
	}
	return sec
}

// Details is the network by process, where this source was given a reader.
func (b *Bandwidth) Details() []view.DetailInfo {
	if b.net == nil {
		return nil
	}
	return []view.DetailInfo{{ID: view.DetailProcesses, Title: view.ProcessTitle(view.DetailProcesses)}}
}

// DetailInterval is ProcessInterval.
func (b *Bandwidth) DetailInterval(string) time.Duration { return ProcessInterval }

// CloseDetail lets go of the reader, which on Windows holds a trace session
// open. The next poll of the list builds another.
func (b *Bandwidth) CloseDetail(string) {
	b.netMu.Lock()
	defer b.netMu.Unlock()
	if b.reader != nil {
		b.reader.Close()
		b.reader = nil
	}
}

/*
PollDetail reads the network by process.

**The list is partial and says so** (spec 060 R11). Its note names what it is
made of, the reader's own words, and the share of the watched interfaces'
traffic the list adds up to, so a list that comes to less than the rows above
it reads as partial and not as wrong.
*/
func (b *Bandwidth) PollDetail(ctx context.Context, id string) (view.Detail, error) {
	key, title := b.Key(), view.ProcessTitle(view.DetailProcesses)
	if id != view.DetailProcesses || b.net == nil {
		return view.Detail{}, fmt.Errorf("the %s section has no detail %q", key, id)
	}
	b.netMu.Lock()
	if b.reader == nil {
		b.reader = b.net()
	}
	reader := b.reader
	b.netMu.Unlock()

	use, err := reader.Read(ctx)
	if err != nil {
		return unread(key, id, title, err), err //nolint:wrapcheck // the account the reader gives of the absence
	}
	uses := make([]view.ProcessUse, 0, len(use.Processes))
	listed := 0.0
	for _, p := range use.Processes {
		name := p.Name
		if p.PID == 0 {
			name = ""
		}
		uses = append(uses, view.ProcessUse{Name: name, Value: p.Rx, Second: p.Tx})
		listed += p.Rx + p.Tx
	}
	d := view.Processes(key, id, uses)
	d.Note = netNote(use, listed, b.Readings())
	return d, nil
}

// shareFloor is the least an interface total has to be, in bytes per second,
// before the share of it the list accounts for is worth saying: under it the
// figure is one packet either way.
const shareFloor = 1024

// netNote is the line under the list: what it counts, and the share of the
// interfaces' traffic it comes to.
func netNote(use core.NetUse, listed float64, readings []core.Rates) string {
	if !use.HasRate {
		return use.Counts + ", measuring"
	}
	total, known := 0.0, false
	for _, r := range readings {
		if r.Present && r.HasRate {
			total += r.RxRate + r.TxRate
			known = true
		}
	}
	if !known || total < shareFloor {
		return use.Counts
	}
	share := min(100*listed/total, 100)
	return fmt.Sprintf("%s, %.0f %% of the interfaces' traffic", use.Counts, share)
}

// link is a Wi-Fi description as the view takes it.
func link(w core.Wireless) view.LinkReading {
	return view.LinkReading{
		Connected: w.Connected,
		RSSI:      w.RSSI, HasRSSI: w.HasRSSI,
		Signal: w.Signal, HasSignal: w.HasSignal,
		Band:    w.Band(),
		Channel: w.Channel, HasChannel: w.HasChannel,
		Generation: w.Generation,
		RxRate:     w.RxRate, TxRate: w.TxRate, HasRx: w.HasRx, HasTx: w.HasTx,
	}
}

// Key names the section.
func (b *Bandwidth) Key() string { return view.BandwidthInfo.Key }

// Data is the last sample as plain values.
func (b *Bandwidth) Data() any { return b.Readings() }
