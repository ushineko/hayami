//go:build !windows

package gui

import (
	"errors"
	"syscall"
)

/*
Ending a process, where a signal is how it is asked.

TERM first and KILL after, which is what endAll's force says. A process that
has already gone is neither sent to nor refused: ESRCH is the outcome that
was wanted, and counting it as a refusal would report a successful kill as a
half-failed one.
*/
func endAll(pids []int, force bool) (sent, refused int, err error) {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	for _, pid := range pids {
		switch e := syscall.Kill(pid, sig); {
		case e == nil:
			sent++
		case errors.Is(e, syscall.ESRCH):
			// Already gone, which is what was asked for.
		default:
			// Anything this user did not start, which is the ordinary case
			// for anything root did.
			refused++
		}
	}
	return sent, refused, nil
}

// alive is which of the processes are still there, asked with signal 0.
func alive(pids []int) []int {
	var out []int
	for _, pid := range pids {
		if syscall.Kill(pid, 0) == nil {
			out = append(out, pid)
		}
	}
	return out
}
