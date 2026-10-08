package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// LHMURL is where LibreHardwareMonitor's web server serves its sensor tree
// when it is left on its default port. A setting overrides it (spec 036).
const LHMURL = "http://127.0.0.1:8085/data.json"

// LHMTimeout bounds one request. The server answers a loopback request in
// milliseconds when it answers at all, and a poll must not wait on one that
// does not.
const LHMTimeout = time.Second

// lhmMaxBody caps what is read of an answer. The tree on a desk with two
// drives, three memory modules and a graphics card is about 60 KiB; a body
// many times that is not the tree, and is not worth holding.
const lhmMaxBody = 8 << 20

/*
LHMCPULabels are the processor temperatures looked for, in order: the first a
machine has is the one read. By label, never by index, for the reason hwmon
sensors are (hayami's architecture rules): the sensors' numbers in
LibreHardwareMonitor's identifiers are the order it found them in, and a
version that finds one more moves every number after it.

The order is hwmon.CPU's, for the reason it gives. Tdie first, because Tctl
on some Ryzen parts carries an offset the fan curve wants and a person does
not; where LibreHardwareMonitor knows no offset it publishes one sensor named
for both, which is next; Tctl alone after that. Intel's package temperature
last, under its own name.
*/
var LHMCPULabels = []string{"Core (Tdie)", "Core (Tctl/Tdie)", "Core (Tctl)", "CPU Package"}

// lhmCPUPrefixes are the identifier prefixes of LibreHardwareMonitor's
// processor hardware. A graphics card has a "GPU Core" temperature too, and
// matching a label alone would read it.
var lhmCPUPrefixes = []string{"/amdcpu/", "/intelcpu/"}

/*
LHMNode is one node of LibreHardwareMonitor's sensor tree, as data.json
carries it: hardware, a group of sensors, or a sensor.

**Value and RawValue are text**, "51.5 °C", formatted by the server. Both are
read through LHMText, which takes a number too, because the format has
changed between releases and is not documented. SensorId is not unique: one
measured tree has "/gpu-nvidia/0/load/3" twice. Nothing here keys on it.
*/
type LHMNode struct {
	Text       string    `json:"Text"`
	SensorID   string    `json:"SensorId"`
	Type       string    `json:"Type"`
	Value      LHMText   `json:"Value"`
	RawValue   LHMText   `json:"RawValue"`
	HardwareID string    `json:"HardwareId"`
	Children   []LHMNode `json:"Children"`
}

// LHMText is a field that is text in every answer measured and may be a
// number in another.
type LHMText string

// UnmarshalJSON takes a string, a number, or null.
func (t *LHMText) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*t = LHMText(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		*t = LHMText(n.String())
		return nil
	}
	*t = ""
	return nil
}

/*
ParseLHMValue is the figure at the front of a value: "51.5 °C" is 51.5.

The decimal mark is whichever the server's locale writes, so a comma is read
as one too: "51,5 °C" is 51.5. LibreHardwareMonitor writes no thousands
separator in a temperature, so a comma is never one here.
*/
func ParseLHMValue(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && (s[end] == '-' || s[end] == '.' || s[end] == ',' || (s[end] >= '0' && s[end] <= '9')) {
		end++
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(s[:end], ",", "."), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// LHMCPUTemperature is the processor's temperature in a tree, and the label it
// was read under, in LHMCPULabels' order.
func LHMCPUTemperature(root LHMNode) (float64, string, bool) {
	found := map[string]float64{}
	var walk func(n LHMNode)
	walk = func(n LHMNode) {
		if n.Type == "Temperature" && cpuSensor(n.SensorID) {
			if _, seen := found[n.Text]; !seen {
				if v, ok := lhmReading(n); ok {
					found[n.Text] = v
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	for _, label := range LHMCPULabels {
		if v, ok := found[label]; ok {
			return v, label, true
		}
	}
	return 0, "", false
}

// lhmReading is a sensor's figure: the raw value where it parses, the shown
// one where it does not.
func lhmReading(n LHMNode) (float64, bool) {
	if v, ok := ParseLHMValue(string(n.RawValue)); ok {
		return v, true
	}
	return ParseLHMValue(string(n.Value))
}

func cpuSensor(id string) bool {
	for _, p := range lhmCPUPrefixes {
		if strings.HasPrefix(id, p) {
			return true
		}
	}
	return false
}

// The ways LibreHardwareMonitor can fail to give a temperature, which the
// reason given for each tells apart (spec 036).
var (
	// ErrLHMUnreachable is nothing answering at the address.
	ErrLHMUnreachable = errors.New("LibreHardwareMonitor's web server is not answering")
	// ErrLHMAuth is a server that wants a password.
	ErrLHMAuth = errors.New("LibreHardwareMonitor's web server wants a password")
	// ErrLHMNoSensor is a server that answered with no processor temperature.
	ErrLHMNoSensor = errors.New("LibreHardwareMonitor has no CPU temperature")
)

// FetchLHM reads the sensor tree at url, within the client's timeout, or
// LHMTimeout for a client that has none (NewLHM gives its client LHMTimeout).
// It only ever asks for the tree: the same server takes requests that set a
// fan's speed, and nothing here makes one.
func FetchLHM(ctx context.Context, client *http.Client, url string) (LHMNode, error) {
	if client.Timeout == 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, LHMTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return LHMNode{}, fmt.Errorf("asking %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled) {
			return LHMNode{}, fmt.Errorf("asking %s: %w", url, ctx.Err())
		}
		return LHMNode{}, fmt.Errorf("%w at %s: %w", ErrLHMUnreachable, url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return LHMNode{}, fmt.Errorf("%w at %s", ErrLHMAuth, url)
	case resp.StatusCode != http.StatusOK:
		return LHMNode{}, fmt.Errorf("%s answered %s", url, resp.Status)
	}

	// An answer that says it is over the cap is refused before a byte of it is
	// read; one that does not say (chunked) is read up to the cap and no
	// further.
	if resp.ContentLength > lhmMaxBody {
		return LHMNode{}, tooLarge(url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, lhmMaxBody+1))
	if err != nil {
		return LHMNode{}, fmt.Errorf("reading %s: %w", url, err)
	}
	if len(body) > lhmMaxBody {
		return LHMNode{}, tooLarge(url)
	}
	var root LHMNode
	if err := json.Unmarshal(body, &root); err != nil {
		return LHMNode{}, fmt.Errorf("%s did not answer with a sensor tree: %w", url, err)
	}
	return root, nil
}

// tooLarge is the error for an answer over lhmMaxBody.
func tooLarge(url string) error {
	return fmt.Errorf("%s answered more than %d MiB, which is not a sensor tree", url, lhmMaxBody>>20)
}

/*
LHM reads the processor's temperature from LibreHardwareMonitor (spec 036).

Windows keeps a processor's temperature behind a kernel driver, and hayami
loads none. LibreHardwareMonitor does, through PawnIO, and publishes what it
reads as data.json from a web server of its own. This reads another
program's output over loopback: not a subprocess, and not a device.

Each field is replaceable, so a test serves a tree of its own and probes no
real service or process.
*/
type LHM struct {
	// URL is where the tree is: LHMURL or the setting's.
	URL string
	// Client makes the request.
	Client *http.Client
	// Host says what is installed and running, for the reason given when
	// the server does not answer.
	Host func() LHMHost
}

// LHMHost is what this machine has of LibreHardwareMonitor.
type LHMHost struct {
	// PawnIO is whether the PawnIO driver's service is installed.
	PawnIO bool
	// Running is whether a LibreHardwareMonitor process is running.
	Running bool
	// Task is whether LibreHardwareMonitor's startup task is registered: the
	// one its own Options > Run On Windows Startup makes, and
	// install_windows.ps1 -WithSensors registers (spec 042).
	Task bool
}

// NewLHM reads the tree at url, or at LHMURL when url is empty.
func NewLHM(url string) *LHM {
	if strings.TrimSpace(url) == "" {
		url = LHMURL
	}
	return &LHM{URL: url, Client: &http.Client{Timeout: LHMTimeout}, Host: ProbeLHMHost}
}

/*
CPUTemperature is the processor's temperature, or an Absence saying which
of the ways it can be missing this is.

A server that does not answer is told apart by what the machine has: a
LibreHardwareMonitor that is running has its web server off; one that is not
running was closed with its startup task in place, or has no startup task, or
is not installed (no PawnIO either). Closing its window quits it unless its
Minimize On Close option is on, which is the usual way it stops (spec 042).
*/
func (l *LHM) CPUTemperature(ctx context.Context) (float64, error) {
	root, err := FetchLHM(ctx, l.Client, l.URL)
	if err == nil {
		if v, _, ok := LHMCPUTemperature(root); ok {
			return v, nil
		}
		return 0, &Absence{Code: AbsenceLHMNoSensor, Err: ErrLHMNoSensor,
			Detail: "LibreHardwareMonitor has no CPU temperature: is PawnIO installed? LibreHardwareMonitor offers it on first start"}
	}
	if ctx.Err() != nil {
		return 0, fmt.Errorf("reading LibreHardwareMonitor: %w", ctx.Err())
	}
	switch {
	case errors.Is(err, ErrLHMAuth):
		return 0, &Absence{Code: AbsenceLHMAuth, Err: err,
			Detail: "LibreHardwareMonitor's web server asks for a password, which hayami does not send: turn its authentication off"}
	case errors.Is(err, ErrLHMUnreachable):
		host := LHMHost{}
		if l.Host != nil {
			host = l.Host()
		}
		switch {
		case host.Running:
			return 0, &Absence{Code: AbsenceLHMServerOff, Err: err,
				Detail: "LibreHardwareMonitor is running but its web server is off: Options > Remote Web Server > Run"}
		case host.Task:
			return 0, &Absence{Code: AbsenceLHMTaskStopped, Err: err,
				Detail: "LibreHardwareMonitor is not running, though its startup task is registered: start it as administrator, or log off and on (Options > Minimize On Close keeps a closed window from quitting it)"}
		case host.PawnIO:
			return 0, &Absence{Code: AbsenceLHMNotRunning, Err: err,
				Detail: "LibreHardwareMonitor is not running and does not start with Windows: start it as administrator and turn on Options > Run On Windows Startup, or run install_windows.ps1 -WithSensors"}
		default:
			return 0, &Absence{Code: AbsenceLHMNotInstalled, Err: err,
				Detail: "Windows needs LibreHardwareMonitor and its PawnIO driver for a CPU temperature: see README, On Windows"}
		}
	default:
		return 0, &Absence{Code: AbsenceLHMUnexpected, Err: err, Detail: "LibreHardwareMonitor did not answer as expected: " + err.Error()}
	}
}
