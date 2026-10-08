package core

import (
	"context"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

/*
The card on Windows, without a subprocess (spec 034).

**D3DKMT for the temperature and the name.** The graphics kernel's thunks in
gdi32 are what Task Manager reads: D3DKMTQueryAdapterInfo with
KMTQAITYPE_ADAPTERPERFDATA gives the temperature in tenths of a degree, from
any vendor's driver that reports one (WDDM 2.4 and later). They are plain
calls into a system library, so no cgo, and the terminal panel keeps building
without it.

**PDH for the load.** "GPU Engine(*)\Utilization Percentage" is the counter
Task Manager's GPU column is made from: one instance per process per engine,
which BusiestEngine reduces to the adapter's figure.

nvidia-smi stays behind both, for whatever they did not say.
*/

var (
	gdi32                 = windows.NewLazySystemDLL("gdi32.dll")
	procEnumAdapters2     = gdi32.NewProc("D3DKMTEnumAdapters2")
	procQueryAdapterInfo  = gdi32.NewProc("D3DKMTQueryAdapterInfo")
	procCloseAdapter      = gdi32.NewProc("D3DKMTCloseAdapter")
	pdh                   = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQuery      = pdh.NewProc("PdhOpenQueryW")
	procPdhAddCounter     = pdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollect        = pdh.NewProc("PdhCollectQueryData")
	procPdhFormattedArray = pdh.NewProc("PdhGetFormattedCounterArrayW")
)

// The D3DKMT query types this reads (d3dkmthk.h).
const (
	kmtqaiAdapterRegistryInfo = 8
	kmtqaiAdapterType         = 15
	kmtqaiAdapterPerfData     = 62
)

// softwareDevice is D3DKMT_ADAPTERTYPE's SoftwareDevice bit: the Microsoft
// Basic Render Driver, which is no card.
const softwareDevice = 1 << 2

// d3dkmtAdapterInfo is D3DKMT_ADAPTERINFO.
type d3dkmtAdapterInfo struct {
	Handle   uint32
	LUIDLow  uint32
	LUIDHigh int32
	Sources  uint32
	Precise  int32
}

// d3dkmtEnumAdapters2 is D3DKMT_ENUMADAPTERS2.
type d3dkmtEnumAdapters2 struct {
	Count    uint32
	Adapters *d3dkmtAdapterInfo
}

// d3dkmtQueryAdapterInfo is D3DKMT_QUERYADAPTERINFO.
type d3dkmtQueryAdapterInfo struct {
	Handle uint32
	Type   uint32
	Data   unsafe.Pointer
	Size   uint32
}

// d3dkmtAdapterPerfData is D3DKMT_ADAPTER_PERFDATA.
type d3dkmtAdapterPerfData struct {
	PhysicalAdapterIndex uint32
	MemoryFrequency      uint64
	MaxMemoryFrequency   uint64
	MaxMemoryFrequencyOC uint64
	MemoryBandwidth      uint64
	PCIEBandwidth        uint64
	FanRPM               uint32
	Power                uint32
	Temperature          uint32 // tenths of a degree Celsius
	PowerStateOverride   uint8
}

// d3dkmtAdapterRegistryInfo is D3DKMT_ADAPTERREGISTRYINFO.
type d3dkmtAdapterRegistryInfo struct {
	AdapterString [windows.MAX_PATH]uint16
	BiosString    [windows.MAX_PATH]uint16
	DacType       [windows.MAX_PATH]uint16
	ChipType      [windows.MAX_PATH]uint16
}

// adapter is one card as D3DKMT describes it.
type adapter struct {
	LUID        string
	Name        string
	Temperature float64
	HasTemp     bool
}

// nativeGraphics is D3DKMT and PDH, behind one reader that keeps the counter
// query open between polls.
func nativeGraphics() func(context.Context) Graphics {
	e := &engineLoad{}
	return func(context.Context) Graphics {
		a, ok := firstAdapter()
		if !ok {
			return Graphics{}
		}
		out := Graphics{Name: a.Name, Temperature: a.Temperature, HasTemperature: a.HasTemp}
		out.Load, out.HasLoad = e.load(a.LUID)
		return out
	}
}

/*
firstAdapter is the first hardware adapter, preferring one that reports a
temperature. A machine lists the Basic Render Driver beside its card, and a
laptop lists two cards of which the integrated one may report nothing.
*/
func firstAdapter() (adapter, bool) {
	adapters := enumAdapters()
	for _, a := range adapters {
		if a.HasTemp {
			return a, true
		}
	}
	if len(adapters) > 0 {
		return adapters[0], true
	}
	return adapter{}, false
}

// enumAdapters is every hardware adapter, opened, read and closed again: the
// set changes with a driver update or a reset, so nothing is held.
func enumAdapters() []adapter {
	var e d3dkmtEnumAdapters2
	if r, _, _ := procEnumAdapters2.Call(uintptr(unsafe.Pointer(&e))); r != 0 || e.Count == 0 {
		return nil
	}
	list := make([]d3dkmtAdapterInfo, e.Count)
	e.Adapters = &list[0]
	if r, _, _ := procEnumAdapters2.Call(uintptr(unsafe.Pointer(&e))); r != 0 {
		return nil
	}

	var out []adapter
	for _, info := range list[:e.Count] {
		a, hardware := describeAdapter(info.Handle)
		closeAdapter(info.Handle)
		if hardware {
			a.LUID = LUIDString(info.LUIDHigh, info.LUIDLow)
			out = append(out, a)
		}
	}
	return out
}

// describeAdapter reads one adapter's type, name and temperature. A software
// adapter is not hardware and is not described.
func describeAdapter(handle uint32) (adapter, bool) {
	var kind uint32
	if !queryAdapter(handle, kmtqaiAdapterType, unsafe.Pointer(&kind), unsafe.Sizeof(kind)) ||
		kind&softwareDevice != 0 {
		return adapter{}, false
	}
	var a adapter
	var reg d3dkmtAdapterRegistryInfo
	if queryAdapter(handle, kmtqaiAdapterRegistryInfo, unsafe.Pointer(&reg), unsafe.Sizeof(reg)) {
		a.Name = windows.UTF16ToString(reg.AdapterString[:])
	}
	// A driver that does not report a temperature answers zero, which no
	// running card is.
	var perf d3dkmtAdapterPerfData
	if queryAdapter(handle, kmtqaiAdapterPerfData, unsafe.Pointer(&perf), unsafe.Sizeof(perf)) &&
		perf.Temperature > 0 {
		a.Temperature, a.HasTemp = float64(perf.Temperature)/10, true
	}
	return a, true
}

// queryAdapter is one D3DKMTQueryAdapterInfo, and whether it succeeded.
func queryAdapter(handle, kind uint32, data unsafe.Pointer, size uintptr) bool {
	q := d3dkmtQueryAdapterInfo{Handle: handle, Type: kind, Data: data, Size: uint32(size)} //nolint:gosec // a struct's size
	r, _, _ := procQueryAdapterInfo.Call(uintptr(unsafe.Pointer(&q)))
	return r == 0
}

// closeAdapter closes a handle EnumAdapters2 opened.
func closeAdapter(handle uint32) {
	closing := struct{ Handle uint32 }{handle}
	_, _, _ = procCloseAdapter.Call(uintptr(unsafe.Pointer(&closing)))
}

// The PDH constants this uses (pdh.h, pdhmsg.h).
const (
	pdhFmtDouble   = 0x00000200
	pdhFmtNoCap100 = 0x00008000
	pdhMoreData    = 0x800007D2
	engineCounter  = `\GPU Engine(*)\Utilization Percentage`
)

// pdhItem is PDH_FMT_COUNTERVALUE_ITEM_W with a double value.
type pdhItem struct {
	Name   *uint16
	Status uint32
	Value  float64
}

/*
engineLoad holds the GPU Engine query open across polls.

The counter is a rate, so a figure needs two collections. **The first call
takes both**, CPUWarmup apart, for the reason CPULoad's does: a command that
polls once would otherwise never say a load. Later calls difference against
the previous call's collection, which makes the load a mean over the poll
interval as the processor's is.

**Two collections are never closer than CPUWarmup.** The engines' running
time is counted coarsely, and over a few milliseconds a busy card reads 0 %:
two reads back to back, which the first poll and a check before it are, said
nothing true. A call that comes too soon waits out the rest.
*/
type engineLoad struct {
	mu      sync.Mutex
	query   uintptr
	counter uintptr
	last    time.Time
	failed  bool
}

// load is the adapter's busiest engine since the previous call.
func (e *engineLoad) load(luid string) (float64, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failed {
		return 0, false
	}
	if e.query == 0 && !e.open() {
		e.failed = true
		return 0, false
	}
	if e.last.IsZero() {
		_, _, _ = procPdhCollect.Call(e.query)
		e.last = time.Now()
	}
	if soon := CPUWarmup - time.Since(e.last); soon > 0 {
		time.Sleep(soon)
	}
	r, _, _ := procPdhCollect.Call(e.query)
	e.last = time.Now()
	if r != 0 {
		return 0, false
	}
	return BusiestEngine(e.values(), luid)
}

// open builds the query. A machine without the counter set (a server core
// install, a disabled performance library) fails here, once, and has no load.
func (e *engineLoad) open() bool {
	if r, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&e.query))); r != 0 {
		return false
	}
	path, err := windows.UTF16PtrFromString(engineCounter)
	if err != nil {
		return false
	}
	r, _, _ := procPdhAddCounter.Call(e.query, uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&e.counter)))
	return r == 0
}

// values is the last collection, instance name to percentage. An instance
// whose value is not valid -- a process that has just appeared has one
// sample, not two -- is left out.
//
// The array is asked for its size and then filled, and is asked again if it
// grew in between: the instances are processes, which come and go.
func (e *engineLoad) values() map[string]float64 {
	for range 3 {
		var size, count uint32
		r, _, _ := procPdhFormattedArray.Call(e.counter, pdhFmtDouble|pdhFmtNoCap100,
			uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
		if uint32(r) != pdhMoreData || size == 0 { //nolint:gosec // PDH_STATUS is 32 bits
			return nil
		}
		// The buffer holds the items and, after them, the strings they point
		// at, so it is allocated as items to keep their alignment and sized
		// in bytes.
		itemSize := uint32(unsafe.Sizeof(pdhItem{}))
		buf := make([]pdhItem, (size+itemSize-1)/itemSize)
		r, _, _ = procPdhFormattedArray.Call(e.counter, pdhFmtDouble|pdhFmtNoCap100,
			uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
		switch {
		case uint32(r) == pdhMoreData: //nolint:gosec // PDH_STATUS is 32 bits
			continue
		case r != 0:
			return nil
		}
		out := make(map[string]float64, count)
		for _, item := range buf[:count] {
			if item.Status > 1 { // neither PDH_CSTATUS_VALID_DATA nor NEW_DATA
				continue
			}
			out[windows.UTF16PtrToString(item.Name)] = item.Value
		}
		return out
	}
	return nil
}

// GPUSensorDetail is every route GraphicsReader took to the card's
// temperature on Windows, for the reason given when none answers.
func GPUSensorDetail() string {
	return "asked D3DKMT and the GPU Engine counters, and tried nvidia-smi"
}
