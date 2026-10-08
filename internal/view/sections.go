package view

/*
SectionInfo is what a section is called and drawn with: the key the settings
and the command line name it by, the title a card carries and the glyph
before it.

These are the one list of the sections this build has (spec 038). Before it a
section's key was written out in seven places that had to agree -- the default
settings, the panel's switch and its list of keys, the source's Key, the
view's Section literal, an icon constant and the window's icon switch -- and
nothing but reading all seven said whether they did. The view holds the list
because it is the package the settings, the panel and both shells already
import; panel adds what builds each one (panel.Specs), and a test there holds
the two to each other.
*/
type SectionInfo struct {
	Key   string
	Title string
	Icon  IconName

	// Default is whether a settings file that names no sections draws this
	// one. Order is the list's.
	Default bool
}

// The sections, by name, for the builder and the source that each belongs
// to.
var (
	BandwidthInfo   = SectionInfo{Key: "bandwidth", Title: "Bandwidth", Icon: IconBandwidth, Default: true}
	UsageInfo       = SectionInfo{Key: "usage", Title: "Usage", Icon: IconUsage, Default: true}
	CoolerInfo      = SectionInfo{Key: "cooler", Title: "Cooler", Icon: IconCooler, Default: true}
	PeripheralsInfo = SectionInfo{Key: "peripherals", Title: "Peripherals", Icon: IconPeripherals, Default: true}
)

// Sections is every section this build has, in the order a new settings file
// draws them.
func Sections() []SectionInfo {
	return []SectionInfo{BandwidthInfo, UsageInfo, CoolerInfo, PeripheralsInfo}
}

// DefaultSections are the keys a settings file that names none draws, in
// order.
func DefaultSections() []string {
	var out []string
	for _, s := range Sections() {
		if s.Default {
			out = append(out, s.Key)
		}
	}
	return out
}

// SectionByKey is the section a key names, and whether this build has one.
func SectionByKey(key string) (SectionInfo, bool) {
	for _, s := range Sections() {
		if s.Key == key {
			return s, true
		}
	}
	return SectionInfo{}, false
}

// section is an empty section carrying the info's key, title and icon: where
// every builder starts.
func (i SectionInfo) section() Section {
	return Section{Key: i.Key, Title: i.Title, Icon: i.Icon}
}
