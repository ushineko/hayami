/*
Package packaging carries the udev rule into a Go program.

The Linux installer fynstall builds (spec 063) puts the rule where udev
reads it, or says how, from this same file: there is one copy of the rule,
and the test that holds it to sanshoku's (tests/structure) holds what the
installer writes.
*/
package packaging

import _ "embed"

// UdevRuleName is the rule's file name, in the repository and in
// /etc/udev/rules.d.
const UdevRuleName = "60-sanshoku.rules"

// UdevRule is the rule that lets the logged-in user open the devices hayami
// reads.
//
//go:embed 60-sanshoku.rules
var UdevRule []byte
