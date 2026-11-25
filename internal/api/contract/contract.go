/*
Package contract is the service's API as Go types (spec 113 R26).

`api/openapi.yaml` is the contract and these types are written to it; a test
holds the two together field by field. They are what the server encodes and
what the reference client decodes, and they import nothing of hayami's: a
client needs these and the standard library, and nothing else.

**A value that is unknown is null, never left out** (R27). Every field is
always present; an optional one is a pointer, which encodes as null.

**Values are facts.** Names whole, numbers in base units (bytes, bytes a
second, degrees Celsius, percent 0-100, RPM, milliseconds of latency), times
RFC 3339 in UTC, durations in seconds. A string longer than MaxText bytes is
cut to it, and the response carrying it says so in Truncated (R13).
*/
package contract

import "time"

// Version is the contract's version, info.version in the document (R27).
const Version = "1.0"

// MaxText is the longest string a response carries, in bytes (R13).
const MaxText = 4096

// Status is a band's verdict on a value, which a client may use or ignore.
type Status string

// The statuses.
const (
	StatusGood Status = "good"
	StatusWarn Status = "warn"
	StatusBad  Status = "bad"
	StatusInfo Status = "info"
	StatusDim  Status = "dim"
)

// Index is GET /v1 (R16).
type Index struct {
	Service  string   `json:"service"`
	Version  string   `json:"version"`
	API      string   `json:"api"`
	Sections []string `json:"sections"`
}

// SectionInfo is one section as GET /v1/sections lists it (R17).
type SectionInfo struct {
	Key       string  `json:"key"`
	Title     string  `json:"title"`
	Default   bool    `json:"default"`
	Shown     bool    `json:"shown"`
	IntervalS float64 `json:"interval_s"`
}

// SectionList is GET /v1/sections.
type SectionList struct {
	Sections []SectionInfo `json:"sections"`
}

// Absence is what a section or a detail could not read, and why (R12).
type Absence struct {
	// Code is the source's typed absence, where it gave one.
	Code       *string `json:"code"`
	Label      *string `json:"label"`
	Text       string  `json:"text"`
	Detail     *string `json:"detail"`
	Status     Status  `json:"status"`
	Actionable bool    `json:"actionable"`
}

// DetailRef is a detail a section offers.
type DetailRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

/*
Section is GET /v1/sections/{key} (R18): one section's latest reading.
Exactly one of the Reading fields is set, the one named by the key; the
others are null. PolledAt is null before the first poll has finished.
*/
type Section struct {
	Key       string      `json:"key"`
	PolledAt  *time.Time  `json:"polled_at"`
	IntervalS float64     `json:"interval_s"`
	Reading   Reading     `json:"reading"`
	Absences  []Absence   `json:"absences"`
	Error     *string     `json:"error"`
	Details   []DetailRef `json:"details"`
	Truncated bool        `json:"truncated"`
}

// Reading is a section's reading, one field per section, the section's own
// set and the rest null.
type Reading struct {
	Clock       *Clock       `json:"clock"`
	Bandwidth   *Bandwidth   `json:"bandwidth"`
	Usage       *Usage       `json:"usage"`
	Cooler      *Cooler      `json:"cooler"`
	Peripherals *Peripherals `json:"peripherals"`
}

// Readings is GET /v1/readings (R19): sections by key.
type Readings struct {
	Readings map[string]Section `json:"readings"`
}

// Health is GET /v1/health (R22).
type Health struct {
	OK bool `json:"ok"`
}

// Error is every response that is not a 2xx.
type Error struct {
	Error ErrorBody `json:"error"`
}

// The error codes.
const (
	ErrNotFound         = "not_found"
	ErrMethodNotAllowed = "method_not_allowed"
	ErrBadRequest       = "bad_request"
	ErrForbidden        = "forbidden"
	ErrBusy             = "busy"
	ErrInternal         = "internal"
	ErrUnavailable      = "unavailable"
)

// ErrorBody says what went wrong. Known lists what there is, for a key or
// an ID that names nothing: the sections, or the details a section offers.
type ErrorBody struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Known   []string `json:"known"`
}

// Clock is the clock section's reading: the time of the poll, in UTC, and
// the zone the machine keeps.
type Clock struct {
	Time       time.Time `json:"time"`
	Zone       string    `json:"zone"`
	UTCOffsetS int       `json:"utc_offset_s"`
}

// Bandwidth is the bandwidth section's reading.
type Bandwidth struct {
	Interfaces []Interface `json:"interfaces"`
	VPNs       []VPN       `json:"vpns"`
}

// Interface is one network interface.
type Interface struct {
	Name string `json:"name"`
	// Present is false for an interface the system does not have now.
	Present bool `json:"present"`
	// RxBPS and TxBPS are null until there are two samples to difference.
	RxBPS   *float64 `json:"rx_bps"`
	TxBPS   *float64 `json:"tx_bps"`
	RxBytes uint64   `json:"rx_bytes"`
	TxBytes uint64   `json:"tx_bytes"`
	// Trail is the recent rates, oldest first, one a poll.
	Trail    RateTrail `json:"trail"`
	Wireless *Wireless `json:"wireless"`
}

// RateTrail is recent down and up rates, oldest first, in bytes a second.
type RateTrail struct {
	Rx []float64 `json:"rx"`
	Tx []float64 `json:"tx"`
}

// Wireless is a Wi-Fi interface's link.
type Wireless struct {
	Connected    bool     `json:"connected"`
	SignalPct    *int     `json:"signal_pct"`
	RSSIdBm      *int     `json:"rssi_dbm"`
	FrequencyMHz *int     `json:"frequency_mhz"`
	Channel      *int     `json:"channel"`
	Generation   *string  `json:"generation"`
	RxRateMbps   *float64 `json:"rx_rate_mbps"`
	TxRateMbps   *float64 `json:"tx_rate_mbps"`
}

// The states a VPN is in.
const (
	VPNUp         = "up"
	VPNDown       = "down"
	VPNConnecting = "connecting"
	VPNUnknown    = "unknown"
)

// VPN is one chosen VPN.
type VPN struct {
	Name           string     `json:"name"`
	Label          *string    `json:"label"`
	State          string     `json:"state"`
	Interface      *string    `json:"interface"`
	ConnectedSince *time.Time `json:"connected_since"`
	LatencyMS      *float64   `json:"latency_ms"`
	Failing        bool       `json:"failing"`
	Stale          bool       `json:"stale"`
	RxBPS          *float64   `json:"rx_bps"`
	TxBPS          *float64   `json:"tx_bps"`
}

// Usage is the usage section's reading.
type Usage struct {
	Accounts  []Account  `json:"accounts"`
	FetchedAt *time.Time `json:"fetched_at"`
}

// Account is one account's windows.
type Account struct {
	Name string `json:"name"`
	// PlanBadge is the letter the card shows for the plan, where known.
	PlanBadge *string       `json:"plan_badge"`
	Windows   []UsageWindow `json:"windows"`
}

// UsageWindow is one quota window: "5h", "7d", a model's.
type UsageWindow struct {
	Name           string     `json:"name"`
	UtilisationPct float64    `json:"utilisation_pct"`
	ResetsAt       *time.Time `json:"resets_at"`
	SpanS          *float64   `json:"span_s"`
	Used           *string    `json:"used"`
	Limit          *string    `json:"limit"`
	Severity       *string    `json:"severity"`
	Note           *string    `json:"note"`
}

// Cooler is the cooler section's reading: the machine's parts.
type Cooler struct {
	Probes  []Probe  `json:"probes"`
	UptimeS *float64 `json:"uptime_s"`
}

// Probe is one part: a processor, a card, the coolant, the memory, the disks.
type Probe struct {
	ID       string    `json:"id"`
	Role     string    `json:"role"`
	Name     *string   `json:"name"`
	LoadPct  *float64  `json:"load_pct"`
	TempC    *float64  `json:"temp_c"`
	RPM      *int      `json:"rpm"`
	Bytes    *float64  `json:"bytes"`
	OfBytes  *float64  `json:"of_bytes"`
	ReadBPS  *float64  `json:"read_bps"`
	WriteBPS *float64  `json:"write_bps"`
	Trail    []float64 `json:"trail"`
	Stale    bool      `json:"stale"`
}

// Peripherals is the peripherals section's reading.
type Peripherals struct {
	Devices []Device `json:"devices"`
}

// The charge states.
const (
	ChargeDraining = "draining"
	ChargeFilling  = "charging"
	ChargeCharged  = "charged"
)

// Device is one peripheral's battery.
type Device struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	// BatteryPct is null for a device that reports a band, or none.
	BatteryPct *int `json:"battery_pct"`
	// Band and Segments are a band a device without a fuel gauge reports.
	Band     *string `json:"band"`
	Segments *int    `json:"segments"`
	Charge   string  `json:"charge"`
	Stale    bool    `json:"stale"`
	// LastSeen is when it last answered.
	LastSeen *time.Time `json:"last_seen"`
}
