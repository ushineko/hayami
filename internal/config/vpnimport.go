package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// vpnToggleMaxBytes bounds the read of vpn-toggle's config. Its own files are a
// few kilobytes.
const vpnToggleMaxBytes = 1 << 20

// VPNTogglePath is where vpn-toggle keeps its settings.
func VPNTogglePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, ".config", "vpn-toggle", "config.json"), nil
}

// VPNImport is what vpn-toggle's settings come to here.
type VPNImport struct {
	VPNs         []VPNItem
	CheckSeconds int
	GraceSeconds int

	// LeftOut names each check that was not carried over, for the person to be
	// told. A geolocation check is one: it told a third party the address, and
	// hayami does not do that (spec 085).
	LeftOut []string
}

// vpnToggleFile is the part of vpn-toggle's config this reads.
type vpnToggleFile struct {
	Monitor struct {
		CheckInterval int `json:"check_interval_seconds"`
		GracePeriod   int `json:"grace_period_seconds"`
	} `json:"monitor"`
	VPNs []struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Asserts     []struct {
			Type           string `json:"type"`
			Hostname       string `json:"hostname"`
			ExpectedPrefix string `json:"expected_prefix"`
			Host           string `json:"host"`
			Timeout        int    `json:"timeout_seconds"`
		} `json:"asserts"`
	} `json:"vpns"`
}

// ReadVPNToggle reads vpn-toggle's settings from path, and only reads them.
func ReadVPNToggle(path string) (VPNImport, error) {
	f, err := os.Open(path) //nolint:gosec // the path vpn-toggle keeps its settings at
	if err != nil {
		return VPNImport{}, fmt.Errorf("opening vpn-toggle's settings: %w", err)
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, vpnToggleMaxBytes))
	if err != nil {
		return VPNImport{}, fmt.Errorf("reading vpn-toggle's settings: %w", err)
	}
	return ParseVPNToggle(raw)
}

// ParseVPNToggle turns vpn-toggle's settings into hayami's. The file is another
// program's: an entry with no name is skipped, and a check this cannot carry over
// is named in LeftOut rather than dropped without a word.
func ParseVPNToggle(raw []byte) (VPNImport, error) {
	var f vpnToggleFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return VPNImport{}, fmt.Errorf("parsing vpn-toggle's settings: %w", err)
	}
	out := VPNImport{CheckSeconds: max(f.Monitor.CheckInterval, 0), GraceSeconds: max(f.Monitor.GracePeriod, 0)}
	for _, v := range f.VPNs {
		if v.Name == "" {
			continue
		}
		item := VPNItem{Name: v.Name}
		if v.DisplayName != v.Name {
			item.Label = v.DisplayName
		}
		for _, a := range v.Asserts {
			switch {
			case a.Type == "dns_lookup" && a.Hostname != "" && a.ExpectedPrefix != "":
				item.Checks = append(item.Checks, VPNCheckItem{Type: "dns", Target: a.Hostname, Prefix: a.ExpectedPrefix})
			case a.Type == "ping" && a.Host != "":
				item.Checks = append(item.Checks, VPNCheckItem{Type: "ping", Target: a.Host, TimeoutSeconds: max(a.Timeout, 0)})
			default:
				out.LeftOut = append(out.LeftOut, fmt.Sprintf("%s: %s check", v.Name, a.Type))
			}
		}
		out.VPNs = append(out.VPNs, item)
	}
	if len(out.VPNs) == 0 {
		return out, errors.New("vpn-toggle's settings name no VPNs")
	}
	return out, nil
}
