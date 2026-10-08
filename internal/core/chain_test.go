package core_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// answers is a provider that says v, and counts how often it was asked.
func answers[T any](name string, v T, asked *int) core.Provider[T] {
	return core.ProviderFunc[T]{N: name, F: func(context.Context) (T, error) {
		*asked++
		return v, nil
	}}
}

// fails is a provider that has nothing, for the reason err.
func fails[T any](name string, err error, asked *int) core.Provider[T] {
	return core.ProviderFunc[T]{N: name, F: func(context.Context) (T, error) {
		*asked++
		var zero T
		return zero, err
	}}
}

// Spec 043. A chain with no Merge takes the first provider that answers and
// asks nobody after it; the ones before it that had nothing are on the record.
func TestAChainTakesTheFirstAnswerAndAsksNoFurther(t *testing.T) {
	var a, b, c int
	ch := core.Chain[float64]{Providers: []core.Provider[float64]{
		fails[float64]("coretemp/Package id 0", errors.New("no such sensor"), &a),
		answers("k10temp/Tdie", 51.5, &b),
		answers("k10temp/Tctl", 61.5, &c),
	}}

	o := ch.Read(t.Context())

	require.True(t, o.Answered)
	assert.InDelta(t, 51.5, o.Value, 0)
	assert.Equal(t, []string{"coretemp/Package id 0", "k10temp/Tdie"}, o.Tried)
	assert.Equal(t, []int{1, 1, 0}, []int{a, b, c}, "the provider after the answer was asked")
}

// Spec 043. A chain that found nothing has tried every provider, in order,
// and keeps the first account a provider gave of its own absence -- not the
// last error, and not a later provider's account.
func TestAChainThatFindsNothingKeepsTheFirstAccount(t *testing.T) {
	first := &core.Absence{Code: core.AbsenceLHMServerOff, Detail: "web server off"}
	later := &core.Absence{Code: core.AbsenceLHMNoSensor, Detail: "no PawnIO"}
	var n int
	ch := core.Chain[float64]{Providers: []core.Provider[float64]{
		fails[float64]("one", errors.New("plain"), &n),
		fails[float64]("two", first, &n),
		fails[float64]("three", later, &n),
	}}

	o := ch.Read(t.Context())

	assert.False(t, o.Answered)
	assert.Equal(t, []string{"one", "two", "three"}, o.Tried)
	assert.Same(t, first, o.Absence)
	assert.EqualError(t, o.Err, "plain", "the first error is kept beside the first account")
	assert.Equal(t, ch.Names(), o.Tried, "a chain that found nothing tried what Names says")
}

// pair is a reading two providers answer halves of.
type pair struct {
	a, b       int
	hasA, hasB bool
}

func mergePair(have, got pair) (pair, bool) {
	if !have.hasA && got.hasA {
		have.a, have.hasA = got.a, true
	}
	if !have.hasB && got.hasB {
		have.b, have.hasB = got.b, true
	}
	return have, have.hasA && have.hasB
}

// Spec 043. With a Merge, what one provider gave is kept and the next fills
// only what is missing; the chain stops the moment the reading is complete.
func TestAMergingChainFillsWhatIsMissingAndStopsWhenComplete(t *testing.T) {
	var x, y, z int
	ch := core.Chain[pair]{Merge: mergePair, Providers: []core.Provider[pair]{
		answers("temperature", pair{a: 41, hasA: true}, &x),
		answers("both", pair{a: 99, hasA: true, b: 4, hasB: true}, &y),
		answers("never", pair{a: 1, hasA: true, b: 1, hasB: true}, &z),
	}}

	o := ch.Read(t.Context())

	require.True(t, o.Answered)
	assert.Equal(t, pair{a: 41, hasA: true, b: 4, hasB: true}, o.Value, "the first answer's half was replaced")
	assert.Equal(t, []int{1, 1, 0}, []int{x, y, z}, "a complete reading went on asking")
}

// Spec 043. A cancelled poll stops the chain where it is, with what it had.
func TestACancelledChainStops(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var a, b int
	ch := core.Chain[float64]{Providers: []core.Provider[float64]{
		core.ProviderFunc[float64]{N: "cancels", F: func(context.Context) (float64, error) {
			a++
			cancel()
			return 0, errors.New("nothing")
		}},
		answers("after", 1.0, &b),
	}}

	o := ch.Read(ctx)

	assert.False(t, o.Answered)
	assert.Equal(t, 0, b, "a provider was asked after the poll was cancelled")
	assert.Equal(t, []string{"cancels"}, o.Tried)
}
