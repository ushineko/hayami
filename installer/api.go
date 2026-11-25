package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ushineko/fynstall/extension"

	"github.com/ushineko/hayami/internal/api/unit"
	"github.com/ushineko/hayami/internal/nethelper"
)

// ParamAPI is the parameter the API page sets, declared in fynstall.yaml:
// Yes to install the service other programs read (spec 113 R25), No to
// leave it off or remove it, Auto to install it on a first install and
// leave an upgrade as it found it. An action, not a setting: it is not
// carried to the next install, so --yes never gives back a service someone
// removed.
const ParamAPI = "api"

// Auto is ParamAPI's default.
const Auto = "auto"

// PageAPI is the API page's ID.
const PageAPI = "api"

/*
API is what the installer found of the service, and what it will do: the
check the page is built on. It reads three files and changes nothing.

Asked is the installer having decided about the service before, in this
install directory: its marker is there. fynstall's install half cannot tell
a first install from an upgrade (Context.Upgrade is the old version's
uninstaller's), so hayami keeps its own note. It sets what Auto does for a
service that is not there: set it up the first time, and never again, so a
service someone removed is not given back.
*/
type API struct {
	Offered, Installed, Asked bool
}

// applies is everywhere the service is set up, and Linux without a service
// manager, where the page says how to run it by hand. Windows and macOS come
// in their own phases of spec 113.
func (a API) applies() bool   { return a.Offered || runtime.GOOS == "linux" }
func (a API) why() string     { return unit.OfferWhy }
func (a API) caution() string { return unit.OfferCaution }

// Found says what the check found.
func (a API) Found() string {
	switch {
	case !a.Offered:
		return unit.OfferNotHere
	case a.Installed:
		return "The service is installed, and this install restarts it on the new version. Clear the box to remove it."
	case a.Asked:
		return "The service is not installed. Without it, the readings are hayami's own windows and panes."
	default:
		return "The service is set up with hayami unless you clear the box."
	}
}

// Offer is the question the page asks, or "" when there is nothing to offer.
func (a API) Offer() string {
	switch {
	case !a.Offered:
		return ""
	case a.Installed:
		return "Keep the service other programs read"
	default:
		return "Install the service other programs read"
	}
}

// Steps are what keeping or installing it does, in order.
func (a API) Steps() []string { return unit.OfferSteps() }

// proposed is what the page proposes when nobody has said: keep a service
// that is there, set one up the first time the installer decides, and after
// that leave an install without one as it is.
func (a API) proposed() string {
	if a.Installed || !a.Asked {
		return Yes
	}
	return No
}

// resolve gives the parameter the page's proposal where it is still Auto, so
// the page shows what will be done; a flag's yes or no is kept.
func (a API) resolve(c *extension.Context) {
	if v := c.Param(ParamAPI); v != Yes && v != No {
		_ = c.SetParam(ParamAPI, a.proposed()) // declared in fynstall.yaml
	}
}

// probeAPI finds what this machine has of the service. A variable, so a test
// answers for a machine it is not on.
var probeAPI = func(c *extension.Context) API {
	return API{Offered: unit.Offered(), Installed: unit.Installed(), Asked: markerThere(askedMarker(c))}
}

// askedMarker is the file that says the installer has decided about the
// service in this install: in the install directory, so an upgrade, whose
// uninstall half removes only the files it installed, keeps it, and an
// uninstall takes it.
func askedMarker(c *extension.Context) string {
	return filepath.Join(c.Root(), "share", "api-asked")
}

// markAsked writes the marker, and unmarkAsked removes it. Variables, so a
// test keeps them off the disk.
var (
	markAsked   = func(c *extension.Context) error { return writeMarker(askedMarker(c)) }
	unmarkAsked = func(c *extension.Context) { _ = os.Remove(askedMarker(c)) }
)

// writeMarker writes the marker at path.
func writeMarker(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // the install's own share directory
		return fmt.Errorf("noting the decision: %w", err)
	}
	const text = "The installer has decided about hayami's service here once (spec 113 R25). Uninstalling hayami removes this file.\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil { //nolint:gosec // a note, readable as the install is
		return fmt.Errorf("noting the decision: %w", err)
	}
	return nil
}

// markerThere reports whether the marker is at path.
func markerThere(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// setUpAPI installs the service from the program just installed, or restarts
// it on that program, and removeAPI removes it. Variables, for the same
// reason.
var (
	setUpAPI = func(ctx context.Context, c *extension.Context) error {
		return unit.Install(ctx, filepath.Join(c.Root(), "bin", nethelper.ProgramName)) //nolint:wrapcheck // says which step
	}
	removeAPI = func(ctx context.Context, _ *extension.Context) error {
		return unit.Remove(ctx) //nolint:wrapcheck // says which step
	}
)

/*
apiCLI is the page on the command line. Its question has a default, unlike
the other pages', because the default is not always no: a first install sets
the service up unless the person says otherwise.
*/
func apiCLI(_ context.Context, c *extension.Context, p extension.Prompter) error {
	a := probeAPI(c)
	p.Say("")
	p.Say(a.Found())
	offer := a.Offer()
	if offer == "" {
		return c.SetParam(ParamAPI, No) //nolint:wrapcheck // names the parameter
	}
	a.resolve(c)
	p.Say("")
	p.Say(a.why())
	for _, step := range a.Steps() {
		p.Say("  - " + step)
	}
	p.Say(a.caution())
	for {
		answer, err := p.Ask(offer+"? (yes or no)", c.Param(ParamAPI))
		if err != nil {
			return fmt.Errorf("asking %q: %w", offer, err)
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "yes", "y":
			return c.SetParam(ParamAPI, Yes) //nolint:wrapcheck // names the parameter
		case "no", "n":
			return c.SetParam(ParamAPI, No) //nolint:wrapcheck // names the parameter
		}
		p.Say("Answer yes or no.")
	}
}

/*
placeAPI does what the page was told, after the install. Yes installs the
service, or keeps an installed one on the program just installed: a running
service is the old program until it is restarted. No removes an installed
one. Auto, which is what --yes leaves when no flag said, sets it up on a
first install and leaves an upgrade as it found it, restarted where it is
there. A failure is a warning, as on the other pages: hayami works without
it.
*/
func placeAPI(ctx context.Context, c *extension.Context) {
	want := c.Param(ParamAPI)
	if want == "" {
		// An installer whose fynstall.yaml has no such parameter.
		return
	}
	a := probeAPI(c)
	if want != Yes && want != No {
		want = a.proposed()
	}
	switch {
	case !a.Offered:
		if c.Param(ParamAPI) == Yes {
			c.Log(extension.Warn, unit.OfferNotHere)
		}
		// Nothing was decided here: a service manager that appears later
		// gets the first install's default.
		return
	case want == Yes:
		if err := setUpAPI(ctx, c); err != nil {
			// Not noted as decided, so the next install tries again.
			c.Logf(extension.Warn, "The service other programs read was not set up, and hayami is installed without it: %v", err)
			return
		}
		if a.Installed {
			c.Log(extension.Info, "The service other programs read was kept, on this version")
		} else {
			c.Log(extension.Info, "The service other programs read is set up: hayami-tui api status")
		}
	case a.Installed:
		c.Log(extension.Info, "Removing the service other programs read")
		if err := removeAPI(ctx, c); err != nil {
			warnNotRemoved(c, err)
		}
	}
	if err := markAsked(c); err != nil {
		c.Logf(extension.Warn, "%v: the next install may set the service up again", err)
	}
}

// dropAPI removes the service with an uninstall, and the note that it was
// decided about. Both outlive an upgrade, whose install half keeps the
// service on the new program.
func dropAPI(ctx context.Context, c *extension.Context) {
	if c.Upgrade() {
		return
	}
	unmarkAsked(c)
	if !probeAPI(c).Installed {
		return
	}
	c.Log(extension.Info, "Removing the service other programs read")
	if err := removeAPI(ctx, c); err != nil {
		warnNotRemoved(c, err)
	}
}

// warnNotRemoved says a removal failed, and how to finish it by hand.
func warnNotRemoved(c *extension.Context, err error) {
	c.Logf(extension.Warn, "The service other programs read was not removed: %v. "+
		"To remove it: systemctl --user disable --now %s, then delete %s and %s from ~/.config/systemd/user",
		err, unit.SocketName, unit.SocketName, unit.ServiceName)
}
