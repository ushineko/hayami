# hayami's threat model

This is what hayami defends, against whom, and how: the guide a change at
one of its boundaries is held to, as `docs/architecture.md` is for its
structure (spec 073). A defence named here points at the file that makes
it, so a reader can check it is still there. A finding not yet fixed is not
written here; it is a private advisory until the release that fixes it.

## Stance

- **The panel runs as the person at the desk and asks for nothing more.**
  It never runs elevated and never asks to. A reading that needs a right
  the panel lacks comes from a helper: the network helper, or on Windows
  the sensor helper for the processor's temperature (spec 103).
- **A privileged component does one thing, for one account, while it is
  read.** The network helpers count network traffic, and the sensor helper
  reads the processor's temperature, and each serves it to the account it
  was installed for; they read nothing a panel sends; they run only while a
  panel reads them.
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
| B1 Privileged helpers | Becoming Local Service, LocalSystem (the sensor helper, spec 103) or a process with kernel capabilities; other accounts' traffic | Assessed 2026-10-31; its finding fixed in 0.11.11 |
| B2 Installing, upgrading, removing | Code running as root, SYSTEM or an administrator | Assessed 2026-10-31; its findings fixed (the advisory in 0.11.12, #244 and #245 since) |
| B3 Credentials | The Claude and Codex tokens; the shared usage cache; the GitHub and GitLab keys (spec 102) | Assessed 2026-10-31; its findings fixed in 0.11.13 |
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
kernel network provider's events, and for the sensor helper LocalSystem and
PawnIO's driver; on Linux, `CAP_BPF`, `CAP_NET_ADMIN` and
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
- The sensor helper (spec 103) runs as LocalSystem because PawnIO's device
  opens for SYSTEM and administrators only; it is its own service so the
  network helper keeps Local Service. Its program and its two PawnIO modules
  are copies in Program Files; it loads a module only when the file's bytes
  equal the copy hayami-tui carries (`checkModules` in
  `internal/sensorhelper/service_windows.go`); it calls only the modules'
  read functions, on the thermal register and the two thermal MSRs
  (sanshoku's `cpu` package). Its pipe is one-way, opens for SYSTEM,
  administrators and the installed account, and refuses remote clients, as
  the network helper's (`Run`; `nethelper.Server`). The panel checks the
  process serving the pipe as it does the network helper's
  (`newSensorHelper` in `internal/core/sensorhelper_windows.go`).
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
  name. The sensor helper's lines are bounded the same way, and a reading
  outside -40 to 150 °C is not kept (`sensorStream` in
  `internal/core/sensorhelper.go`).

**Accepted.**

- The installed account sees every account's connections, their byte
  counts and which program holds them (Linux), or process IDs and byte
  counts (Windows). Installing the helper is agreeing to that, and the
  README says so.
- A program holding the Windows pipe's name can still keep the helper from
  starting while it holds it; the panel says which process that is.
- PawnIO is one maintainer's signed driver, and loads any module its key
  ever signed; the newest signed driver (2.2.0) predates hardening fixes
  made in August 2026, which matter to a caller already elevated. A review
  on 2026-11-20 found no published vulnerability and no malware or
  blocklist report. hayami does not install PawnIO; the person does.
- Any account can keep the processor's temperature from an AMD desk: the
  `Global\Access_PCI` mutex the readers share by convention is anybody's to
  create with a hostile access list, or to hold. The sensor helper waits two
  seconds, misses that reading, and stops after 30 misses in a row
  (`missLimit` in `internal/sensorhelper/service_windows.go`); nothing it
  shows can be changed this way. LibreHardwareMonitor takes the same mutex.
- The sensor helper runs all of `hayami-tui` as LocalSystem, as the network
  helper runs it as Local Service: every module linked into it, the Fyne
  fork's `init` included, runs with those rights.

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
- The VPN keepalive helper is a user service, written and removed as the user with no
  elevation, only when the person ticks the installer's page or runs `vpn-keepalive
  install` (`internal/keepalive/service.go`, spec 085). `keepalive` is a `once` parameter
  too; an upgrade restarts a helper already there and installs none. The unit has one
  argument from outside it, the program's path: it must be absolute with no control
  character, quote or backslash, so a path cannot add a line to the unit. A file at the
  unit's path that does not start with hayami's mark is neither replaced nor removed. The
  unit is written with a rename, so a link at the path is replaced and not followed, with
  mode `0600`.
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
usage cache, which other programs read and write; since spec 102, gh's
GitHub token and a GitLab key, from `~/.secrets.env`, glab's config or the
settings file.

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
- A refresh token the endpoint refused (401, 403, or 400 `invalid_grant`) is
  not sent again by the process; only its SHA-256 is kept, in memory. A
  refusal's error code reaches the panel only as printable ASCII of at most
  64 characters, and the rest of the body never does. A 429 from the token
  endpoint is waited out under the same one-hour cap (`refused`, `errorCode`,
  `internal/claude/refresh.go`; spec 089). A 403 is not remembered, since a
  proxy or a challenge page answers one too; a refusal of a token Claude
  Code spent meanwhile takes the store's new tokens (`refusal`).
- A refusal is recorded in the shared cache as `login_refused_at`, a time
  and nothing else, so every viewer says "needs a login" (`Cached`,
  `internal/usage/cached.go`; spec 089).

- The forges' keys (spec 102) are found only when a person has switched a
  forge on, and held for one reading (`internal/forge/creds.go`). gh and
  glab are asked with fixed arguments and a five-second timeout, and their
  standard error is dropped, since a tool that fails may print what it was
  given. `~/.secrets.env` is parsed, never run, and of its lines only
  `GITLAB_TOKEN`, `GLAB_TOKEN` and `GITLAB_HOST` are kept. A key found goes
  only to the host it was found with: the secrets file's only to its
  `GITLAB_HOST`, glab's only to the host glab keeps it for, and glab is run
  without the token variables it would otherwise answer for any host from
  (`glabEnv`). A host typed into Preferences is taken on Enter or Apply, never
  at each key, since a host half typed is another host. Every request goes
  through `forge.NewClient`: HTTPS only, no redirect followed, each response
  read to 4 MiB and refused past it, fifteen seconds a request. A key is in
  no error, reason or log line; a failure from either forge is reported
  without its URL or the proxy's, which name hosts, and GitHub's own error
  message is not carried (`hostless`, `internal/forge/gitlab.go`).

**Accepted.** A key typed into Preferences is kept in the settings file,
which is written `0600` (fynedesygn's `settings`) and is the same account's
as `~/.secrets.env`; nothing prints the configuration. The GitLab host is
a setting too, so a process of the same account that rewrites the settings
can send the GitLab key to a host of its choosing; it can already read
`~/.secrets.env` and glab's config, where the key is.

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
- The page files' answer from `NtQuerySystemInformation` is walked from its
  bytes, each entry and offset checked against the buffer, and a figure that
  does not add up is an absence (`pageFiles`, `internal/core/pagefile.go`;
  spec 094). `/proc/meminfo`'s cache and swap lines come through the parser
  that already read its memory lines.
- The processor table from `NtQuerySystemInformation` is walked from its
  bytes and checked whole (`ParseProcessorTimes`); `/proc/loadavg` and the
  cpufreq files are single numbers, and a core number in `/proc/stat` is
  bounded (`maxCores`), so no file sizes an allocation
  (`internal/core/processor.go`; spec 095).
- The GPU summary's nvidia-smi answer is parsed field by field, a field
  that is not a number being that part absent (`ParseSMICard`); an AMD
  card's files are single numbers and labels read by name under its device
  (`AMDCard`, `internal/core/card.go`; spec 096).
- The IO readers parse `/proc/<pid>/io`, `/proc/diskstats`, `/proc/vmstat`
  and `/proc/pressure/io` line by line, a field that is not a number being
  absent (`ParseProcIO`, `ParseDiskStats`, `ParseVMStatSwap`,
  `ParsePressure`, `internal/core/disks.go`; spec 097). A process's io is
  read only where the kernel's ptrace check lets this account, so hayami
  learns nothing of another's IO but what the disks' totals already say.
  Disk names are not drawn: the strip's cells are unlabelled.
- `/proc/uptime` is its first field, a number that is not finite and
  non-negative being an error rather than a zero (`ParseUptime`, fuzzed,
  `internal/core/uptime.go`; spec 099). Windows' is GetTickCount64, which
  takes no input.
- The VPN health store is bounded, 0600, read to a size limit and tolerates a cut
  line (`loadVPNPoints`, `internal/panel/vpnhealth.go`). `/proc/net/route` lines that
  do not parse are skipped (`ParseRouteTable`, fuzzed).
- VPN state comes from the system bus, whose names only the bus's own policy
  lets a service own. Each property is taken as its expected type or as blank
  (`variant`, `internal/core/vpn_linux.go`), a connection's name is free text and
  passes the printable filter on its way out. No poll starts a daemon that is not
  running, and no call asks for a secret. Listing what can be chosen may start
  OpenVPN3's configuration manager, when a person has opened the preferences (spec 085).
- vpn-toggle's settings are read once, on an offer a person answers: bounded,
  parsed as another program's data and fuzzed, never written (`ParseVPNToggle`,
  `internal/config/vpnimport.go`). They could aim a check at a host, which the person
  sees in the list before it runs, and they name no secret.

**Accepted.** LibreHardwareMonitor's address can be set to another URL in
the settings, which are the user's own; hayami then fetches that URL.

- Every parser of what hayami does not control is fuzzed, in CI on every PR
  and by hand before a release (`make fuzz`; spec 074): `/proc/net/dev`
  (`FuzzParseNetDev`), the helper's stream in both versions, with its
  bounds checked as a property (`FuzzHelperStream`), `sock_diag` replies
  (`FuzzParseInetDiag`), LibreHardwareMonitor's tree and values
  (`FuzzLHMTree`, `FuzzParseLHMValue`), the Claude and Codex payloads and
  breakdowns (`FuzzClaudePayload`, `FuzzCodexPayload`), the shared cache
  file (`FuzzCacheFile`), Codex's reply (`FuzzNormalise`), the page files'
  answer (`FuzzPageFiles`), the processor table (`FuzzParseProcessorTimes`), the card's answer
  (`FuzzParseSMICard`), the disks and a process's IO (`FuzzParseDiskStats`,
  `FuzzParseProcIO`, `FuzzParsePressure`), the
  load average (`FuzzParseLoadAvg`) and the settings file (`FuzzSettingsFile`).

- The forges' answers (spec 102) are read to `forge.MaxBody` and decoded into
  typed structs: GitHub's GraphQL answer by `encoding/json`, GitLab's events
  by client-go. A count is only ever added up and a date compared as text;
  titles, branch and repository names are the attacker's text (anyone can
  open an issue on a public repository) and reach a screen only through
  `safetext.Printable`, applied where the panel takes them and again to
  every part of a detail as it is drawn (`Detail.printable`). A reading keeps
  at most 10 pages, 5,000 items and 200 project lookups, whatever a host
  sends. Fuzzed: `FuzzGitHubResponse`, `FuzzGitLabEvent`, and
  the secrets file's lines, `FuzzParseLine` (`internal/forge/fuzz_test.go`).

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
  told its path (`writeScript`, `internal/desktop/script.go`). The scripts
  that stay loaded, the position watch and the hold scripts (an edge for the
  drawers, a corner while the panel is tucked, spec 091), act only on the
  window whose class and title are the panel's, with both quoted into the
  script, and only move or resize it. A panel unloads them when it quits
  only if no other process holds the panel's bus name `io.ushineko.hayami`
  (spec 100), so a process of the same account that took the name first
  keeps a quitting panel from tidying up: the scripts stay loaded and still
  act only on the panel's window. Such a process can already load and
  unload KWin scripts itself, so nothing is gained by it.
- VPN health checks send one ICMP echo and look up one name per check, at the
  interval chosen, only for a tunnel that is connected and only to a target in the
  settings (`internal/core/vpncheck_linux.go`, spec 085). A settings file aiming a
  check at a host makes this machine probe it, and nothing more; the settings are the
  user's own. Nothing is sent to a third party. No program is run.
- A press changes a VPN only through NetworkManager's and OpenVPN3's own calls on the
  system bus, so polkit and the session manager decide what is allowed, and a refusal is
  reported (`internal/core/vpn_linux.go`, spec 085). Disconnect and bounce take two
  presses. hayami registers no secret agent and sees no VPN secret. Control is in the
  desktop panel only.
- The keepalive helper acts unasked, and only as the user, through the same calls, so
  polkit decides for it too. It acts only on a VPN switched on for it in the settings,
  never on an OpenVPN3 session (which it only notifies on), and re-dials only a tunnel it
  has seen connected (`internal/keepalive/engine.go`, spec 085). The first failure to
  bring a tunnel back in an outage is notified on. The state file a press writes is in
  the user's own state directory, as are the settings. A connection NetworkManager
  refuses to show this account is another account's, whatever it is called, and is not
  read as this person's tunnel (`nmVPNs.started`, `internal/core/vpn_linux.go`). A
  notification's body is escaped for a server that reads it as markup (`notifyBody`).
- An OpenVPN3 sign-in address comes from the session manager and goes to the browser only
  if it is `https` with a host (`SignInURL`, `internal/core/vpn.go`). It is not logged,
  printed, drawn or carried in an error, since such an address can hold a one-time token.
- The Development drawer reads two network destinations, `api.github.com`
  and the GitLab host the settings or `~/.secrets.env` name, only while a
  drawer shows a forge a person switched on (they are off by default, #360),
  at most once per refresh interval or per click on the forge's heading
  (`forges`, `internal/panel/development.go`; spec 102). It runs `gh auth
  token` and `glab config get token` for a key, from the user's `PATH`, as
  the user, with constant arguments and a timeout, never through a shell.
- hayami runs one program to read: `nvidia-smi`, with a timeout
  (`internal/core/processors.go`, `internal/core/gpuprocs.go`,
  `internal/core/card.go`), found on the user's `PATH`, as the user, with
  constant arguments. The GPU drawer's summary runs it once more per drawer
  poll while the GPU list is shown (spec 096).

**Accepted.** A PID can be reused between the drill and the press; the
window between them is a person's reaction time, and the signal is the
user's own to send.

Whoever is on the network path can make the keepalive helper take a tunnel down: a
blocked transport fails a check, and the bounce's disconnect removes the routes that were
holding traffic in a black hole, so blocking the handshake then has it in the clear. Passing
handshakes and dropping the rest three times disables the VPN until a person re-enables it.
This is what vpn-toggle does and what the operator chose to keep. The person is told, the
switch and the README say so, and the helper is off unless asked for. A kill switch is
outside this program. The bound on it, and the alternative of not disconnecting until the
server answers, are in #305.

## B6 What hayami shows and prints

**Assets.** Host names and addresses of peers; process and account names;
usage figures; the machine's devices and interfaces.

**Attackers.** Anyone who sees the screen or a shared terminal, or reads a
bug report someone pasted `doctor` or `readings --json` into.

**Defences.**

- Network names (SSID, BSSID) are never read (`docs/architecture.md`,
  rule 9).
- A VPN's address, gateway and routes are read only for the drawer and `detail`.
  They are not on the card, in `readings` or in `doctor`, which is what gets pasted
  into a bug report (`DetailInfo.OnRequest`, spec 085).
- A process list's columns (a count, a share, a byte count; spec 092) are
  numbers hayami formats; the name is still the one process-supplied text
  on a line, and no process attribute beyond those of spec 060 is read.
  They pass through `safetext.Printable` with the rest of the line.
- A process's name is the kernel's, cut at fifteen characters, and on Linux
  a name at that length is completed from the command line: the file name of
  its first word, and only when it begins with the kernel's name
  (`core.FullProcName`). The arguments are never taken, including from a
  program that rewrote its command line as one string.
- `tools/screenshot.sh` runs the panel from a throwaway copy of the
  settings, so no path of the user's is shown.
- Tokens are never printed (B3).
- The Development drawer shows repository names without their owner or group,
  which is a user name, and the titles of a person's own pull requests,
  merge requests and issues; the GitLab host is never drawn. A day's page is
  a drill, not a listed detail, so `readings` and `doctor` never carry a
  repository name or a title; the overview's strips are counts (spec 102).
  A repository named for its owner (a profile, a Pages site) is shown as
  "profile" or "pages". `tools/screenshot.sh` leaves the GitLab host and
  the keys out of its copy of the settings.
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
`encoding/json` does: JSON is for programs.

- The bidirectional format characters (embeddings, overrides, isolates,
  marks) are taken out with the control characters (`unsafe`,
  `internal/safetext/safetext.go`; #361): a title someone else wrote could
  otherwise reorder its line in a terminal that does bidi.

**Not yet known.** The rest of `doctor` and the JSON outputs has not been
audited for identifiers other than the home path.

## B7 Supply chain and releases

**Assets.** The binaries and installers people run with administrator or
root rights. The two signed PawnIO modules hayami-tui carries for the
sensor helper (spec 103), which run in PawnIO's kernel driver.

**Attackers.** A compromised Go module, GitHub Action or maintainer
account.

**Defences.**

- `make vuln` (govulncheck) before every tagged release. It scans twice while
  Fyne is replaced by a fork: govulncheck asks about a replaced module by the
  fork's path and Fyne's advisories are under its own, so the second scan drops
  the `replace` (spec 084). That reads upstream's source at the fork's base, not
  the fork's own commits.
- The PawnIO modules are PawnIO.Modules 0.2.12's, committed unchanged with
  their licence, and pinned by SHA-256 (`TestTheModulesAreTheReleasesOwn` in
  `internal/sensorhelper/modules`). A change to either is a change of what
  runs in the kernel, made on purpose. PawnIO checks their signature before
  it loads them.
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
- `tools/bpf.sh check`, a CI job on every PR and on `main`, rebuilds the BPF
  object from its source and fails if it differs by a byte (spec 081). It builds
  in a Go image pinned by digest; LLVM and libbpf are pinned by hash, `bpf2go` by
  version, and `vmlinux.h` is committed.
- `make check-no-binaries` fails on any tracked compiled file, by type and by
  name (`.o`, `.a`, `.so`, `.syso`), with `netcount_bpfel.o` the one exception.
- OpenSSF Scorecard runs weekly and on `main`, and publishes its result.
- Repository settings: `sha_pinning_required` makes GitHub refuse an Action not
  pinned by SHA; immutable releases are on; a ruleset stops a `v*` tag moving or
  being deleted, with no bypass, and another lets only an administrator create
  one; the secrets and BPF checks are required on `main`; Dependabot alerts and
  security updates are on (specs 079, 080).

- Spec 102 added `github.com/google/go-github/v93` and
  `gitlab.com/gitlab-org/api/client-go`, which bring
  `github.com/google/go-querystring`, `github.com/hashicorp/go-cleanhttp`,
  `github.com/hashicorp/go-retryablehttp`, `golang.org/x/oauth2` and
  `golang.org/x/time`. They are in both binaries, including the
  privileged helper's copy of `hayami-tui`, as every module is; `make vuln`
  covers them, and no advisory was open against them when they were added.

**Not yet known.** The BPF comparison is about the object, not the process that
loads it: Go code in the helper can replace the bytes before loading them, and any
module linked into it runs with its capabilities. A payload for that need not look
like a binary: an XOR'd or base64 file passes `check-no-binaries`, and a recipe in
the `Makefile` can add `-overlay`. A PR can change the script, the job, the tests or
`generate.go` along with the object, so a change to `.github/`, `tools/`, the
`Makefile`, `generate.go` or those tests in such a PR is the thing to look for; there
is one maintainer and no second pair of eyes to require. A `vmlinux.h` can only hold
types, but its values are not checked: `IPPROTO_TCP = 200` builds and passes, and
gives wrong counts. A diff there from a kernel bump is the place to hide that.
Some inputs sit unpinned behind pinned Actions: Scorecard's container is a
mutable tag, the test jobs' `setup-go` reads its manifest from a branch, and
`gitleaks-action` downloads its binary with no checksum check found. Those run
apart from the release. The build jobs take Go from go.dev with no checksum,
because a hash in the repository would give up the newest patch on each run, so
a tampered Go download would be built with and signed. A pin's commit is not
checked to belong to the repository it names: a fork's commit resolves through
the parent's name, so a reviewer reading a changed pin is the check.
The PawnIO modules' pin is one test in the same repository: a PR can change
`internal/sensorhelper/modules/*.bin` and `modules_test.go` together, and no
job compares them with PawnIO.Modules' release. PawnIO's signature check
limits a swap to modules its author signed, older faulty ones included. Such
a PR is the thing to look for.
**Accepted.** The panel builds Fyne from a fork, `github.com/ushineko/fyne`,
through one `replace` to a tag (spec 084). `go.sum` and the checksum database
hold that tag's contents, so moving the tag fails the build, and new code needs
a new tag and a reviewed change to `go.mod`. The fork reaches more than the
panel: `hayami-tui` links its packages through `fynedesygn/theme`, so their
`init` runs in `sudo hayami-tui network-helper install` and in the helpers,
the sensor helper as LocalSystem (spec 103), as upstream Fyne's did. The fork is a further repository to protect: it has no
ruleset and no immutable releases, like fynedesygn and sanshoku, which the same
account publishes. Its base is an untagged `develop` commit, 479 past v2.8.2
and not reviewed here, and moving it by hand skips Dependabot's week. Nothing
proposes the fork's updates, since Dependabot does not follow a `replace`, so an
upstream fix reaches hayami when someone moves the fork. The `replace` goes at
the first tagged Fyne release that carries its fixes.

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
tag keep the suffix. Immutable releases cover only releases published since they
were turned on, on 2026-11-01: that token can still replace the files of an
earlier one. The tag rulesets stop a mistake or a lesser token, not this account:
it is the administrator who may create a tag and may edit a ruleset or turn a
setting off. Whether a token without the `workflow` scope can push a new commit
on an old base was not tested.

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
| 2026-11-01 | B7 | The adversarial review of #264 (spec 075): one reviewer given the code and the attackers, then a separate refutation pass. Read-only on GitHub; experiments on a desk and against published releases. | Nothing refuted outright. #268–#272, and the tag limit under B7's "Not yet known". Two private advisories, both fixed in 0.11.14: GHSA-g9rj-2f3c-8cv4 (the Go cache) and GHSA-j38w-g54m-r8mh (the release build's rights). |
| 2026-11-01 | B7 | The adversarial review of specs 079 and 080 before they were pushed: one reviewer, then a refutation pass. Read-only; local experiments. | A tag named `origin/main` passing the `main` check; artifacts taken by name; a re-run able to publish a partial draft; gaps in both structure tests and the binary check. All fixed before merge. Limits recorded under B7's "Not yet known". |
| 2026-11-03 | B7 | The adversarial review of spec 084 (the Fyne fork, #293): one reviewer given the change and the attackers, then a separate refutation pass. Read-only. | No way found to change what builds without a reviewed `go.mod` change. `make vuln` went blind to Fyne under the `replace`: fixed before merge with a second scan. The fork reaching the privileged paths and skipping the cooldown held, both accepted in the section above. |
| 2026-11-14 | B3 | The adversarial review of spec 089 (a dead login, #325): one reviewer given the code and the attackers, then a separate refutation pass. Read-only. | Five paths, none across a boundary. Fixed before merge: a future `fetched_at` clearing the remembered failure; a 403 remembered as a refusal; a token Claude Code spent meanwhile read as a dead login; an error code carrying a sentence. Accepted: a cache writer hiding the line, as it could already hide every figure. |
| 2026-11-03 | B3 | The adversarial review of spec 083 (the plan read from the usage reading, #288): one reviewer given the code and the attackers, then a separate refutation pass. Read-only. | No exploit path: a label and a badge are all it gives a writer of the cache, who already sets every number beside them. A caption collision with a cache-only account fixed before merge; the drawer asking for a stale caption filed as #289. |
| 2026-11-19 | B3, B4, B6, B7 | The adversarial review of spec 102 (the forges, #360): one reviewer given the change and the attackers, then a separate refutation pass. Read-only; overlay tests on a desk. | Nothing refuted. Fixed before merge: titles reaching the terminal drawer with escapes (parts were not cleaned); the secrets file's GitLab key lent to whichever host was found, and a host saved half typed; GitHub failures naming a proxy's host; a host able to fill memory; the host in screenshots; repositories named for their owner. #361 (bidi characters, older than the change) filed and fixed in the same PR. |
| 2026-11-20 | B1, B2, B7 | The adversarial review of spec 103 (the sensor helper, #365): one reviewer given the change and the attackers, then a separate refutation pass. Read-only. A separate review of PawnIO itself (published issues, signing, the site). | No path to LocalSystem or the kernel; no regression in the network helper. Fixed before merge: a held PCI mutex stopping the helper (now a missed reading); PawnIO's library able to load its own imports from the machine's PATH (now System32 only); the server's wake-up open allowing impersonation (now anonymous). Accepted above: the mutex convention, the whole program as LocalSystem, the modules' pin being one test. |
| 2026-11-03 | B3 | The adversarial review of spec 082 (the remembered plan, #286): one reviewer given the code and the attackers, then a separate refutation pass. Read-only. | Two paths, both refuted: neither gives an attacker anything the files hayami already reads did not. One part held as a bug without an attacker, a profile named like the plan sharing its caption; fixed before merge. |

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

### B3, spec 102, 2026-11-19

- A token through a redirect: `forge.NewClient` returns the last response
  rather than following; go-github attaches its token only to its own API
  origins; client-go's retryablehttp uses hayami's client, so the redirect
  rule holds there too.
- client-go retrying or logging a request: `WithoutRetries`, and its logger
  is nil, so no URL or host is written anywhere.
- Plain HTTP: the transport refuses any scheme but https, and a host typed
  with `http://` has it taken off.
- A token in a URL or an error: both clients send it in a header; `gh` and
  `glab` failures keep only their exit status.
- The secrets file's other secrets: only three names are kept, a line too
  long ends the read, and nothing of the file is echoed.
- An argument injected into `gh` or `glab`: fixed arguments, no shell; a
  host from glab's config is taken by `--host` as its value.
- Escapes through `detail` or JSON: `Text()` cleans each part, JSON escapes
  control characters. The strip's labels and the heading's state are the
  program's own words.
- A malformed answer crashing the panel: pointers are checked, and every
  answer is read to a bound.

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
  no `go.work` or proxy override. At this review there was no `replace`
  either; spec 084 added one, to a tagged module, which `go.sum` and the
  checksum database hold the same way.
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

- Saving tampered state to the Linux Go cache from `go test -race` or lint: the
  Fuzz job reserves the key first, while it finishes first. Release builds read
  no cache since 0.11.14 either way.
- A PR run poisoning `main` or a release through the cache: PR caches are scoped
  to `refs/pull/N/merge`.
- One tag's cache reaching another tag's release: GitHub's scope rules forbid it.
- Fork-forced eviction planting anything: it only forces a miss, and the re-save
  comes from whatever code runs next.

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

### B7, spec 081, 2026-11-02

- A tag changing only the BPF object: the release jobs need `bpf-rebuilt`, in the
  tag's own workflow.
- A `//go:generate` in another file, `//go:build ignore` on `generate.go`, `GOFLAGS`
  in the `Makefile`, or a no-op compiler: none passes the comparison.
- `.gitattributes` `text`, `eol` or `ident` on the object: each fails the comparison
  or the status check.
- A shadow `bpf/bpf/` header: `TestVmlinuxHoldsOnlyTypes` holds the directory.
- `BPF_TOOLS` in CI: nothing sets it; on a desk, what is in it is not checked again.
- The container's writable mount: no token, no socket, and no later step in the job.
- The image's multi-architecture digest: an index pins its children.

### B7, spec 084, 2026-11-03

- Moving the fork's tag, or a proxy serving other contents: the tag's hash is in
  `go.sum` and the checksum database, and `go mod verify` passes.
- A CI or Makefile override: no `GOFLAGS`, `GOPROXY` or `GONOSUM*` in either.
- The base not being upstream's: `b655332` is on Fyne's `develop`.
- The fork's four commits doing something else: no network, exec or file writes;
  the only new `unsafe` is `idOf`, for weak keys.

### B2 and B5, spec 085, 2026-11-05

Reviewed with the code and the attackers and not the author's reasoning; each path below
was then checked by a separate pass. Three survived and were fixed before any release
carried them: a failed bounce left traffic outside the tunnel with nothing said
(`TestAFailedRedialBacksOffAndIsVisible`, `TestEachOutageIsSaidOnce`), another account's
same-named VPN read as this person's tunnel
(`TestAnotherAccountsVPNOfTheSameNameIsNotThisPersonsTunnel`; the refusal is by
NetworkManager's documented rule and was not seen from a second real account), and a VPN's
name reached a notification as markup (`TestAVPNsNameCannotBeMarkupInANotification`).
Others were filed: #305 (on the network path), #306 (a hold outlasting a panel) and #307
(a FIFO at a path, same account only).

- Persisting a program another account can replace: the unit names the binary the person
  just ran, and another account's write to that was already a compromise. The exposure is
  that of any autostart entry.
- Adding a line to the unit through the program's path: `execArgument` refuses control
  characters, quotes and backslashes, and escapes a percent.
- A link at the unit's path, on install and on removal: a rename replaces it, `rm -f` removes
  the link, and a planted unit fails the mark.
- Another account reading or locking the state file or locks: `0700` directories and `0600`
  files. A custom `XDG_STATE_HOME` in a shared directory reopens this, and is the person's.
- Another account adding a system-wide profile of the same name for a connect to find: that
  takes an administrator under the default polkit rules.
- A private duplicate chosen by a connect: it is not listed to this account (`eachVPN`).
- Forging a ping or a DNS answer to pass a check: it only keeps a dead tunnel quiet. The
  reply needs the kernel's echo ID and the source address.
- A name in a path or a `systemctl` argument: names are map keys, and those arguments are
  constants.
- A PID or name race: the helper acts on no PID and re-reads bus paths per action.

### B3, spec 089, 2026-11-14

- A future `fetched_at` written into the cache clearing the remembered
  failure on every poll: a reading after `now` is not taken as newer
  (`settle`, `internal/panel/usage.go`).
- A spurious 403 or 401 giving up on a working refresh token for the life of
  the process: a 403 is not remembered; a 401 or `invalid_grant` needs the
  fixed HTTPS token endpoint to say so, and a new refresh token in the store
  is always asked about (`refusal`, `internal/claude/refresh.go`).
- A sentence or a link in a refusal's error code: only `[a-z_]`, 64 at most
  (`printable`).
- `refusal` adopting the store's tokens: they are the store's own, read as
  every fetch reads them, sent only to the fixed usage endpoint, and nothing
  is written.
- The refused set growing: 32 bytes per refused token, one round trip each.
- A cache writer hiding or forcing "needs a login": accepted. It already sets
  every figure and the gate; `login_refused_at` is a time and nothing else,
  and forcing it shows a line telling the person to sign in.

### B3, spec 083, 2026-11-03

- A label selecting a token, store or cache file for writing: fetching is keyed
  on the account's name (`refresh`, `loadClaude`, `internal/panel/usage.go`).
  A caption does choose which cache file the drawer reads, read-only and with
  no token, and a writer of those files already controls them.
- A forged reading or a hostile API response naming the plan: it flips a label
  and a badge between max and ultra, and sets every number beside them already.
- A hostile payload: a wrong type or malformed JSON reads as no sign of a plan
  (`ClaudeWeekOnly`, `internal/usage/payload.go`), and `encoding/json` reports
  excess depth as an error.
- A huge file or a FIFO at the cache path: `usage.Read` is unbounded, and
  `gather` already read the same file; this adds reads, not an exposure.
- A store's name reaching a path: `sanitise` removes separators and dots
  (`internal/usage/path.go`).
- A token reaching the label: `ObservedPlan` is only ever "ultra", and nothing
  new logs, prints or marshals the credentials.

### B3, spec 082, 2026-11-03

Spec 083 removed `claude-plans.json`; these stand as what was checked of it.

- A token in `claude-plans.json`: it holds a store's plan, subscription type
  and tier (`seenPlan`, `internal/panel/plans.go`); the credential store is
  never written from it.
- A planted plan choosing a token, a store or a cache file: all are keyed by
  the store's name, which the plan does not change. It changes a label and a
  badge, as `planDisplayName` in `~/.claude.json` already could, and passing
  the check needs the store's tier, read from the file that holds the tokens.
- A FIFO or `/dev/zero` at the path: the same poll already reads the
  credential stores, `~/.claude.json` and the usage cache unbounded; any of
  them stalls it first.
- A link at the path redirecting the write: `WriteAtomic` renames a new file
  over it (`internal/readings/readings.go`).
- Another account: the file is created `0600` inside the user's cache
  directory.
