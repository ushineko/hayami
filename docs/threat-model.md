# hayami's threat model

This is what hayami defends, against whom, and how: the guide a change at
one of its boundaries is held to, as `docs/architecture.md` is for its
structure (spec 073). A defence named here points at the file that makes
it, so a reader can check it is still there. A finding not yet fixed is not
written here; it is a private advisory until the release that fixes it.

## Stance

- **The panel runs as the person at the desk and asks for nothing more.**
  It never runs elevated and never asks to. A reading that needs a right
  the panel lacks comes from the network helper, and from nowhere else.
- **A privileged component does one thing, for one account, while it is
  read.** The helpers count network traffic and serve it to the account
  they were installed for; they read nothing a panel sends; they run only
  while a panel reads them.
- **hayami never becomes root on Linux.** A step that needs root is a line
  the person runs with sudo, printed and never executed for them.
- **Credentials are the tools' own.** hayami reads the Claude and Codex
  tokens from their stores and never copies, logs or prints them.
- **What hayami shows about other accounts is said.** The network by
  process for every account is the helper's, installed by choice, with
  what it exposes stated where it is offered.

## Boundaries

| Boundary | What is at stake | Status |
|---|---|---|
| B1 Privileged helpers | Becoming Local Service or a process with kernel capabilities; other accounts' traffic | Assessed 2026-10-31; its finding fixed in 0.11.11 |
| B2 Installing, upgrading, removing | Code running as root, SYSTEM or an administrator | Assessed 2026-10-31; its findings fixed (the advisory in 0.11.12, #244 and #245 since) |
| B3 Credentials | The Claude and Codex tokens; the shared usage cache | Assessed 2026-10-31; its findings fixed in 0.11.13 |
| B4 What hayami parses | The panel's availability; the helper's input | Fuzzed (spec 074); sanshoku's decoders open |
| B5 What hayami does outside itself | Ending the wrong process; code run in the compositor | Modelled |
| B6 What hayami shows and prints | Names, addresses and figures reaching someone they should not | Modelled |
| B7 Supply chain and releases | A tampered binary installed with administrator or root rights | Pinned Actions, provenance, secret scanning (spec 075) |

**Accepted.** The opt-in LibreHardwareMonitor install runs `winget` with no
version pinned (`scripts/install_windows.ps1`, `installer/sensors.go`), and
that program runs as administrator and loads a kernel driver. Winget checks an
installer against its manifest's hash by its own documentation, not verified
here. A pin would hold people on a version after its fixes, so hayami does not
pin it (#269).

---

## B1 Privileged helpers

**Assets.** The rights the helpers hold: on Windows, Local Service and the
kernel network provider's events; on Linux, `CAP_BPF`, `CAP_NET_ADMIN` and
`CAP_PERFMON`, and programs attached to every socket on the machine and to
the forward hook. The traffic of every account. The panel's availability,
which reads what the helper sends.

**Attackers.** Another local account. Malware running as the installed
user, after more than that user has. A helper that sends something hostile,
from the panel's side.

**Assumptions.**

- Only an administrator or root can write `%ProgramFiles%`, `/usr/lib`,
  `/etc/systemd/system` and `/run`.
- The kernel vouches for the peer of a Unix socket (`SO_PEERCRED`) and for
  the account a named pipe's access list names.
- systemd hands the socket only to the service it starts.

**Defences.**

- The program the service runs is a copy the user cannot write:
  `copyProgram` into `/usr/lib/hayami` (`internal/nethelper/service_linux.go`);
  the copy into Program Files (`internal/nethelper/service_windows.go`).
- One account may read: on Linux the socket is that account's, mode `0600`
  (`SocketUnit`), and each peer's credentials are checked again
  (`accept`, `peerUID` in `internal/nethelper/server_linux.go`); on Windows
  the pipe's access list names the account (`Run` in
  `internal/nethelper/service_windows.go`) and remote clients are refused
  (`PIPE_REJECT_REMOTE_CLIENTS` in `server_windows.go`).
- One way: neither server reads from a reader (`serve` in both server
  files).
- Least rights on Linux: a dynamic user with three capabilities and the
  hardening in `ServiceUnit` (`systemd-analyze security`: 1.9, spec 072).
- Counting only: every BPF program returns "pass" and the netfilter one
  `NF_ACCEPT` (`internal/netcount/bpf/netcount.bpf.c`); nothing is pinned,
  so a helper that dies leaves nothing attached (`Counter.Close`,
  `internal/netcount/netcount_linux.go`).
- Runs only while read, and lets the programs go when the last reader
  leaves (`serve`; `TestTheServerCountsOnlyWhileSomebodyReads`).
- The Windows panel reads the pipe only when the process serving it is the
  one the service manager started for `hayami-network`, and opens it
  anonymously, so a program that took the pipe's name while the helper
  was stopped is refused and cannot impersonate the panel
  (`serverIsTheService`, `openPipe` in `internal/core/nethelper_windows.go`;
  `TestAPipeHeldByAnotherProgramIsRefused`,
  `TestTheServerCannotImpersonateThePanel`). GHSA-94c3-mf55-vv8v, fixed in
  0.11.11.
- The panel bounds what it reads: a line is at most 16 MiB, a stream keeps
  only its own version's fields, each table is capped at the helper's own
  size between readings, long strings are dropped, and the helper's error
  text loses its control characters (`helperStream`, `printable` in
  `internal/core/nethelper.go`). A protocol it does not know is refused by
  name.

**Accepted.**

- The installed account sees every account's connections, their byte
  counts and which program holds them (Linux), or process IDs and byte
  counts (Windows). Installing the helper is agreeing to that, and the
  README says so.
- A program holding the Windows pipe's name can still keep the helper from
  starting while it holds it; the panel says which process that is.

**Open findings.** None of its own since 0.11.11 (GHSA-94c3-mf55-vv8v,
published). B2's #244 covers how the Windows helper's program is copied
into place. The Linux helper held against every path tried; see "Checked".

## B2 Installing, upgrading and removing

**Assets.** The administrator, SYSTEM or root rights an install step runs
with, and the files it leaves that later run with them.

**Attackers.** A local user or malware running as the installing user,
planting files, links or directories before an elevated step, or racing
it. Whoever sets the environment or working directory of that step.

**Assumptions.** A person who runs the sudo line or accepts the
administrator prompt means to install hayami's helper from the binary they
name.

**Defences.**

- Linux installs per user into `~/.local`. It asks for root only for the
  network helper, only when the person ticks the Network page's box or runs
  `network-helper install`, through polkit's prompt, and only through
  root's setuid `pkexec` (`trustedPkexec`, `RunElevated` in
  `internal/nethelper/service_linux.go`; spec 078). Without `pkexec` the
  install log says the sudo line. The udev rule is printed, never asked for
  (`install.sh`, `installer/rule_linux.go`).
- An upgrade does not carry the installer's actions: `sensors` and
  `network` are fynstall `once` parameters, so a choice made at one install
  is never made again, and no administrator asked for, unless the person
  makes it again (`fynstall.yaml`; #259, fynstall spec 009).
- The Linux helper is removed by its own root-owned copy in
  `/usr/lib/hayami`, not by elevating the user's `hayami-tui`
  (`nethelper.RemoveFromCopy`, `removeNetwork` in `installer/network.go`).
- The helper's copy is written as `.new` in a root-owned directory and
  renamed into place (`copyProgram`, `internal/nethelper/service_linux.go`).
- On Windows the helper's program goes into Program Files as the known
  folder names it, not as `%ProgramFiles%` says, and is copied from a handle
  the install opens first, which no other process may rename or replace
  while it is held (`Dir`, `openSelf`, `internal/nethelper/service_windows.go`;
  #244).
- The elevated PowerShell step carries `lhm_settings.ps1`'s text in its
  command rather than running the file, which is in a directory the user can
  write (`Install-Sensors`, `scripts/install_windows.ps1`; #244).
- The udev rule grants the logged-in user (`uaccess`) the devices hayami
  reads, matched by vendor on the device itself and never on a parent, so
  nothing plugged into one of those vendors' hubs or docks is opened up
  (`packaging/60-sanshoku.rules`, sanshoku 0.1.18; #245).
- An uninstall removes the udev rule only when it is the copy the install
  wrote (`uninstall.sh`, `installer/rule_linux.go`).
- The LibreHardwareMonitor startup task, which runs with the user's full
  administrator token at every logon, runs a copy in Program Files that
  only an administrator can change, not winget's copy under
  `%LOCALAPPDATA%`; the folder is the known folder, not `%ProgramFiles%`
  from the environment (`Install-Sensors`, `scripts/install_windows.ps1`;
  `TestTheStartupTaskRunsACopyOnlyAnAdministratorCanChange`).
  GHSA-32hh-7v39-jrjg, fixed in 0.11.12.

**Accepted.** Installing the Linux helper elevates the installed
`hayami-tui`, which the user can write, through `pkexec` or `sudo`; code
already running as the user could replace it before the prompt. polkit's
prompt names the program, and the person who approves it is trusted to
mean it.

**Open findings.** None from the first assessment: #244 and #245 are fixed.
Its advisory is fixed in 0.11.12 (GHSA-32hh-7v39-jrjg). The Linux install
paths held in the first assessment, and the polkit route in its own review
(spec 078); see "Checked".

## B3 Credentials

**Assets.** The Claude and Codex OAuth tokens and refresh tokens; the
usage cache, which other programs read and write.

**Attackers.** Malware running as the user; another program writing the
cache; another account; anyone who sees hayami's output; a network attacker
or a hostile API response.

**Assumptions.** The tools' own stores protect the tokens at rest; the
providers' APIs are reached over TLS.

**Defences.**

- Responses are bounded: `io.LimitReader` on the usage and refresh
  responses (`internal/claude/fetch.go`, `internal/claude/refresh.go`) and
  a ten-second timeout.
- One upstream read per freshness window, under a non-blocking `flock`
  (`internal/usage/lock_unix.go`), shared with the Python tools.
- The HTTP client follows no redirect, so none can carry a token elsewhere
  (`refuseRedirects`, `internal/panel/http.go`; spec 076).
- A server's Retry-After is honoured for an hour at most, and a gate another
  program wrote into the shared cache holds for at most an hour past the
  file's writing (`MaxRetryAfter`, `internal/claude/fetch.go`; `bound`,
  `internal/usage/entry.go`; spec 076).
- A refresh reads Claude Code's credential file again just before writing
  it, leaves it alone if Claude Code refreshed meanwhile, and keeps every key
  written meanwhile (`writeBack`, `internal/claude/refresh.go`; spec 077).

**Accepted.** A write by Claude Code between hayami's second read and its
rename can still be lost: milliseconds, where it was a network round trip.
Closing it would mean taking Claude Code's undocumented lock.

**Open findings.** None from the first assessment: #246–#250 and #248 were
fixed in 0.11.13 (specs 076, 077), as was #256 (the token endpoint had
moved, and every refresh failed). No path gave another account anything or
put a token into output; see "Checked".

## B4 What hayami parses

**Assets.** The panel's availability; for the helper protocol, the
boundary between a privileged process and the panel.

**Attackers.** Whoever controls a source: a program answering on
LibreHardwareMonitor's port, a process writing the usage cache, a device
sending reports, a helper sending lines, a provider's API.

**Defences.**

- LibreHardwareMonitor's tree is read with a size bound and a timeout
  (`lhmMaxBody`, `LHMTimeout` in `internal/core/lhm.go`), and only ever with
  GET: the same server takes requests that set fan speeds.
- `/proc/net/dev` lines that do not parse are skipped (`ParseNetDev`,
  `internal/core/netdev.go`); `sock_diag` replies are length-checked before
  each field (`parseInetDiag`, `internal/core/netprocs_linux.go`).
- The helper's lines are bounded (B1).

**Accepted.** LibreHardwareMonitor's address can be set to another URL in
the settings, which are the user's own; hayami then fetches that URL.

- Every parser of what hayami does not control is fuzzed, in CI on every PR
  and by hand before a release (`make fuzz`; spec 074): `/proc/net/dev`
  (`FuzzParseNetDev`), the helper's stream in both versions, with its
  bounds checked as a property (`FuzzHelperStream`), `sock_diag` replies
  (`FuzzParseInetDiag`), LibreHardwareMonitor's tree and values
  (`FuzzLHMTree`, `FuzzParseLHMValue`), the Claude and Codex payloads and
  breakdowns (`FuzzClaudePayload`, `FuzzCodexPayload`), the shared cache
  file (`FuzzCacheFile`), Codex's reply (`FuzzNormalise`) and the settings
  file (`FuzzSettingsFile`).

**Not yet known.** The HID decoders are sanshoku's and are not fuzzed
(ushineko/sanshoku#67).

## B5 What hayami does outside itself

**Assets.** Other processes; the compositor; the device nodes the udev
rule opens.

**Attackers.** A process that can make a name, a PID or a path mean
something else between hayami reading it and acting on it.

**Defences.**

- Ending processes signals PIDs the person drilled into, TERM before KILL,
  behind two presses, and counts what was refused rather than claiming
  (`endAll`, `internal/gui/kill_unix.go`; `internal/gui/kill_windows.go`).
  The kernel refuses another account's processes.
- The KWin script is written to a new file with mode `0600` before KWin is
  told its path (`writeScript`, `internal/desktop/script.go`).
- hayami runs one program to read: `nvidia-smi`, with a timeout
  (`internal/core/processors.go`, `internal/core/gpuprocs.go`), found on the
  user's `PATH`, as the user.

**Accepted.** A PID can be reused between the drill and the press; the
window between them is a person's reaction time, and the signal is the
user's own to send.

## B6 What hayami shows and prints

**Assets.** Host names and addresses of peers; process and account names;
usage figures; the machine's devices and interfaces.

**Attackers.** Anyone who sees the screen or a shared terminal, or reads a
bug report someone pasted `doctor` or `readings --json` into.

**Defences.**

- Network names (SSID, BSSID) are never read (`docs/architecture.md`,
  rule 9).
- `tools/screenshot.sh` runs the panel from a throwaway copy of the
  settings, so no path of the user's is shown.
- Tokens are never printed (B3).
- What the pane, `doctor` and `detail` write has its control characters
  taken out, in the one place every section passes on its way out, before
  the pane adds colours of its own (`safetext.Printable`;
  `view.Painter.paint`, `view.Detail.Text`, `line` in
  `internal/cli/doctor.go`; spec 076).
- An error's text in `doctor` and the readings JSON writes the user's home
  as `~`, matched only as a whole path (`safetext.WithoutHome`; spec 076).

**Accepted.** The panel shows what it reads, to whoever sees the screen.
Peer host names come from reverse DNS and are names on the network the
desk is on.

**Accepted.** JSON output passes C1 characters as UTF-8, as
`encoding/json` does: JSON is for programs. Bidirectional characters are
not stripped: they can reorder a pasted report, not drive a terminal.

**Not yet known.** The rest of `doctor` and the JSON outputs has not been
audited for identifiers other than the home path.

## B7 Supply chain and releases

**Assets.** The binaries and installers people run with administrator or
root rights.

**Attackers.** A compromised Go module, GitHub Action or maintainer
account.

**Defences.**

- `make vuln` (govulncheck) before every tagged release.
- SHA256SUMS on every release.
- Build provenance on every file a release publishes
  (`actions/attest-build-provenance`, spec 075). The README's `gh attestation
  verify` names the repository, `build.yml` and the tag, so it says this
  repository's release workflow built the file from that tag. It does not
  check the file's name: a genuine file of another version under an
  installer's name fails only on the tag.
- Every Action is pinned by full commit SHA, or digest for `docker://`, and
  `tests/structure/actions_pinned_test.go` parses the workflows and fails one
  that is not, in the required tests. Dependabot proposes the next commit, and
  the Go modules, weekly, a week after a version appears. The linter's
  checksums file is pinned in the `Makefile`.
- Each job declares its own `permissions` and the workflows' default is
  none. Only the release job writes contents, and only it may mint an
  OIDC token and write attestations; Scorecard writes security events.
- What a release publishes is built apart from what publishes it (spec
  079). `build-linux` and `build-windows` run no tests, read no cache, keep
  no token and can write nothing. The `release` job runs no code from the
  repository or its modules. `tests/structure/release_workflow_test.go`
  holds the workflow to this.
- A release without a suffix must be a commit on `main`; the `release` job
  checks. This catches a tag pushed from the wrong place, not a stolen token
  (see "Not yet known").
- gitleaks scans every PR's commits and `main`; its allow-list
  (`.gitleaks.toml`) is by pattern and path, never by turning a rule off.
- The committed BPF object is tied to its source by
  `TestTheObjectWasBuiltFromThisSource` (`internal/netcount/netcount_test.go`);
  the PR template asks about a changed object, and CI warns on a PR that
  changes the object without its source.
- `make check-no-binaries` fails on any tracked compiled file, by type and by
  name (`.o`, `.a`, `.so`, `.syso`), with `netcount_bpfel.o` the one exception.
- OpenSSF Scorecard runs weekly and on `main`, and publishes its result.

**Not yet known.** The BPF object's checks catch a mistake, not intent: the
test hashes the source only, and an object built from other source passes with
a comment changed in the `.c`. Only a reviewer running `make bpf` compares it.
A rebuild does reproduce the committed object byte for byte with `bpf2go`,
clang 23 and this kernel's `vmlinux.h`; CI has neither, and a comparison there
needs a digest-pinned clang and a committed `vmlinux.h`.
Some inputs sit unpinned behind pinned Actions: Scorecard's container is a
mutable tag, the test jobs' `setup-go` reads its manifest from a branch, and
`gitleaks-action` downloads its binary with no checksum check found. Those run
apart from the release. The build jobs take Go from go.dev with no checksum,
because a hash in the repository would give up the newest patch on each run, so
a tampered Go download would be built with and signed. A pin's commit is not
checked to belong to the repository it names: a fork's commit resolves through
the parent's name, so a reviewer reading a changed pin is the check.
**Not yet known.** Builds are not reproducible, so provenance says which
workflow built a file, not that the same source always builds the same
file. Provenance proves where a file came from, not that its code is sound.
The attestation names the `release` job, which signs what the build jobs
hand it: a module linked into the panel still ships its code, and is signed
with it. The release takes each artifact by the ID its job reported, so
another job cannot swap one; a build job can still tamper with its own.
A maintainer account that can push a tag can still publish a release: no
rule limits which commit a tag names, and the tag's own workflow runs, so a
tag on a commit from before spec 079 runs the workflow as it was then, without
these separations, and signs as the same `build.yml`. An admin's token can also
put a commit directly on `main`, since `enforce_admins` is off (it has to be:
one maintainer cannot review their own PR). A tag with a suffix skips the `main`
check and can be promoted afterwards with `gh release edit`; its file names and
tag keep the suffix. With immutable releases off, that token can also replace a
published release's files. Immutable releases and a tag ruleset narrow this; a
ruleset needs a bypass for whoever tags, which is the same account. Whether a
token without the `workflow` scope can push a new commit on an old base was not
tested.

---

## Keeping it true

### The gate

A spec that touches a boundary above carries a section "Threat model
delta": which boundary, what changed for the attacker, and the section of
this document updated in the same PR. "No boundary touched" is an answer
and is given. The PR template asks it.

### Adversarial review

For a PR that touches B1, B2, B3 or B7:

1. **A reviewer is given the changed code and the boundary's attacker** from
   this document, and not the author's reasoning, the spec's Risks section
   or this document's Defences. The review is not to be anchored on what
   the author believes is defended.
2. **The reviewer is asked for exploit paths**: preconditions, steps and
   what the attacker gains, each step with the line of code it depends on.
   A worry without a path is not a finding. The reviewer also lists what
   was tried and found not to work.
3. **A second, separate pass tries to refute each path** against the code.
   A path that survives is a finding; one refuted is added to "Checked"
   below with what stops it, so the next review does not try it again.
4. **A finding is filed, labelled `security`.** One an attacker can use
   across a boundary (another account, an elevation, a privilege the
   helper holds) is a private advisory until the release that fixes it;
   the rest are public issues. Once fixed, each gets a regression test
   that fails when the fix is broken.

This runs alongside Ralph's per-change security pass, which stays.

### Red-team exercises

Each release cycle that ships a change to B1, B2 or B7, and at least once a
quarter otherwise: one boundary, a fresh checkout, a virtual machine, and
the goal of a working proof of concept.

| Date | Boundary | Tried | Found |
|---|---|---|---|
| 2026-10-31 | B1, B2, B3 | The first assessment (spec 073 R7): one reviewer per boundary given the code and the attacker, then a separate refutation pass for each. Read-only, no proof of concept on a machine. | Two private advisories (Windows), the B1 one fixed in 0.11.11 (GHSA-94c3-mf55-vv8v) and the B2 one in 0.11.12 (GHSA-32hh-7v39-jrjg); #244–#250. The Linux helper and install paths held. |
| 2026-10-31 | B1 | Tests on Windows 11 (build 26200) of the fix for GHSA-94c3-mf55-vv8v: a squatted pipe, and a server impersonating the panel. | The assessment's impersonation claim did not hold: Windows lets a pipe server impersonate only after reading from the pipe, and the panel never writes. Recorded under "Checked". |
| 2026-11-01 | B3, B6 | Review of specs 076 and 077 (one reviewer, findings checked against the code). | Poll errors bypassing the home redaction; substring matches in it; the shared cache bypassing the Retry-After cap; a failed re-read losing refreshed tokens; and, from its question about redirects, the token endpoint having moved (#256). All fixed before 0.11.13. |
| 2026-11-01 | B2 | Review of spec 078, the Linux installer's polkit route (one reviewer). | No path to root beyond what `sudo` on the same file gave. The uninstaller elevating the user's `hayami-tui` instead of the helper's own copy, and `pkexec` taken from the PATH as found; both fixed before 0.11.13. Installing 0.11.13 then found #259. |
| 2026-11-01 | B7 | The adversarial review of #264 (spec 075): one reviewer given the code and the attackers, then a separate refutation pass. Read-only on GitHub; experiments on a desk and against published releases. | Nothing refuted outright. #268–#272, and the tag limit under B7's "Not yet known". Further findings are held private until fixed. |
| 2026-11-01 | B7 | The adversarial review of specs 079 and 080 before they were pushed: one reviewer, then a refutation pass. Read-only; local experiments. | A tag named `origin/main` passing the `main` check; artifacts taken by name; a re-run able to publish a partial draft; gaps in both structure tests and the binary check. All fixed before merge. Limits recorded under B7's "Not yet known". |

## Checked

Paths that were tried against the code and found not to work, with what
stops each. A review starts by reading these.

### B1, 2026-10-31

- Another account connecting to the Linux helper: the socket is the
  installed account's, mode `0600` (`SocketUnit`), and `SO_PEERCRED` admits
  only that account or root even if the mode is loosened (`accept`,
  `peerUID`, `internal/nethelper/server_linux.go`).
- Squatting the Linux socket path: it is in `/run`, which only root writes.
- The hand-run listener's chown race: between listen and chmod the socket
  is root's and `0755`, which nobody else can connect to, and the peer
  check still applies (`listen`, `internal/nethelper/service_linux.go`).
- Spoofed `LISTEN_PID`/`LISTEN_FDS`: the PID must match this process
  (`activated`), sudo resets the environment, and a handed-in listener
  still goes through the peer check.
- Feeding either helper input: the Linux server never reads a connection;
  the Windows pipe is outbound only, refuses remote clients, and gives the
  user read only (`server_windows.go`, `Run` in `service_windows.go`).
- Making the Linux service run attacker code: a fixed root-owned
  `ExecStart`, root-written units, a numeric account (no name lookup), and
  the BPF object embedded in the binary.
- The BPF programs changing packets: every program returns pass or
  `NF_ACCEPT`, reads with `bpf_skb_load_bytes` only, and is attached with
  multi-attach links that do not displace systemd's own filters.
- Starting or stopping the Windows service as the user: start and query
  only (`service_windows.go`).

- Impersonating the panel from the Windows pipe: Windows allows it only
  after the server has read from the pipe, and the panel never writes; it
  also opens anonymously since 0.11.11 (`TestTheServerCannotImpersonateThePanel`,
  with a control client that does give a token).

### B2, 2026-10-31

- The Linux helper's binary or units in a user-writable place: `sysRoot`
  is `/`, and `/usr/lib/hayami` and `/etc/systemd/system` are root's.
- Swapping `~/.local/bin/hayami-tui` after `sudo ... install` on Linux:
  it works, but the person runs sudo on a file they can write, so code as
  that user gains nothing it could not by swapping before.
- Environment influence on the sudo step: `SUDO_UID` is read only as root;
  `systemctl` is found on sudo's `secure_path`.
- The installer's udev write: only when `/etc/udev/rules.d` is already
  writable; `udevadm` runs unprivileged.
- Uninstall deleting more than intended: fixed paths, and the udev rule
  only on a byte-for-byte match (`installer/rule_linux.go`, `uninstall.sh`).
- Reconfiguring the Windows service: the user's access is query, start,
  interrogate and read.
- An unquoted service path, or a DLL planted beside the Go program: paths
  are quoted, and Go loads system DLLs from system32 only.
- Raw block access through the udev rule: `SUBSYSTEM=="usb"` never matches
  a block device.

### B3, 2026-10-31

- A token in error text: errors carry status codes; URLs carry no token.
- Redirecting the API addresses: `UsageURL` and `TokenURL` are constants;
  no environment or setting changes them.
- TLS: the default transport and system roots; no `InsecureSkipVerify`.
- Response size: capped at 1 MiB (`internal/claude/fetch.go`,
  `refresh.go`); Codex lines likewise (`internal/codex/codex.go`).
- File modes: the cache's temporary file is `0600` and renamed into place
  (`internal/usage/entry.go`); the credential file is written `0600` in a
  `0700` directory (`internal/claude/refresh.go`).
- Another account planting files or links: the cache, lock and credential
  directories are all inside the user's home.
- Path traversal through account names: names are sanitised before a path
  is built (`internal/usage/path.go`), and symlinked profile directories
  are skipped (`internal/claude/store.go`).
- A poisoned cache used as a URL or path: the payload is decoded into
  numbers and labels only, and an unreadable cache is no cache.
- Corrupting the credential store with a bad refresh reply: a store that
  does not parse is never written, and an empty token is refused.

### B7, 2026-11-01

- Secrets or write access from a fork or Dependabot PR: `pull_request` only
  with a read-only token; neither workflow uses `pull_request_target` or
  `workflow_run`.
- A PR's artifact reaching the release: `download-artifact` has no `run-id`,
  so it reads the same run only.
- Script injection through a tag or PR value: values reach the shell through
  `env` or `with:`, never expanded inside `run:`.
- `workflow_dispatch` on a branch reaching the release job: its condition
  needs `refs/tags/v`.
- A pin's comment lying about its version: all nine matched their claimed
  tag at this review.
- Swapping a module on the proxy: `go.sum` and the checksum database, with
  no `replace`, `go.work` or proxy override.
- A file changed after attestation: `gh attestation verify` exits 1.
- The gitleaks allow-list's `invented[-_a-z]*` hiding a real leak: gitleaks
  tests it against the secret found, not the line, so a random token
  containing it is negligible.
- The Windows upload placing Linux filenames over release files: its
  patterns match only `*.exe`; it would take a forged artifact.
- `run` steps in test jobs using the runtime token: it is not in their
  environment (runner `ScriptHandler.cs`); reaching it would mean scraping
  runner memory, which was not tried.
- An unreferenced `.syso` running in `hayami-tui`: built without cgo, the
  internal linker drops it (#271 has the desktop panel's case).
- Renaming the BPF object and replacing its content entirely: git reports a
  delete and an add, so the warning fires (#271 has the forms that do not).

### B7, specs 079 and 080, 2026-11-01

- A branch named `origin/main` shadowing the `main` check: it lands at
  `refs/remotes/origin/origin/main`. A tag does shadow it, so the ref is spelled
  out.
- A test job pre-creating an artifact's name: the builder's upload fails, which
  is a denial of service and no more.
- `permissions: write-all` or `read-all` hiding from the release test: a decode
  error, and the test fails.
- The committed BPF object differing from its source: rebuilt byte-identical.
- Notes dropped by `--notes-file` with `--generate-notes`: GitHub's API puts the
  body before the generated notes.
- `file` missing on the runner: it is in the image's toolset, and the check now
  fails without it.
