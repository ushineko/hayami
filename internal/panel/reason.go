package panel

import (
	"os"
	"strings"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/view"
)

/*
reason is the one way an error becomes a view.Reason (spec 040).

base carries what the section knows: the label, the status, whether the line
is aside, the verdict, and a detail for an error that does not account for
itself. A core.Absence anywhere in err's chain is the source's own account and
supplies its verdict and detail, where it has them, and whether a reader can
act on it. Any other error gives its message as the detail, unless base
already has one.
*/
func reason(base view.Reason, err error) view.Reason {
	if a, ok := core.AbsenceOf(err); ok {
		if a.Text != "" {
			base.Text = a.Text
		}
		if a.Detail != "" {
			base.Detail = withoutHome(a.Detail)
		}
		base.Actionable = a.Actionable
		return base
	}
	if base.Detail == "" && err != nil {
		base.Detail = withoutHome(err.Error())
	}
	return base
}

/*
withoutHome is an error's text with the user's home directory written as ~.

An error from the filesystem names the file, and the file is under the
user's home: `open /home/<user>/.claude/.credentials.json: permission
denied`. A reason ends up in `doctor` and the readings JSON, which people
paste into bug reports; the user's name does not go with it (spec 076).
*/
func withoutHome(text string) string {
	home, err := userHome()
	if err != nil || len(home) < 2 {
		return text
	}
	return strings.ReplaceAll(text, home, "~")
}

// userHome is os.UserHomeDir; a variable, so a test can name a home.
var userHome = os.UserHomeDir
