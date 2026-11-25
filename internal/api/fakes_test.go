package api_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
The test's sources. The hub and the server are real; a source is the one
thing a test replaces, so it says what a real one says -- Data, its reasons,
its details read raw and drawn -- and counts what was asked of it.
*/

// Names longer than any drawer's, so a cut anywhere shows.
const (
	longProcess = "a-program-with-a-name-of-forty-letters-x"
	longDevice  = "A Wireless Mouse With A Forty-Byte Name!"
	longAccount = "an account whose label is forty bytes .."
)

// longTitle is a pull request's title of two hundred bytes.
var longTitle = strings.Repeat("Make the thing do the other thing ", 6)[:200]

// polledAt is when every fixture was read.
var polledAt = time.Date(2026, 11, 24, 10, 15, 2, 0, time.UTC)

type fakeSource struct {
	key      string
	interval time.Duration
	data     any
	reasons  []view.Reason
	pollErr  error
	delay    time.Duration
	panics   bool

	// anyDrill takes any ID under a detail it offers as a drill, as the real
	// sources do: a drill's path is the client's to make up.
	anyDrill bool

	rates   []core.Rates
	trails  map[string]core.Trail
	vpns    []view.VPNReading
	fetched time.Time

	infos []view.DetailInfo
	raws  map[string]panel.RawDetail
	drawn map[string]view.Detail

	polls  atomic.Int32
	inPoll atomic.Int32
	twice  atomic.Bool
	reads  atomic.Int32

	mu     sync.Mutex
	closed map[string]int
}

func (f *fakeSource) Key() string { return f.key }

func (f *fakeSource) Interval() time.Duration {
	if f.interval == 0 {
		return time.Hour
	}
	return f.interval
}

func (f *fakeSource) Poll(ctx context.Context) (bool, error) {
	if f.inPoll.Add(1) > 1 {
		f.twice.Store(true)
	}
	defer f.inPoll.Add(-1)
	f.polls.Add(1)
	if f.panics {
		panic("a source fell over at /home/someone/.config")
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
		}
	}
	return true, f.pollErr
}

func (f *fakeSource) Section() view.Section { return view.Section{Key: f.key, Reasons: f.reasons} }
func (f *fakeSource) Data() any             { return f.data }

func (f *fakeSource) Readings() []core.Rates         { return f.rates }
func (f *fakeSource) Trail(name string) core.Trail   { return f.trails[name] }
func (f *fakeSource) VPNReadings() []view.VPNReading { return f.vpns }
func (f *fakeSource) FetchedAt() time.Time           { return f.fetched }
func (f *fakeSource) Details() []view.DetailInfo     { return f.infos }
func (f *fakeSource) DetailInterval(string) time.Duration {
	if f.interval == 0 {
		return time.Hour
	}
	return f.interval
}

func (f *fakeSource) CloseDetail(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed == nil {
		f.closed = map[string]int{}
	}
	f.closed[id]++
}

func (f *fakeSource) closes(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed[id]
}

// Drills are the fixture's details that are not offered at the top.
func (f *fakeSource) Drills(id string) bool {
	if f.anyDrill {
		root, steps := panel.DrillRoot(id)
		_, ok := f.raws[root]
		return ok && len(steps) > 0
	}
	if _, ok := f.raws[id]; !ok {
		return false
	}
	for _, info := range f.infos {
		if info.ID == id {
			return false
		}
	}
	return true
}

func (f *fakeSource) DrillUp(id string) string { return panel.DrillOut(id) }

func (f *fakeSource) PollDetail(ctx context.Context, id string) (view.Detail, error) {
	raw, err := f.ReadDetail(ctx, id)
	return f.DrawDetail(raw), err
}

func (f *fakeSource) ReadDetail(_ context.Context, id string) (panel.RawDetail, error) {
	f.reads.Add(1)
	raw, ok := f.raws[id]
	if !ok {
		return panel.RawDetail{}, errors.New("no such detail")
	}
	return raw, nil
}

func (f *fakeSource) DrawDetail(raw panel.RawDetail) view.Detail { return f.drawn[raw.ID] }

// fixtures are a source for every section, each reading what a real one
// would, with names longer than a drawer takes.
func fixtures() []*fakeSource {
	return []*fakeSource{clockFixture(), bandwidthFixture(), usageFixture(), coolerFixture(), peripheralsFixture()}
}

func clockFixture() *fakeSource {
	return &fakeSource{key: "clock", interval: time.Second, data: struct{}{}}
}

func bandwidthFixture() *fakeSource {
	wifi := &core.Wireless{Connected: true, RSSI: -52, HasRSSI: true, Frequency: 5180, HasFrequency: true,
		Channel: 36, HasChannel: true, Generation: "Wi-Fi 6", RxRate: 866.7, HasRx: true}
	return &fakeSource{
		key: "bandwidth", interval: 2 * time.Second,
		rates: []core.Rates{
			{Name: "eth0", RxRate: 125000, TxRate: 2500, RxTotal: 9 << 30, TxTotal: 1 << 30, Present: true, HasRate: true},
			{Name: "wlan0", RxTotal: 1024, Present: true, Wireless: wifi},
		},
		trails: map[string]core.Trail{"eth0": {Rx: []float64{100000, 125000}, Tx: []float64{2000, 2500}}},
		vpns: []view.VPNReading{{
			Name: "office", Label: "work", State: view.VPNUp, Interface: "tun0",
			Connected: view.Some(90 * time.Minute), Latency: view.Some(23.5),
			Traffic: &view.BandwidthReading{RxRate: 4000, TxRate: 1000, HasRate: true},
		}},
		reasons: []view.Reason{{Label: "VPN", Text: "would not answer", Detail: "dbus: no reply", Status: view.Warn, Code: "vpn"}},
		infos:   []view.DetailInfo{view.ProcessInfo(view.DetailPeers), view.VPNDetailInfo()},
		raws: map[string]panel.RawDetail{
			"peers": {Section: "bandwidth", ID: "peers", Title: "Bandwidth by peer", At: polledAt, Net: netFixture()},
			"peers/eth0/" + longProcess: {Section: "bandwidth", ID: "peers/eth0/" + longProcess, Title: "Bandwidth by peer",
				At: polledAt, Net: netFixture()},
			"vpn": {Section: "bandwidth", ID: "vpn", Title: "VPNs", At: polledAt, VPN: &panel.RawVPN{VPNs: []view.VPNDetailReading{{
				VPNReading: view.VPNReading{Name: "office", State: view.VPNUp, Interface: "tun0", Latency: view.Some(23.5)},
				Keepalive:  "on", Addresses: []string{"10.0.0.2/24"}, Gateway: "10.0.0.1", Routes: []string{"10.0.0.0/8"},
				Stats:      view.VPNStats{Checks: 10, Failures: 1, Bounces: 0, AverageMS: 22},
				LastChecks: []view.VPNCheckLine{{Type: "ping", Latency: 23.5, OK: true}, {Type: "dns", DidNotRun: true}},
				Samples:    []float64{21, 23.5}, Failed: []bool{false, true}, CheckEvery: 30 * time.Second,
			}}}},
		},
	}
}

func netFixture() *panel.RawNet {
	return &panel.RawNet{
		Use: core.NetUse{HasRate: true, Counts: "TCP and UDP", Processes: []core.NetProcess{
			{PID: 4411, Name: longProcess, Iface: "eth0", Rx: 120000, Tx: 2000, Peers: []core.NetPeer{
				{Addr: "192.0.2.10", Rx: 120000, Tx: 2000, Sockets: []core.NetSocket{{LocalPort: 51000, PeerPort: 443, Rx: 120000, Tx: 2000}}},
			}},
			{PID: 0, Iface: "eth0", Rx: 5000, Tx: 500},
		}},
		Interfaces: []core.Rates{{Name: "eth0", RxRate: 125000, TxRate: 2500, Present: true, HasRate: true}},
		Names:      map[string]string{"192.0.2.10": "edge-7.cache.example.com"},
	}
}

func usageFixture() *fakeSource {
	resets := time.Date(2026, 11, 26, 16, 0, 0, 0, time.UTC)
	day := view.DevReading{
		Provider: "github", Title: "GitHub", Unit: "contributions", ReadAt: polledAt,
		Days:  []view.DevDay{{Date: "2026-11-23", Count: 1}, {Date: "2026-11-24", Count: 3}},
		Items: []view.DevItem{{Provider: "github", Date: "2026-11-24", Repo: "someone/project", Kind: "pr", Ref: "#12", Title: longTitle, Action: "opened", Count: 1}},
	}
	breakdown := view.UsageBreakdown{Account: longAccount, Plan: "Max", Limits: []view.UsageLimit{
		{Name: "Week", Fraction: 0.4, ResetsAt: resets},
	}, Shares: []view.UsageShare{{Name: "Opus", Fraction: 0.25}}, Facts: []view.UsageFact{{Name: "Extra usage", Value: "off"}}}
	changeID := "accounts/github/2026-11-24/pr:someone/project#12"
	return &fakeSource{
		key: "usage", interval: 30 * time.Second, fetched: polledAt,
		data: []view.UsageWindow{
			{Account: longAccount, Badge: "M", Name: "5h", Fraction: 0.12, ResetsAt: resets, Span: 5 * time.Hour},
			{Account: longAccount, Badge: "M", Name: "7d", Fraction: 0.4, ResetsAt: resets, Span: 7 * 24 * time.Hour},
		},
		infos: []view.DetailInfo{{ID: "accounts", Title: view.DevTitle}, {ID: longAccount, Title: longAccount}},
		raws: map[string]panel.RawDetail{
			"accounts": {Section: "usage", ID: "accounts", Title: view.DevTitle, At: polledAt, Overview: &panel.RawOverview{
				Accounts: []panel.RawAccount{{Label: longAccount, Breakdown: &breakdown}},
				Forges:   []view.DevReading{day},
			}},
			longAccount:                  {Section: "usage", ID: longAccount, Title: longAccount, At: polledAt, Account: &breakdown},
			"accounts/github/2026-11-24": {Section: "usage", ID: "accounts/github/2026-11-24", Title: "GitHub", At: polledAt, Day: &panel.RawDay{Forge: day, Date: "2026-11-24"}},
			changeID: {Section: "usage", ID: changeID, Title: "GitHub", At: polledAt, Change: &panel.RawChange{
				State: panel.ChangeFound, Forge: "GitHub", Open: "pr", Repo: "someone/project", Ref: "#12", Line: "#12 " + longTitle,
				Change: &view.DevChange{Kind: "pr", Ref: "#12", Title: longTitle, State: "open", Opened: polledAt,
					Additions: 10, Deletions: 2, FilesCount: 1, CommitCount: 1, SizeKnown: true, Approvals: 1,
					Labels: []string{"enhancement"}, Body: "First line.\n\nSecond paragraph.",
					Commits: []view.DevCommit{{SHA: "abc1234", Title: "Do it", When: polledAt}},
					Files:   []view.DevFile{{Path: "main.go", Additions: 10, Deletions: 2, Status: "modified"}}},
			}},
		},
		drawn: map[string]view.Detail{
			"accounts": {ID: "accounts", Parts: []view.Detail{
				{ID: longAccount, Open: longAccount},
				{ID: "github", Open: view.RefreshAct("github"), Days: &view.DayStrip{Opens: []string{"accounts/github/2026-11-23", "accounts/github/2026-11-24"}}},
			}},
			"accounts/github/2026-11-24": {ID: "accounts/github/2026-11-24", Parts: []view.Detail{{Lines: []view.DetailLine{{Open: changeID}}}}},
		},
	}
}

func coolerFixture() *fakeSource {
	cpu := view.CPUFigures{Load: view.Some(12.5), Trail: []float64{10, 12.5}, TrailEvery: 5 * time.Second,
		Cores: []float64{20, 5}, HasCores: true, Clock: view.Some(4800.0), MaxClock: 5700,
		LoadKind: view.LoadAverage, LoadAvg: view.Some([3]float64{1.5, 1.2, 1.0}), Processes: 412, Threads: 2048, HasTasks: true}
	uses := []panel.RawUse{
		{PID: 41, ProcessUse: view.ProcessUse{Name: longProcess, Value: 10, CPU: view.Some(10.0), Memory: view.Some(2e9), Share: view.Some(6.25)}},
		{PID: 42, ProcessUse: view.ProcessUse{Name: longProcess, Value: 2.5}},
		{PID: 7, ProcessUse: view.ProcessUse{Name: "kworker/0:0", Value: 0.1}},
	}
	page := panel.RawProcess{List: "cpu", Name: longProcess, Now: &uses[0], Note: "share of every processor together",
		Proc: panel.RawProc{PID: 41, Name: longProcess, PPID: 1, ParentName: "systemd", State: "S (sleeping)", Threads: 12, Sockets: 3,
			Children: []core.ProcRef{{PID: 43, Name: longProcess}}}}
	return &fakeSource{
		key: "cooler", interval: 5 * time.Second,
		data: view.CoolerReading{Probes: []view.Probe{
			{ID: "cpu", Role: view.RoleCPU, Name: "A Processor With A Long Name, Sixteen Cores", Load: view.Some(12.5), Temp: view.Some(52.0), Trail: []float64{50, 52}},
			{ID: "memory", Role: view.RoleMemory, Load: view.Some(50.0), Bytes: view.Some(16e9), Of: view.Some(32e9)},
			{ID: "pump", Role: view.RolePump, RPM: view.Some(2400), Stale: true},
		}, Uptime: view.Some(36 * time.Hour)},
		infos: []view.DetailInfo{view.ProcessInfo(view.DetailCPU)},
		raws: map[string]panel.RawDetail{
			"cpu": {Section: "cooler", ID: "cpu", Title: "CPU by process", At: polledAt, Processes: &panel.RawProcesses{
				List: "cpu", Uses: uses, Note: "share of every processor together", CPU: &cpu}},
			"cpu/" + longProcess: {Section: "cooler", ID: "cpu/" + longProcess, Title: "CPU by process", At: polledAt,
				Processes: &panel.RawProcesses{List: "cpu", Name: longProcess, Uses: uses[:2]}},
			"cpu/" + longProcess + "/41": {Section: "cooler", ID: "cpu/" + longProcess + "/41", Title: "CPU by process", At: polledAt, Process: &page},
		},
	}
}

func peripheralsFixture() *fakeSource {
	return &fakeSource{
		key: "peripherals", interval: 15 * time.Second,
		data: view.PeripheralsReading{Devices: []view.PeripheralReading{
			{Name: longDevice, Level: 76, Kind: view.KindMouse, Seen: polledAt},
			{Name: "Headset", Band: "good", Segments: 3, Charge: view.Filling, Kind: view.KindHeadset},
			{Name: "Keyboard", NoLevel: true, Charge: view.Charged, Kind: view.KindKeyboard, Stale: true},
		}},
	}
}

// sources are fakes as the hub takes them.
func sources(fakes []*fakeSource) []panel.Source {
	out := make([]panel.Source, len(fakes))
	for i, f := range fakes {
		out[i] = f
	}
	return out
}
