package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Spec 060 R10. The counters' instances carry a process each; a process's
// figure is its busiest engine, held to a hundred, on the adapter asked for.
func TestEachProcessIsItsBusiestEngine(t *testing.T) {
	const luid = "0x00000000_0x0000e985"
	values := map[string]float64{
		"pid_10_luid_0x00000000_0x0000E985_phys_0_eng_0_engtype_3D":          40,
		"pid_10_luid_0x00000000_0x0000E985_phys_0_eng_3_engtype_VideoDecode": 72,
		"pid_11_luid_0x00000000_0x0000E985_phys_0_eng_0_engtype_3D":          130,
		"pid_12_luid_0x00000000_0x0000E985_phys_0_eng_0_engtype_3D":          0,
		"pid_13_luid_0x00000000_0x0000AAAA_phys_0_eng_0_engtype_3D":          99,
		"_Total": 500,
	}

	got := BusiestByProcess(values, luid)

	assert.Equal(t, map[int]float64{10: 72, 11: 100, 12: 0}, got)
}

// The card's own figure is unchanged by the instances keeping their process:
// each engine summed over every process, and the busiest.
func TestTheCardsOwnFigureIsUnchanged(t *testing.T) {
	values := map[string]float64{
		"pid_10_luid_0x00000000_0x0000E985_phys_0_eng_0_engtype_3D": 40,
		"pid_11_luid_0x00000000_0x0000E985_phys_0_eng_0_engtype_3D": 25,
		"pid_10_luid_0x00000000_0x0000E985_phys_0_eng_3_engtype_3D": 50,
	}

	load, ok := BusiestEngine(values, "0x00000000_0x0000e985")

	require.True(t, ok)
	assert.InDelta(t, 65, load, 1e-9)
}

// A DRM client is its driver, its ID and the time each engine gave it.
func TestAnFDInfoFileGivesAClientsEngines(t *testing.T) {
	driver, client, engines := ParseFDInfo("pos:\t0\nflags:\t02\ndrm-driver:\ti915\ndrm-pdev:\t0000:00:02.0\n" +
		"drm-client-id:\t42\ndrm-engine-render:\t1500 ns\ndrm-engine-video:\t0 ns\n" +
		"drm-engine-capacity-video:\t2\ndrm-memory-system:\t100 KiB\n")

	assert.Equal(t, "i915", driver)
	assert.Equal(t, "42", client)
	assert.Equal(t, map[string]uint64{"render": 1500, "video": 0}, engines)

	_, client, engines = ParseFDInfo("pos:\t0\nflags:\t02\n")
	assert.Empty(t, client, "an ordinary file is not a client")
	assert.Empty(t, engines)
}

// driTree writes a /proc in which each process holds DRM descriptors with
// the fdinfo given, and returns its root. It needs symbolic links, which
// Windows gives only to some users.
func driTree(t *testing.T, fds map[int]map[int]string) string {
	t.Helper()
	root := t.TempDir()
	for pid, files := range fds {
		dir := filepath.Join(root, strconv.Itoa(pid))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "fd"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "fdinfo"), 0o755))
		for fd, info := range files {
			if err := os.Symlink("/dev/dri/renderD128", filepath.Join(dir, "fd", strconv.Itoa(fd))); err != nil {
				t.Skipf("this user cannot make a symbolic link: %v", err)
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "fdinfo", strconv.Itoa(fd)), []byte(info), 0o600))
		}
	}
	return root
}

func fdinfo(client string, render, video int) string {
	return "drm-driver:\ti915\ndrm-client-id:\t" + client + "\n" +
		"drm-engine-render:\t" + strconv.Itoa(render) + " ns\ndrm-engine-video:\t" + strconv.Itoa(video) + " ns\n"
}

// R10. An engine's time is differenced over the time that passed, and a
// process is its busiest engine; a client two descriptors share is counted
// once.
func TestFDInfoIsEngineTimeOverTheInterval(t *testing.T) {
	root := driTree(t, map[int]map[int]string{
		50: {3: fdinfo("1", 0, 0), 4: fdinfo("1", 0, 0)},
		// The same client again, handed to a helper process: one open of the
		// device, counted for the first process found with it.
		55: {9: fdinfo("1", 0, 0)},
		60: {5: fdinfo("2", 1000, 0)},
	})
	f := NewFDInfoGPU(root)
	clock := time.Unix(100, 0)
	f.now = func() time.Time { return clock }
	f.wait = func() {
		// A second passes: process 50's client has half of it on render and
		// a quarter on video, through both descriptors; 60's has none.
		clock = clock.Add(time.Second)
		for _, at := range [][2]string{{"50", "3"}, {"50", "4"}, {"55", "9"}} {
			require.NoError(t, os.WriteFile(filepath.Join(root, at[0], "fdinfo", at[1]),
				[]byte(fdinfo("1", 500_000_000, 250_000_000)), 0o600))
		}
	}

	got, err := f.Read(t.Context())
	require.NoError(t, err)
	sort.Slice(got, func(i, j int) bool { return got[i].PID < got[j].PID })

	require.Len(t, got, 2, "the helper holding the same client is not a second user of the card")
	assert.Equal(t, 50, got[0].PID)
	assert.InDelta(t, 50, got[0].Load, 1e-6, "the busier engine, counted once though two descriptors hold it")
	assert.Equal(t, GPUProcess{PID: 60, Load: 0}, got[1])
}

// A machine where nothing publishes its engines has nothing to say, which a
// chain takes as a reason to ask the next provider.
func TestFDInfoWithNoClientHasNothing(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "70", "fd"), 0o755))
	f := NewFDInfoGPU(root)
	f.wait = func() {}

	_, err := f.Read(t.Context())

	assert.ErrorIs(t, err, errNothing)
}

// nvidia-smi's table is a process per line, its share of the card in the
// fourth column, a dash where it did nothing and a dash where there is no
// process at all.
func TestPMONIsAProcessPerLine(t *testing.T) {
	out := "# gpu         pid   type     sm    mem    enc    dec    jpg    ofa    command \n" +
		"# Idx           #    C/G      %      %      %      %      %      %    name \n" +
		"    0       1234     G     31      1      -      -      -      -    compositor     \n" +
		"    0       5678   C+G      -      -      -      -      -      -    idler          \n" +
		"    1       1234     G     80      1      -      -      -      -    compositor     \n" +
		"    1          -     -      -      -      -      -      -      -    -              \n"

	assert.Equal(t, []GPUProcess{
		{PID: 1234, Name: "compositor", Load: 100},
		{PID: 5678, Name: "idler", Load: 0},
	}, ParsePMON(out))
	assert.Empty(t, ParsePMON("# gpu pid\n"))
}

// answers is a provider that returns found and records that it was asked.
func answers(asked *[]string, name string, found []GPUProcess, err error) func(context.Context) ([]GPUProcess, error) {
	return func(context.Context) ([]GPUProcess, error) {
		*asked = append(*asked, name)
		return found, err
	}
}

// R10. On Linux the kernel is asked first and nvidia-smi last, and only when
// the kernel had nothing; a card read through nvidia-smi is asked of
// nvidia-smi alone, because another card may publish plenty.
func TestTheCardsListIsOfTheCardTheRowIsFor(t *testing.T) {
	var asked []string
	intel := []GPUProcess{{PID: 1, Load: 5}}
	nvidia := []GPUProcess{{PID: 2, Load: 9}}
	chain := linuxGPUProcesses(answers(&asked, "kernel", intel, nil), answers(&asked, "tool", nvidia, nil))

	got := chain("").Read(t.Context())
	assert.Equal(t, intel, got.Value)
	assert.Equal(t, []string{"kernel"}, asked, "the subprocess is not run when the kernel answered")

	asked = nil
	got = chain(smiName).Read(t.Context())
	assert.Equal(t, nvidia, got.Value)
	assert.Equal(t, []string{"tool"}, asked, "the other card's fdinfo is not this card's list")
	assert.Equal(t, []string{pmonName}, chain(smiName).Names())
	assert.Equal(t, []string{fdinfoName, pmonName}, chain("").Names())

	asked = nil
	quiet := linuxGPUProcesses(answers(&asked, "kernel", nil, errNothing), answers(&asked, "tool", nvidia, nil))
	got = quiet("").Read(t.Context())
	assert.Equal(t, nvidia, got.Value)
	assert.Equal(t, []string{"kernel", "tool"}, asked)
}

// The processes are named by the process table, one that has gone is left
// out, and a provider's own name stands where the table has none.
func TestTheCardsProcessesAreNamedByTheProcessTable(t *testing.T) {
	g := &gpuProcesses{
		chain: windowsGPUProcesses(func(context.Context) ([]GPUProcess, error) {
			return []GPUProcess{{PID: 1, Load: 30}, {PID: 2, Load: 5}, {PID: 3, Name: "from-the-tool", Load: 1}}, nil
		}),
		samples: func(context.Context) ([]ProcessSample, error) {
			return []ProcessSample{{PID: 1, Name: "game"}}, nil
		},
	}

	got, err := g.Read(t.Context(), "")

	require.NoError(t, err)
	assert.Equal(t, []GPUProcess{{PID: 1, Name: "game", Load: 30}, {PID: 3, Name: "from-the-tool", Load: 1}}, got)
}

// No list is a typed absence that names every provider tried.
func TestNoListOfTheCardIsATypedAbsence(t *testing.T) {
	boom := errors.New("not installed")
	var asked []string
	g := &gpuProcesses{
		chain:   linuxGPUProcesses(answers(&asked, "kernel", nil, errNothing), answers(&asked, "tool", nil, boom)),
		samples: func(context.Context) ([]ProcessSample, error) { return nil, nil },
	}

	_, err := g.Read(t.Context(), "")

	a, ok := AbsenceOf(err)
	require.True(t, ok)
	assert.Equal(t, AbsenceGPUProcesses, a.Code)
	assert.Contains(t, a.Detail, fdinfoName)
	assert.Contains(t, a.Detail, pmonName)

	none := &gpuProcesses{chain: noGPUProcesses, samples: g.samples}
	_, err = none.Read(t.Context(), "")
	a, ok = AbsenceOf(err)
	require.True(t, ok)
	assert.Contains(t, a.Detail, "this build reads no graphics card by process")
}

// The reading of the card says which provider its temperature came from,
// and only nvidia-smi names itself: that is what the list is chosen by.
func TestACardReadThroughNvidiaSMISaysSo(t *testing.T) {
	g := &GraphicsReader{SMI: func(context.Context) (string, error) { return "40, 31, Some Card\n", nil }}
	assert.Equal(t, smiName, g.Read(t.Context()).Via)

	native := &GraphicsReader{NativeName: "native", Native: func(context.Context) Graphics {
		return Graphics{Temperature: 50, HasTemperature: true, Load: 1, HasLoad: true}
	}}
	assert.Empty(t, native.Read(t.Context()).Via)
}
