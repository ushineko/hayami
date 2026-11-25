package client

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// LowerPriority puts this process below normal, so a load run on a desk in
// use takes the processors only when nothing else wants them (R32).
func LowerPriority() error {
	if err := windows.SetPriorityClass(windows.CurrentProcess(), windows.BELOW_NORMAL_PRIORITY_CLASS); err != nil {
		return fmt.Errorf("lowering the priority: %w", err)
	}
	return nil
}
