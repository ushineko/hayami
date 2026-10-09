package structure_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	shell32                = syscall.NewLazyDLL("shell32.dll")
	extractIconEx          = shell32.NewProc("ExtractIconExW")
	version                = syscall.NewLazyDLL("version.dll")
	getFileVersionInfoSize = version.NewProc("GetFileVersionInfoSizeW")
	getFileVersionInfo     = version.NewProc("GetFileVersionInfoW")
	verQueryValue          = version.NewProc("VerQueryValueW")
)

/*
The panel's executable carries its icon and its version (spec 053).

Without an icon resource, the Start menu entry and the shortcuts the installer
writes are blank, and a file's Properties name no version. scripts/winres.ps1
makes the resources, as the installer and CI do before they build; the panel
is built from them here and Windows is asked, through the same calls Explorer
uses, for its icons and its version strings.
*/
func TestThePanelsExecutableCarriesItsIconAndVersion(t *testing.T) {
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("no Windows PowerShell")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no gcc: the panel links OpenGL through cgo")
	}
	root := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "VERSION"))
	require.NoError(t, err)
	want := strings.TrimSpace(string(body))

	gen := exec.CommandContext(t.Context(), powershell, "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(root, "scripts", "winres.ps1"))
	out, err := gen.CombinedOutput()
	require.NoError(t, err, "%s", out)

	exe := filepath.Join(t.TempDir(), "hayami.exe")
	build := exec.CommandContext(t.Context(), "go", "build", "-tags", "migrated_fynedo",
		"-ldflags", "-H windowsgui", "-o", exe, "./cmd/hayami")
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=1")
	out, err = build.CombinedOutput()
	require.NoError(t, err, "%s", out)

	assert.Positive(t, iconCount(t, exe), "the executable has no icon")
	assert.Equal(t, want, versionString(t, exe, "ProductVersion"), "the product version is not VERSION's")
	assert.Equal(t, "A glance panel", versionString(t, exe, "FileDescription"))
	assert.Equal(t, "hayami", versionString(t, exe, "ProductName"))
}

// iconCount is how many icons the file holds, as Explorer would find them.
func iconCount(t *testing.T, path string) int {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	require.NoError(t, err)
	n, _, _ := extractIconEx.Call(uintptr(unsafe.Pointer(p)), ^uintptr(0), 0, 0, 0) // index -1: the count
	return int(n)
}

// versionString is one string of the file's version resource, in its first
// language, or empty when it has none.
func versionString(t *testing.T, path, name string) string {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	require.NoError(t, err)
	size, _, _ := getFileVersionInfoSize.Call(uintptr(unsafe.Pointer(p)), 0)
	if size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if ok, _, _ := getFileVersionInfo.Call(uintptr(unsafe.Pointer(p)), 0, size, uintptr(unsafe.Pointer(&buf[0]))); ok == 0 {
		return ""
	}
	query := func(q string) (unsafe.Pointer, uint32) {
		qp, _ := syscall.UTF16PtrFromString(q)
		var ptr unsafe.Pointer
		var n uint32
		if ok, _, _ := verQueryValue.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(qp)),
			uintptr(unsafe.Pointer(&ptr)), uintptr(unsafe.Pointer(&n))); ok == 0 || n == 0 {
			return nil, 0
		}
		return ptr, n
	}
	tr, n := query(`\VarFileInfo\Translation`)
	if n < 4 {
		return ""
	}
	lang, cp := *(*uint16)(tr), *(*uint16)(unsafe.Add(tr, 2))
	v, n := query(`\StringFileInfo\` + hex4(lang) + hex4(cp) + `\` + name)
	if v == nil {
		return ""
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(v), n))
}

func hex4(v uint16) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[v>>12&0xF], digits[v>>8&0xF], digits[v>>4&0xF], digits[v&0xF]})
}
