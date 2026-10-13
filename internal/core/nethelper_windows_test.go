package core

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixedNet struct {
	use    NetUse
	err    error
	reads  int
	closed bool
}

func (f *fixedNet) Read(context.Context) (NetUse, error) { f.reads++; return f.use, f.err }
func (f *fixedNet) Close()                               { f.closed = true }

// The helper is asked first. Only its not being installed sends the reading
// on to this process's own trace: any other answer, a reading or a failure,
// is the answer, so two programs never start a session of the same name.
func TestTheHelperIsAskedBeforeThisProcessTraces(t *testing.T) {
	helper := &fixedNet{use: NetUse{Counts: "from the helper"}}
	own := &fixedNet{use: NetUse{Counts: "from this process"}}
	c := &chainNet{helper: helper, own: own}

	use, err := c.Read(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "from the helper", use.Counts)
	assert.Zero(t, own.reads)

	helper.err = errors.New("the helper closed its pipe")
	_, err = c.Read(t.Context())
	require.Error(t, err)
	assert.Zero(t, own.reads, "a helper that failed is not papered over with a second session")

	helper.err = errNoNetHelper
	use, err = c.Read(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "from this process", use.Counts)

	c.Close()
	assert.True(t, helper.closed)
	assert.True(t, own.closed)
}

// With no service registered the reader says so in the one way the chain
// understands.
func TestAHelperThatIsNotThereIsSaidAsThat(t *testing.T) {
	r := NewNetHelperReader(`\\.\pipe\hayami-test-nobody-serves-this`, func() error { return errNoNetHelper }, nil)
	defer r.Close()
	_, err := r.Read(t.Context())
	assert.ErrorIs(t, err, errNoNetHelper)
}
