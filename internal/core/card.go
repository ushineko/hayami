package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

/*
Card is the graphics card as the GPU drawer's summary says it (spec 096): its
memory used of its total, its graphics and memory clocks, and the power it
draws of its limit. Each part says whether it was read, and a card with some
of them is a reading.
*/
type Card struct {
	VRAMUsed  float64 `json:"vram_used,omitempty"`
	VRAMTotal float64 `json:"vram_total,omitempty"`
	HasVRAM   bool    `json:"has_vram,omitempty"`

	// Clock and MemClock are in MHz.
	Clock       float64 `json:"clock,omitempty"`
	HasClock    bool    `json:"has_clock,omitempty"`
	MemClock    float64 `json:"mem_clock,omitempty"`
	HasMemClock bool    `json:"has_mem_clock,omitempty"`

	// Power is what the card draws and PowerLimit what it may, in watts;
	// the limit is zero where it is not known.
	Power      float64 `json:"power,omitempty"`
	PowerLimit float64 `json:"power_limit,omitempty"`
	HasPower   bool    `json:"has_power,omitempty"`
}

// SMICardFlag is what nvidia-smi is asked for the summary, in the order
// ParseSMICard expects: a constant, as SMIFlag is.
const SMICardFlag = "--query-gpu=memory.used,memory.total,clocks.gr,clocks.mem,power.draw,power.limit"

// NvidiaSMICard asks nvidia-smi for the card's memory, clocks and power,
// within SMITimeout (spec 096 R4). It runs only while the GPU list is shown.
func NvidiaSMICard(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, SMITimeout)
	defer cancel()
	//nolint:gosec // every argument is a constant; nothing here is input
	out, err := exec.CommandContext(ctx, "nvidia-smi", SMICardFlag, "--format=csv,noheader,nounits").Output()
	if err != nil {
		return "", fmt.Errorf("running nvidia-smi: %w", err)
	}
	return string(out), nil
}

/*
ParseSMICard reads the first line of nvidia-smi's answer to SMICardFlag:

	11169, 24564, 780, 810, 57.52, 400.00

memory used and total in MiB, the graphics and memory clocks in MHz, the
power drawn and its limit in watts. A field that does not parse ("[N/A]",
"[Not Supported]") is that part absent, as ParseSMI takes one.
*/
func ParseSMICard(out string) Card {
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	parts := strings.Split(line, ",")
	field := func(i int) (float64, bool) {
		if i >= len(parts) {
			return 0, false
		}
		v, err := strconv.ParseFloat(leading(parts[i]), 64)
		if err != nil || v < 0 {
			return 0, false
		}
		return v, true
	}
	var c Card
	used, ok1 := field(0)
	total, ok2 := field(1)
	if ok1 && ok2 && total > 0 && used <= total {
		c.VRAMUsed, c.VRAMTotal, c.HasVRAM = used*(1<<20), total*(1<<20), true
	}
	c.Clock, c.HasClock = field(2)
	c.MemClock, c.HasMemClock = field(3)
	if p, ok := field(4); ok {
		c.Power, c.HasPower = p, true
		if limit, ok := field(5); ok {
			c.PowerLimit = limit
		}
	}
	return c
}

/*
AMDCard reads an AMD card from the files its kernel driver keeps beside the
busy file (spec 096 R5): mem_info_vram_used and mem_info_vram_total in bytes;
the hwmon's clocks by their labels, sclk and mclk, in Hz; power1_average (or
power1_input) and power1_cap in microwatts. These are the files btop and nvtop
read. No AMD card has been in reach of these tests: they read files laid out
as the kernel documents them.
*/
func AMDCard(busyGlob string) (Card, error) {
	_, path, ok := busyIn(busyGlob)
	if !ok {
		return Card{}, errNothing
	}
	device := filepath.Dir(path)
	var c Card
	used, err1 := readNumber(filepath.Join(device, "mem_info_vram_used"))
	total, err2 := readNumber(filepath.Join(device, "mem_info_vram_total"))
	if err1 == nil && err2 == nil && total > 0 && used <= total {
		c.VRAMUsed, c.VRAMTotal, c.HasVRAM = used, total, true
	}
	hwmons, _ := filepath.Glob(filepath.Join(device, "hwmon", "hwmon*"))
	for _, h := range hwmons {
		readAMDHwmon(h, &c)
	}
	return c, nil
}

// readAMDHwmon takes the clocks by label and the power from one hwmon
// directory of the card.
func readAMDHwmon(dir string, c *Card) {
	labels, _ := filepath.Glob(filepath.Join(dir, "freq*_label"))
	for _, label := range labels {
		name, err := os.ReadFile(label) //nolint:gosec // sysfs, or a test's file
		if err != nil {
			continue
		}
		hz, err := readNumber(strings.TrimSuffix(label, "_label") + "_input")
		if err != nil {
			continue
		}
		switch strings.TrimSpace(string(name)) {
		case "sclk":
			c.Clock, c.HasClock = hz/1e6, true
		case "mclk":
			c.MemClock, c.HasMemClock = hz/1e6, true
		}
	}
	for _, name := range []string{"power1_average", "power1_input"} {
		if uw, err := readNumber(filepath.Join(dir, name)); err == nil && !c.HasPower {
			c.Power, c.HasPower = uw/1e6, true
		}
	}
	if c.HasPower {
		if uw, err := readNumber(filepath.Join(dir, "power1_cap")); err == nil {
			c.PowerLimit = uw / 1e6
		}
	}
}

// readNumber is a sysfs file's one number.
func readNumber(path string) (float64, error) {
	body, err := os.ReadFile(path) //nolint:gosec // sysfs, or a test's file
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", path, err)
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(body)), 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("%s: %w", path, errNotANumber)
	}
	return v, nil
}

var errNotANumber = errors.New("not a number")

// CardReader reads the card the GPU list is about, by the provider its
// reading came from (Graphics.Via), as the list of its processes is chosen.
type CardReader func(ctx context.Context, via string) (Card, error)

// linuxCard is nvidia-smi for a card on NVIDIA's driver, and the AMD
// driver's files for one on AMD's.
func linuxCard(busyGlob string, smi func(context.Context) (string, error)) CardReader {
	return func(ctx context.Context, via string) (Card, error) {
		if via == smiName {
			return smiCard(ctx, smi)
		}
		return AMDCard(busyGlob)
	}
}

// windowsCard is nvidia-smi, which only an NVIDIA card has; another card's
// figures are not read on Windows yet (spec 096).
func windowsCard(smi func(context.Context) (string, error)) CardReader {
	return func(ctx context.Context, _ string) (Card, error) { return smiCard(ctx, smi) }
}

func smiCard(ctx context.Context, smi func(context.Context) (string, error)) (Card, error) {
	text, err := smi(ctx)
	if err != nil {
		return Card{}, err
	}
	return ParseSMICard(text), nil
}
