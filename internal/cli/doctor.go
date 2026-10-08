package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
Doctor reports what every section this build knows found, and what it did not.

**The report a person pastes into an issue.** Diagnosing a machine where three
of four sections were missing took ssh, a throwaway binary built against the
internal packages, and half an hour, because neither panel nor `readings` would
say why anything was absent (issue #54). Everything that took is here.

Every section, not only the configured ones. "I turned it off" is one of the
answers somebody needs, and a section that is off reports `off` rather than not
appearing -- which is the same mistake in miniature.

Nothing here prints a token, a credential or a path inside a credential store.
A profile name appears, because the cache filenames already carry it and the
panel already draws it.
*/

// State is what doctor says about one section.
type State string

// The states, in the order they are worth worrying about.
const (
	// StateOK is readings and nothing missing.
	StateOK State = "ok"
	// StatePartial is readings with something missing beside them: a cooler
	// with a processor and no coolant.
	StatePartial State = "partial"
	// StateAbsent is no readings, and a reason for having none.
	StateAbsent State = "absent"
	// StateSilent is no readings and no reason, which is a source that should
	// have said something and did not. It is the state this whole command
	// exists to make impossible, so it is reported loudly rather than
	// quietly.
	StateSilent State = "silent"
	// StateOff is a section the settings do not ask for.
	StateOff State = "off"
)

// Finding is one section's line in the report.
type Finding struct {
	Key     string
	State   State
	Summary string
	Reasons []view.Reason

	// Names are the full names the section's labels were shortened from,
	// "CPU: Intel(R) Core(TM) i9-14900K" (spec 031). The window shows them on
	// hover; a report has no hover, so it prints them under the summary.
	Names []string

	// Err is a poll that failed outright, as distinct from a section that
	// reported what it could not read.
	Err error
}

/*
Diagnose polls every section this build knows and says what each one found.

configured names the sections the settings ask for; the rest are still polled,
because a section being off is not a reason to be unable to say whether it
would have worked.
*/
func Diagnose(ctx context.Context, configured []string, env panel.Env) []Finding {
	on := make(map[string]bool, len(configured))
	for _, k := range configured {
		on[k] = true
	}

	return diagnose(ctx, on, panel.Sources(panel.Keys(), env))
}

// diagnose is Diagnose over sources already built, which is the seam a test
// uses to give it a section of its own.
func diagnose(ctx context.Context, on map[string]bool, sources []panel.Source) []Finding {
	out := make([]Finding, 0, len(sources))
	for _, s := range sources {
		drawn, err := s.Poll(ctx)
		sec := s.Section()
		f := Finding{Key: s.Key(), Reasons: sec.Reasons, Err: err, Summary: summarise(sec), Names: names(sec)}

		switch {
		case !on[s.Key()]:
			f.State = StateOff
		case drawn && !missing(sec.Reasons):
			f.State = StateOK
		case drawn:
			f.State = StatePartial
		case len(sec.Reasons) > 0 || err != nil:
			f.State = StateAbsent
		default:
			f.State = StateSilent
		}
		out = append(out, f)
	}
	return out
}

/*
missing reports whether any reason is one a card shows.

An Aside reason is kept off the card because it is not worth a line there --
"no GPU sensor" on a machine that has no card this build can read -- and a
section whose only reasons are those is not partial: it read everything it
draws. doctor still lists the reason under it.
*/
func missing(reasons []view.Reason) bool {
	for _, r := range reasons {
		if !r.Aside {
			return true
		}
	}
	return false
}

/*
summarise is the one line a section leads with: what it did read.

The section's own rows, not a source's internals. A doctor that reached past
view.Section for its summary would be a third renderer of the same readings,
and the two that exist already have a parity test holding them together.
*/
func summarise(s view.Section) string {
	var parts []string
	for _, r := range s.Rows {
		if value := tidy(r.Value + " " + r.Unit); value != "" {
			parts = append(parts, tidy(r.Label+" "+value))
		}
	}
	for _, c := range s.Cells {
		if c.Placeholder {
			// An empty slot is the card keeping its shape, not a reading.
			continue
		}
		parts = append(parts, tidy(c.Label+" "+c.Value+c.Unit))
	}
	for _, m := range s.Meters {
		// The caption already carries the window, so the label and the
		// caption are the whole of it. The three name parts are columns in a
		// pane and this is one line of prose.
		parts = append(parts, tidy(m.Label+" "+m.Caption))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

// names are the rows' full names, in the rows' order.
func names(s view.Section) []string {
	var out []string
	for _, r := range s.Rows {
		if r.Tip != "" {
			out = append(out, r.Tip)
		}
	}
	return out
}

// tidy collapses the padding a section carries for its columns. A pane aligns
// its values by padding them, and a report that repeated the padding would be
// a wall of spaces.
func tidy(s string) string { return strings.Join(strings.Fields(s), " ") }

// column is how wide the key and state columns are. Fixed rather than
// measured: the keys are a closed set this build owns, and a report whose
// columns move between two machines is one nobody can diff.
const (
	keyColumn   = 14
	stateColumn = 8
)

// Report writes the findings as the plain text a person reads.
func Report(w io.Writer, findings []Finding) error {
	for _, f := range findings {
		lead := f.Summary
		if lead == "" && f.State == StateOff {
			lead = "not in the settings"
		}
		if err := line(w, f.Key, string(f.State), lead); err != nil {
			return err
		}
		for _, n := range f.Names {
			if err := line(w, "", "", n); err != nil {
				return err
			}
		}
		for _, r := range f.Reasons {
			if err := line(w, "", "", reasonText(r)); err != nil {
				return err
			}
			if r.Detail != "" {
				if err := line(w, "", "", "  "+r.Detail); err != nil {
					return err
				}
			}
		}
		if f.Err != nil {
			if err := line(w, "", "", "! "+f.Err.Error()); err != nil {
				return err
			}
		}
	}
	return nil
}

// reasonText is a reason as one line of the report, marked when it is a
// failure rather than an absence.
func reasonText(r view.Reason) string {
	text := r.Text
	if r.Label != "" {
		text = r.Label + ": " + text
	}
	if r.Status == view.Warn || r.Status == view.Bad {
		return "! " + text
	}
	return text
}

// line writes one row of the report, indenting a continuation under the
// columns the first line established.
func line(w io.Writer, key, state, text string) error {
	if text == "" && key == "" {
		return nil
	}
	// Trimmed on the right: a state with no summary beside it would end the
	// line in padding, and a report that is pasted into an issue should not
	// carry trailing whitespace into it.
	row := strings.TrimRight(fmt.Sprintf("%-*s%-*s%s", keyColumn, key, stateColumn, state, text), " ")
	if _, err := fmt.Fprintln(w, row); err != nil {
		return fmt.Errorf("writing the report: %w", err)
	}
	return nil
}

// Keys are the findings' keys in report order, for a test that wants to know
// what was covered without parsing the text.
func Keys(findings []Finding) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.Key)
	}
	sort.Strings(out)
	return out
}
