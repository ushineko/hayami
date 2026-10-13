package desktop

import "context"

// DrawerAsksCompositor is whether the panel's drawer needs the compositor
// for the room beside the window, a left edge and the keyboard. False here:
// on Windows the window reads its own place, moves itself and hands the
// foreground back, all in the design system.
const DrawerAsksCompositor = false

// Drawer is the compositor's half of the panel's drawer, which on Windows is
// nothing: every method reports that it did not act.
type Drawer struct{}

// NewDrawer builds the helper. The app ID and the title are Linux's matching
// keys and unused here.
func NewDrawer(_, _ string) *Drawer { return &Drawer{} }

// Asks reports whether the drawer needs this helper on this system
// (DrawerAsksCompositor).
func (*Drawer) Asks() bool { return DrawerAsksCompositor }

// Room reports nothing: the design system reads the room itself.
func (*Drawer) Room(context.Context) (left, right float32, ok bool) { return 0, 0, false }

// HoldEdge holds nothing: the window moves itself.
func (*Drawer) HoldEdge(context.Context, bool) bool { return false }

// Yield does nothing: the design system hands the foreground back.
func (*Drawer) Yield(context.Context) {}

// Stop has nothing to unload.
func (*Drawer) Stop() {}
