package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/hayami/internal/api/contract"
	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/safetext"
	"github.com/ushineko/hayami/internal/view"
)

/*
Hub holds one source per section and polls each only while someone reads it
(spec 113 R9-R11, R31).

**A lease, not a subscription count.** A section is leased while a request
for it came within the last three of its intervals (never less than
MinLease), while a stream names it, or while one of its details is leased: a
detail's offer depends on the section's reading. A detail is leased the same
way at its own interval. When a lease lapses the poller stops, the reading
is let go, and a detail's CloseDetail is called; the next request starts it
again and waits for a fresh reading.

**One poller per source.** A section's poller is the only goroutine that
polls it, so a source is never polled twice at once, and any number of
clients cause one set of polls. A detail's reads take the source's read lock,
so two details of one source are never read at once either. A request is
answered from the latest snapshot and waits only when there is none yet.
*/
type Hub struct {
	ctx context.Context
	log *slog.Logger

	// now is the clock; MinLease, FirstWait and PollTimeout the times above,
	// all replaced by tests.
	now         func() time.Time
	MinLease    time.Duration
	FirstWait   time.Duration
	PollTimeout time.Duration

	mu           sync.Mutex
	settings     config.Config
	order        []string
	sections     map[string]*section
	lastActivity time.Time
}

// section is one source and what the hub knows of it. Every field after
// readMu is under Hub.mu.
type section struct {
	key string
	src panel.Source

	// readMu is held for a detail's read: one at a time per source.
	readMu sync.Mutex

	snap     *contract.Section
	ready    chan struct{}
	isReady  bool
	touched  time.Time
	watchers int
	running  bool
	subs     map[*subscriber]struct{}
	details  map[string]*detail
}

// detail is one detail of a section, under Hub.mu.
type detail struct {
	id       string
	snap     *contract.Detail
	ready    chan struct{}
	isReady  bool
	touched  time.Time
	watchers int
	running  bool
	subs     map[*subscriber]struct{}
}

// The hub's defaults.
const (
	// MinLease is the shortest a request keeps a section read (R9).
	MinLease = 30 * time.Second
	// FirstWait is the longest a first request waits for a reading (R11).
	FirstWait = 15 * time.Second
	// PollTimeout bounds one poll of a source.
	PollTimeout = 30 * time.Second
)

// NewHub holds sources, in the order given, under the settings given. ctx
// ends every poller.
func NewHub(ctx context.Context, sources []panel.Source, settings config.Config, log *slog.Logger) *Hub {
	h := &Hub{
		ctx: ctx, log: log, now: time.Now,
		MinLease: MinLease, FirstWait: FirstWait, PollTimeout: PollTimeout,
		settings: settings, sections: map[string]*section{},
	}
	for _, src := range sources {
		key := src.Key()
		h.order = append(h.order, key)
		h.sections[key] = &section{key: key, src: src, subs: map[*subscriber]struct{}{}, details: map[string]*detail{}}
	}
	h.lastActivity = h.now()
	return h
}

// SetClock replaces the hub's clock, for a test.
func (h *Hub) SetClock(now func() time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.now = now
}

// Keys are the sections, in order.
func (h *Hub) Keys() []string { return append([]string(nil), h.order...) }

// SectionMeta is a section as the registry names it, and how often its
// source is polled.
type SectionMeta struct {
	Key, Title string
	Default    bool
	Interval   time.Duration
}

// Infos are the sections, in order: their titles and defaults from the
// registry, a section it does not know named by its key.
func (h *Hub) Infos() []SectionMeta {
	known := map[string]view.SectionInfo{}
	for _, info := range view.Sections() {
		known[info.Key] = info
	}
	out := make([]SectionMeta, 0, len(h.order))
	for _, key := range h.order {
		info, ok := known[key]
		if !ok {
			info = view.SectionInfo{Key: key, Title: key}
		}
		out = append(out, SectionMeta{Key: key, Title: info.Title, Default: info.Default, Interval: h.sections[key].src.Interval()})
	}
	return out
}

// Settings are the settings the sources read now.
func (h *Hub) Settings() config.Config {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.settings
}

// SetSettings hands every source a changed settings file (R8).
func (h *Hub) SetSettings(c config.Config) {
	h.mu.Lock()
	h.settings = c
	h.mu.Unlock()
	for _, key := range h.order {
		panel.ApplySettings(h.sections[key].src, c)
	}
}

// touch marks a request now, for the idle exit.
func (h *Hub) touchLocked() time.Time {
	now := h.now()
	h.lastActivity = now
	return now
}

// Idle is how long since the last request, and whether a stream is open:
// a hub with an open stream is never idle.
func (h *Hub) Idle() (time.Duration, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.sections {
		if s.watchers > 0 {
			return 0, false
		}
		for _, d := range s.details {
			if d.watchers > 0 {
				return 0, false
			}
		}
	}
	return h.now().Sub(h.lastActivity), true
}

// lease is how long a request keeps something read whose interval is every.
func (h *Hub) lease(every time.Duration) time.Duration {
	return max(3*every, h.MinLease)
}

func (h *Hub) sectionLeasedLocked(s *section, now time.Time) bool {
	if s.watchers > 0 || now.Sub(s.touched) < h.lease(s.src.Interval()) {
		return true
	}
	for _, d := range s.details {
		if h.detailLeasedLocked(s, d, now) {
			return true
		}
	}
	return false
}

func (h *Hub) detailLeasedLocked(s *section, d *detail, now time.Time) bool {
	every := s.src.Interval()
	if dt, ok := s.src.(panel.Detailer); ok {
		every = dt.DetailInterval(d.id)
	}
	return d.watchers > 0 || now.Sub(d.touched) < h.lease(every)
}

// startSectionLocked starts a section's poller if it is not running, and
// returns what its first reading closes.
func (h *Hub) startSectionLocked(s *section) chan struct{} {
	if !s.running {
		s.running, s.snap, s.isReady = true, nil, false
		s.ready = make(chan struct{})
		go h.runSection(s)
	}
	return s.ready
}

// wait waits for ready, the hub's first wait, or ctx.
func (h *Hub) wait(ctx context.Context, ready chan struct{}) {
	t := time.NewTimer(h.FirstWait)
	defer t.Stop()
	select {
	case <-ready:
	case <-t.C:
	case <-ctx.Done():
	}
}

// ErrUnknown is a section or a detail that is not there; Known lists what
// is.
type ErrUnknown struct {
	What  string
	Known []string
}

func (e *ErrUnknown) Error() string { return "no " + e.What }

// ErrBusy is a request over one of the hub's limits; the client comes back.
type ErrBusy struct{ What string }

func (e *ErrBusy) Error() string { return e.What }

// MaxDetails is the most details of one section read at once, and
// MaxStreamDetails the most one stream may name.
const (
	MaxDetails       = 32
	MaxStreamDetails = 16
)

// Section is one section's latest reading, waiting for the first if there is
// none yet.
func (h *Hub) Section(ctx context.Context, key string) (contract.Section, error) {
	h.mu.Lock()
	s, ok := h.sections[key]
	if !ok {
		h.mu.Unlock()
		return contract.Section{}, &ErrUnknown{What: fmt.Sprintf("section %q", key), Known: h.Keys()}
	}
	s.touched = h.touchLocked()
	ready := h.startSectionLocked(s)
	h.mu.Unlock()

	h.wait(ctx, ready)

	h.mu.Lock()
	defer h.mu.Unlock()
	if s.snap == nil {
		return unreadSection(s), nil
	}
	return *s.snap, nil
}

// unreadSection is a section with no reading yet.
func unreadSection(s *section) contract.Section {
	return contract.Section{
		Key: s.key, IntervalS: s.src.Interval().Seconds(), Absences: []contract.Absence{},
		Error: ptr("not read yet"), Details: []contract.DetailRef{},
	}
}

// runSection polls a section until its lease lapses or the hub ends.
func (h *Hub) runSection(s *section) {
	for {
		snap := h.readSection(s)

		h.mu.Lock()
		s.snap = snap
		if !s.isReady {
			s.isReady = true
			close(s.ready)
		}
		subs := subscribers(s.subs)
		h.mu.Unlock()
		for _, sub := range subs {
			sub.offer(contract.EventSection+"/"+s.key, contract.EventSection, snap)
		}

		if !h.sleep(panel.Wait(s.src, h.clock())) {
			h.mu.Lock()
			s.running = false
			h.mu.Unlock()
			return
		}
		h.mu.Lock()
		if !h.sectionLeasedLocked(s, h.now()) {
			s.running, s.snap = false, nil
			h.mu.Unlock()
			return
		}
		h.mu.Unlock()
	}
}

// clock is the hub's time now.
func (h *Hub) clock() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.now()
}

// sleep waits d, and reports false when the hub ended first.
func (h *Hub) sleep(d time.Duration) bool {
	t := time.NewTimer(max(d, 10*time.Millisecond))
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-h.ctx.Done():
		return false
	}
}

// errPanicked is a poll that panicked; the service keeps running (R31).
var errPanicked = errors.New("the reading failed unexpectedly")

/*
readSection is one poll of a section's source and its snapshot. A panic
anywhere in it -- the poll, the reading, the reasons, the details offered --
is the section's error, and the service keeps running (R31).
*/
func (h *Hub) readSection(s *section) (snap *contract.Section) {
	at := h.clock()
	defer func() {
		if r := recover(); r != nil {
			h.log.Error("a section's read panicked", "section", s.key, "panic", safetext.WithoutHome(fmt.Sprint(r)))
			out := unreadSection(s)
			out.PolledAt, out.Error = when(at), errorText(errPanicked)
			snap = &out
		}
	}()
	ctx, cancel := context.WithTimeout(h.ctx, h.PollTimeout)
	defer cancel()
	_, err := s.src.Poll(ctx)
	snap = &contract.Section{
		Key: s.key, PolledAt: when(at), IntervalS: s.src.Interval().Seconds(),
		Reading: mapReading(s.key, s.src, at), Absences: absences(s.src.Section().Reasons),
		Error: errorText(err), Details: detailRefs(s.src),
	}
	snap.Truncated = cleanText(snap)
	return snap
}

// Detail is one detail's latest reading, waiting for the first if there is
// none yet.
func (h *Hub) Detail(ctx context.Context, key, id string) (contract.Detail, error) {
	s, d, err := h.leaseDetail(ctx, key, id, false)
	if err != nil {
		return contract.Detail{}, err
	}
	h.mu.Lock()
	ready := d.ready
	h.mu.Unlock()
	h.wait(ctx, ready)

	h.mu.Lock()
	defer h.mu.Unlock()
	if d.snap == nil {
		out := contract.Detail{
			Section: s.key, ID: id, Title: id, Absences: []contract.Absence{}, Children: []string{},
			Error: ptr("not read yet"),
		}
		out.Truncated = cleanText(&out)
		return out, nil
	}
	return *d.snap, nil
}

/*
leaseDetail finds a detail a section offers, leases it (watch adds a
watcher, for a stream) and starts its poller. A section not read yet is read
first, because what it offers depends on its reading.
*/
func (h *Hub) leaseDetail(ctx context.Context, key, id string, watch bool) (*section, *detail, error) {
	h.mu.Lock()
	s, ok := h.sections[key]
	if !ok {
		h.mu.Unlock()
		return nil, nil, &ErrUnknown{What: fmt.Sprintf("section %q", key), Known: h.Keys()}
	}
	s.touched = h.touchLocked()
	ready := h.startSectionLocked(s)
	h.mu.Unlock()
	h.wait(ctx, ready)

	rd, ok := s.src.(panel.RawDetailer)
	if !ok || !panel.Offers(s.src, id) {
		return nil, nil, &ErrUnknown{What: fmt.Sprintf("detail %q in %s", id, key), Known: offered(s.src)}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := s.details[id]
	if !ok {
		// A drill's ID is a path a client may make up, and each one read is
		// a poller and a read of the source: a section reads at most
		// MaxDetails at once, and the rest are told to come back.
		if len(s.details) >= MaxDetails {
			return nil, nil, &ErrBusy{What: fmt.Sprintf("%d details of %s are being read", MaxDetails, key)}
		}
		d = &detail{id: id, subs: map[*subscriber]struct{}{}}
		s.details[id] = d
	}
	d.touched = h.touchLocked()
	if watch {
		d.watchers++
	}
	if !d.running {
		d.running, d.snap, d.isReady = true, nil, false
		d.ready = make(chan struct{})
		go h.runDetail(s, rd, d)
	}
	return s, d, nil
}

// offered are the IDs a source offers.
func offered(src panel.Source) []string {
	out := []string{}
	if d, ok := src.(panel.Detailer); ok {
		for _, info := range d.Details() {
			out = append(out, info.ID)
		}
	}
	return out
}

// runDetail reads a detail until its lease lapses, then closes it.
func (h *Hub) runDetail(s *section, rd panel.RawDetailer, d *detail) {
	defer rd.CloseDetail(d.id)
	for {
		snap := h.readDetail(s, rd, d.id)

		h.mu.Lock()
		d.snap = snap
		if !d.isReady {
			d.isReady = true
			close(d.ready)
		}
		subs := subscribers(d.subs)
		h.mu.Unlock()
		for _, sub := range subs {
			sub.offer(contract.EventDetail+"/"+s.key+"/"+d.id, contract.EventDetail, snap)
		}

		if !h.sleep(rd.DetailInterval(d.id)) {
			h.mu.Lock()
			d.running = false
			h.mu.Unlock()
			return
		}
		h.mu.Lock()
		if !h.detailLeasedLocked(s, d, h.now()) {
			// Let go of it: a detail nobody reads holds nothing, so a
			// client's made-up IDs do not pile up.
			d.running, d.snap = false, nil
			if s.details[d.id] == d {
				delete(s.details, d.id)
			}
			h.mu.Unlock()
			return
		}
		h.mu.Unlock()
	}
}

// readDetail is one read of a detail, a panic in it made an error.
func (h *Hub) readDetail(s *section, rd panel.RawDetailer, id string) (snap *contract.Detail) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			h.log.Error("a detail's read panicked", "section", s.key, "panic", safetext.WithoutHome(fmt.Sprint(r)))
			out := mapDetail(rd, panel.RawDetail{Section: s.key, ID: id, Title: id, At: h.clock(), Unread: errPanicked}, errPanicked)
			out.Truncated = cleanText(&out)
			snap = &out
		}
	}()
	ctx, cancel := context.WithTimeout(h.ctx, h.PollTimeout)
	defer cancel()
	raw, err := rd.ReadDetail(ctx, id)
	if raw.Section == "" {
		raw.Section, raw.ID, raw.Title = s.key, id, id
	}
	if raw.At.IsZero() {
		raw.At = h.clock()
	}
	out := mapDetail(rd, raw, err)
	out.Truncated = cleanText(&out)
	return &out
}

// subscribe opens a stream on sections and details ("key/id"): each one's
// latest reading is offered at once, then each new one. The returned cancel
// ends the stream's leases.
func (h *Hub) subscribe(ctx context.Context, keys, details []string) (*subscriber, func(), error) {
	sub := newSubscriber(h.now)
	var undo []func()
	cancel := func() {
		for _, u := range undo {
			u()
		}
	}
	for _, key := range keys {
		h.mu.Lock()
		s, ok := h.sections[key]
		if !ok {
			h.mu.Unlock()
			cancel()
			return nil, nil, &ErrUnknown{What: fmt.Sprintf("section %q", key), Known: h.Keys()}
		}
		s.touched = h.touchLocked()
		s.watchers++
		s.subs[sub] = struct{}{}
		h.startSectionLocked(s)
		if s.snap != nil {
			sub.offer(contract.EventSection+"/"+key, contract.EventSection, s.snap)
		}
		h.mu.Unlock()
		undo = append(undo, func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			s.watchers--
			delete(s.subs, sub)
		})
	}
	for _, kd := range details {
		key, id, found := strings.Cut(kd, "/")
		if !found || id == "" {
			cancel()
			return nil, nil, &ErrUnknown{What: fmt.Sprintf("detail %q (want section/id)", kd), Known: h.Keys()}
		}
		s, d, err := h.leaseDetail(ctx, key, id, true)
		if err != nil {
			cancel()
			return nil, nil, err
		}
		h.mu.Lock()
		d.subs[sub] = struct{}{}
		if d.snap != nil {
			sub.offer(contract.EventDetail+"/"+key+"/"+id, contract.EventDetail, d.snap)
		}
		h.mu.Unlock()
		undo = append(undo, func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			d.watchers--
			delete(d.subs, sub)
			_ = s
		})
	}
	return sub, func() {
		cancel()
		h.mu.Lock()
		h.touchLocked()
		h.mu.Unlock()
	}, nil
}

// subscribers are a set's members, to offer to outside the lock.
func subscribers(set map[*subscriber]struct{}) []*subscriber {
	out := make([]*subscriber, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	return out
}

// Active reports how many sections and details are being read now, for a
// test of the leases.
func (h *Hub) Active() (sections, details int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.sections {
		if s.running {
			sections++
		}
		for _, d := range s.details {
			if d.running {
				details++
			}
		}
	}
	return sections, details
}
