package gui

import "errors"

/*
Ending a process is not done on Windows yet.

Windows has no signals: there is no TERM to ask with and no process group to
ask across, and ending a tree there means opening each process and calling
TerminateProcess, which is a different piece of work from the one this does.
The page says so rather than reporting nothing sent as though every process
had refused.

In practice the offer does not arrive here: the kill is carried on a page
built from /proc (panel.ProcessLines), which a Windows machine does not
have, so there is no button to press. This is what answers if there ever is.
*/
func endAll([]int, bool) (sent, refused int, err error) {
	return 0, 0, errors.New("ending a process is not done on Windows yet")
}

// alive reports every process as still there: with no way to ask, the
// honest answer is not to claim they have gone.
func alive(pids []int) []int { return pids }
