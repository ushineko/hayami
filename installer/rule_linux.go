package installer

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ushineko/fynstall/extension"

	"github.com/ushineko/hayami/packaging"
)

// Where udev reads its rules, and where the install keeps a copy of the
// device rule for a person to install: beside the programs, when the
// installer could not put it in place itself.
const (
	udevRules = "/etc/udev/rules.d"
	ruleCopy  = "share/" + packaging.UdevRuleName
)

// UdevDirEnv names a directory to use in place of /etc/udev/rules.d. It is
// install.sh's variable and exists for the same reason: the tests point it
// at a directory of their own.
const UdevDirEnv = "HAYAMI_UDEV_DIR"

func udevDir() string {
	if dir := os.Getenv(UdevDirEnv); dir != "" {
		return dir
	}
	return udevRules
}

/*
placeRule puts the device rule where udev reads it, when it may, and says
how when it may not: what install.sh does, in the installer (spec 063).

The rule belongs to root's directory and this is a per-user install, so on
most desks it is not the installer's to write. It never asks to become root
for it: the panel works without the rule, saying which devices it was not
permitted to open, and the two commands are one paste. A rule that is there
already and says the same is left as it is, whatever its comments.
*/
func placeRule(ctx context.Context, c *extension.Context) {
	if c.Root() == "" {
		return
	}
	dest := filepath.Join(udevDir(), packaging.UdevRuleName)
	if have, err := os.ReadFile(dest); err == nil && rulesOf(have) == rulesOf(packaging.UdevRule) { //nolint:gosec // udev's directory
		return
	}
	if writeRule(dest, packaging.UdevRule) == nil {
		reloadUdev(ctx)
		c.Logf(extension.Info, "Installed the device rule to %s", dest)
		c.Log(extension.Info, "Replug the receivers, docks and cooler hayami reads, or reboot, for it to apply.")
		return
	}
	// A file for the command to name: the rule is inside this program and
	// nowhere on the disk until it is written here.
	src := filepath.Join(c.Root(), filepath.FromSlash(ruleCopy))
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil { //nolint:gosec // beside the programs, which anyone may read
		c.Logf(extension.Warn, "The device rule was not written out: %v", err)
		return
	}
	if err := writeRule(src, packaging.UdevRule); err != nil {
		c.Logf(extension.Warn, "The device rule was not written out: %v", err)
		return
	}
	for _, line := range []string{
		"hayami reads the devices on your desk as you, and needs a udev rule to open them.",
		"Without it they are found and reported as not permitted. Install it with:",
		"  sudo install -m644 " + src + " " + dest,
		"  sudo udevadm control --reload",
		"then replug the receivers, docks and cooler, or reboot.",
	} {
		c.Log(extension.Warn, line)
	}
}

/*
dropCopy removes the copy of the rule placeRule wrote into the install, and
its directory when that leaves it empty. fynstall did not write the file and
does not remove it, and an uninstall that leaves a file behind leaves the
install directory behind. An upgrade's install writes it again if it is
still wanted.
*/
func dropCopy(c *extension.Context) {
	if c.Root() == "" {
		return
	}
	src := filepath.Join(c.Root(), filepath.FromSlash(ruleCopy))
	_ = os.Remove(src)
	_ = os.Remove(filepath.Dir(src))
}

/*
removeRule takes the device rule away with an uninstall, when it is the copy
this installer carries. A file of the same name with other contents came from
somewhere else, sanshoku's own packaging or another program that reads the
same devices, and is not the uninstaller's to remove.
*/
func removeRule(ctx context.Context, c *extension.Context) {
	dest := filepath.Join(udevDir(), packaging.UdevRuleName)
	have, err := os.ReadFile(dest) //nolint:gosec // udev's directory
	if err != nil {
		return
	}
	if !bytes.Equal(have, packaging.UdevRule) {
		c.Logf(extension.Info, "Left %s alone: it is not the copy this installer wrote", dest)
		return
	}
	if os.Remove(dest) == nil {
		reloadUdev(ctx)
		c.Logf(extension.Info, "Removed the device rule %s", dest)
		return
	}
	for _, line := range []string{
		"The device rule is root's to remove:",
		"  sudo rm " + dest,
		"  sudo udevadm control --reload",
	} {
		c.Log(extension.Info, line)
	}
}

// rulesOf is a udev file's rules without its comments and blank lines, so a
// copy of the rule with another header is the same rule.
func rulesOf(file []byte) string {
	var rules []string
	for _, line := range strings.Split(string(file), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			rules = append(rules, line)
		}
	}
	return strings.Join(rules, "\n")
}

// writeRule writes the rule whole or not at all: udev reads the directory
// when it likes, and half a rule is a rule.
func writeRule(dest string, rule []byte) error {
	tmp := dest + ".new"
	if err := os.WriteFile(tmp, rule, 0o644); err != nil { //nolint:gosec // udev reads it as any user
		return err //nolint:wrapcheck // only whether it failed is used
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err //nolint:wrapcheck // only whether it failed is used
	}
	return nil
}

// reloadUdev tells udev its rules changed. Where it cannot be told, the
// rules are read at the next boot.
func reloadUdev(ctx context.Context) {
	udevadm, err := exec.LookPath("udevadm")
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, udevadm, "control", "--reload").Run() //nolint:gosec // a fixed command
}
