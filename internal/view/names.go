package view

import (
	"regexp"
	"strings"
)

/*
LabelWidth is the widest a hardware name may be in a label, in characters
(spec 031).

Fifteen, because it is the widest of the short names the two desks this panel
runs on produce -- "Kraken Elite V2" -- and it holds every common one whole:
"i9-14900K", "i7-13700K", "Ryzen 9 7950X", "RTX 4090", "RX 7900 XTX". A name
longer than this is cut with an ellipsis and its full form is the row's tip.

**Fixed, not measured.** A column measured from its contents moves when the
contents change, and a name can arrive a poll after the "CPU" it replaces. A
row that may carry a name keeps this width in the label column whatever it
says (Row.LabelWidth), so the values beside it never move.
*/
const LabelWidth = 15

// nameRule is one step of shortening a hardware name, and why it is there.
type nameRule struct {
	pattern *regexp.Regexp
	with    string
}

/*
nameRules shorten a part's name to what identifies it (spec 031, R2.4).

A name comes from three places -- /proc/cpuinfo, nvidia-smi and the PCI ID
database -- and each pads it its own way: trademark marks, a clock speed the
processor does not run at, a core count, the vendor said twice. What is left is
the model, which is what a reader can tell two machines apart by.

In order, because the order matters: the PCI database's bracket is taken first,
since everything outside it is the chip's code name ("AD102") and not the
product; the marks go before the vendor words, so "Core(TM)" is a word by the
time the vendor words are matched.

Each is tested against a real name in names_test.go.
*/
var nameRules = []nameRule{
	// "AD102 [GeForce RTX 4090]": the PCI ID database's form, chip then
	// product. The product is in the bracket.
	{regexp.MustCompile(`^[^\[]*\[([^\]]+)\].*$`), "$1"},
	// "Intel(R) Core(TM)", "AMD Ryzen™": the trademark marks.
	{regexp.MustCompile(`(?i)\((r|tm)\)|®|™`), ""},
	// "CPU @ 3.20GHz": the base clock on older Intel names.
	{regexp.MustCompile(`(?i)\s*\bCPU\b\s*@.*$`), ""},
	{regexp.MustCompile(`(?i)\s*@\s*[\d.]+\s*GHz\b`), ""},
	// "16-Core Processor", "Eight-Core Processor": the core count AMD appends.
	{regexp.MustCompile(`(?i)\s*\b[\w]+-Core Processor\b`), ""},
	// "with Radeon Graphics": the integrated graphics on an AMD APU's name.
	{regexp.MustCompile(`(?i)\s*\bwith Radeon\b.*$`), ""},
	// "13th Gen Intel": the generation Intel puts before its name, already in
	// the model number that follows it.
	{regexp.MustCompile(`(?i)^\s*\d+(st|nd|rd|th) Gen\b`), ""},
	// "Lite Hash Rate": a variant of the same card, a fact about mining.
	{regexp.MustCompile(`(?i)\s*\bLite Hash Rate\b`), ""},
	// The vendor and family words: the model number says which vendor, and
	// "NVIDIA GeForce RTX 4090" is "RTX 4090" with eleven characters of
	// brand in front of it.
	{regexp.MustCompile(`(?i)\b(Intel|Core|AMD|NVIDIA|GeForce|Radeon|NZXT|Processor|Corporation)\b`), ""},
}

// spaces is a run of whitespace, which the rules leave behind them.
var spaces = regexp.MustCompile(`\s+`)

// ShortName is a part's name with the boilerplate taken off: "Intel(R)
// Core(TM) i9-14900K" is "i9-14900K", "NVIDIA GeForce RTX 4090" is "RTX 4090".
// A name the rules would empty is returned whole, trimmed: a long name is
// better than no name.
func ShortName(full string) string {
	s := full
	for _, r := range nameRules {
		s = r.pattern.ReplaceAllString(s, r.with)
	}
	s = strings.TrimSpace(spaces.ReplaceAllString(s, " "))
	if s == "" {
		return strings.TrimSpace(full)
	}
	return s
}

// NameLabel is the label a row named for its hardware draws: the short name,
// cut with an ellipsis at LabelWidth. An unread name keeps the role's own
// label -- "CPU", "GPU", "Coolant" (R2.6).
func NameLabel(role, full string) string {
	if strings.TrimSpace(full) == "" {
		return role
	}
	return truncate(ShortName(full), LabelWidth)
}

// nameTip is what the row says on hover: the role and the name in full, so the
// reader who sees "Kraken Elite V2" can still find the word "Coolant".
func nameTip(role, full string) string {
	full = strings.TrimSpace(full)
	if full == "" {
		return ""
	}
	return role + ": " + full
}

// named labels a row for the hardware it reads and keeps the label column at
// LabelWidth, so the name arriving moves nothing.
func named(r Row, role, full string) Row {
	r.Label = NameLabel(role, full)
	r.Tip = nameTip(role, full)
	r.LabelWidth = LabelWidth
	return r
}
