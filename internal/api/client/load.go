package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/hayami/internal/api/contract"
)

// LoadOptions shape a load run.
type LoadOptions struct {
	Clients  int
	Streams  int
	Duration time.Duration
	// Address is the socket: each client is its own Client, with its own
	// connections, as many programs would be.
	Address string
	// Token, where it is set, makes Address the TCP port's and is sent with
	// every request (spec 115).
	Token string
}

// LoadReport is what a load run saw.
type LoadReport struct {
	Requests int
	Statuses map[int]int
	// Latency is of every answered request, sorted.
	P50, P90, P99, Max time.Duration
	// Events is each stream's count of events.
	Events []int
	// Errors are the first of what went wrong: a request not answered, a
	// status the contract does not document, a response it does not allow.
	Errors []string
	// ErrorCount is all of them.
	ErrorCount int
}

// OK reports whether the run saw nothing go wrong: every answer one the
// contract documents, no 500, and every stream receiving.
func (r LoadReport) OK() bool {
	if r.ErrorCount > 0 || r.Statuses[http.StatusInternalServerError] > 0 {
		return false
	}
	return !slices.Contains(r.Events, 0)
}

// Write is the report as lines.
func (r LoadReport) Write(w io.Writer) {
	_, _ = fmt.Fprintf(w, "requests  %d\n", r.Requests)
	codes := make([]int, 0, len(r.Statuses))
	for c := range r.Statuses {
		codes = append(codes, c)
	}
	sort.Ints(codes)
	for _, c := range codes {
		_, _ = fmt.Fprintf(w, "  %d     %d\n", c, r.Statuses[c])
	}
	_, _ = fmt.Fprintf(w, "latency   p50 %s  p90 %s  p99 %s  max %s\n", r.P50, r.P90, r.P99, r.Max)
	_, _ = fmt.Fprintf(w, "events    %v\n", r.Events)
	_, _ = fmt.Fprintf(w, "errors    %d\n", r.ErrorCount)
	for _, e := range r.Errors {
		_, _ = fmt.Fprintf(w, "  %s\n", e)
	}
}

// target is a path a load client asks for, and what decodes it.
type target struct {
	path   string
	decode func(*Response) error
}

func targetOf[T any](path string) target {
	return target{path: path, decode: func(r *Response) error {
		var v T
		return r.Into(&v)
	}}
}

/*
Load is the reference client's load generator (spec 113 R32). Clients loop
over random GETs of the service's sections, details and their first drills,
and things that are not there; streams connect, read, and a third of the
time leave early and come back. Every answer is decoded against the
contract, so a run is also a check under load.
*/
func Load(ctx context.Context, o LoadOptions) (LoadReport, error) {
	if o.Clients < 0 || o.Streams < 0 || o.Duration <= 0 {
		return LoadReport{}, fmt.Errorf("clients and streams are counts, and the duration a time: %d, %d, %s", o.Clients, o.Streams, o.Duration)
	}
	ctx, stop := context.WithTimeout(ctx, o.Duration)
	defer stop()
	targets, keys, details, err := discover(ctx, o.newClient())
	if err != nil {
		return LoadReport{}, err
	}
	l := &loader{report: LoadReport{Statuses: map[int]int{}, Events: make([]int, o.Streams)}}
	var wg sync.WaitGroup
	for range o.Clients {
		wg.Go(func() { l.client(ctx, o.newClient(), targets) })
	}
	for i := range o.Streams {
		wg.Go(func() { l.stream(ctx, o.newClient(), i, keys, details) })
	}
	wg.Wait()
	slices.Sort(l.latencies)
	if n := len(l.latencies); n > 0 {
		at := func(p int) time.Duration { return l.latencies[min(n-1, n*p/100)] }
		l.report.P50, l.report.P90, l.report.P99, l.report.Max = at(50), at(90), at(99), l.latencies[n-1]
	}
	return l.report, nil
}

// discover is what there is to ask for: every section, its details, and
// their first few drills.
func discover(ctx context.Context, c *Client) ([]target, []string, []string, error) {
	defer c.Close()
	list, err := c.Sections(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("listing the sections: %w", err)
	}
	targets := []target{
		targetOf[contract.Index]("/v1"), targetOf[contract.Health]("/v1/health"),
		targetOf[contract.SectionList]("/v1/sections"), targetOf[contract.Readings]("/v1/readings"),
		targetOf[contract.Section]("/v1/sections/no-such-section"),
	}
	var keys, details []string
	for _, s := range list.Sections {
		keys = append(keys, s.Key)
		targets = append(targets, targetOf[contract.Section](SectionPath(s.Key)))
		sec, err := c.Section(ctx, s.Key)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("reading %s: %w", s.Key, err)
		}
		for _, d := range sec.Details {
			details = append(details, s.Key+"/"+d.ID)
			targets = append(targets, targetOf[contract.Detail](DetailPath(s.Key, d.ID)))
			det, err := c.Detail(ctx, s.Key, d.ID)
			if err != nil {
				continue
			}
			for _, child := range det.Children[:min(3, len(det.Children))] {
				targets = append(targets, targetOf[contract.Detail](DetailPath(s.Key, child)))
			}
		}
	}
	return targets, keys, details, nil
}

type loader struct {
	mu        sync.Mutex
	report    LoadReport
	latencies []time.Duration
}

func (l *loader) fail(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.report.ErrorCount++
	if len(l.report.Errors) < 10 {
		l.report.Errors = append(l.report.Errors, fmt.Sprintf(format, args...))
	}
}

func (l *loader) client(ctx context.Context, c *Client, targets []target) {
	defer c.Close()
	for ctx.Err() == nil {
		t := targets[pick(len(targets))]
		began := time.Now()
		r, err := c.Get(ctx, t.path)
		if err != nil {
			if ctx.Err() == nil {
				l.fail("%s: %v", t.path, err)
			}
			continue
		}
		took := time.Since(began)
		l.mu.Lock()
		l.report.Requests++
		l.report.Statuses[r.Status]++
		l.latencies = append(l.latencies, took)
		l.mu.Unlock()
		err = t.decode(r)
		var api *APIError
		switch {
		case err == nil:
		case errors.As(err, &api):
			// A drill whose process ended is a 404, as is the section that
			// is not there; over a limit is a 503 that says when to return.
			if api.Status == http.StatusServiceUnavailable {
				sleep(ctx, api.RetryAfter)
			} else if api.Status != http.StatusNotFound {
				l.fail("%s: %v", t.path, err)
			}
		default:
			l.fail("%s: %v", t.path, err)
		}
	}
}

func (l *loader) stream(ctx context.Context, c *Client, i int, keys, details []string) {
	defer c.Close()
	if len(keys) == 0 {
		return
	}
	for ctx.Err() == nil {
		// A stream names a section or two and, some of the time, a detail.
		sections := []string{keys[pick(len(keys))]}
		if len(keys) > 1 && pick(2) == 0 {
			sections = append(sections, keys[pick(len(keys))])
		}
		var named []string
		if len(details) > 0 && pick(3) == 0 {
			named = []string{details[pick(len(details))]}
		}
		life := time.Until(deadline(ctx))
		if pick(3) == 0 {
			life = time.Duration(1000+pick(4000)) * time.Millisecond
		}
		sctx, cancel := context.WithTimeout(ctx, life)
		s, err := c.Events(sctx, slices.Compact(slices.Sorted(slices.Values(sections))), named)
		if err != nil {
			cancel()
			var api *APIError
			switch {
			case errors.As(err, &api) && api.Status == http.StatusServiceUnavailable:
				sleep(ctx, api.RetryAfter)
			case sctx.Err() == nil:
				l.fail("stream %s: %v", strings.Join(sections, ","), err)
			}
			continue
		}
		for {
			ev, err := s.Next()
			if err != nil {
				if sctx.Err() == nil && !errors.Is(err, io.EOF) {
					l.fail("stream %s: %v", strings.Join(sections, ","), err)
				}
				break
			}
			if ev.Name != EventComment {
				l.mu.Lock()
				l.report.Events[i]++
				l.mu.Unlock()
			}
		}
		_ = s.Close()
		cancel()
		// A stream the service ended at once is not asked for again at once.
		sleep(ctx, 0)
	}
}

// pick is a number in [0, n): which request a load client makes next.
func pick(n int) int {
	return rand.IntN(n) //nolint:gosec // a load run's choice of request, not a secret
}

func deadline(ctx context.Context) time.Time {
	d, _ := ctx.Deadline()
	return d
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(max(d, 100*time.Millisecond)):
	}
}

// newClient is a client of the service at the options' address.
func (o LoadOptions) newClient() *Client {
	if o.Token != "" {
		return NewTCP(o.Address, o.Token)
	}
	return New(o.Address)
}
