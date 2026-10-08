package window_test

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32                   = syscall.NewLazyDLL("user32.dll")
	gdi32                    = syscall.NewLazyDLL("gdi32.dll")
	enumWindows              = user32.NewProc("EnumWindows")
	getWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	getWindowTextW           = user32.NewProc("GetWindowTextW")
	isWindowVisible          = user32.NewProc("IsWindowVisible")
	getWindowRect            = user32.NewProc("GetWindowRect")
	printWindow              = user32.NewProc("PrintWindow")
	getDC                    = user32.NewProc("GetDC")
	releaseDC                = user32.NewProc("ReleaseDC")
	createCompatibleDC       = gdi32.NewProc("CreateCompatibleDC")
	createCompatibleBitmap   = gdi32.NewProc("CreateCompatibleBitmap")
	selectObject             = gdi32.NewProc("SelectObject")
	deleteObject             = gdi32.NewProc("DeleteObject")
	deleteDC                 = gdi32.NewProc("DeleteDC")
	getDIBits                = gdi32.NewProc("GetDIBits")
	setDPIAwarenessContext   = user32.NewProc("SetProcessDpiAwarenessContext")
)

// The test reads the window in physical pixels, as the panel draws it. A
// process that has not said it is DPI-aware is handed rectangles scaled to
// 96 dpi, and a picture taken at that size is the panel cropped.
func init() {
	perMonitorAwareV2 := ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2, (HANDLE)-4
	_, _, _ = setDPIAwarenessContext.Call(perMonitorAwareV2)
}

// pwRenderFullContent asks PrintWindow for what DWM composes, which is the
// only way to get an OpenGL window's pixels rather than a black rectangle.
const pwRenderFullContent = 2

type rect struct{ Left, Top, Right, Bottom int32 }

// findWindow is the visible top-level window of process pid titled title.
func findWindow(pid int, title string) uintptr {
	var found uintptr
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var owner uint32
		_, _, _ = getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
		if int(owner) != pid {
			return 1
		}
		if v, _, _ := isWindowVisible.Call(hwnd); v == 0 {
			return 1
		}
		buf := make([]uint16, 256)
		n, _, _ := getWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if syscall.UTF16ToString(buf[:n]) == title {
			found = hwnd
			return 0
		}
		return 1
	})
	_, _, _ = enumWindows.Call(cb, 0)
	return found
}

// waitWindow waits up to d for the window to appear.
func waitWindow(pid int, title string, d time.Duration) (uintptr, error) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if hwnd := findWindow(pid, title); hwnd != 0 {
			return hwnd, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0, errors.New("the window did not appear")
}

// capture is the window's own pixels, as DWM composes them, alpha and all.
func capture(hwnd uintptr) (*image.NRGBA, error) {
	var r rect
	if ok, _, err := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return nil, err
	}
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	screen, _, _ := getDC.Call(0)
	defer func() { _, _, _ = releaseDC.Call(0, screen) }()
	mem, _, _ := createCompatibleDC.Call(screen)
	defer func() { _, _, _ = deleteDC.Call(mem) }()
	bmp, _, _ := createCompatibleBitmap.Call(screen, uintptr(w), uintptr(h))
	defer func() { _, _, _ = deleteObject.Call(bmp) }()
	old, _, _ := selectObject.Call(mem, bmp)
	ok, _, err := printWindow.Call(hwnd, mem, pwRenderFullContent)
	_, _, _ = selectObject.Call(mem, old)
	if ok == 0 {
		return nil, err
	}

	// BITMAPINFOHEADER for a top-down 32-bit DIB.
	header := struct {
		Size                         uint32
		Width, Height                int32
		Planes, BitCount             uint16
		Compression, SizeImage       uint32
		XPels, YPels, Used, Imported int32
	}{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	header.Size = uint32(unsafe.Sizeof(header))
	px := make([]byte, w*h*4)
	if n, _, err := getDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&px[0])),
		uintptr(unsafe.Pointer(&header)), 0); n == 0 {
		return nil, err
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Pix[i*4+0] = px[i*4+2]
		img.Pix[i*4+1] = px[i*4+1]
		img.Pix[i*4+2] = px[i*4+0]
		img.Pix[i*4+3] = 255
	}
	return img, nil
}

// save writes the picture, so a person can look at what was measured.
func save(img image.Image, path string) error {
	f, err := os.Create(path) //nolint:gosec // the test's own directory
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}

// luminance is a pixel's brightness, 0 to 255.
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 257
}
