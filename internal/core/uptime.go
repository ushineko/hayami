package core

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// UptimePath is where Linux says how long it has been up.
const UptimePath = "/proc/uptime"

var errNoUptime = errors.New("no uptime")

/*
ParseUptime is the first field of /proc/uptime, the seconds since boot:
"350735.47 234388.90". It is CLOCK_BOOTTIME, so time spent suspended counts,
as uptime(1) counts it (spec 099).

A field that is not a finite, non-negative number is an error, not a zero: a
zero would draw a machine that has just booted.
*/
func ParseUptime(s string) (time.Duration, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0, errNoUptime
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || math.IsNaN(v) || v < 0 || v > math.MaxInt64/float64(time.Second) {
		return 0, fmt.Errorf("%q: %w", fields[0], errNoUptime)
	}
	return time.Duration(v * float64(time.Second)), nil
}

// ProcUptime is the uptime from path, /proc/uptime or a test's file.
func ProcUptime(path string) func(context.Context) (time.Duration, error) {
	return func(context.Context) (time.Duration, error) {
		body, err := os.ReadFile(path) //nolint:gosec // procfs, or a test's file
		if err != nil {
			return 0, fmt.Errorf("reading %s: %w", path, err)
		}
		d, err := ParseUptime(string(body))
		if err != nil {
			return 0, fmt.Errorf("%s: %w", path, err)
		}
		return d, nil
	}
}
