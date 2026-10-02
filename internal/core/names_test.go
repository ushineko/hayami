package core_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// write puts body at path under dir, making the directories it needs.
func write(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

// Spec 031, R2.1. The processor's model is the first "model name" in
// /proc/cpuinfo, as the kernel prints it on the desk this was written on.
func TestTheProcessorIsNamedFromCPUInfo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cpuinfo")
	write(t, path, "processor\t: 0\nvendor_id\t: GenuineIntel\n"+
		"model name\t: Intel(R) Core(TM) i9-14900K\nflags\t\t: fpu\n\n"+
		"processor\t: 1\nmodel name\t: Intel(R) Core(TM) i9-14900K\n")

	assert.Equal(t, "Intel(R) Core(TM) i9-14900K", core.CPUName(path))
}

// R2.6. No file, or no such field (an ARM machine), is no name.
func TestAProcessorWithoutAModelNameIsUnnamed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cpuinfo")
	write(t, path, "processor\t: 0\nBogoMIPS\t: 48.00\n")

	assert.Empty(t, core.CPUName(path))
	assert.Empty(t, core.CPUName(filepath.Join(dir, "absent")))
}

// pciIDs is a cut of the public database: two vendors, the devices the tests
// ask about, a subsystem line that must not be taken for a device, and the
// class list that ends the file.
const pciIDs = `# comment
10de  NVIDIA Corporation
	2684  AD102 [GeForce RTX 4090]
	2216  GA102 [GeForce RTX 3080 Lite Hash Rate]
1002  Advanced Micro Devices, Inc. [AMD/ATI]
	73bf  Navi 21 [Radeon RX 6800/6800 XT / 6900 XT]
		1002 0e3a  Radeon RX 6900 XT
	744c  Navi 31 [Radeon RX 7900 XT/7900 XTX/7900 GRE/7900M]
1022  Advanced Micro Devices, Inc. [AMD]
	744c  Not the card
C 00  Unclassified device
`

// R2.2. A card is named from the PCI ID database by its vendor and device.
func TestACardIsNamedFromThePCIDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pci.ids")
	write(t, path, pciIDs)

	assert.Equal(t, "Navi 31 [Radeon RX 7900 XT/7900 XTX/7900 GRE/7900M]", core.PCIName(path, 0x1002, 0x744c))
	assert.Equal(t, "Navi 21 [Radeon RX 6800/6800 XT / 6900 XT]", core.PCIName(path, 0x1002, 0x73bf),
		"a subsystem line is not the device")
	assert.Equal(t, "AD102 [GeForce RTX 4090]", core.PCIName(path, 0x10de, 0x2684))
	assert.Empty(t, core.PCIName(path, 0x1002, 0x1234), "a device the vendor does not list")
	assert.Empty(t, core.PCIName(path, 0xabcd, 0x744c), "a vendor the database does not list")
	assert.Empty(t, core.PCIName(filepath.Join(t.TempDir(), "absent"), 0x1002, 0x744c),
		"no database is no name, and the label stays GPU")
}

// R2.2. An AMD card found through hwmon is named from the database, by the
// PCI function behind the chip, and the lookup is made once.
func TestAnAMDCardIsNamedThroughItsChip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a sysfs PCI address has colons, which a Windows filename cannot")
	}
	dir := t.TempDir()
	root, busy := amdgpu(t, dir)
	device := filepath.Join(dir, "pci", "0000:03:00.0")
	write(t, filepath.Join(device, "vendor"), "0x1002\n")
	write(t, filepath.Join(device, "device"), "0x744c\n")
	require.NoError(t, os.Symlink(device, filepath.Join(root, "hwmon3", "device")))
	ids := filepath.Join(dir, "pci.ids")
	write(t, ids, pciIDs)

	g := core.GraphicsReader{Root: root, Busy: busy, PCIIDs: ids}
	assert.Equal(t, "Navi 31 [Radeon RX 7900 XT/7900 XTX/7900 GRE/7900M]", g.Read(t.Context()).Name)

	require.NoError(t, os.Remove(ids))
	assert.Equal(t, "Navi 31 [Radeon RX 7900 XT/7900 XTX/7900 GRE/7900M]", g.Read(t.Context()).Name,
		"the database is read once, not every poll")
}

// R2.2. A card on NVIDIA's driver is named by nvidia-smi, in the answer the
// panel already asks for.
func TestAnNvidiaCardIsNamedByNvidiaSMI(t *testing.T) {
	dir := t.TempDir()
	g := core.GraphicsReader{
		Root: filepath.Join(dir, "hwmon"), Busy: filepath.Join(dir, "none", "gpu_busy_percent"),
		SMI: func(context.Context) (string, error) { return "40, 31, NVIDIA GeForce RTX 4090\n", nil },
	}

	assert.Equal(t, core.Graphics{Temperature: 40, HasTemperature: true, Load: 31, HasLoad: true,
		Name: "NVIDIA GeForce RTX 4090"}, g.Read(t.Context()))
}

// An answer without the third column, or one that does not know, is no name.
func TestAnSMIAnswerWithoutANameIsUnnamed(t *testing.T) {
	for _, out := range []string{"", "40, 31", "40, 31, [N/A]", "No devices were found"} {
		assert.Empty(t, core.SMIName(out), "%q was read as a name", out)
	}
	assert.Equal(t, "Some Card, Rev. 2", core.SMIName("40, 31, Some Card, Rev. 2\n"),
		"a name with a comma in it is kept whole")
}
