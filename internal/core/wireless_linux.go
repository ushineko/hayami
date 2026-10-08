package core

import (
	"context"
	"fmt"
	"os"

	"github.com/mdlayher/wifi"
)

// ReadWireless asks nl80211 about every station-mode Wi-Fi interface, keyed
// by its netdev name, and falls back to /proc/net/wireless where nl80211 does
// not answer. A machine with no Wi-Fi returns nothing, not an error. Neither
// question needs privileges.
func ReadWireless(ctx context.Context) (map[string]Wireless, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("reading Wi-Fi: %w", err)
	}
	if c, err := wifi.New(); err == nil {
		defer c.Close() //nolint:errcheck // a netlink socket
		if out, err := readNL80211(c); err == nil {
			return out, nil
		}
	}
	f, err := os.Open(ProcWirelessPath)
	if err != nil {
		// No nl80211 and no wireless extensions: no Wi-Fi.
		return nil, nil //nolint:nilerr // absence is the answer
	}
	defer f.Close() //nolint:errcheck // read-only
	return parseProcWireless(f), nil
}
