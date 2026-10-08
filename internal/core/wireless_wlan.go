package core

import "encoding/binary"

/*
The layouts of the two answers Windows' WLAN service gives about a connection,
decoded from bytes so that a test on any platform can hold a fixture against
them. The reader that asks for them is wireless_windows.go.

**WLAN_CONNECTION_ATTRIBUTES** (wlan_intf_opcode_current_connection), 604
bytes on x64:

	  0  isState               uint32
	  4  wlanConnectionMode    uint32
	  8  strProfileName        [256]uint16   -- the profile, usually the SSID
	520  dot11Ssid             uint32 + [32]byte
	556  dot11BssType          uint32
	560  dot11Bssid            [6]byte (+2 padding)
	568  dot11PhyType          uint32
	572  uDot11PhyIndex        uint32
	576  wlanSignalQuality     uint32  (0-100)
	580  ulRxRate              uint32  (kbit/s)
	584  ulTxRate              uint32  (kbit/s)
	588  wlanSecurityAttributes

**Bytes 8 to 568 are never read.** The profile name, the SSID and the BSSID
say where the machine is, and nothing in this program has a use for that.

**WLAN_REALTIME_CONNECTION_QUALITY** (wlan_intf_opcode_realtime_connection_quality,
Windows 11 24H2 and later), 296 bytes for one link:

	 0  dot11PhyType     uint32
	 4  ulLinkQuality    uint32  (0-100)
	 8  ulRxRate         uint32  (kbit/s)
	12  ulTxRate         uint32  (kbit/s)
	16  bIsMLOConnection BOOL
	20  ulNumLinks       uint32
	24  linksInfo[0]: ucLinkID uint8 (+3), ulChannelCenterFrequencyMhz uint32 (28),
	    ulBandwidth uint32 (32), lRssi int32 (36), wlanRateSet (40)

It is the one that gives the frequency, and so the band.
*/

// wlanConnected is wlan_interface_state_connected.
const wlanConnected = 1

// The offsets read, by the tables above.
const (
	connPhy      = 568
	connQuality  = 576
	connRx       = 580
	connTx       = 584
	connMinimum  = 588
	rtPhy        = 0
	rtQuality    = 4
	rtRx         = 8
	rtTx         = 12
	rtLinks      = 20
	rtFrequency  = 28
	rtRSSI       = 36
	rtMinimum    = 40
	kbitPerMbit  = 1000
	rtMinLinkLen = 1
)

// le32 is the little-endian word at off.
func le32(b []byte, off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }

// phyGeneration is the Wi-Fi generation of a DOT11_PHY_TYPE, or 0 for one the
// Alliance never numbered (11a, 11b, 11g, 11ad).
func phyGeneration(phy uint32) int {
	switch phy {
	case 7: // dot11_phy_type_ht
		return 4
	case 8: // dot11_phy_type_vht
		return 5
	case 10: // dot11_phy_type_he
		return 6
	case 11: // dot11_phy_type_eht
		return 7
	default:
		return 0
	}
}

// decodeConnection reads the connection attributes into w: the state, the
// signal quality, the rates and the PHY. It reports whether the answer was
// long enough to be the structure at all.
func decodeConnection(b []byte, w *Wireless) bool {
	if len(b) < connMinimum {
		return false
	}
	w.Connected = le32(b, 0) == wlanConnected
	if !w.Connected {
		return true
	}
	w.Signal, w.HasSignal = int(le32(b, connQuality)), true
	if !w.HasRx {
		if rx := le32(b, connRx); rx > 0 {
			w.RxRate, w.HasRx = float64(rx)/kbitPerMbit, true
		}
	}
	if !w.HasTx {
		if tx := le32(b, connTx); tx > 0 {
			w.TxRate, w.HasTx = float64(tx)/kbitPerMbit, true
		}
	}
	if w.Generation == "" {
		if n := phyGeneration(le32(b, connPhy)); n > 0 {
			w.Generation = generation(n, *w)
		}
	}
	return true
}

// decodeRealtime reads the realtime connection quality into w: the frequency
// and RSSI of the first link, the rates, and the PHY. A connection on several
// links at once (Wi-Fi 7's MLO) is described by its first.
func decodeRealtime(b []byte, w *Wireless) bool {
	if len(b) < rtMinimum || le32(b, rtLinks) < rtMinLinkLen {
		return false
	}
	w.Connected = true
	w.withFrequency(int(le32(b, rtFrequency)))
	if rssi := int32(le32(b, rtRSSI)); rssi < 0 { //nolint:gosec // lRssi is a LONG
		w.RSSI, w.HasRSSI = int(rssi), true
	}
	if q := le32(b, rtQuality); q <= 100 && !w.HasSignal {
		w.Signal, w.HasSignal = int(q), true
	}
	if rx := le32(b, rtRx); rx > 0 {
		w.RxRate, w.HasRx = float64(rx)/kbitPerMbit, true
	}
	if tx := le32(b, rtTx); tx > 0 {
		w.TxRate, w.HasTx = float64(tx)/kbitPerMbit, true
	}
	if n := phyGeneration(le32(b, rtPhy)); n > 0 {
		w.Generation = generation(n, *w)
	}
	return true
}
