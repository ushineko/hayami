/*
Command appicon draws hayami's icon, internal/gui/assets/hayami.svg, as a PNG.

The Windows executable carries its icon as a resource, and the tool that makes
the resource (go-winres, scripts/winres.ps1) reads a PNG, not an SVG. The PNG is
drawn from the SVG at build time rather than committed, so the window's icon
and the executable's are the same drawing and no image sits in the repository
to drift from it (spec 053). The SVG is drawn with the rasteriser Fyne itself
draws SVGs with.

	go run ./tools/appicon -size 256 -out hayami.png
*/
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"

	"github.com/ushineko/hayami/internal/gui/assets"
)

func main() {
	size := flag.Int("size", 256, "width and height of the PNG, in pixels")
	out := flag.String("out", "hayami.png", "the PNG to write")
	flag.Parse()
	if err := draw(*size, *out); err != nil {
		fmt.Fprintln(os.Stderr, "appicon:", err)
		os.Exit(1)
	}
}

// draw renders the icon at size by size and writes it to path.
func draw(size int, path string) error {
	if size < 16 || size > 1024 {
		return fmt.Errorf("size %d: want 16 to 1024", size)
	}
	icon, err := oksvg.ReadIconStream(bytes.NewReader(assets.IconSVG()))
	if err != nil {
		return fmt.Errorf("reading the icon: %w", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	icon.SetTarget(0, 0, float64(size), float64(size))
	icon.Draw(rasterx.NewDasher(size, size, rasterx.NewScannerGV(size, size, img, img.Bounds())), 1)

	f, err := os.Create(path) //nolint:gosec // the path is the build's own output
	if err != nil {
		return fmt.Errorf("writing the icon: %w", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing the icon: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("writing the icon: %w", err)
	}
	return nil
}
