package panel

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/readings"
	"github.com/ushineko/hayami/internal/view"
)

const (
	// vpnHealthFile and vpnHealthLock are the store and its lock, in the
	// readings' cache directory.
	vpnHealthFile = "vpn-health.jsonl"
	vpnHealthLock = "vpn-health.lock"

	// vpnCheckEvery and vpnGrace are vpn-toggle's defaults.
	vpnCheckEvery = 120 * time.Second
	vpnGrace      = 15 * time.Second

	// vpnStoreCap is how many points the store keeps, and vpnStoreSlack how far
	// past it an append-only file may run before it is rewritten. Two days at the
	// default interval for each of a dozen tunnels.
	vpnStoreCap   = 10_000
	vpnStoreSlack = 500

	// vpnStoreMaxBytes bounds a read of the store. A point is under 300 bytes,
	// so this is past any file this program wrote: what is read is the tail.
	vpnStoreMaxBytes = 8 << 20
)

// vpnPoint is one check cycle of one VPN, as vpn-toggle's DataPoint is. It holds
// no address (spec 085 R12).
type vpnPoint struct {
	At        time.Time             `json:"at"`
	VPN       string                `json:"vpn"`
	LatencyMS float64               `json:"latency_ms"`
	OK        bool                  `json:"ok"`
	Checks    []core.VPNCheckResult `json:"checks,omitempty"`
}

/*
vpnHealth runs the chosen VPNs' checks and keeps what they said in a file every
panel reads.

**One set of probes, however many panels** (spec 085 R11). A desk runs a panel and
any number of terminal panes. Each cycle takes a non-blocking lock and then asks
the store whether a check is due, which is the arrangement the usage cache has: a
panel that loses the race, or finds the check already made, does nothing and reads
what is there. Nobody holds the lock between cycles, so nothing is handed over
when a panel exits, and a panel that hides the section stops asking and stands in
nobody's way.
*/
type vpnHealth struct {
	checkers core.VPNCheckers
	now      func() time.Time
	running  atomic.Bool

	mu     sync.Mutex
	checks map[string][]core.VPNCheck
	every  time.Duration
	grace  time.Duration

	// The file as last read, so a poll that finds it unchanged parses nothing.
	readSize int64
	readTime time.Time
	read     map[string][]vpnPoint
}

func newVPNHealth(checkers core.VPNCheckers) *vpnHealth {
	return &vpnHealth{checkers: checkers, now: time.Now, every: vpnCheckEvery, grace: vpnGrace}
}

// configure takes the checks and their timing from the settings.
func (h *vpnHealth) configure(settings config.BandwidthSettings) {
	checks := make(map[string][]core.VPNCheck, len(settings.VPNs))
	for _, v := range settings.VPNs {
		for _, c := range v.Checks {
			checks[v.Name] = append(checks[v.Name], core.VPNCheck{
				Type: c.Type, Target: c.Target, Prefix: c.Prefix,
				Timeout: time.Duration(c.TimeoutSeconds) * time.Second,
			})
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks = checks
	h.every, h.grace = vpnCheckEvery, vpnGrace
	if settings.VPNCheckSeconds > 0 {
		h.every = time.Duration(settings.VPNCheckSeconds) * time.Second
	}
	if settings.VPNGraceSeconds > 0 {
		h.grace = time.Duration(settings.VPNGraceSeconds) * time.Second
	}
}

// tick starts a cycle if none is running. It does not wait for it: a check can
// take its whole timeout, and the rates on the same card are read every two
// seconds.
func (h *vpnHealth) tick(ctx context.Context, vpns []core.VPN) {
	if !h.running.CompareAndSwap(false, true) {
		return
	}
	if len(h.due(vpns, h.points())) == 0 {
		h.running.Store(false)
		return
	}
	go func() {
		defer h.running.Store(false)
		h.cycle(ctx, vpns)
	}()
}

// due are the connected VPNs past their grace period whose last check is older
// than the interval. Only a connected tunnel is checked: a down one has nothing
// to answer with.
func (h *vpnHealth) due(vpns []core.VPN, points map[string][]vpnPoint) []core.VPN {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	var out []core.VPN
	for _, v := range vpns {
		if v.State != core.VPNUp || v.Stale || len(h.checks[v.Name]) == 0 {
			continue
		}
		if !v.Since.IsZero() && now.Sub(v.Since) < h.grace {
			continue
		}
		if p := points[v.Name]; len(p) > 0 && now.Sub(p[len(p)-1].At) < h.every {
			continue
		}
		out = append(out, v)
	}
	return out
}

// cycle checks what is due, under the lock. The store is read again once the
// lock is held, because another panel may have finished between the look that
// started this and the lock.
func (h *vpnHealth) cycle(ctx context.Context, vpns []core.VPN) {
	lockPath, err := readings.File(vpnHealthLock)
	if err != nil {
		return
	}
	release, ok, err := readings.TryLock(lockPath)
	if err != nil || !ok {
		return
	}
	defer release()

	for _, v := range h.due(vpns, h.points()) {
		h.mu.Lock()
		checks := h.checks[v.Name]
		h.mu.Unlock()

		start := h.now()
		point := vpnPoint{At: start, VPN: v.Name, OK: true}
		ran := false
		for _, c := range checks {
			result := h.checkers.Run(ctx, c)
			point.Checks = append(point.Checks, result)
			if result.DidNotRun {
				continue
			}
			ran = true
			point.LatencyMS += result.LatencyMS
			point.OK = point.OK && result.OK
		}
		// A cancelled check is a panel closing, which says nothing of the
		// tunnel. A cycle in which nothing could run says nothing either, and
		// is kept only as the reason it could not (spec 085 R23).
		if ctx.Err() != nil {
			return
		}
		point.OK = point.OK && ran
		if err := appendVPNPoint(point); err != nil {
			return
		}
	}
}

// points are what the store holds, by VPN, oldest first. It is read again only
// when the file has changed.
func (h *vpnHealth) points() map[string][]vpnPoint {
	path, err := readings.File(vpnHealthFile)
	if err != nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if h.read != nil && info.Size() == h.readSize && info.ModTime().Equal(h.readTime) {
		return h.read
	}
	loaded, err := loadVPNPoints(path)
	if err != nil {
		return h.read
	}
	h.read, h.readSize, h.readTime = byVPN(loaded), info.Size(), info.ModTime()
	return h.read
}

// current is the last point if it is about the tunnel that is up now: since it
// came up, and recent enough that the checks are still being made.
func (h *vpnHealth) current(points []vpnPoint, since, now time.Time) (vpnPoint, bool) {
	if len(points) == 0 {
		return vpnPoint{}, false
	}
	h.mu.Lock()
	every := h.every
	h.mu.Unlock()
	last := points[len(points)-1]
	if last.At.Before(since) || now.Sub(last.At) > 3*every {
		return vpnPoint{}, false
	}
	if !checksRan(last) {
		return vpnPoint{}, false
	}
	return last, true
}

// checksRan reports whether a point says anything about the tunnel: whether any
// of its checks could be attempted. A point with no record of its checks is one
// from a test.
func checksRan(p vpnPoint) bool {
	if len(p.Checks) == 0 {
		return true
	}
	for _, c := range p.Checks {
		if !c.DidNotRun {
			return true
		}
	}
	return false
}

func byVPN(points []vpnPoint) map[string][]vpnPoint {
	out := map[string][]vpnPoint{}
	for _, p := range points {
		out[p.VPN] = append(out[p.VPN], p)
	}
	return out
}

/*
loadVPNPoints reads the store. A line that does not parse is skipped, which is what
a panel killed mid-write leaves: the last line cut short, and every other line
sound. Only the tail is read if the file has somehow grown past any size this
program writes.
*/
func loadVPNPoints(path string) ([]vpnPoint, error) {
	f, err := os.Open(path) //nolint:gosec // a path readings composed
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening the VPN health store: %w", err)
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err == nil && info.Size() > vpnStoreMaxBytes {
		if _, err := f.Seek(-vpnStoreMaxBytes, io.SeekEnd); err != nil {
			return nil, fmt.Errorf("seeking the VPN health store: %w", err)
		}
	}
	var out []vpnPoint
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		var p vpnPoint
		if err := json.Unmarshal(sc.Bytes(), &p); err == nil && p.VPN != "" {
			out = append(out, p)
		}
	}
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("reading the VPN health store: %w", err)
	}
	return out, nil
}

// appendVPNPoint adds one point, and rewrites the file when it has run past its
// cap. The caller holds the lock, so the rewrite loses nobody's append.
func appendVPNPoint(p vpnPoint) error {
	path, err := readings.File(vpnHealthFile)
	if err != nil {
		return fmt.Errorf("finding the VPN health store: %w", err)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encoding a VPN health point: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // a path readings composed
	if err != nil {
		return fmt.Errorf("opening the VPN health store: %w", err)
	}
	_, werr := f.Write(append(raw, '\n'))
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		return fmt.Errorf("writing the VPN health store: %w", err)
	}
	return compactVPNPoints(path)
}

func compactVPNPoints(path string) error {
	points, err := loadVPNPoints(path)
	if err != nil || len(points) <= vpnStoreCap+vpnStoreSlack {
		return err
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	for _, p := range points[len(points)-vpnStoreCap:] {
		if err := enc.Encode(p); err != nil {
			return fmt.Errorf("encoding the VPN health store: %w", err)
		}
	}
	if err := readings.WriteAtomic(path, b.Bytes()); err != nil {
		return fmt.Errorf("compacting the VPN health store: %w", err)
	}
	return nil
}

// vpnStats sums a VPN's history as view.VPNDetail takes it. A cycle in which no
// check could run counts for neither side.
func vpnStats(points []vpnPoint) view.VPNStats {
	var s view.VPNStats
	total := 0.0
	for _, p := range points {
		if !checksRan(p) {
			continue
		}
		s.Checks++
		total += p.LatencyMS
		if !p.OK {
			s.Failures++
		}
	}
	if s.Checks > 0 {
		s.AverageMS = total / float64(s.Checks)
	}
	return s
}

// vpnDetailTrail is how many points of a VPN's history the drawer plots.
const vpnDetailTrail = 60

// vpnDetail is the drawer's view of the chosen VPNs. Addresses are read here and
// not on the poll: nobody asked for them until now (spec 085 R9).
func (b *Bandwidth) vpnDetail() view.Detail {
	points := b.health.points()
	card := b.vpnReadings()
	vpns := b.vpns.Readings()
	out := make([]view.VPNDetailReading, 0, len(vpns))
	for i, v := range vpns {
		history := points[v.Name]
		r := view.VPNDetailReading{VPNReading: card[i], Stats: vpnStats(history)}
		if len(history) > vpnDetailTrail {
			history = history[len(history)-vpnDetailTrail:]
		}
		for _, p := range history {
			if checksRan(p) {
				r.Samples = append(r.Samples, p.LatencyMS)
				r.Failed = append(r.Failed, !p.OK)
			}
		}
		if len(history) > 0 {
			for _, c := range history[len(history)-1].Checks {
				r.LastChecks = append(r.LastChecks, view.VPNCheckLine{
					Type: c.Type, Latency: c.LatencyMS, OK: c.OK, DidNotRun: c.DidNotRun,
				})
			}
		}
		if v.State == core.VPNUp && v.Interface != "" {
			if a, err := core.ReadTunnelAddressing(v.Interface); err == nil {
				r.Addresses, r.Gateway, r.Routes = a.Addresses, a.Gateway, a.Routes
			}
		}
		out = append(out, r)
	}
	return view.VPNDetail(b.Key(), out)
}
