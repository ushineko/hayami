//go:build !windows

package core

import "context"

// nativeGraphics is the platform's own report of the card. Off Windows there
// is none: the kernel's hwmon and busy file are read by GraphicsReader itself.
func nativeGraphics() func(context.Context) Graphics { return nil }
