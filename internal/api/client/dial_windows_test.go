package client_test

import (
	"context"
	"os"
	"testing"

	"github.com/Microsoft/go-winio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api/client"
)

// The client dials a pipe this account's process serves.
func TestTheClientDialsThisAccountsPipe(t *testing.T) {
	l, addr := listen(t)
	t.Cleanup(func() { _ = l.Close() })
	// A pipe takes a client only once it is accepting.
	go func() {
		if c, err := l.Accept(); err == nil {
			_ = c.Close()
		}
	}()
	c, err := client.Dial(context.Background(), addr)
	require.NoError(t, err)
	assert.Equal(t, addr, c.RemoteAddr().String())
	require.NoError(t, c.Close())
}

/*
A pipe at the address served by another account's process is refused before
a byte is written: someone who made the pipe first, under the name the
service would have taken. It needs a second account, so it runs on the
Windows VM, in two halves of this test:

	HAYAMI_SQUAT=serve HAYAMI_SQUAT_PIPE=<name>  run as SYSTEM (a scheduled
	  task): makes the pipe open to everyone and holds it.
	HAYAMI_SQUAT=dial HAYAMI_SQUAT_PIPE=<name>   run as the person: dials it.
*/
func TestAPipeOfAnotherAccountIsRefused(t *testing.T) {
	pipe := os.Getenv("HAYAMI_SQUAT_PIPE")
	switch os.Getenv("HAYAMI_SQUAT") {
	case "serve":
		l, err := winio.ListenPipe(pipe, &winio.PipeConfig{SecurityDescriptor: "D:(A;;GA;;;WD)"})
		require.NoError(t, err)
		defer func() { _ = l.Close() }()
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	case "dial":
		_, err := client.Dial(context.Background(), pipe)
		require.ErrorIs(t, err, client.ErrNotOurs)
	default:
		t.Skip("needs a second account: HAYAMI_SQUAT=serve as it, then HAYAMI_SQUAT=dial as this one")
	}
}
