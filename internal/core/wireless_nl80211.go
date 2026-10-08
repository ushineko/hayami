package core

import (
	"bufio"
	"io"
	"strconv"
	"strings"

	"github.com/mdlayher/wifi"
)

/*
nl80211 is the part of mdlayher/wifi's client the Linux reader uses, so a test
can stand in for the kernel. Two questions: which interfaces are wireless, and
what does the station each is associated with say about the link.

The client can also be asked for the BSS, which carries the network's name; it
is not, and this interface has no method for it.
*/
type nl80211 interface {
	Interfaces() ([]*wifi.Interface, error)
	StationInfo(ifi *wifi.Interface) ([]*wifi.StationInfo, error)
}

/*
readNL80211 describes every station-mode interface the client lists, keyed by
its netdev name -- the name /proc/net/dev and ReadCounters use.

An interface with no station is not associated: Connected false and nothing
else, which is the true state of a Wi-Fi card with the lid of a laptop open
in a field. An interface in another mode (an access point, a monitor) is not a
link the bandwidth section has a use for describing, and is left out.

The station's own hardware address is the access point's, so it is not kept.
*/
func readNL80211(c nl80211) (map[string]Wireless, error) {
	ifaces, err := c.Interfaces()
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller wraps
	}
	out := map[string]Wireless{}
	for _, ifi := range ifaces {
		if ifi == nil || ifi.Name == "" || ifi.Type != wifi.InterfaceTypeStation {
			continue
		}
		w := Wireless{}
		w.withFrequency(ifi.Frequency)
		stations, err := c.StationInfo(ifi)
		if err == nil && len(stations) > 0 && stations[0] != nil {
			fromStation(stations[0], &w)
		} else {
			w = Wireless{}
		}
		out[ifi.Name] = w
	}
	return out, nil
}

// fromStation reads one station's signal, rates and generation into w.
func fromStation(st *wifi.StationInfo, w *Wireless) {
	w.Connected = true
	if st.Signal < 0 {
		w.RSSI, w.HasRSSI = st.Signal, true
	}
	if st.ReceiveBitrate > 0 {
		w.RxRate, w.HasRx = float64(st.ReceiveBitrate)/1e6, true
	}
	if st.TransmitBitrate > 0 {
		w.TxRate, w.HasTx = float64(st.TransmitBitrate)/1e6, true
	}
	n := modulationGeneration(st.ReceiveRateInfo.ModulationType)
	if t := modulationGeneration(st.TransmitRateInfo.ModulationType); t > n {
		n = t
	}
	if n > 0 {
		w.Generation = generation(n, *w)
	}
}

// modulationGeneration is the Wi-Fi generation of a rate's modulation, or 0
// for a legacy rate that carries none.
func modulationGeneration(t wifi.RateModulationInfoType) int {
	switch t {
	case wifi.RateModulationInfoTypeHT:
		return 4
	case wifi.RateModulationInfoTypeVHT:
		return 5
	case wifi.RateModulationInfoTypeHE:
		return 6
	case wifi.RateModulationInfoTypeEHT:
		return 7
	default:
		return 0
	}
}

// ProcWirelessPath is the kernel's older account of the wireless interfaces.
const ProcWirelessPath = "/proc/net/wireless"

/*
parseProcWireless reads /proc/net/wireless, the fallback where nl80211 does
not answer. It has the signal level and nothing else a panel would show:

	Inter-| sta-|   Quality        |   Discarded packets
	 face | tus | link level noise |  nwid  crypt   frag  retry   misc | beacon | 22
	wlan0: 0000   70.  -40.  -256        0      0      0      0     12        0

A listed interface is associated; the level is dBm when it is negative.
*/
func parseProcWireless(r io.Reader) map[string]Wireless {
	out := map[string]Wireless{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		fields := strings.Fields(rest)
		if name == "" || len(fields) < 3 {
			continue
		}
		w := Wireless{Connected: true}
		level, err := strconv.ParseFloat(strings.TrimSuffix(fields[2], "."), 64)
		if err == nil && level < 0 {
			w.RSSI, w.HasRSSI = int(level), true
		}
		out[name] = w
	}
	return out
}
