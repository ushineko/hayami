/*
Package sensorhelper is hayami's sensor helper on Windows (spec 103): a
service that reads the processor's temperature through PawnIO, which serves
only an administrator, and hands it to a panel that runs as the person at
the desk. It replaces LibreHardwareMonitor as where that temperature comes
from.

It runs as LocalSystem, because PawnIO's device opens for SYSTEM and
administrators only, and only while a panel reads it. Its program is a copy
of hayami-tui in Program Files, beside the two signed PawnIO modules it
loads, all of which only an administrator can replace. It loads a module
only when its bytes are the ones this program carries, and calls only the
module's read functions, through sanshoku's cpu package. It writes down a
pipe and reads nothing back, and the pipe opens for the one account it was
installed for.

Elsewhere the kernel gives the temperature without help, and there is no
helper.
*/
package sensorhelper

import "errors"

// ErrNotWindows is every operation here on a system with no helper.
var ErrNotWindows = errors.New("the sensor helper is for Windows")
