package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ushineko/hayami/internal/api/contract"
)

// The names an event has: the two the contract sends, and a comment, which
// the service sends to keep an idle stream from looking dead.
const (
	EventSection = "section"
	EventDetail  = "detail"
	EventComment = "comment"
)

// Event is one event of a stream: a section's reading, a detail's, or a
// comment, whose Text is what it said.
type Event struct {
	Name    string
	Section *contract.Section
	Detail  *contract.Detail
	Text    string
}

// Stream is GET /v1/events read as typed events. Close it when done: that
// ends the service's lease on what it names.
type Stream struct {
	body io.ReadCloser
	r    *bufio.Scanner
}

/*
Events opens a stream of the sections and details named, each detail as
key/id. Over the service's limit on streams it is an *APIError of 503, with
RetryAfter. The context ends the stream.
*/
func (c *Client) Events(ctx context.Context, sections, details []string) (*Stream, error) {
	q := url.Values{}
	if len(sections) > 0 {
		q.Set("sections", strings.Join(sections, ","))
	}
	if len(details) > 0 {
		q.Set("details", strings.Join(details, ","))
	}
	resp, err := c.do(ctx, "/v1/events?"+q.Encode())
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody))
		if err != nil {
			return nil, fmt.Errorf("reading the stream's refusal: %w", err)
		}
		return nil, (&Response{Status: resp.StatusCode, Header: resp.Header, Body: body}).apiError()
	}
	if err := mediaType(resp.Header, "text/event-stream"); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	// A line, and an event, is at most MaxBody: a stream is read no further
	// than a response is.
	r := bufio.NewScanner(resp.Body)
	r.Buffer(make([]byte, 0, 64<<10), MaxBody)
	return &Stream{body: resp.Body, r: r}, nil
}

// Close ends the stream.
func (s *Stream) Close() error {
	if err := s.body.Close(); err != nil {
		return fmt.Errorf("closing the stream: %w", err)
	}
	return nil
}

/*
Next is the stream's next event. An event whose data the contract does not
allow is a *ContractError, as is an event name it does not have; the end of
the stream is io.EOF.
*/
func (s *Stream) Next() (Event, error) {
	var name, comment string
	var data strings.Builder
	for {
		if !s.r.Scan() {
			if err := s.r.Err(); err != nil {
				return Event{}, fmt.Errorf("reading the stream: %w", err)
			}
			return Event{}, io.EOF
		}
		line := strings.TrimSuffix(s.r.Text(), "\r")
		if data.Len() > MaxBody {
			return Event{}, fmt.Errorf("an event of more than %d bytes", MaxBody)
		}
		switch {
		case line == "":
			if name == "" && data.Len() == 0 {
				if comment != "" {
					return Event{Name: EventComment, Text: comment}, nil
				}
				continue
			}
			return decodeEvent(name, data.String())
		case strings.HasPrefix(line, ":"):
			comment = strings.TrimSpace(line[1:])
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(line[len("data:"):], " "))
		default:
			return Event{}, &ContractError{Path: "events", Reason: fmt.Sprintf("a line no event has: %.80q", line)}
		}
	}
}

func decodeEvent(name, data string) (Event, error) {
	path := "events." + name
	switch name {
	case EventSection:
		var v contract.Section
		if err := Decode([]byte(data), &v); err != nil {
			return Event{}, within(path, err)
		}
		return Event{Name: name, Section: &v}, nil
	case EventDetail:
		var v contract.Detail
		if err := Decode([]byte(data), &v); err != nil {
			return Event{}, within(path, err)
		}
		return Event{Name: name, Detail: &v}, nil
	default:
		return Event{}, &ContractError{Path: "events", Reason: fmt.Sprintf("an event the contract does not have: %q", name)}
	}
}

// within puts a contract error's path under prefix.
func within(prefix string, err error) error {
	var ce *ContractError
	if errors.As(err, &ce) {
		return &ContractError{Path: prefix + strings.TrimPrefix(ce.Path, "$"), Reason: ce.Reason}
	}
	return err
}
