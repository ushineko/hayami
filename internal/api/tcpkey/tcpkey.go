/*
Package tcpkey is the file that lets a program of this account use the
service's loopback TCP port (spec 115 R6, R7): the port's address and a
token, in api-tcp.json in the settings directory, readable by this account
alone.

A loopback port is reachable by every account on the machine and by every
local process, where the service's Unix socket and named pipe are reachable
by one account. The token is what tells the account's own programs from the
rest: a program that can read this file is running as the account.

It is the installer's as well as the service's and the client's, so it
imports the standard library alone (and the system's on Windows).
*/
package tcpkey

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

// DefaultPort is the port set up when none is named (spec 115, Decisions):
// not in /etc/services, and in use by nothing on the desks it was chosen on.
const DefaultPort = 47930

// FileName is the file's name in the settings directory.
const FileName = "api-tcp.json"

// Host is the only address the port is ever on (R1).
const Host = "127.0.0.1"

// tokenBytes is the token's length before encoding (R7).
const tokenBytes = 32

// maxFile bounds what Read takes: the file is two short strings.
const maxFile = 4096

// ErrNoFile is a missing file: the port is not set up.
var ErrNoFile = errors.New("the TCP port is not set up")

// Key is the file's content.
type Key struct {
	// Address is "127.0.0.1:<port>".
	Address string `json:"address"`
	// Token is the bearer token: 32 random bytes, base64url without padding.
	Token string `json:"token"`
}

// Port is the address's port.
func (k Key) Port() int {
	_, p, _ := net.SplitHostPort(k.Address)
	n, _ := strconv.Atoi(p)
	return n
}

// Path is the file's place: api-tcp.json in hayami's settings directory.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding the configuration directory: %w", err)
	}
	return filepath.Join(dir, "hayami", FileName), nil
}

// Address is the port's address.
func Address(port int) string { return net.JoinHostPort(Host, strconv.Itoa(port)) }

// check is nil for a key the service may serve with: an address on
// 127.0.0.1 with a port, and a token of the right length.
func (k Key) check() error {
	host, p, err := net.SplitHostPort(k.Address)
	if err != nil || host != Host {
		return errors.New("the address is not 127.0.0.1 and a port")
	}
	if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != p {
		return errors.New("the address's port is not a port")
	}
	raw, err := base64.RawURLEncoding.DecodeString(k.Token)
	if err != nil || len(raw) != tokenBytes {
		return errors.New("the token is not 32 bytes in base64url")
	}
	return nil
}

// NewToken is a fresh token (R7).
func NewToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("making a token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Equal compares a token a client sent with the key's, in constant time
// (R8).
func (k Key) Equal(token string) bool {
	return subtle.ConstantTimeCompare([]byte(token), []byte(k.Token)) == 1
}

/*
Read is the file at path, refused unless only this account can read it and
its content is a key the service may serve with (R11). A missing file is
ErrNoFile. Errors never carry the token.
*/
func Read(path string) (Key, error) {
	f, err := openPrivate(path)
	if errors.Is(err, os.ErrNotExist) {
		return Key{}, ErrNoFile
	}
	if err != nil {
		return Key{}, err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, maxFile+1)
	n, _ := readFull(f, buf)
	if n > maxFile {
		return Key{}, fmt.Errorf("%s is larger than a key", path)
	}
	var k Key
	if err := json.Unmarshal(buf[:n], &k); err != nil {
		return Key{}, fmt.Errorf("%s is not a key: not JSON", path)
	}
	if err := k.check(); err != nil {
		return Key{}, fmt.Errorf("%s is not a key: %w", path, err)
	}
	return k, nil
}

func readFull(f *os.File, buf []byte) (int, error) {
	n := 0
	for n < len(buf) {
		m, err := f.Read(buf[n:])
		n += m
		if err != nil {
			return n, err //nolint:wrapcheck // io.EOF, which the caller does not look at
		}
	}
	return n, nil
}

/*
Make writes the key for port at path, readable by this account alone (R6,
R7). The token already in a valid file is kept unless renew; otherwise one is
made. The file is written in its directory under another name and moved into
place, so a reader sees the old key or the new one, never half of one.
*/
func Make(path string, port int, renew bool) (Key, error) {
	k := Key{Address: Address(port)}
	if old, err := Read(path); err == nil && !renew {
		k.Token = old.Token
	} else {
		tok, err := NewToken()
		if err != nil {
			return Key{}, err
		}
		k.Token = tok
	}
	if err := k.check(); err != nil {
		return Key{}, err
	}
	body, err := json.Marshal(k)
	if err != nil {
		return Key{}, fmt.Errorf("writing the key: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Key{}, fmt.Errorf("making the settings directory: %w", err)
	}
	if err := writePrivate(path, append(body, '\n')); err != nil {
		return Key{}, err
	}
	return k, nil
}

// Remove takes the file away; a file not there is not a failure.
func Remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing the TCP key: %w", err)
	}
	return nil
}
