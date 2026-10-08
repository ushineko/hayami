/*
Package panel joins a reading to its drawing.

core polls and knows nothing about drawing; view describes a section and knows
nothing about polling. A Source is the one place the two meet, and both shells
hold the same set of them. A shell that built its own sections from core's
values would be a shell deciding what a section says, which is the thing the
parity test exists to catch.
*/
package panel

import (
	"context"
	"errors"
	"time"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/view"
)

// Source is a section that polls itself and describes itself.
type Source interface {
	// Key names the section in the settings and on the command line.
	Key() string

	// Poll takes one reading, reporting whether the source has anything to
	// say. A source with nothing to say is not drawn.
	Poll(ctx context.Context) (bool, error)

	// Interval is how often this source wants polling.
	Interval() time.Duration

	// Section is the current reading, drawn.
	Section() view.Section

	// Data is the current reading as plain values, for the JSON the command
	// line prints. It is deliberately not what Section returns: that one is
	// formatted, and a column width is not a fact about a network interface.
	Data() any
}

// Bandwidth is the network interfaces as a Source.
type Bandwidth struct{ *core.BandwidthSection }

// NewBandwidth builds the bandwidth source over the named interfaces. read is
// the source of counters; nil means the kernel's own table.
func NewBandwidth(names []string, read func() (map[string]core.Counters, error)) *Bandwidth {
	return &Bandwidth{core.NewBandwidthSection(names, read)}
}

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
	if errors.Is(b.WirelessErr(), core.ErrWirelessDenied) {
		// Aside: the row already shows what it could read, blank where it
		// could not, and a line on the card about a privacy setting would be
		// there every day on every machine that keeps it. The pointer and
		// doctor say it.
		reasons = append(reasons, view.Reason{
			Label: "Wi-Fi", Text: "details withheld", Status: view.Info, Aside: true,
			Detail: "Windows withholds Wi-Fi details from desktop apps without location access: " +
				"Settings, Privacy & security, Location (ms-settings:privacy-location)",
		})
	}
	sec.Reasons = reasons
	return sec
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

// Data is the last sample as plain values.
func (b *Bandwidth) Data() any { return b.Readings() }

// Option is a setting Sources passes to the source it belongs to.
type Option func(*options)

type options struct {
	lhm string
}

// WithLHM is where LibreHardwareMonitor serves its sensor tree, for the
// processor's temperature on Windows (spec 036). Empty is its default address.
func WithLHM(url string) Option { return func(o *options) { o.lhm = url } }

// Sources builds the sources a configuration asks for, in the order it asks
// for them. A key that names no source is skipped: a settings file written by
// a newer build should not stop an older one starting.
func Sources(keys, interfaces []string, read func() (map[string]core.Counters, error), opts ...Option) []Source {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	var out []Source
	for _, key := range keys {
		switch key {
		case "bandwidth":
			out = append(out, NewBandwidth(interfaces, read))
		case "usage":
			out = append(out, NewUsage())
		case "cooler":
			out = append(out, NewCooler(o.lhm))
		case "peripherals":
			out = append(out, NewPeripherals())
		}
	}
	return out
}

// Keys are the keys of every source this build knows, for the command line's
// help and for the parity test.
func Keys() []string { return []string{"bandwidth", "usage", "cooler", "peripherals"} }
