package installer

import (
	"context"
	"errors"

	"github.com/ushineko/fynstall/extension"
)

/*
No test asks the machine it runs on for an administrator (spec 078 R7).
Every step that would (installing or removing the network helper, setting
up LibreHardwareMonitor) starts each test run as a stand-in that refuses at
once, and a test that wants one puts in its own (helperCalls, answerSetUp).
A test once reached the real removal through a path nobody had faked, and
waited on a polkit prompt on the desk running it until it timed out.
*/
func init() {
	refuse := func(context.Context, *extension.Context) error { return errNoElevationInTests }
	setUpNet, removeNet, setUp = refuse, refuse, refuse
}

var errNoElevationInTests = errors.New("a test reached a step that asks for an administrator")
