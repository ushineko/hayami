/*
Package client is the reference client of hayami's service (spec 113 R29): a
typed client over the contract in api/openapi.yaml.

It finds the address, dials the socket, calls every endpoint, reads the
event stream into typed events, and reports a response the contract does not
allow as a *ContractError rather than a zero value. It imports the contract
types and the standard library and nothing else of hayami's, so it shows what
any client needs; a test holds it to that.

	c := client.New(addr)
	s, err := c.Section(ctx, "cooler")
*/
package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ushineko/hayami/internal/api/contract"
)

// MaxBody is the most of a response, or of a stream's event, the client
// reads. A process list is the largest reading, and is a fraction of this;
// checking one decodes it into JSON values first, which take several times
// its size, so the bound is not set higher than a reading needs.
const MaxBody = 16 << 20

// statuses are the statuses the contract documents for an error.
var statuses = []int{
	http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound,
	http.StatusMethodNotAllowed, http.StatusInternalServerError, http.StatusServiceUnavailable,
}

// Client is a client of the service at one address.
type Client struct {
	http *http.Client
	// base is the URL a path is put after: http://hayami on the socket,
	// whose host is ignored, and the port's own address on TCP, whose host
	// the service checks (spec 115 R9).
	base string
	// token is the TCP port's, sent with every request there; empty on the
	// socket.
	token string
}

/*
New is a client of the service whose socket is at address. The context of
each call bounds it: the first request for a section the service has not
read waits for its first poll, up to 15 s, so a deadline is longer than that.
*/
func New(address string) *Client {
	return &Client{base: "http://hayami", http: &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return Dial(ctx, address)
		},
		MaxIdleConnsPerHost: 4,
	}}}
}

/*
NewTCP is a client of the service's loopback TCP port at address
("127.0.0.1:<port>"), sending token with every request (spec 115 R8). The
address and the token are in api-tcp.json in hayami's settings directory;
reading it is the caller's, so this package needs nothing but the contract.
The client does not follow a redirect, which would carry the token
elsewhere.
*/
func NewTCP(address, token string) *Client {
	return &Client{base: "http://" + address, token: token, http: &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "tcp", address) //nolint:wrapcheck // the dialer's error names the address
			},
			MaxIdleConnsPerHost: 4,
			Proxy:               nil,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Close lets go of the connections the client keeps for its next request.
func (c *Client) Close() { c.http.CloseIdleConnections() }

/*
APIError is an answer the contract documents that is not a reading: a 404
for a section that is not there, with Known listing those that are; a 503
over a limit, with RetryAfter.
*/
type APIError struct {
	Status     int
	Body       contract.ErrorBody
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%d %s: %s", e.Status, e.Body.Code, e.Body.Message)
}

// Response is a response as it came: for `api get`, and for a check that
// reads a body before it decodes it.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Get is a GET of path, which begins with /v1, read whole and not decoded.
func (c *Client) Get(ctx context.Context, path string) (*Response, error) {
	resp, err := c.do(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(body) > MaxBody {
		return nil, fmt.Errorf("%s answered more than %d bytes", path, MaxBody)
	}
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: body}, nil
}

func (c *Client) do(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("asking for %s: %w", path, err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asking for %s: %w", path, err)
	}
	return resp, nil
}

// Into checks r against the contract and decodes it into v: a 200 into v,
// anything else into an *APIError.
func (r *Response) Into(v any) error {
	if r.Status == http.StatusOK {
		if err := jsonType(r.Header); err != nil {
			return err
		}
		return Decode(r.Body, v)
	}
	return r.apiError()
}

func (r *Response) apiError() error {
	if !slices.Contains(statuses, r.Status) {
		return &ContractError{Path: "status", Reason: fmt.Sprintf("%d is a status the contract does not document", r.Status)}
	}
	if err := jsonType(r.Header); err != nil {
		return err
	}
	var body contract.Error
	if err := Decode(r.Body, &body); err != nil {
		return err
	}
	e := &APIError{Status: r.Status, Body: body.Error}
	if r.Status == http.StatusServiceUnavailable {
		s, err := strconv.Atoi(r.Header.Get("Retry-After"))
		if err != nil || s < 0 {
			return &ContractError{Path: "Retry-After", Reason: fmt.Sprintf("a 503 says when to come back in seconds, not %q", r.Header.Get("Retry-After"))}
		}
		e.RetryAfter = time.Duration(s) * time.Second
	}
	return e
}

func jsonType(h http.Header) error {
	return mediaType(h, "application/json")
}

func mediaType(h http.Header, want string) error {
	got, _, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil || got != want {
		return &ContractError{Path: "Content-Type", Reason: fmt.Sprintf("%q, where the contract has %s", h.Get("Content-Type"), want)}
	}
	return nil
}

func get[T any](ctx context.Context, c *Client, path string) (T, error) {
	var v T
	r, err := c.Get(ctx, path)
	if err != nil {
		return v, err
	}
	return v, r.Into(&v)
}

// Index is GET /v1: the service, its version and the contract's, and its
// sections.
func (c *Client) Index(ctx context.Context) (contract.Index, error) {
	return get[contract.Index](ctx, c, "/v1")
}

// Health is GET /v1/health, which reads nothing.
func (c *Client) Health(ctx context.Context) (contract.Health, error) {
	return get[contract.Health](ctx, c, "/v1/health")
}

// Sections is GET /v1/sections.
func (c *Client) Sections(ctx context.Context) (contract.SectionList, error) {
	return get[contract.SectionList](ctx, c, "/v1/sections")
}

// Section is GET /v1/sections/{key}: one section's latest reading.
func (c *Client) Section(ctx context.Context, key string) (contract.Section, error) {
	return get[contract.Section](ctx, c, SectionPath(key))
}

// Readings is GET /v1/readings: the sections named, or every one.
func (c *Client) Readings(ctx context.Context, keys ...string) (contract.Readings, error) {
	path := "/v1/readings"
	if len(keys) > 0 {
		path += "?sections=" + url.QueryEscape(strings.Join(keys, ","))
	}
	return get[contract.Readings](ctx, c, path)
}

// Detail is GET /v1/sections/{key}/details/{id}: one detail, or a drill by
// its ID.
func (c *Client) Detail(ctx context.Context, key, id string) (contract.Detail, error) {
	return get[contract.Detail](ctx, c, DetailPath(key, id))
}

// SectionPath is a section's path.
func SectionPath(key string) string {
	return "/v1/sections/" + url.PathEscape(key)
}

// DetailPath is a detail's path: a drill's ID is its steps, each
// percent-encoded and joined by "/" (the contract's rule).
func DetailPath(key, id string) string {
	steps := strings.Split(id, "/")
	for i, s := range steps {
		steps[i] = url.PathEscape(s)
	}
	return SectionPath(key) + "/details/" + strings.Join(steps, "/")
}

// IsStatus reports whether err is the service answering status.
func IsStatus(err error, status int) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == status
}
