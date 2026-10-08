package core

import (
	"context"
	"errors"
)

/*
Provider is one source of a reading: a hwmon sensor, LibreHardwareMonitor,
D3DKMT, nvidia-smi (spec 043). Name is what a person is told was tried, in the
reason given when nothing answered, so it is written for them ("k10temp/Tdie",
"nvidia-smi"). Read returns the reading, or an error -- a *Absence where the
provider can say why it has none.
*/
type Provider[T any] interface {
	Name() string
	Read(ctx context.Context) (T, error)
}

// ProviderFunc is a Provider made of a name and a function.
type ProviderFunc[T any] struct {
	N string
	F func(context.Context) (T, error)
}

// Name is N.
func (p ProviderFunc[T]) Name() string { return p.N }

// Read calls F.
func (p ProviderFunc[T]) Read(ctx context.Context) (T, error) { return p.F(ctx) }

/*
Chain is the providers of one reading, in the order they are asked, and how
their answers combine (docs/architecture.md, rule 4).

A platform declares its chains in its host file; adding a source is a provider
in the list, and the reason given when none answers names every one tried,
from the chain's own record, so the words cannot drift from what was asked.

Merge folds a provider's answer into what the chain has so far and says
whether the reading is complete, which stops the chain: a graphics card's
temperature from one provider and its load from the next. A nil Merge takes
the first answer and stops.
*/
type Chain[T any] struct {
	Providers []Provider[T]
	Merge     func(have, got T) (T, bool)
}

// Outcome is what a chain read: the merged value, whether any provider
// answered, every provider asked in order, and the first account a provider
// gave of its own absence (or failing that the first error).
type Outcome[T any] struct {
	Value    T
	Answered bool
	Tried    []string
	Absence  *Absence
	Err      error
}

// Read asks the providers in order until Merge says the reading is complete or
// the list is done. A cancelled context stops it, with what it had.
func (c Chain[T]) Read(ctx context.Context) Outcome[T] {
	var out Outcome[T]
	for _, p := range c.Providers {
		if err := ctx.Err(); err != nil {
			if out.Err == nil {
				out.Err = err
			}
			return out
		}
		out.Tried = append(out.Tried, p.Name())
		got, err := p.Read(ctx)
		if err != nil {
			if a, ok := AbsenceOf(err); ok && out.Absence == nil {
				out.Absence = a
			}
			if out.Err == nil {
				out.Err = err
			}
			continue
		}
		if c.Merge == nil {
			out.Value, out.Answered = got, true
			return out
		}
		// The first answer merges into the zero value, which Merge must take
		// as "nothing yet".
		var complete bool
		out.Value, complete = c.Merge(out.Value, got)
		out.Answered = true
		if complete {
			return out
		}
	}
	return out
}

// Names are the chain's providers' names, in order: what a read that found
// nothing would have tried.
func (c Chain[T]) Names() []string {
	out := make([]string, 0, len(c.Providers))
	for _, p := range c.Providers {
		out = append(out, p.Name())
	}
	return out
}

// errNothing is a provider that answered with nothing in it.
var errNothing = errors.New("nothing to report")
