package contract

import "time"

// The kinds of detail, Detail.Kind: which of its body fields is set.
const (
	KindProcesses = "processes"
	KindProcess   = "process"
	KindNetwork   = "network"
	KindVPNs      = "vpns"
	KindAccount   = "account"
	KindOverview  = "overview"
	KindDay       = "day"
	KindChange    = "change"
)

/*
Detail is GET /v1/sections/{key}/details/{id} (R20): one detail's raw form.

Kind names the body: exactly one of the fields after Children is set, the
one Kind names, and the rest are null. VPNs is the one list that may be null,
and its `nullable` tag says so. Children are the IDs of the details
below this one, which a client opens the same way. No action (ending a
process, a VPN's controls) is ever part of a detail.
*/
type Detail struct {
	Section   string     `json:"section"`
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Kind      *string    `json:"kind" enum:"processes,process,network,vpns,account,overview,day,change"`
	PolledAt  *time.Time `json:"polled_at"`
	Absences  []Absence  `json:"absences"`
	Error     *string    `json:"error"`
	Children  []string   `json:"children"`
	Truncated bool       `json:"truncated"`

	Processes *ProcessList   `json:"processes"`
	Process   *ProcessPage   `json:"process"`
	Network   *Network       `json:"network"`
	VPNs      []VPNDetail    `json:"vpns" nullable:"true"`
	Account   *AccountDetail `json:"account"`
	Overview  *Overview      `json:"overview"`
	Day       *ForgeDay      `json:"day"`
	Change    *ChangePage    `json:"change"`
}

/*
ProcessList is one of the cooler's lists (cpu, gpu, memory, io), or the
processes of one name in it. One entry a process, by PID: summing a name's
processes is the client's choice.
*/
type ProcessList struct {
	List string `json:"list" enum:"cpu,gpu,memory,io"`
	// Name is a drill's process name; null for the whole list.
	Name      *string         `json:"name"`
	Note      *string         `json:"note"`
	Processes []ProcessUse    `json:"processes"`
	Summary   *ProcessSummary `json:"summary"`
}

// ProcessUse is one process's figures. Each is null where the list did not
// read it. A process with PID 0 and no name is use no process accounts for.
type ProcessUse struct {
	PID         int      `json:"pid"`
	Name        *string  `json:"name"`
	CPUPct      *float64 `json:"cpu_pct"`
	GPUPct      *float64 `json:"gpu_pct"`
	MemoryBytes *float64 `json:"memory_bytes"`
	MemoryPct   *float64 `json:"memory_pct"`
	ReadBPS     *float64 `json:"read_bps"`
	WriteBPS    *float64 `json:"write_bps"`
}

// ProcessSummary is the figures over a list: one of its fields is set, the
// list's own.
type ProcessSummary struct {
	CPU    *CPUSummary    `json:"cpu"`
	GPU    *GPUSummary    `json:"gpu"`
	Memory *MemorySummary `json:"memory"`
	IO     *IOSummary     `json:"io"`
}

// CPUSummary is the processors as a whole.
type CPUSummary struct {
	LoadPct *float64 `json:"load_pct"`
	// Trail is the recent load, oldest first, one every TrailEveryS.
	Trail       []float64 `json:"trail"`
	TrailEveryS float64   `json:"trail_every_s"`
	CoresPct    []float64 `json:"cores_pct"`
	ClockMHz    *float64  `json:"clock_mhz"`
	MaxClockMHz *float64  `json:"max_clock_mhz"`
	LoadAverage []float64 `json:"load_average"`
	Queue       *float64  `json:"queue"`
	Processes   *int      `json:"processes"`
	Threads     *int      `json:"threads"`
}

// GPUSummary is the card as a whole.
type GPUSummary struct {
	VRAMBytes      *float64 `json:"vram_bytes"`
	VRAMTotalBytes *float64 `json:"vram_total_bytes"`
	ClockMHz       *float64 `json:"clock_mhz"`
	MemClockMHz    *float64 `json:"mem_clock_mhz"`
	PowerW         *float64 `json:"power_w"`
	PowerLimitW    *float64 `json:"power_limit_w"`
}

// MemorySummary is the memory as a whole.
type MemorySummary struct {
	UsedBytes      float64  `json:"used_bytes"`
	TotalBytes     float64  `json:"total_bytes"`
	CacheBytes     *float64 `json:"cache_bytes"`
	SwapBytes      *float64 `json:"swap_bytes"`
	SwapTotalBytes float64  `json:"swap_total_bytes"`
	SwapName       *string  `json:"swap_name"`
}

// IOSummary is the disks as a whole.
type IOSummary struct {
	ReadBPS     *float64  `json:"read_bps"`
	WriteBPS    *float64  `json:"write_bps"`
	ReadTrail   []float64 `json:"read_trail"`
	WriteTrail  []float64 `json:"write_trail"`
	TrailEveryS float64   `json:"trail_every_s"`
	// BusyPct is each disk's share of time busy; a disk not read is null.
	BusyPct     []*float64 `json:"busy_pct"`
	PressurePct *float64   `json:"pressure_pct"`
	Queue       *float64   `json:"queue"`
	SwapInBPS   *float64   `json:"swap_in_bps"`
	SwapOutBPS  *float64   `json:"swap_out_bps"`
}

// ProcessPage is one process, reached through a list.
type ProcessPage struct {
	List string `json:"list"`
	Name string `json:"name"`
	// Now is its figures in the list this reading; null where it moved
	// nothing.
	Now *ProcessUse `json:"now"`
	// Idle is a reading in which no process of its name moved anything.
	Idle    bool    `json:"idle"`
	Process Process `json:"process"`
}

// Process is a process as its page says: never its command line, path or
// owner. Gone is a process no longer there, of which nothing else is known.
type Process struct {
	PID        int          `json:"pid"`
	Name       *string      `json:"name"`
	PPID       *int         `json:"ppid"`
	ParentName *string      `json:"parent_name"`
	State      *string      `json:"state"`
	Threads    *int         `json:"threads"`
	Sockets    *int         `json:"sockets"`
	Children   []ProcessRef `json:"children"`
	Gone       bool         `json:"gone"`
}

// ProcessRef is a process by PID and name.
type ProcessRef struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
}

/*
Network is the network by process, for the bandwidth section's lists and
their drills. On a drill it holds what the drill is about: the processes of
one name on one interface, and on a peer's page only that peer.
*/
type Network struct {
	// Counts is what the reader counts, in its own words: "TCP only".
	Counts string `json:"counts"`
	// Measuring is a first reading, which has no rates yet.
	Measuring bool `json:"measuring"`
	// Complete is a reader that counts every protocol and forwarded
	// traffic.
	Complete   bool            `json:"complete"`
	Processes  []NetProcess    `json:"processes"`
	Forwarded  []Forwarded     `json:"forwarded"`
	Interfaces []InterfaceRate `json:"interfaces"`
	// Process is the process a process's page is about; null elsewhere.
	Process *Process `json:"process"`
}

// NetProcess is one process's traffic on one interface.
type NetProcess struct {
	PID       int       `json:"pid"`
	Name      *string   `json:"name"`
	Interface *string   `json:"interface"`
	RxBPS     float64   `json:"rx_bps"`
	TxBPS     float64   `json:"tx_bps"`
	Tunnel    bool      `json:"tunnel"`
	Peers     []NetPeer `json:"peers"`
}

// NetPeer is one far end.
type NetPeer struct {
	Address string `json:"address"`
	// Name is the address's whole host name, where one is known.
	Name    *string     `json:"name"`
	RxBPS   float64     `json:"rx_bps"`
	TxBPS   float64     `json:"tx_bps"`
	Served  bool        `json:"served"`
	Sockets []NetSocket `json:"sockets"`
}

// NetSocket is one connection to a peer.
type NetSocket struct {
	LocalPort int     `json:"local_port"`
	PeerPort  int     `json:"peer_port"`
	RxBPS     float64 `json:"rx_bps"`
	TxBPS     float64 `json:"tx_bps"`
}

// Forwarded is traffic this machine passed between two of its interfaces.
type Forwarded struct {
	Interface string  `json:"interface"`
	Via       string  `json:"via"`
	RxBPS     float64 `json:"rx_bps"`
	TxBPS     float64 `json:"tx_bps"`
}

// InterfaceRate is an interface's rates at the reading.
type InterfaceRate struct {
	Name  string   `json:"name"`
	RxBPS *float64 `json:"rx_bps"`
	TxBPS *float64 `json:"tx_bps"`
}

// VPNDetail is everything said of one chosen VPN.
type VPNDetail struct {
	VPN
	Keepalive         *string     `json:"keepalive"`
	KeepaliveDisabled bool        `json:"keepalive_disabled"`
	Addresses         []string    `json:"addresses"`
	Gateway           *string     `json:"gateway"`
	Routes            []string    `json:"routes"`
	Checks            VPNStats    `json:"checks"`
	LastChecks        []VPNCheck  `json:"last_checks"`
	Samples           []VPNSample `json:"samples"`
	CheckEveryS       float64     `json:"check_every_s"`
}

// VPNStats are the checks' totals.
type VPNStats struct {
	Count     int     `json:"count"`
	Failures  int     `json:"failures"`
	Bounces   int     `json:"bounces"`
	AverageMS float64 `json:"average_ms"`
}

// VPNCheck is one check of the last cycle.
type VPNCheck struct {
	Type      string   `json:"type"`
	LatencyMS *float64 `json:"latency_ms"`
	OK        bool     `json:"ok"`
	Ran       bool     `json:"ran"`
}

// VPNSample is one recent check's latency, oldest first.
type VPNSample struct {
	LatencyMS float64 `json:"latency_ms"`
	OK        bool    `json:"ok"`
}

// AccountDetail is one account's breakdown.
type AccountDetail struct {
	Account     string       `json:"account"`
	Plan        *string      `json:"plan"`
	Limits      []UsageLimit `json:"limits"`
	SharesTitle *string      `json:"shares_title"`
	Shares      []UsageShare `json:"shares"`
	Facts       []UsageFact  `json:"facts"`
}

// UsageLimit is one quota.
type UsageLimit struct {
	Name           string     `json:"name"`
	UtilisationPct float64    `json:"utilisation_pct"`
	ResetsAt       *time.Time `json:"resets_at"`
	Severity       *string    `json:"severity"`
	Amount         *string    `json:"amount"`
}

// UsageShare is one part of a whole: a product's share of the week.
type UsageShare struct {
	Name string  `json:"name"`
	Pct  float64 `json:"pct"`
}

// UsageFact is a named value with no bar.
type UsageFact struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Overview is the usage section's whole detail: every account's breakdown
// and every forge's fortnight.
type Overview struct {
	Accounts []OverviewAccount `json:"accounts"`
	Forges   []Forge           `json:"forges"`
}

// OverviewAccount is an account's breakdown, or why it could not be read.
type OverviewAccount struct {
	Account   string         `json:"account"`
	Breakdown *AccountDetail `json:"breakdown"`
	Error     *string        `json:"error"`
}

// Forge is a forge's fortnight: what was done where, a day each.
type Forge struct {
	Provider string `json:"provider"`
	Title    string `json:"title"`
	// Unit names what a day counts: "contributions", "events".
	Unit    *string      `json:"unit"`
	ReadAt  *time.Time   `json:"read_at"`
	Reading bool         `json:"reading"`
	Failed  *Absence     `json:"failed"`
	Days    []ForgeCount `json:"days"`
	Items   []ForgeItem  `json:"items"`
}

// ForgeCount is one day's count.
type ForgeCount struct {
	Date  string `json:"date" format:"date"`
	Count int    `json:"count"`
}

// ForgeItem is one thing done.
type ForgeItem struct {
	Date   string `json:"date" format:"date"`
	Repo   string `json:"repo"`
	Kind   string `json:"kind"`
	Ref    string `json:"ref"`
	Title  string `json:"title"`
	Action string `json:"action"`
	Count  int    `json:"count"`
}

// ForgeDay is one day of a forge's fortnight: the forge, with its items for
// that day only.
type ForgeDay struct {
	Date  string `json:"date" format:"date"`
	Forge Forge  `json:"forge"`
}

// The states a change's page is in.
const (
	ChangeWaiting = "waiting"
	ChangeGone    = "gone"
	ChangeFound   = "found"
)

// ChangePage is a change's page: what the day's line said, and the change
// once read.
type ChangePage struct {
	State   string   `json:"state" enum:"waiting,gone,found"`
	Forge   string   `json:"forge"`
	Open    *string  `json:"open"`
	Repo    *string  `json:"repo"`
	Ref     *string  `json:"ref"`
	Line    *string  `json:"line"`
	Reading bool     `json:"reading"`
	Failed  *Absence `json:"failed"`
	Change  *Change  `json:"change"`
}

// Change is a pull request, merge request, issue or a day's commits.
type Change struct {
	Kind          string     `json:"kind"`
	Ref           string     `json:"ref"`
	Title         string     `json:"title"`
	State         *string    `json:"state"`
	StateReason   *string    `json:"state_reason"`
	Draft         bool       `json:"draft"`
	Opened        *time.Time `json:"opened"`
	Merged        *time.Time `json:"merged"`
	Closed        *time.Time `json:"closed"`
	Source        *string    `json:"source"`
	Target        *string    `json:"target"`
	Additions     *int       `json:"additions"`
	Deletions     *int       `json:"deletions"`
	Files         *int       `json:"files"`
	Commits       *int       `json:"commits"`
	Checks        *string    `json:"checks"`
	Approvals     int        `json:"approvals"`
	ChangesWanted int        `json:"changes_wanted"`
	Decision      *string    `json:"decision"`
	Labels        []string   `json:"labels"`
	// Body is the description, its lines kept.
	Body       string        `json:"body" text:"lines"`
	Comments   int           `json:"comments"`
	CommitList []Commit      `json:"commit_list"`
	FileList   []ChangedFile `json:"file_list"`
}

// Commit is one commit of a change.
type Commit struct {
	SHA       string     `json:"sha"`
	Title     string     `json:"title"`
	When      *time.Time `json:"when"`
	Additions *int       `json:"additions"`
	Deletions *int       `json:"deletions"`
}

// ChangedFile is a file a change touched.
type ChangedFile struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Status    string `json:"status"`
}

// The events of GET /v1/events (R21).
const (
	EventSection = "section"
	EventDetail  = "detail"
)
