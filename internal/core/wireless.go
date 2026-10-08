package core

import (
	"errors"
	"strconv"
)

/*
Wireless is what a Wi-Fi interface says about its link (spec 037): how strong
the signal is, where on the spectrum it is, and what it is running at.

Every figure can be missing, and each says so with its own flag rather than a
zero: a link rate of nought is a claim, and a driver that did not report one
has not made it. An interface that is wireless and not associated is
Connected false and nothing else.

**The network's name is not here, and never will be.** The SSID and the
access point's address identify a place. The Windows answer carries both, and
the reader skips their bytes; the Linux reader never asks for them.
*/
type Wireless struct {
	// Connected is whether the interface is associated with an access point.
	Connected bool `json:"connected"`

	// Signal is the link quality in percent, as Windows reports it. Linux
	// has no such figure and leaves it out.
	Signal    int  `json:"signal"`
	HasSignal bool `json:"has_signal"`

	// RSSI is the received signal strength in dBm: -30 is in the same room,
	// -80 is about to drop.
	RSSI    int  `json:"rssi"`
	HasRSSI bool `json:"has_rssi"`

	// Frequency is the channel's centre in MHz, which is what says the band.
	Frequency    int  `json:"frequency"`
	HasFrequency bool `json:"has_frequency"`

	// Channel is the channel number.
	Channel    int  `json:"channel"`
	HasChannel bool `json:"has_channel"`

	// Generation is the standard the link is running, "Wi-Fi 5", where the
	// driver says. Empty otherwise.
	Generation string `json:"generation,omitempty"`

	// RxRate and TxRate are the link rates in megabits per second: what the
	// radio negotiated, not what is flowing (the bandwidth section's rates
	// are that).
	RxRate float64 `json:"rx_rate"`
	TxRate float64 `json:"tx_rate"`
	HasRx  bool    `json:"has_rx"`
	HasTx  bool    `json:"has_tx"`
}

// ErrWirelessDenied is Windows declining to describe the Wi-Fi connection to
// a program without location access. Since Windows 11 24H2 the connection's
// details are behind the location consent, because the access point's address
// would say where the machine is.
var ErrWirelessDenied = errors.New("the WLAN service withholds Wi-Fi details without location access")

// Band is the band the link is on, "5 GHz", from the frequency where there is
// one and from the channel where there is only that. Empty when neither says.
//
// A channel alone cannot tell 6 GHz from 5 GHz: the numbers overlap. Without a
// frequency anything above channel 14 is called 5 GHz, which is right for every
// link that is not Wi-Fi 6E, and a reader that gives no frequency is one old
// enough not to know 6 GHz.
func (w Wireless) Band() string {
	switch {
	case w.HasFrequency && w.Frequency >= 2400 && w.Frequency < 2500:
		return "2.4 GHz"
	case w.HasFrequency && w.Frequency >= 4900 && w.Frequency < 5925:
		return "5 GHz"
	case w.HasFrequency && w.Frequency >= 5925 && w.Frequency <= 7125:
		return "6 GHz"
	case w.HasFrequency:
		return ""
	case w.HasChannel && w.Channel >= 1 && w.Channel <= 14:
		return "2.4 GHz"
	case w.HasChannel && w.Channel > 14:
		return "5 GHz"
	default:
		return ""
	}
}

// channelOf is the channel number of a centre frequency in MHz, and whether it
// is one of the three bands' channels at all.
func channelOf(mhz int) (int, bool) {
	switch {
	case mhz == 2484:
		return 14, true
	case mhz >= 2412 && mhz <= 2472:
		return (mhz - 2407) / 5, true
	case mhz >= 5150 && mhz < 5925:
		return (mhz - 5000) / 5, true
	case mhz >= 5955 && mhz <= 7115:
		return (mhz - 5950) / 5, true
	default:
		return 0, false
	}
}

// withFrequency sets the frequency and, where it names one, the channel.
func (w *Wireless) withFrequency(mhz int) {
	if mhz <= 0 {
		return
	}
	w.Frequency, w.HasFrequency = mhz, true
	if ch, ok := channelOf(mhz); ok {
		w.Channel, w.HasChannel = ch, true
	}
}

// generation names a Wi-Fi generation by its number, and calls 6 on the
// 6 GHz band what the Wi-Fi Alliance does: 6E.
func generation(n int, w Wireless) string {
	if n == 6 && w.Band() == "6 GHz" {
		return "Wi-Fi 6E"
	}
	return "Wi-Fi " + strconv.Itoa(n)
}
