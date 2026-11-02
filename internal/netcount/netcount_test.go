package netcount

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

var (
	near = netip.MustParseAddr("192.0.2.1")
	far  = netip.MustParseAddr("198.51.100.9")
)

// Spec 072. A line is what moved between two readings: an entry that did
// not move is left out, a new one counts whole, and one the table let go
// and made again, so that it reads lower, counts whole too.
func TestALineIsWhatMovedBetweenTwoReadings(t *testing.T) {
	steady := Flow{PID: 7, Proto: 6, Local: near, LocalPort: 5000, Peer: far, PeerPort: 443}
	moving := Flow{PID: 7, Proto: 17, Local: near, LocalPort: 5001, Peer: far, PeerPort: 443}
	fresh := Flow{PID: 9, Proto: 17, Local: near, LocalPort: 5002, Peer: far, PeerPort: 53}
	remade := Flow{PID: 0, Proto: 17, Local: near, LocalPort: 5003, Peer: far, PeerPort: 123}

	was := Snapshot{
		Flows: map[Flow]Bytes{
			steady: {Rx: 100, Tx: 10}, moving: {Rx: 100, Tx: 10}, remade: {Rx: 900, Tx: 900},
		},
		Forwarded: map[Hop]uint64{{In: "vnet0", Out: "eth0"}: 1000, {In: "eth0", Out: "vnet0"}: 50},
	}
	now := Snapshot{
		Flows: map[Flow]Bytes{
			steady: {Rx: 100, Tx: 10}, moving: {Rx: 350, Tx: 30}, fresh: {Rx: 40, Tx: 4}, remade: {Rx: 5, Tx: 6},
		},
		Forwarded: map[Hop]uint64{{In: "vnet0", Out: "eth0"}: 1600, {In: "eth0", Out: "vnet0"}: 50},
		Tunnels:   []uint32{12, 3},
		Full:      true,
	}

	line := Line(was, now)
	assert.ElementsMatch(t, []core.NetHelperFlow{
		{PID: 7, Proto: 17, Local: "192.0.2.1", LocalPort: 5001, Peer: "198.51.100.9", PeerPort: 443, Rx: 250, Tx: 20},
		{PID: 9, Proto: 17, Local: "192.0.2.1", LocalPort: 5002, Peer: "198.51.100.9", PeerPort: 53, Rx: 40, Tx: 4},
		{PID: 0, Proto: 17, Local: "192.0.2.1", LocalPort: 5003, Peer: "198.51.100.9", PeerPort: 123, Rx: 5, Tx: 6},
	}, line.F)
	assert.Equal(t, []core.NetHelperHop{{In: "vnet0", Out: "eth0", Bytes: 600}}, line.H)
	assert.Equal(t, []uint32{3, 12}, line.T)
	assert.True(t, line.Full)
}

// The committed object is built from the committed source. Editing the
// program without `make bpf` would ship the old program under the new
// source, and a reviewer reading the C would be reading the wrong thing.
// The preprocessor lines bpftool writes into a vmlinux.h.
var vmlinuxDirectives = map[string]bool{
	"#ifndef __VMLINUX_H__":                true,
	"#define __VMLINUX_H__":                true,
	"#ifndef BPF_NO_PRESERVE_ACCESS_INDEX": true,
	"#pragma clang attribute push (__attribute__((preserve_access_index)), apply_to = record)": true,
	"#endif":         true,
	"#ifndef __ksym": true,
	"#define __ksym __attribute__((section(\".ksyms\")))": true,
	"#ifndef __weak":                                       true,
	"#define __weak __attribute__((weak))":                 true,
	"#ifndef __bpf_fastcall":                               true,
	"#if __has_attribute(bpf_fastcall)":                    true,
	"#define __bpf_fastcall __attribute__((bpf_fastcall))": true,
	"#else":                           true,
	"#define __bpf_fastcall":          true,
	"#ifndef BPF_NO_KFUNC_PROTOTYPES": true,
	"#pragma clang attribute pop":     true,
	"#endif /* __VMLINUX_H__ */":      true,
}

var vmlinuxCode = regexp.MustCompile(`\basm\b|__asm|incbin|\)\s*\{|^\s*(%:|\?\?=)|\\$`)

// vmlinuxFaults names the lines of a vmlinux.h that are not what bpftool writes.
func vmlinuxFaults(src string) []string {
	var faults []string
	for number, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") && !vmlinuxDirectives[line] {
			faults = append(faults, fmt.Sprintf("line %d: %s", number+1, line))
		} else if vmlinuxCode.MatchString(line) {
			faults = append(faults, fmt.Sprintf("line %d: %s", number+1, line))
		}
	}
	return faults
}

func TestTheObjectWasBuiltFromThisSource(t *testing.T) {
	src, err := os.ReadFile("bpf/netcount.bpf.c")
	require.NoError(t, err)
	sum := sha256.Sum256(src)
	assert.Equal(t, sourceHash, hex.EncodeToString(sum[:]),
		"bpf/netcount.bpf.c changed since netcount_bpfel.o was built: run `make bpf`")
}

// The rebuild trusts what it compiles, and nobody reads 3 MB of generated types.
// So vmlinux.h may hold what bpftool writes and nothing else: a macro or a function
// in it would change the program with the source unchanged, and a second file in
// bpf/ could stand in for a pinned header.
func TestVmlinuxHoldsOnlyTypes(t *testing.T) {
	entries, err := os.ReadDir("bpf")
	require.NoError(t, err)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.Equal(t, []string{"netcount.bpf.c", "vmlinux.h"}, names)

	src, err := os.ReadFile("bpf/vmlinux.h")
	require.NoError(t, err)
	assert.Empty(t, vmlinuxFaults(string(src)))
}

func TestVmlinuxFaultsAreFound(t *testing.T) {
	assert.Empty(t, vmlinuxFaults("#ifndef __VMLINUX_H__\n#define __VMLINUX_H__\nstruct a {\n\tu16 disasm_flags;\n};\n#endif /* __VMLINUX_H__ */\n"))

	for name, src := range map[string]string{
		"a macro":       "#define bpf_skb_load_bytes(s,o,t,l) x\n",
		"an include":    "#include \"evil.h\"\n",
		"a spaced hash": "  #  define x 1\n",
		"an undef":      "#undef __weak\n",
		"a line macro":  "%:define x 1\n",
		"asm":           "__asm__(\".incbin \\\"x\\\"\");\n",
		"a function":    "static inline int f(void) {\n\treturn 1;\n}\n",
		"a trigraph":    "??=define x 1\n",
		"a continued":   "#define __weak \\\n__attribute__((weak))\n",
	} {
		t.Run(name, func(t *testing.T) {
			assert.NotEmpty(t, vmlinuxFaults(src))
		})
	}
}
