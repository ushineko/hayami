package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/ushineko/hayami/internal/api/contract"
)

// CheckOptions shape a check.
type CheckOptions struct {
	// Depth is how far below a section's details the check follows their
	// children: 1 is the details alone, 3 reaches `cpu/firefox/4411`.
	Depth int
	// Breadth is how many of a detail's children are followed, 0 for every
	// one. A process list's children are every process name on the machine,
	// and the service reads at most 32 details of a section at once, so
	// every one to a depth of three is minutes of waiting for its leases.
	Breadth int
	// StreamWait is how long the stream has to send each section's first
	// event.
	StreamWait time.Duration
	// Out has a line for each thing checked; nil for none.
	Out io.Writer
}

// CheckReport counts what a check read.
type CheckReport struct {
	Requests int
	Sections int
	Details  int
	Events   int
	// Gone are children that were offered and had gone by the time they
	// were asked for: a process that ended between two reads.
	Gone int
}

/*
Check walks the service (spec 113 R30): health, the index, the section list,
every section, its details and their drills to Depth, every reading at once,
what is not there, and a stream of every section. Each response is held to
the contract by Decode and to the full-value rules (R12, R13): no string
starts or ends in padding or ends in an ellipsis a formatter adds, the
reading the key names is the one set, a detail's kind names its body. The
first violation ends it, named.
*/
func Check(ctx context.Context, c *Client, o CheckOptions) (CheckReport, error) {
	k := &checker{c: c, o: o}
	if k.o.Out == nil {
		k.o.Out = io.Discard
	}
	if k.o.StreamWait == 0 {
		k.o.StreamWait = 30 * time.Second
	}
	err := k.run(ctx)
	return k.report, err
}

type checker struct {
	c      *Client
	o      CheckOptions
	report CheckReport
}

func (k *checker) say(format string, args ...any) {
	_, _ = fmt.Fprintf(k.o.Out, format+"\n", args...)
}

func (k *checker) run(ctx context.Context) error {
	health, err := k.c.Health(ctx)
	k.report.Requests++
	if err != nil {
		return fmt.Errorf("/v1/health: %w", err)
	}
	if !health.OK {
		return errors.New("/v1/health: the service says it is not well")
	}
	index, err := k.c.Index(ctx)
	k.report.Requests++
	if err != nil {
		return fmt.Errorf("/v1: %w", err)
	}
	if major, _, _ := strings.Cut(index.API, "."); major != "1" {
		return fmt.Errorf("/v1: the service keeps contract %s, and this client knows 1.x", index.API)
	}
	k.say("ok   /v1          %s %s, contract %s", index.Service, index.Version, index.API)
	list, err := k.c.Sections(ctx)
	k.report.Requests++
	if err != nil {
		return fmt.Errorf("/v1/sections: %w", err)
	}
	var keys []string
	for _, s := range list.Sections {
		keys = append(keys, s.Key)
		if err := fullValues("/v1/sections", s); err != nil {
			return err
		}
	}
	if !slices.Equal(keys, index.Sections) {
		return fmt.Errorf("/v1/sections lists %v and /v1 lists %v", keys, index.Sections)
	}
	for _, key := range keys {
		if err := k.section(ctx, key); err != nil {
			return err
		}
	}
	if err := k.readings(ctx, keys); err != nil {
		return err
	}
	if err := k.absent(ctx, keys); err != nil {
		return err
	}
	return k.stream(ctx, keys)
}

func (k *checker) section(ctx context.Context, key string) error {
	path := SectionPath(key)
	s, err := k.c.Section(ctx, key)
	k.report.Requests++
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := sectionRules(path, key, s); err != nil {
		return err
	}
	k.report.Sections++
	k.say("ok   %s  %d details", path, len(s.Details))
	for _, d := range s.Details {
		if err := k.detail(ctx, key, d.ID, 1, false); err != nil {
			return err
		}
	}
	return nil
}

func (k *checker) detail(ctx context.Context, key, id string, depth int, child bool) error {
	path := DetailPath(key, id)
	d, err := k.askDetail(ctx, key, id)
	if child && IsStatus(err, http.StatusNotFound) {
		k.report.Gone++
		k.say("gone %s", path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := detailRules(path, key, id, d); err != nil {
		return err
	}
	k.report.Details++
	k.say("ok   %s", path)
	if depth >= k.o.Depth {
		return nil
	}
	children := d.Children
	if k.o.Breadth > 0 && len(children) > k.o.Breadth {
		children = children[:k.o.Breadth]
	}
	for _, c := range children {
		if err := k.detail(ctx, key, c, depth+1, true); err != nil {
			return err
		}
	}
	return nil
}

// askDetail asks for a detail, and over the service's limit on a section's
// details waits as it says and asks again: a check reads more of them than
// the service holds at once.
func (k *checker) askDetail(ctx context.Context, key, id string) (contract.Detail, error) {
	for waited := false; ; waited = true {
		d, err := k.c.Detail(ctx, key, id)
		k.report.Requests++
		var e *APIError
		if !errors.As(err, &e) || e.Status != http.StatusServiceUnavailable {
			return d, err
		}
		if !waited {
			k.say("wait %s: %s", DetailPath(key, id), e.Body.Message)
		}
		select {
		case <-ctx.Done():
			return d, fmt.Errorf("waiting for the service: %w", ctx.Err())
		case <-time.After(max(e.RetryAfter, time.Second)):
		}
	}
}

func (k *checker) readings(ctx context.Context, keys []string) error {
	r, err := k.c.Readings(ctx)
	k.report.Requests++
	if err != nil {
		return fmt.Errorf("/v1/readings: %w", err)
	}
	var got []string
	for key, s := range r.Readings {
		got = append(got, key)
		if err := sectionRules("/v1/readings."+key, key, s); err != nil {
			return err
		}
	}
	slices.Sort(got)
	want := slices.Sorted(slices.Values(keys))
	if !slices.Equal(got, want) {
		return fmt.Errorf("/v1/readings has %v, and the sections are %v", got, want)
	}
	k.say("ok   /v1/readings  %d sections", len(got))
	return nil
}

// absent asks for what is not there: a 404 that names what is.
func (k *checker) absent(ctx context.Context, keys []string) error {
	const none = "no-such-section"
	_, err := k.c.Section(ctx, none)
	k.report.Requests++
	var e *APIError
	if !errors.As(err, &e) || e.Status != http.StatusNotFound {
		return fmt.Errorf("%s: want the contract's 404, got %w", SectionPath(none), err)
	}
	if !slices.Equal(slices.Sorted(slices.Values(e.Body.Known)), slices.Sorted(slices.Values(keys))) {
		return fmt.Errorf("%s: the 404 knows %v, and the sections are %v", SectionPath(none), e.Body.Known, keys)
	}
	if len(keys) > 0 {
		_, err := k.c.Detail(ctx, keys[0], "no-such-detail")
		k.report.Requests++
		if !errors.As(err, &e) || e.Status != http.StatusNotFound {
			return fmt.Errorf("%s: want the contract's 404, got %w", DetailPath(keys[0], "no-such-detail"), err)
		}
	}
	k.say("ok   what is not there is a 404 that names what is")
	return nil
}

// stream opens a stream of every section and waits for each one's first
// event.
func (k *checker) stream(ctx context.Context, keys []string) error {
	ctx, cancel := context.WithTimeout(ctx, k.o.StreamWait)
	defer cancel()
	s, err := k.c.Events(ctx, keys, nil)
	k.report.Requests++
	if err != nil {
		return fmt.Errorf("/v1/events: %w", err)
	}
	defer func() { _ = s.Close() }()
	waiting := slices.Clone(keys)
	for len(waiting) > 0 {
		ev, err := s.Next()
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("/v1/events: no event in %s for %v", k.o.StreamWait, waiting)
			}
			return fmt.Errorf("/v1/events: %w", err)
		}
		if ev.Name != EventSection {
			continue
		}
		k.report.Events++
		if err := sectionRules("/v1/events."+ev.Section.Key, ev.Section.Key, *ev.Section); err != nil {
			return err
		}
		waiting = slices.DeleteFunc(waiting, func(w string) bool { return w == ev.Section.Key })
	}
	k.say("ok   /v1/events  an event from each section")
	return nil
}

// sectionRules are a section's rules beyond its shape: its key, the one
// reading set, and full values.
func sectionRules(path, key string, s contract.Section) error {
	if s.Key != key {
		return fmt.Errorf("%s: the section says it is %q", path, s.Key)
	}
	set := setFields(s.Reading)
	if len(set) > 1 || (len(set) == 1 && set[0] != key) {
		return fmt.Errorf("%s: the reading sets %v, where the contract sets %s's alone", path, set, key)
	}
	if len(set) == 0 && s.PolledAt != nil && s.Error == nil && len(s.Absences) == 0 {
		return fmt.Errorf("%s: polled, with no reading, no error and no absence", path)
	}
	return fullValues(path, s)
}

// detailRules are a detail's: its section and ID, the body its kind names,
// and full values.
func detailRules(path, key, id string, d contract.Detail) error {
	if d.Section != key || d.ID != id {
		return fmt.Errorf("%s: the detail says it is %s/%s", path, d.Section, d.ID)
	}
	set := setFields(d, "kind", "polled_at", "absences", "error", "children")
	switch {
	case d.Kind == nil && len(set) > 0:
		return fmt.Errorf("%s: no kind, and %v set", path, set)
	case d.Kind != nil && (len(set) != 1 || set[0] != *d.Kind):
		return fmt.Errorf("%s: kind %q, and %v set", path, *d.Kind, set)
	}
	return fullValues(path, d)
}

// setFields are the JSON names of a struct's pointer and list fields that
// are not null, but those named in skip.
func setFields(v any, skip ...string) []string {
	rv := reflect.ValueOf(v)
	var out []string
	for _, f := range structFields(rv.Type()) {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		fv := rv.FieldByIndex(f.Index)
		if (fv.Kind() == reflect.Pointer || fv.Kind() == reflect.Slice) && !fv.IsNil() && !slices.Contains(skip, name) {
			out = append(out, name)
		}
	}
	return out
}

/*
fullValues holds every string in v to R12 and R13: a value is the source's
whole, not a rendering of it. A string that starts or ends in white space was
padded, and one that ends in "…" was cut by a formatter; a text of lines
(a change's description) is the source's and may do either.
*/
func fullValues(path string, v any) error {
	return walkStrings(path, reflect.ValueOf(v), false)
}

func walkStrings(path string, v reflect.Value, lines bool) error {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return walkStrings(path, v.Elem(), lines)
	case reflect.Struct:
		if v.Type() == timeType {
			return nil
		}
		for _, f := range structFields(v.Type()) {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if err := walkStrings(path+"."+name, v.FieldByIndex(f.Index), f.Tag.Get("text") == "lines"); err != nil {
				return err
			}
		}
	case reflect.Slice:
		for i := range v.Len() {
			if err := walkStrings(fmt.Sprintf("%s[%d]", path, i), v.Index(i), lines); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			if err := walkStrings(path+"."+key.String(), v.MapIndex(key), lines); err != nil {
				return err
			}
		}
	case reflect.String:
		if lines {
			return nil
		}
		s := v.String()
		trimmed := strings.TrimFunc(s, unicode.IsSpace)
		switch {
		case trimmed != s:
			return &ContractError{Path: path, Reason: fmt.Sprintf("%q is padded; a value is the source's whole", s)}
		case strings.HasSuffix(s, "…"):
			return &ContractError{Path: path, Reason: fmt.Sprintf("%q ends in an ellipsis; a value is the source's whole", s)}
		}
	default:
	}
	return nil
}
