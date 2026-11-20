package claude

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
	"time"
)

// KeychainService is the generic-password service Claude Code stores its
// credentials under in the macOS login Keychain. The secret is the same JSON
// object the credential file held, so a keychain store parses and writes back
// exactly as a file store does.
const KeychainService = "Claude Code-credentials"

// KeychainServiceEnv overrides the service the default keychain store uses. The
// login Keychain is not under HOME, so a test cannot take it away with HOME
// alone; pointing this at a service that does not exist makes discovery find
// none. Empty uses KeychainService.
const KeychainServiceEnv = "CLAUDE_USAGE_KEYCHAIN_SERVICE"

// securityBin is the macOS tool that reads and writes the login Keychain. The
// absolute path is used because a keychain store is reached from processes
// (the dock's panel among them) started with a minimal PATH.
const securityBin = "/usr/bin/security"

// securityItemNotFound is the exit status `security` returns when no matching
// item exists (errSecItemNotFound, reported as 44). It is mapped to
// fs.ErrNotExist so a keychain store that is gone takes the same path a removed
// file does.
const securityItemNotFound = 44

// securityTimeout bounds a `security` call so a panel's fetch cannot wedge if a
// keychain access prompt is ever left unanswered. The normal call returns at
// once; the window is generous only to let a present user answer a first-time
// prompt.
const securityTimeout = 30 * time.Second

// keychainArgs builds a find/delete argument list, adding the account only when
// one is known so a service-only lookup stays possible.
func keychainArgs(verb, service, account string) []string {
	args := []string{verb, "-s", service}
	if account != "" {
		args = append(args, "-a", account)
	}
	return args
}

// keychainRead returns the secret stored for a generic password, or an error
// wrapping fs.ErrNotExist when no such item exists.
func keychainRead(service, account string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), securityTimeout)
	defer cancel()
	args := append(keychainArgs("find-generic-password", service, account), "-w")
	out, err := exec.CommandContext(ctx, securityBin, args...).Output() //nolint:gosec // a fixed program; the service and account are this package's own
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == securityItemNotFound {
			return nil, fmt.Errorf("the keychain item %q is not there: %w", service, fs.ErrNotExist)
		}
		return nil, fmt.Errorf("reading the keychain item %q: %w", service, err)
	}
	// `security -w` prints the secret followed by a newline.
	return []byte(strings.TrimRight(string(out), "\n")), nil
}

// keychainWrite stores the secret for a generic password, creating the item or
// updating it in place (-U). The secret is never echoed: the error carries
// security's stderr, which does not include it.
func keychainWrite(service, account string, secret []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), securityTimeout)
	defer cancel()
	args := keychainArgs("add-generic-password", service, account)
	args = append(args, "-U", "-w", string(secret))
	cmd := exec.CommandContext(ctx, securityBin, args...) //nolint:gosec // a fixed program; the service and account are this package's own
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("writing the keychain item %q: %w: %s", service, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// keychainExists reports whether the generic password item is present.
func keychainExists(service, account string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), securityTimeout)
	defer cancel()
	args := keychainArgs("find-generic-password", service, account)
	return exec.CommandContext(ctx, securityBin, args...).Run() == nil //nolint:gosec // a fixed program; the service and account are this package's own
}
