package api

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ushineko/hayami/internal/api/contract"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
A detail's raw form as the contract says it (spec 113 R20): the plain values
a source read (panel.RawDetail), with the IDs of the details below it.
Nothing here comes from the drawing but the usage section's links, whose IDs
are built from whole values; no action is ever a child.
*/

// mapDetail is one reading of a detail. src draws the usage section's links.
func mapDetail(src panel.RawDetailer, raw panel.RawDetail, err error) contract.Detail {
	d := contract.Detail{
		Section: raw.Section, ID: raw.ID, Title: raw.Title, PolledAt: when(raw.At),
		Absences: []contract.Absence{}, Children: []string{}, Error: errorText(err),
	}
	if raw.Unread != nil {
		d.Absences = append(d.Absences, unreadAbsence(raw.Unread))
		return d
	}
	at := raw.At
	switch {
	case raw.Processes != nil:
		d.Kind, d.Processes = ptr(contract.KindProcesses), mapProcessList(*raw.Processes)
		d.Children = processChildren(raw.ID, *raw.Processes)
	case raw.Process != nil:
		d.Kind, d.Process = ptr(contract.KindProcess), mapProcessPage(*raw.Process)
	case raw.Net != nil:
		d.Kind, d.Network = ptr(contract.KindNetwork), mapNetwork(raw.ID, *raw.Net)
		d.Children = netChildren(raw.ID, d.Network)
	case raw.VPN != nil:
		d.Kind, d.VPNs = ptr(contract.KindVPNs), mapVPNDetails(raw.VPN.VPNs, at)
	case raw.Account != nil:
		d.Kind, d.Account = ptr(contract.KindAccount), mapAccount(*raw.Account)
	case raw.Overview != nil:
		d.Kind, d.Overview = ptr(contract.KindOverview), mapOverview(*raw.Overview)
		d.Children = drawnChildren(src, raw)
	case raw.Day != nil:
		d.Kind, d.Day = ptr(contract.KindDay), &contract.ForgeDay{Date: raw.Day.Date, Forge: mapForge(raw.Day.Forge, raw.Day.Date)}
		d.Children = drawnChildren(src, raw)
	case raw.Change != nil:
		d.Kind, d.Change = ptr(contract.KindChange), mapChangePage(*raw.Change)
	}
	return d
}

// unreadAbsence is why a detail could not be read, in the source's own
// words where it gave them.
func unreadAbsence(err error) contract.Absence {
	r := view.Reason{Text: "not available", Detail: err.Error(), Status: view.Info}
	if a, ok := core.AbsenceOf(err); ok {
		r.Code, r.Actionable = string(a.Code), a.Actionable
		if a.Text != "" {
			r.Text = a.Text
		}
		r.Detail = a.Detail
	}
	return absence(r)
}

// processUse is one process's figures, each in its own field: which of them
// the list measures is the list's.
func processUse(listID string, u panel.RawUse) contract.ProcessUse {
	out := contract.ProcessUse{
		PID: u.PID, Name: nonEmpty(u.Name),
		CPUPct: opt(u.CPU), MemoryBytes: opt(u.Memory), MemoryPct: opt(u.Share),
	}
	switch listID {
	case view.DetailCPU:
		out.CPUPct = ptr(u.Value)
	case view.DetailGPU:
		out.GPUPct = ptr(u.Value)
	case view.DetailMemory:
		out.MemoryBytes = ptr(u.Value)
	case view.DetailIO:
		out.ReadBPS, out.WriteBPS = ptr(u.Value), ptr(u.Second)
	}
	return out
}

func mapProcessList(p panel.RawProcesses) *contract.ProcessList {
	out := &contract.ProcessList{
		List: p.List, Name: nonEmpty(p.Name), Note: nonEmpty(p.Note),
		Processes: make([]contract.ProcessUse, 0, len(p.Uses)),
	}
	for _, u := range p.Uses {
		out.Processes = append(out.Processes, processUse(p.List, u))
	}
	switch {
	case p.CPU != nil:
		out.Summary = &contract.ProcessSummary{CPU: mapCPU(*p.CPU)}
	case p.GPU != nil:
		out.Summary = &contract.ProcessSummary{GPU: mapGPU(*p.GPU)}
	case p.IO != nil:
		out.Summary = &contract.ProcessSummary{IO: mapIO(*p.IO)}
	case p.Memory != nil:
		out.Summary = &contract.ProcessSummary{Memory: mapMemory(*p.Memory)}
	}
	return out
}

func mapCPU(f view.CPUFigures) *contract.CPUSummary {
	out := &contract.CPUSummary{
		LoadPct: opt(f.Load), Trail: list(slices.Clone(f.Trail)), TrailEveryS: f.TrailEvery.Seconds(),
		CoresPct: []float64{}, ClockMHz: opt(f.Clock), Queue: opt(f.Queue), LoadAverage: []float64{},
	}
	if f.HasCores {
		out.CoresPct = list(slices.Clone(f.Cores))
	}
	if f.MaxClock > 0 {
		out.MaxClockMHz = ptr(f.MaxClock)
	}
	if f.LoadAvg.OK {
		out.LoadAverage = f.LoadAvg.V[:]
	}
	if f.HasTasks {
		out.Processes, out.Threads = ptr(f.Processes), ptr(f.Threads)
	}
	return out
}

func mapGPU(f view.GPUFigures) *contract.GPUSummary {
	out := &contract.GPUSummary{ClockMHz: opt(f.Clock), MemClockMHz: opt(f.MemClock), PowerW: opt(f.Power)}
	if f.HasVRAM {
		out.VRAMBytes, out.VRAMTotalBytes = ptr(f.VRAM), ptr(f.VRAMTotal)
	}
	if f.PowerLimit > 0 {
		out.PowerLimitW = ptr(f.PowerLimit)
	}
	return out
}

func mapIO(f view.IOFigures) *contract.IOSummary {
	out := &contract.IOSummary{
		ReadBPS: opt(f.Read), WriteBPS: opt(f.Write),
		ReadTrail: list(slices.Clone(f.ReadTrail)), WriteTrail: list(slices.Clone(f.WriteTrail)),
		TrailEveryS: f.TrailEvery.Seconds(), BusyPct: []*float64{},
		PressurePct: opt(f.Pressure), Queue: opt(f.Queue),
	}
	if f.HasBusy {
		for _, b := range f.Busy {
			if b < 0 {
				out.BusyPct = append(out.BusyPct, nil)
				continue
			}
			out.BusyPct = append(out.BusyPct, ptr(b))
		}
	}
	if f.HasSwap {
		out.SwapInBPS, out.SwapOutBPS = ptr(f.SwapIn), ptr(f.SwapOut)
	}
	return out
}

func mapMemory(f view.MemoryFigures) *contract.MemorySummary {
	return &contract.MemorySummary{
		UsedBytes: f.Used, TotalBytes: f.Total, CacheBytes: opt(f.Cache), SwapBytes: opt(f.Swap),
		SwapTotalBytes: f.SwapTotal, SwapName: nonEmpty(f.SwapName),
	}
}

/*
processChildren are a list's names, or a name's processes. A name with a "/"
in it -- a kernel worker's, "kworker/0:0" -- is not a child: a drill's ID is
its path, and that name would read as two steps of it.
*/
func processChildren(id string, p panel.RawProcesses) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, u := range p.Uses {
		var child string
		switch {
		case p.Name == "" && u.Name != "" && !strings.Contains(u.Name, "/"):
			child = id + "/" + u.Name
		case p.Name != "" && u.PID != 0:
			child = id + "/" + strconv.Itoa(u.PID)
		default:
			continue
		}
		if !seen[child] {
			seen[child] = true
			out = append(out, child)
		}
	}
	return out
}

func mapProc(p panel.RawProc) contract.Process {
	if p.Gone {
		return contract.Process{PID: p.PID, Children: []contract.ProcessRef{}, Gone: true}
	}
	out := contract.Process{
		PID: p.PID, Name: nonEmpty(p.Name), State: nonEmpty(p.State),
		Threads: ptr(p.Threads), Sockets: ptr(p.Sockets),
		Children: make([]contract.ProcessRef, 0, len(p.Children)),
	}
	if p.PPID > 0 {
		out.PPID, out.ParentName = ptr(p.PPID), nonEmpty(p.ParentName)
	}
	for _, c := range p.Children {
		out.Children = append(out.Children, contract.ProcessRef{PID: c.PID, Name: c.Name})
	}
	return out
}

func mapProcessPage(p panel.RawProcess) *contract.ProcessPage {
	out := &contract.ProcessPage{List: p.List, Name: p.Name, Idle: p.Idle, Process: mapProc(p.Proc)}
	if p.Now != nil {
		out.Now = ptr(processUse(p.List, *p.Now))
	}
	return out
}

// netPath is a bandwidth drill's path: the interface and name a page is
// about, and the peer for a peer's page. ok is false for a whole list.
type netPath struct {
	iface, name, peer string
	ok                bool
}

func parseNetPath(id string) netPath {
	parts := strings.Split(id, "/")
	if len(parts) < 3 || parts[0] != view.DetailPeers {
		return netPath{}
	}
	p := netPath{iface: parts[1], name: parts[2], ok: true}
	if len(parts) >= 4 && strings.HasPrefix(parts[3], "@") {
		p.peer = strings.TrimPrefix(parts[3], "@")
	}
	return p
}

// processName is a network process's name as the drills name it.
func processName(p core.NetProcess) string {
	if p.PID == 0 {
		return view.OtherProcesses
	}
	return p.Name
}

// mapNetwork is the network by process, cut to what a drill is about.
func mapNetwork(id string, n panel.RawNet) *contract.Network {
	path := parseNetPath(id)
	out := &contract.Network{
		Counts: n.Use.Counts, Measuring: !n.Use.HasRate, Complete: n.Use.Complete,
		Processes: []contract.NetProcess{}, Forwarded: []contract.Forwarded{}, Interfaces: []contract.InterfaceRate{},
	}
	for _, p := range n.Use.Processes {
		if path.ok && (p.Iface != path.iface || processName(p) != path.name) {
			continue
		}
		np := contract.NetProcess{
			PID: p.PID, Name: nonEmpty(p.Name), Interface: nonEmpty(p.Iface),
			RxBPS: p.Rx, TxBPS: p.Tx, Tunnel: p.Tunnel, Peers: []contract.NetPeer{},
		}
		if p.PID == 0 {
			np.Name = nil
		}
		for _, far := range p.Peers {
			if path.peer != "" && far.Addr != path.peer {
				continue
			}
			peer := contract.NetPeer{
				Address: far.Addr, Name: nonEmpty(n.Names[far.Addr]), RxBPS: far.Rx, TxBPS: far.Tx,
				Served: far.Served, Sockets: make([]contract.NetSocket, 0, len(far.Sockets)),
			}
			for _, s := range far.Sockets {
				peer.Sockets = append(peer.Sockets, contract.NetSocket{
					LocalPort: int(s.LocalPort), PeerPort: int(s.PeerPort), RxBPS: s.Rx, TxBPS: s.Tx,
				})
			}
			np.Peers = append(np.Peers, peer)
		}
		if path.peer != "" && len(np.Peers) == 0 {
			continue
		}
		out.Processes = append(out.Processes, np)
	}
	for _, f := range n.Use.Forwarded {
		if path.ok {
			break
		}
		out.Forwarded = append(out.Forwarded, contract.Forwarded{Interface: f.Iface, Via: f.Via, RxBPS: f.Rx, TxBPS: f.Tx})
	}
	for _, r := range n.Interfaces {
		if !r.Present || (path.ok && r.Name != path.iface) {
			continue
		}
		ir := contract.InterfaceRate{Name: r.Name}
		if r.HasRate {
			ir.RxBPS, ir.TxBPS = ptr(r.RxRate), ptr(r.TxRate)
		}
		out.Interfaces = append(out.Interfaces, ir)
	}
	if n.Proc != nil {
		out.Process = ptr(mapProc(*n.Proc))
	}
	return out
}

/*
netChildren are the pages below a bandwidth page: under the breakdown, each
name on each interface and each of its peers; under a name, each of its
processes and peers. The process list and a process's or a peer's page lead
nowhere further.
*/
func netChildren(id string, n *contract.Network) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(child string) {
		if !seen[child] {
			seen[child] = true
			out = append(out, child)
		}
	}
	path := parseNetPath(id)
	switch {
	case id == view.DetailPeers:
		for _, p := range n.Processes {
			if p.Interface == nil {
				continue
			}
			name := view.OtherProcesses
			if p.PID != 0 && p.Name != nil {
				name = *p.Name
			}
			if strings.Contains(name, "/") {
				continue
			}
			base := view.DetailPeers + "/" + *p.Interface + "/" + name
			add(base)
			for _, far := range p.Peers {
				add(base + "/@" + far.Address)
			}
		}
	case path.ok && path.peer == "" && len(strings.Split(id, "/")) == 3:
		for _, p := range n.Processes {
			if p.PID != 0 {
				add(id + "/" + strconv.Itoa(p.PID))
			}
		}
		for _, p := range n.Processes {
			for _, far := range p.Peers {
				add(id + "/@" + far.Address)
			}
		}
	}
	return out
}

func mapVPNDetails(in []view.VPNDetailReading, at time.Time) []contract.VPNDetail {
	out := make([]contract.VPNDetail, 0, len(in))
	for _, r := range in {
		v := contract.VPNDetail{
			VPN: mapVPN(r.VPNReading, at), Keepalive: nonEmpty(r.Keepalive), KeepaliveDisabled: r.KeepaliveDisabled,
			Addresses: list(slices.Clone(r.Addresses)), Gateway: nonEmpty(r.Gateway), Routes: list(slices.Clone(r.Routes)),
			Checks: contract.VPNStats{
				Count: r.Stats.Checks, Failures: r.Stats.Failures, Bounces: r.Stats.Bounces, AverageMS: r.Stats.AverageMS,
			},
			LastChecks:  make([]contract.VPNCheck, 0, len(r.LastChecks)),
			Samples:     make([]contract.VPNSample, 0, len(r.Samples)),
			CheckEveryS: r.CheckEvery.Seconds(),
		}
		for _, c := range r.LastChecks {
			check := contract.VPNCheck{Type: c.Type, OK: c.OK, Ran: !c.DidNotRun}
			if !c.DidNotRun {
				check.LatencyMS = ptr(c.Latency)
			}
			v.LastChecks = append(v.LastChecks, check)
		}
		for i, s := range r.Samples {
			v.Samples = append(v.Samples, contract.VPNSample{LatencyMS: s, OK: i >= len(r.Failed) || !r.Failed[i]})
		}
		out = append(out, v)
	}
	return out
}

func mapAccount(b view.UsageBreakdown) *contract.AccountDetail {
	out := &contract.AccountDetail{
		Account: b.Account, Plan: nonEmpty(b.Plan), SharesTitle: nonEmpty(b.SharesTitle),
		Limits: make([]contract.UsageLimit, 0, len(b.Limits)), Shares: make([]contract.UsageShare, 0, len(b.Shares)),
		Facts: make([]contract.UsageFact, 0, len(b.Facts)),
	}
	for _, l := range b.Limits {
		out.Limits = append(out.Limits, contract.UsageLimit{
			Name: l.Name, UtilisationPct: 100 * l.Fraction, ResetsAt: when(l.ResetsAt),
			Severity: nonEmpty(l.Severity), Amount: nonEmpty(l.Amount),
		})
	}
	for _, s := range b.Shares {
		out.Shares = append(out.Shares, contract.UsageShare{Name: s.Name, Pct: 100 * s.Fraction})
	}
	for _, f := range b.Facts {
		out.Facts = append(out.Facts, contract.UsageFact{Name: f.Name, Value: f.Value})
	}
	return out
}

func mapOverview(o panel.RawOverview) *contract.Overview {
	out := &contract.Overview{
		Accounts: make([]contract.OverviewAccount, 0, len(o.Accounts)),
		Forges:   make([]contract.Forge, 0, len(o.Forges)),
	}
	for _, a := range o.Accounts {
		oa := contract.OverviewAccount{Account: a.Label, Error: errorText(a.Err)}
		if a.Breakdown != nil {
			oa.Breakdown = mapAccount(*a.Breakdown)
		}
		out.Accounts = append(out.Accounts, oa)
	}
	for _, f := range o.Forges {
		out.Forges = append(out.Forges, mapForge(f, ""))
	}
	return out
}

// mapForge is a forge's fortnight; date, where given, keeps only that day's
// items.
func mapForge(r view.DevReading, date string) contract.Forge {
	out := contract.Forge{
		Provider: r.Provider, Title: r.Title, Unit: nonEmpty(r.Unit), ReadAt: when(r.ReadAt), Reading: r.Reading,
		Days: make([]contract.ForgeCount, 0, len(r.Days)), Items: []contract.ForgeItem{},
	}
	if r.Failed != nil {
		out.Failed = ptr(absence(*r.Failed))
	}
	for _, d := range r.Days {
		out.Days = append(out.Days, contract.ForgeCount{Date: d.Date, Count: d.Count})
	}
	for _, it := range r.Items {
		if date != "" && it.Date != date {
			continue
		}
		out.Items = append(out.Items, contract.ForgeItem{
			Date: it.Date, Repo: it.Repo, Kind: it.Kind, Ref: it.Ref, Title: it.Title, Action: it.Action, Count: it.Count,
		})
	}
	return out
}

func mapChangePage(c panel.RawChange) *contract.ChangePage {
	out := &contract.ChangePage{
		State: c.State, Forge: c.Forge, Open: nonEmpty(c.Open), Repo: nonEmpty(c.Repo), Ref: nonEmpty(c.Ref),
		Line: nonEmpty(c.Line), Reading: c.Reading,
	}
	if c.Failed != nil {
		out.Failed = ptr(absence(*c.Failed))
	}
	if ch := c.Change; ch != nil {
		change := &contract.Change{
			Kind: ch.Kind, Ref: ch.Ref, Title: ch.Title, State: nonEmpty(ch.State), StateReason: nonEmpty(ch.StateReason),
			Draft: ch.Draft, Opened: when(ch.Opened), Merged: when(ch.Merged), Closed: when(ch.Closed),
			Source: nonEmpty(ch.Source), Target: nonEmpty(ch.Target), Checks: nonEmpty(ch.Checks),
			Approvals: ch.Approvals, ChangesWanted: ch.ChangesWanted, Decision: nonEmpty(ch.Decision),
			Labels: list(slices.Clone(ch.Labels)), Body: ch.Body, Comments: ch.Comments,
			CommitList: make([]contract.Commit, 0, len(ch.Commits)), FileList: make([]contract.ChangedFile, 0, len(ch.Files)),
		}
		if ch.SizeKnown {
			change.Additions, change.Deletions = ptr(ch.Additions), ptr(ch.Deletions)
			change.Files, change.Commits = ptr(ch.FilesCount), ptr(ch.CommitCount)
		}
		for _, cm := range ch.Commits {
			commit := contract.Commit{SHA: cm.SHA, Title: cm.Title, When: when(cm.When)}
			if cm.Stats {
				commit.Additions, commit.Deletions = ptr(cm.Additions), ptr(cm.Deletions)
			}
			change.CommitList = append(change.CommitList, commit)
		}
		for _, f := range ch.Files {
			change.FileList = append(change.FileList, contract.ChangedFile{
				Path: f.Path, Additions: f.Additions, Deletions: f.Deletions, Status: f.Status,
			})
		}
		out.Change = change
	}
	return out
}

/*
drawnChildren are the usage section's links below a page: an account's own
page, a forge's day, a change on a day. They are taken from the drawing,
which is where the forge's IDs are built, and the IDs in it are whole values,
never a name cut to fit. Actions (a refresh, a pause) are not children.
*/
func drawnChildren(src panel.RawDetailer, raw panel.RawDetail) []string {
	out := []string{}
	seen := map[string]bool{raw.ID: true}
	add := func(id string) {
		if id == "" || strings.HasPrefix(id, "!") || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	var walk func(d view.Detail)
	walk = func(d view.Detail) {
		add(d.Open)
		for _, l := range d.Lines {
			add(l.Open)
			for _, o := range l.DayOpens {
				add(o)
			}
		}
		if d.Days != nil {
			for _, o := range d.Days.Opens {
				add(o)
			}
		}
		for _, p := range d.Parts {
			walk(p)
		}
	}
	walk(src.DrawDetail(raw))
	return out
}
