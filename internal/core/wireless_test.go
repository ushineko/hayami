package core

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/mdlayher/wifi"
)

// put writes a little-endian word into b at off.
func put(b []byte, off int, v uint32) { binary.LittleEndian.PutUint32(b[off:], v) }

// connection is a WLAN_CONNECTION_ATTRIBUTES with invented values. The
// profile, SSID and BSSID region is filled with a pattern, so a decoder that
// read any of it would show it.
func connection(state, phy, quality, rx, tx uint32) []byte {
	b := make([]byte, 604)
	put(b, 0, state)
	for i := 8; i < connPhy; i++ {
		b[i] = 0xA5
	}
	put(b, connPhy, phy)
	put(b, connQuality, quality)
	put(b, connRx, rx)
	put(b, connTx, tx)
	return b
}

// realtime is a WLAN_REALTIME_CONNECTION_QUALITY for one link, invented.
func realtime(phy, quality, rx, tx, mhz uint32, rssi int32) []byte {
	b := make([]byte, 296)
	put(b, rtPhy, phy)
	put(b, rtQuality, quality)
	put(b, rtRx, rx)
	put(b, rtTx, tx)
	put(b, rtLinks, 1)
	put(b, rtFrequency, mhz)
	put(b, rtRSSI, uint32(rssi)) //nolint:gosec // a LONG, as Windows writes it
	return b
}

func TestTheConnectionGivesSignalRatesAndGeneration(t *testing.T) {
	var w Wireless
	if !decodeConnection(connection(wlanConnected, 8, 72, 433300, 390000), &w) {
		t.Fatal("a 604-byte answer was not decoded")
	}
	if !w.Connected || w.Signal != 72 || !w.HasSignal {
		t.Errorf("signal: %+v", w)
	}
	if w.RxRate != 433.3 || w.TxRate != 390 || !w.HasRx || !w.HasTx {
		t.Errorf("rates: rx %v tx %v", w.RxRate, w.TxRate)
	}
	if w.Generation != "Wi-Fi 5" {
		t.Errorf("generation %q", w.Generation)
	}
}

// Nothing derived from the profile, SSID or BSSID bytes can appear: the only
// text a decoded connection carries is its generation.
func TestTheNetworksNameIsNeverDecoded(t *testing.T) {
	var w Wireless
	decodeConnection(connection(wlanConnected, 0, 50, 0, 0), &w)
	if w.Generation != "" {
		t.Errorf("an unnumbered PHY gave %q", w.Generation)
	}
	if strings.Contains(strings.ToLower(w.Generation), "a5") {
		t.Error("the SSID region leaked into a field")
	}
}

func TestADisconnectedInterfaceSaysOnlyThat(t *testing.T) {
	var w Wireless
	decodeConnection(connection(4, 8, 72, 433300, 390000), &w)
	if w.Connected || w.HasSignal || w.HasRx || w.Generation != "" {
		t.Errorf("a disconnected interface reported %+v", w)
	}
}

func TestAShortAnswerIsNotTheStructure(t *testing.T) {
	var w Wireless
	if decodeConnection(make([]byte, 100), &w) || decodeRealtime(make([]byte, 30), &w) {
		t.Error("a short answer was decoded")
	}
}

func TestTheRealtimeQualityGivesTheBandAndRSSI(t *testing.T) {
	var w Wireless
	if !decodeRealtime(realtime(10, 64, 1200950, 960000, 6115, -61), &w) {
		t.Fatal("not decoded")
	}
	if w.RSSI != -61 || !w.HasRSSI {
		t.Errorf("rssi %d", w.RSSI)
	}
	if w.Band() != "6 GHz" || w.Channel != 33 {
		t.Errorf("band %q channel %d", w.Band(), w.Channel)
	}
	if w.Generation != "Wi-Fi 6E" {
		t.Errorf("Wi-Fi 6 on 6 GHz is %q", w.Generation)
	}
	if w.RxRate != 1200.95 {
		t.Errorf("rx %v", w.RxRate)
	}
}

// The realtime answer goes first and the connection fills what it left; the
// connection's rates do not overwrite the realtime ones.
func TestTheConnectionFillsWhatTheRealtimeLeft(t *testing.T) {
	var w Wireless
	decodeRealtime(realtime(8, 80, 866700, 0, 5745, -48), &w)
	decodeConnection(connection(wlanConnected, 8, 80, 780000, 650000), &w)
	if w.RxRate != 866.7 {
		t.Errorf("rx was overwritten: %v", w.RxRate)
	}
	if w.TxRate != 650 || !w.HasTx {
		t.Errorf("tx not filled: %v", w.TxRate)
	}
	if w.Band() != "5 GHz" || w.Channel != 149 {
		t.Errorf("%q ch %d", w.Band(), w.Channel)
	}
}

func TestBandsAndChannels(t *testing.T) {
	cases := []struct {
		mhz     int
		band    string
		channel int
	}{
		{2412, "2.4 GHz", 1}, {2437, "2.4 GHz", 6}, {2484, "2.4 GHz", 14},
		{5180, "5 GHz", 36}, {5745, "5 GHz", 149}, {5955, "6 GHz", 1}, {6415, "6 GHz", 93},
	}
	for _, c := range cases {
		var w Wireless
		w.withFrequency(c.mhz)
		if w.Band() != c.band || w.Channel != c.channel {
			t.Errorf("%d MHz: %q ch %d, want %q ch %d", c.mhz, w.Band(), w.Channel, c.band, c.channel)
		}
	}
	if b := (Wireless{Channel: 6, HasChannel: true}).Band(); b != "2.4 GHz" {
		t.Errorf("channel 6 alone: %q", b)
	}
	if b := (Wireless{Channel: 149, HasChannel: true}).Band(); b != "5 GHz" {
		t.Errorf("channel 149 alone: %q", b)
	}
	if b := (Wireless{}).Band(); b != "" {
		t.Errorf("nothing gave %q", b)
	}
}

// fakeNL80211 stands in for the kernel's nl80211.
type fakeNL80211 struct {
	ifaces   []*wifi.Interface
	stations map[string][]*wifi.StationInfo
	err      error
}

func (f fakeNL80211) Interfaces() ([]*wifi.Interface, error) { return f.ifaces, f.err }

func (f fakeNL80211) StationInfo(ifi *wifi.Interface) ([]*wifi.StationInfo, error) {
	st, ok := f.stations[ifi.Name]
	if !ok {
		return nil, errors.New("not associated")
	}
	return st, nil
}

func TestNL80211DescribesStationsByNetdevName(t *testing.T) {
	c := fakeNL80211{
		ifaces: []*wifi.Interface{
			{Name: "wlp3s0", Type: wifi.InterfaceTypeStation, Frequency: 5180},
			{Name: "wlan1", Type: wifi.InterfaceTypeStation, Frequency: 2437},
			{Name: "ap0", Type: wifi.InterfaceTypeAP, Frequency: 2412},
		},
		stations: map[string][]*wifi.StationInfo{
			"wlp3s0": {{
				HardwareAddr:     []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
				Signal:           -58,
				ReceiveBitrate:   1_200_900_000,
				TransmitBitrate:  864_800_000,
				ReceiveRateInfo:  wifi.RateInfo{ModulationType: wifi.RateModulationInfoTypeHE},
				TransmitRateInfo: wifi.RateInfo{ModulationType: wifi.RateModulationInfoTypeVHT},
			}},
		},
	}
	out, err := readNL80211(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["ap0"]; ok {
		t.Error("an access-point interface was described")
	}
	w := out["wlp3s0"]
	if !w.Connected || w.RSSI != -58 || w.RxRate != 1200.9 || w.TxRate != 864.8 {
		t.Errorf("wlp3s0: %+v", w)
	}
	if w.Generation != "Wi-Fi 6" || w.Band() != "5 GHz" || w.Channel != 36 {
		t.Errorf("wlp3s0: %q %q ch %d", w.Generation, w.Band(), w.Channel)
	}
	if idle := out["wlan1"]; idle.Connected || idle.HasFrequency {
		t.Errorf("an unassociated interface said %+v", idle)
	}
}

func TestProcWirelessGivesTheLevel(t *testing.T) {
	text := `Inter-| sta-|   Quality        |   Discarded packets               | Missed | WE
 face | tus | link level noise |  nwid  crypt   frag  retry   misc | beacon | 22
wlan0: 0000   70.  -40.  -256        0      0      0      0     12        0
`
	out := parseProcWireless(strings.NewReader(text))
	w, ok := out["wlan0"]
	if !ok || !w.Connected || w.RSSI != -40 || !w.HasRSSI {
		t.Errorf("wlan0: %+v (%v)", w, ok)
	}
	if len(out) != 1 {
		t.Errorf("the header lines were read as interfaces: %v", out)
	}
}
