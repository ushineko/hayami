//go:build !windows

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/ushineko/sanshoku/hwmon"
)

// nativeGraphics is the platform's own report of the card. Off Windows there
// is none: the kernel's hwmon and busy file are read by GraphicsReader itself.
func nativeGraphics() func(context.Context) Graphics { return nil }

// GPUSensorDetail is every route GraphicsReader took to the card's
// temperature, for the reason given when none answers: hwmon's GPU sensors,
// then nvidia-smi.
func GPUSensorDetail() string {
	names := make([]string, 0, len(hwmon.GPU)+1)
	for _, s := range hwmon.GPU {
		names = append(names, s.String())
	}
	return fmt.Sprintf("looked under %s and tried %s", hwmon.Root, strings.Join(append(names, "nvidia-smi"), ", "))
}
