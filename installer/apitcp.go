package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ushineko/fynstall/extension"

	"github.com/ushineko/hayami/internal/api/tcpkey"
	"github.com/ushineko/hayami/internal/api/unit"
)

// ParamAPITCP is the parameter the Port page sets, declared in
// fynstall.yaml: Yes to serve the service on its loopback TCP port as well
// (spec 115 R12), No to leave it off or remove it, Auto to set it up on a
// first install and leave an upgrade as it found it, as ParamAPI does.
const ParamAPITCP = "apitcp"

// PageAPITCP is the Port page's ID.
const PageAPITCP = "apitcp"

/*
APITCP is what the installer found of the service's TCP port: the check the
Port page is built on. Service is the service set up or about to be (the
port is the service's, and without it there is nothing to offer); Set is the
port's key there, one the service may serve with; Asked is the installer
having decided about the port before, in this install directory.
*/
type APITCP struct {
	Service, Set, Asked bool
	Port                int
}

func (a APITCP) applies() bool { return a.Service }

func (a APITCP) why() string {
	return "Some programs, a Herdr panel among them, can open a TCP connection but not the service's socket. " +
		"The port answers only programs that send the token kept in " + tcpkey.FileName + " in your settings."
}

func (a APITCP) caution() string {
	return "Any program running as you can read the token, as it can read your other credentials. " +
		"Other accounts on this computer reach the port and are refused without it; it is on 127.0.0.1, so no other computer reaches it."
}

// Found says what the check found.
func (a APITCP) Found() string {
	switch {
	case a.Set:
		return fmt.Sprintf("The service also listens on %s. Clear the box to remove the port.", tcpkey.Address(a.Port))
	case a.Asked:
		return "The service listens on its socket only."
	default:
		return "The service also listens on a local TCP port unless you clear the box."
	}
}

// Offer is the question the page asks.
func (a APITCP) Offer() string {
	if a.Set {
		return "Keep the service's local TCP port"
	}
	return "Also serve on a local TCP port"
}

// Steps are what keeping or setting up the port does, in order.
func (a APITCP) Steps() []string {
	steps := []string{"writes a token to " + tcpkey.FileName + " in your settings, readable by your account alone"}
	addr := tcpkey.Address(a.Port)
	switch runtime.GOOS {
	case "windows":
		steps = append(steps, "your service listens on "+addr+" from its next start")
	case "darwin":
		steps = append(steps, "adds "+addr+" to your service's launchd agent, which starts the service when a program connects")
	default:
		steps = append(steps, "adds a systemd socket on "+addr+" that starts the service when a program connects")
	}
	return steps
}

// proposed is what the page proposes when nobody has said, as for the
// service: keep a port that is there, set one up the first time, and after
// that leave an install without one as it is.
func (a APITCP) proposed() string {
	if a.Set || !a.Asked {
		return Yes
	}
	return No
}

// resolve gives the parameter the page's proposal where it is still Auto,
// so the box shows what will be done (the window asks for it); a flag's yes
// or no is kept.
func (a APITCP) resolve(c *extension.Context) {
	if v := c.Param(ParamAPITCP); v != Yes && v != No {
		_ = c.SetParam(ParamAPITCP, a.proposed()) // declared in fynstall.yaml
	}
}

// probeAPITCP finds what this machine has of the port. A variable, so a
// test answers for a machine it is not on.
var probeAPITCP = func(c *extension.Context) APITCP {
	a := APITCP{Asked: markerThere(tcpAskedMarker(c)), Port: tcpkey.DefaultPort}
	api := probeAPI(c)
	a.Service = api.Offered && (api.Installed || c.Param(ParamAPI) == Yes || (c.Param(ParamAPI) != No && api.proposed() == Yes))
	if p, err := tcpkey.Path(); err == nil {
		if k, err := tcpkey.Read(p); err == nil {
			a.Set, a.Port = true, k.Port()
		}
	}
	return a
}

// tcpAskedMarker is the Port page's note that it was decided, beside the
// Service page's.
func tcpAskedMarker(c *extension.Context) string {
	return filepath.Join(c.Root(), "share", "api-tcp-asked")
}

// The port's steps and its marker. Variables, so a test keeps them off the
// disk and the service manager.
var (
	setUpAPITCP     = setUpAPITCPHere
	removeAPITCP    = removeAPITCPHere
	markTCPAsked    = func(c *extension.Context) error { return writeMarker(tcpAskedMarker(c)) }
	unmarkTCPAsked  = func(c *extension.Context) { _ = os.Remove(tcpAskedMarker(c)) }
	apiTCPInstalled = func() bool { return unit.Installed() }
)

// setUpAPITCPHere sets the port up on the installed service, as the person.
// On Windows the program is not needed: the key is all there is to write.
func setUpAPITCPHere(ctx context.Context, c *extension.Context, port int) error {
	return unit.InstallTCP(ctx, helperProgram(c), port, false) //nolint:wrapcheck // says which step
}

// removeAPITCPHere takes the port away, as the person.
func removeAPITCPHere(ctx context.Context, c *extension.Context) error {
	return unit.RemoveTCP(ctx, helperProgram(c)) //nolint:wrapcheck // says which step
}

// apiTCPCLI is the Port page on the command line, with a default as the
// Service page has.
func apiTCPCLI(_ context.Context, c *extension.Context, p extension.Prompter) error {
	a := probeAPITCP(c)
	if !a.applies() {
		return c.SetParam(ParamAPITCP, No) //nolint:wrapcheck // names the parameter
	}
	a.resolve(c)
	p.Say("")
	p.Say(a.Found())
	p.Say("")
	p.Say(a.why())
	for _, step := range a.Steps() {
		p.Say("  - " + step)
	}
	p.Say(a.caution())
	for {
		answer, err := p.Ask(a.Offer()+"? (yes or no)", c.Param(ParamAPITCP))
		if err != nil {
			return fmt.Errorf("asking %q: %w", a.Offer(), err)
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "yes", "y":
			return c.SetParam(ParamAPITCP, Yes) //nolint:wrapcheck // names the parameter
		case "no", "n":
			return c.SetParam(ParamAPITCP, No) //nolint:wrapcheck // names the parameter
		}
		p.Say("Answer yes or no.")
	}
}

/*
placeAPITCP does what the Port page was told, after the service is placed.
The port is the service's: with no service installed there is no port, and a
key left from one is removed. Yes sets the port up, keeping its token and
port; No removes it; Auto sets it up on a first install and leaves an
upgrade as it found it. A failure is a warning: the socket serves without it.
*/
func placeAPITCP(ctx context.Context, c *extension.Context) {
	want := c.Param(ParamAPITCP)
	if want == "" {
		return
	}
	a := probeAPITCP(c)
	if want != Yes && want != No {
		want = a.proposed()
	}
	switch {
	case !apiTCPInstalled():
		if a.Set {
			if err := removeAPITCP(ctx, c); err != nil {
				c.Logf(extension.Warn, "The service's TCP port key was not removed: %v", err)
			}
		}
		return
	case want == Yes:
		if err := setUpAPITCP(ctx, c, a.Port); err != nil {
			c.Logf(extension.Warn, "The service's TCP port was not set up; the socket serves without it: %v", err)
			return
		}
		if a.Set {
			c.Logf(extension.Info, "The service's TCP port was kept, on %s", tcpkey.Address(a.Port))
		} else {
			c.Logf(extension.Info, "The service also listens on %s, for programs holding the token in %s", tcpkey.Address(a.Port), tcpkey.FileName)
		}
	case a.Set:
		c.Log(extension.Info, "Removing the service's TCP port")
		if err := removeAPITCP(ctx, c); err != nil {
			c.Logf(extension.Warn, "The service's TCP port was not removed: %v. To remove it: hayami-tui api-service tcp --remove", err)
		}
	}
	if err := markTCPAsked(c); err != nil {
		c.Logf(extension.Warn, "%v: the next install may set the port up again", err)
	}
}

// dropAPITCP removes the port's key with an uninstall, and the note that it
// was decided about; an upgrade keeps both.
func dropAPITCP(ctx context.Context, c *extension.Context) {
	if c.Upgrade() {
		return
	}
	unmarkTCPAsked(c)
	if !probeAPITCP(c).Set {
		return
	}
	if err := removeAPITCP(ctx, c); err != nil {
		c.Logf(extension.Warn, "The service's TCP port key was not removed: %v", err)
	}
}
