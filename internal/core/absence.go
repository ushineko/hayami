package core

import "errors"

// AbsenceCode names one way a reading can be missing. A caller decides on the
// code -- whether a line stays on the card, whether a test saw the right
// cause -- and never on the words, which are free to change (spec 040).
type AbsenceCode string

// The ways a reading is known to be missing, by the source that knows.
const (
	// AbsenceCPUSensor is no processor temperature on this platform's route.
	AbsenceCPUSensor AbsenceCode = "cpu-sensor"
	// AbsenceGPUSensor is no graphics card temperature on any route this
	// platform tried.
	AbsenceGPUSensor AbsenceCode = "gpu-sensor"
	// AbsenceLHMNoSensor is LibreHardwareMonitor answering with no processor
	// temperature: PawnIO, most likely, is not installed.
	AbsenceLHMNoSensor AbsenceCode = "lhm-no-sensor"
	// AbsenceLHMAuth is LibreHardwareMonitor's web server asking for a
	// password hayami does not send.
	AbsenceLHMAuth AbsenceCode = "lhm-auth"
	// AbsenceLHMServerOff is LibreHardwareMonitor running with its web server
	// off.
	AbsenceLHMServerOff AbsenceCode = "lhm-server-off"
	// AbsenceLHMNotRunning is PawnIO installed and LibreHardwareMonitor not
	// started.
	AbsenceLHMNotRunning AbsenceCode = "lhm-not-running"
	// AbsenceLHMNotInstalled is neither LibreHardwareMonitor nor PawnIO.
	AbsenceLHMNotInstalled AbsenceCode = "lhm-not-installed"
	// AbsenceLHMUnexpected is LibreHardwareMonitor answering in a way this
	// build does not read.
	AbsenceLHMUnexpected AbsenceCode = "lhm-unexpected"
	// AbsenceWirelessDenied is Windows withholding the Wi-Fi details from a
	// desktop app without location access.
	AbsenceWirelessDenied AbsenceCode = "wireless-denied"
	// AbsencePermission is a device the system would not let this program
	// open.
	AbsencePermission AbsenceCode = "permission"
)

/*
Absence is a reading that is not there, told by the source that knows why.

The words live here, beside the code that tried, so they cannot drift from
what it tried: Text is the short verdict ("details withheld"), empty to leave
the caller's; Detail is the sentence the hover note and doctor show; Actionable
marks the one kind of line a reader can do something about now, which a full
card keeps drawn. Err is what went wrong underneath, for errors.Is and logs.

Two absences are the same error when their codes are, so a sentinel like
ErrWirelessDenied is matched by errors.Is however it was wrapped or copied.
*/
type Absence struct {
	Code       AbsenceCode
	Text       string
	Detail     string
	Actionable bool
	Err        error
}

// Error is the detail: the sentence a person reads.
func (a *Absence) Error() string { return a.Detail }

// Unwrap is the error underneath.
func (a *Absence) Unwrap() error { return a.Err }

// Is matches another absence with the same code.
func (a *Absence) Is(target error) bool {
	var t *Absence
	return errors.As(target, &t) && t.Code != "" && t.Code == a.Code
}

// SensorAbsence is the name spec 036 gave an absence, kept for its callers.
type SensorAbsence = Absence

// AbsenceOf is the absence anywhere in err's chain.
func AbsenceOf(err error) (*Absence, bool) {
	var a *Absence
	if errors.As(err, &a) {
		return a, true
	}
	return nil, false
}
