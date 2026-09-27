/*
Package codex asks the Codex app-server what the account's rate limits are.

It is a subprocess and a protocol rather than an HTTP call: `codex app-server
--stdio` speaks JSON-RPC, and the request this package makes is the one the
supported API offers. No credential is read here — the app-server holds the
login — which is why this package is much smaller than the Claude one.
*/
package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// Timeout is how long the whole exchange may take.
//
// Generous, because starting a Node program is not fast, and bounded, because
// a panel that waited on a JSON-RPC read would take its poll loop with it.
const Timeout = 12 * time.Second

// ErrNotInstalled is the absence of the codex executable. It is a section that
// is not drawn rather than an error to show: a machine without Codex is not a
// machine with a problem.
var ErrNotInstalled = errors.New("codex is not installed")

// The request ids. Two calls, so two constants rather than a counter.
const (
	initializeID = 1
	rateLimitsID = 2
)

// Usage returns the rate limits as the app-server reports them.
//
// The reply is passed through as raw JSON, the way the Claude payload is: the
// cache holds what the provider said, and the decoding lives in one place with
// the other provider's.
func Usage(ctx context.Context) (json.RawMessage, error) {
	path, err := exec.LookPath("codex")
	if err != nil {
		return nil, ErrNotInstalled
	}

	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	// The path comes from LookPath and the arguments are constants: this runs
	// the codex on the user's own PATH, which is the one they would run.
	cmd := exec.CommandContext(ctx, path, "app-server", "--stdio") //nolint:gosec // the executable the user has installed
	// Kill the whole thing on a timeout rather than closing a pipe and hoping:
	// an app-server that is wedged is one this program cannot wait for.
	cmd.Cancel = func() error { return cmd.Process.Kill() }

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("talking to the app-server: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("talking to the app-server: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting the app-server: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 0, 64*1024), maxLine)

	send := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("encoding a request: %w", err)
		}
		if _, err := stdin.Write(append(b, '\n')); err != nil {
			return fmt.Errorf("writing to the app-server: %w", err)
		}
		return nil
	}

	if err := send(request{JSONRPC: "2.0", ID: initializeID, Method: "initialize",
		Params: map[string]any{"clientInfo": map[string]string{"name": "hayami", "version": "1"}}}); err != nil {
		return nil, err
	}
	if _, err := await(lines, initializeID); err != nil {
		return nil, err
	}
	if err := send(request{JSONRPC: "2.0", Method: "initialized", Params: map[string]any{}}); err != nil {
		return nil, err
	}
	if err := send(request{JSONRPC: "2.0", ID: rateLimitsID, Method: "account/rateLimits/read",
		Params: map[string]any{"excludeResetCreditDetails": true}}); err != nil {
		return nil, err
	}
	return await(lines, rateLimitsID)
}

// maxLine is a ceiling on one line of a protocol this program does not
// control.
const maxLine = 1 << 20

// request is one JSON-RPC message. A notification has no id, which is why it
// is omitted when empty.
type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// response is one reply.
type response struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Message string `json:"message"`
}

// await reads until the reply with this id arrives, skipping anything else the
// app-server says on the way. A server that talks about its own progress is
// not a server that failed.
func await(lines *bufio.Scanner, id int) (json.RawMessage, error) {
	for lines.Scan() {
		var r response
		if err := json.Unmarshal(lines.Bytes(), &r); err != nil {
			continue // not a reply to us
		}
		if r.ID != id {
			continue
		}
		if r.Error != nil {
			return nil, fmt.Errorf("the app-server refused: %s", r.Error.Message)
		}
		return r.Result, nil
	}
	if err := lines.Err(); err != nil {
		return nil, fmt.Errorf("reading from the app-server: %w", err)
	}
	return nil, errors.New("the app-server closed without answering")
}
