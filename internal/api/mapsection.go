package api

import (
	"slices"
	"time"

	"github.com/ushineko/hayami/internal/api/contract"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/safetext"
	"github.com/ushineko/hayami/internal/view"
)

/*
The sections' readings as contract types (spec 113 R15).

A section is mapped by what its source can say, not by its Go type: the
bandwidth source's interfaces, trails and VPNs, the usage source's windows
and when they were fetched, the cooler's probes, the peripherals' devices.
A source refactor changes this mapping and not the API, and a test's source
that says the same things is mapped the same way.
*/

// What the bandwidth section can say, beyond Data.
type (
	ratesReader interface{ Readings() []core.Rates }
	trailReader interface{ Trail(name string) core.Trail }
	vpnReader   interface{ VPNReadings() []view.VPNReading }
	fetchReader interface{ FetchedAt() time.Time }
)

// ptr is v's address, for a field that may be null.
func ptr[T any](v T) *T { return &v }

// opt is an Opt as a field that may be null.
func opt[T any](o view.Opt[T]) *T {
	if !o.OK {
		return nil
	}
	return &o.V
}

// nonEmpty is s, or null where it is empty.
func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// list is s, or an empty list where it is nil: a list is never null.
func list[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// when is t in UTC, or null where it is zero.
func when(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

// statusWord is a view status as the contract says it.
func statusWord(s view.Status) contract.Status {
	switch s {
	case view.Good:
		return contract.StatusGood
	case view.Warn:
		return contract.StatusWarn
	case view.Bad:
		return contract.StatusBad
	case view.Dim:
		return contract.StatusDim
	default:
		return contract.StatusInfo
	}
}

// absences are reasons as the contract says them.
func absences(in []view.Reason) []contract.Absence {
	out := make([]contract.Absence, 0, len(in))
	for _, r := range in {
		out = append(out, absence(r))
	}
	return out
}

func absence(r view.Reason) contract.Absence {
	return contract.Absence{
		Code: nonEmpty(r.Code), Label: nonEmpty(r.Label), Text: r.Text,
		Detail: nonEmpty(safetext.WithoutHome(r.Detail)), Status: statusWord(r.Status), Actionable: r.Actionable,
	}
}

// errorText is an error as a response says it: no path under the home.
func errorText(err error) *string {
	if err == nil {
		return nil
	}
	return ptr(safetext.WithoutHome(err.Error()))
}

// detailRefs are the details a source offers.
func detailRefs(src panel.Source) []contract.DetailRef {
	d, ok := src.(panel.Detailer)
	if !ok {
		return []contract.DetailRef{}
	}
	infos := d.Details()
	out := make([]contract.DetailRef, 0, len(infos))
	for _, info := range infos {
		out = append(out, contract.DetailRef{ID: info.ID, Title: info.Title})
	}
	return out
}

// mapReading is a source's reading, polled at at, as its section's contract
// type.
func mapReading(key string, src panel.Source, at time.Time) contract.Reading {
	var r contract.Reading
	switch key {
	case view.ClockInfo.Key:
		r.Clock = mapClock(at)
	case view.BandwidthInfo.Key:
		r.Bandwidth = mapBandwidth(src, at)
	case view.UsageInfo.Key:
		r.Usage = mapUsage(src)
	case view.CoolerInfo.Key:
		r.Cooler = mapCooler(src)
	case view.PeripheralsInfo.Key:
		r.Peripherals = mapPeripherals(src)
	}
	return r
}

// mapClock is the time of the poll, and the zone this machine keeps.
func mapClock(at time.Time) *contract.Clock {
	local := at.In(time.Local)
	name, offset := local.Zone()
	if loc := time.Local.String(); loc != "" && loc != "Local" {
		name = loc
	}
	return &contract.Clock{Time: at.UTC(), Zone: name, UTCOffsetS: offset}
}

func mapBandwidth(src panel.Source, at time.Time) *contract.Bandwidth {
	out := &contract.Bandwidth{Interfaces: []contract.Interface{}, VPNs: []contract.VPN{}}
	if rr, ok := src.(ratesReader); ok {
		tr, trails := src.(trailReader)
		for _, r := range rr.Readings() {
			i := contract.Interface{
				Name: r.Name, Present: r.Present, RxBytes: r.RxTotal, TxBytes: r.TxTotal,
				Trail: contract.RateTrail{Rx: []float64{}, Tx: []float64{}},
			}
			if r.HasRate {
				i.RxBPS, i.TxBPS = ptr(r.RxRate), ptr(r.TxRate)
			}
			if trails {
				t := tr.Trail(r.Name)
				i.Trail = contract.RateTrail{Rx: list(slices.Clone(t.Rx)), Tx: list(slices.Clone(t.Tx))}
			}
			if w := r.Wireless; w != nil {
				i.Wireless = mapWireless(*w)
			}
			out.Interfaces = append(out.Interfaces, i)
		}
	}
	if vr, ok := src.(vpnReader); ok {
		for _, v := range vr.VPNReadings() {
			out.VPNs = append(out.VPNs, mapVPN(v, at))
		}
	}
	return out
}

func mapWireless(w core.Wireless) *contract.Wireless {
	out := &contract.Wireless{Connected: w.Connected, Generation: nonEmpty(w.Generation)}
	if w.HasSignal {
		out.SignalPct = ptr(w.Signal)
	}
	if w.HasRSSI {
		out.RSSIdBm = ptr(w.RSSI)
	}
	if w.HasFrequency {
		out.FrequencyMHz = ptr(w.Frequency)
	}
	if w.HasChannel {
		out.Channel = ptr(w.Channel)
	}
	if w.HasRx {
		out.RxRateMbps = ptr(w.RxRate)
	}
	if w.HasTx {
		out.TxRateMbps = ptr(w.TxRate)
	}
	return out
}

// vpnStates are the view's states as the contract says them.
var vpnStates = map[view.VPNState]string{
	view.VPNDown: contract.VPNDown, view.VPNConnecting: contract.VPNConnecting,
	view.VPNUp: contract.VPNUp, view.VPNUnknown: contract.VPNUnknown,
}

func mapVPN(v view.VPNReading, at time.Time) contract.VPN {
	out := contract.VPN{
		Name: v.Name, Label: nonEmpty(v.Label), State: vpnStates[v.State], Interface: nonEmpty(v.Interface),
		LatencyMS: opt(v.Latency), Failing: v.Failing, Stale: v.Stale,
	}
	if out.State == "" {
		out.State = contract.VPNUnknown
	}
	if v.Connected.OK {
		out.ConnectedSince = when(at.Add(-v.Connected.V))
	}
	if t := v.Traffic; t != nil && t.HasRate {
		out.RxBPS, out.TxBPS = ptr(t.RxRate), ptr(t.TxRate)
	}
	return out
}

// mapUsage is the windows, an account each in the order the card has them.
func mapUsage(src panel.Source) *contract.Usage {
	out := &contract.Usage{Accounts: []contract.Account{}}
	windows, _ := src.Data().([]view.UsageWindow)
	index := map[string]int{}
	for _, w := range windows {
		i, ok := index[w.Account]
		if !ok {
			i = len(out.Accounts)
			index[w.Account] = i
			out.Accounts = append(out.Accounts, contract.Account{
				Name: w.Account, PlanBadge: nonEmpty(w.Badge), Windows: []contract.UsageWindow{},
			})
		}
		if out.Accounts[i].PlanBadge == nil {
			out.Accounts[i].PlanBadge = nonEmpty(w.Badge)
		}
		uw := contract.UsageWindow{
			Name: w.Name, UtilisationPct: 100 * w.Fraction, ResetsAt: when(w.ResetsAt),
			Used: nonEmpty(w.Used), Limit: nonEmpty(w.Limit), Severity: nonEmpty(w.Severity), Note: nonEmpty(w.Detail),
		}
		if w.Span > 0 {
			uw.SpanS = ptr(w.Span.Seconds())
		}
		out.Accounts[i].Windows = append(out.Accounts[i].Windows, uw)
	}
	if f, ok := src.(fetchReader); ok {
		out.FetchedAt = when(f.FetchedAt())
	}
	return out
}

func mapCooler(src panel.Source) *contract.Cooler {
	reading, _ := src.Data().(view.CoolerReading)
	out := &contract.Cooler{Probes: make([]contract.Probe, 0, len(reading.Probes))}
	for _, p := range reading.Probes {
		out.Probes = append(out.Probes, contract.Probe{
			ID: p.ID, Role: string(p.Role), Name: nonEmpty(p.Name),
			LoadPct: opt(p.Load), TempC: opt(p.Temp), RPM: opt(p.RPM),
			Bytes: opt(p.Bytes), OfBytes: opt(p.Of), ReadBPS: opt(p.Read), WriteBPS: opt(p.Write),
			Trail: list(slices.Clone(p.Trail)), Stale: p.Stale,
		})
	}
	if reading.Uptime.OK {
		out.UptimeS = ptr(reading.Uptime.V.Seconds())
	}
	return out
}

// The contract's words for a device's kind and its charge.
var (
	kindWords = map[view.Kind]string{
		view.KindOther: "other", view.KindMouse: "mouse", view.KindKeyboard: "keyboard",
		view.KindHeadset: "headset", view.KindGamepad: "gamepad",
	}
	chargeWords = map[view.Charge]string{
		view.Draining: contract.ChargeDraining, view.Filling: contract.ChargeFilling, view.Charged: contract.ChargeCharged,
	}
)

func mapPeripherals(src panel.Source) *contract.Peripherals {
	reading, _ := src.Data().(view.PeripheralsReading)
	out := &contract.Peripherals{Devices: make([]contract.Device, 0, len(reading.Devices))}
	for _, d := range reading.Devices {
		dev := contract.Device{
			Name: d.Name, Kind: kindWords[d.Kind], Charge: chargeWords[d.Charge],
			Stale: d.Stale, LastSeen: when(d.Seen),
		}
		if dev.Kind == "" {
			dev.Kind = "other"
		}
		if dev.Charge == "" {
			dev.Charge = contract.ChargeDraining
		}
		switch {
		case d.Band != "":
			dev.Band, dev.Segments = ptr(d.Band), ptr(d.Segments)
		case !d.NoLevel:
			dev.BatteryPct = ptr(d.Level)
		}
		out.Devices = append(out.Devices, dev)
	}
	return out
}
