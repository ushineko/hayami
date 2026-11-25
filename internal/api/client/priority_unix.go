//go:build unix

package client

import (
	"fmt"
	"syscall"
)

// LowerPriority makes this process nice 19, so a load run on a desk in use
// takes the processors only when nothing else wants them (R32).
func LowerPriority() error {
	if err := syscall.Setpriority(syscall.PRIO_PROCESS, 0, 19); err != nil {
		return fmt.Errorf("lowering the priority: %w", err)
	}
	return nil
}
