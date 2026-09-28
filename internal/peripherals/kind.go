package peripherals

/*
Kind is what sort of device a battery belongs to.

It is read because the panel orders its cells by it, and the panel orders by it
because a desk has one mouse and several things that are sometimes there: the
mouse is the reading a glance is usually after, and a cell that changes place
when a headset connects is a cell that has to be found again. Ordering by name
put "Arctis Nova Pro Wireless" first on the machine this was written on, which
is the device the reader thinks about least.

It is deliberately coarse. The panel needs to tell a mouse from a keyboard from
a headset, and nothing else about the device matters to where its cell goes; a
taxonomy with a case per protocol would have to be kept in step with three
readers that each name things differently.
*/
type Kind int

const (
	// KindOther is the zero value and means "not one of the below", which
	// covers a device that did not say as well as one that said something
	// this program has no case for. The two are not distinguished because
	// nothing would be done differently about them.
	KindOther Kind = iota
	// KindMouse is a mouse, and also a trackball or a touchpad: what they
	// have in common is being the pointing device on the desk, which is the
	// property the ordering is about.
	KindMouse
	// KindKeyboard is a keyboard, and also a numpad.
	KindKeyboard
	// KindHeadset is a headset or a pair of headphones or earbuds.
	KindHeadset
)

// String names a kind for a test's failure message and for the JSON the
// command line prints.
func (k Kind) String() string {
	switch k {
	case KindMouse:
		return "mouse"
	case KindKeyboard:
		return "keyboard"
	case KindHeadset:
		return "headset"
	default:
		return "other"
	}
}
