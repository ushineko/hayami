package panel

import (
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
			base.Detail = a.Detail
		}
		base.Actionable = a.Actionable
		return base
	}
	if base.Detail == "" && err != nil {
		base.Detail = err.Error()
	}
	return base
}
