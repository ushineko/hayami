# Security

hayami is a desktop panel, and parts of it reach further than a panel
usually does: helpers that run with rights the panel does not have, a
service that answers other programs, installers that ask for an
administrator, the credentials it reads usage and forge activity with, and
what it shows about the machine and other accounts. What it defends, against
whom, and how is in [docs/threat-model.md](docs/threat-model.md); every way
it runs, and as which account, is in [ARCHITECTURE.md](ARCHITECTURE.md).

## Reporting a vulnerability

Report it privately, through GitHub: the repository's **Security** tab,
**Report a vulnerability**. Please do not open a public issue for it.

Say what you found, how to reproduce it, and on which version and system.
A proof of concept helps; one that only shows a crash is still worth
sending.

You will hear back within a week. A confirmed finding is fixed in a
release, with a regression test, and credited in the changelog unless you
ask otherwise. Until the release is out, the finding stays in the private
advisory.

## In scope

The boundaries in the threat model:

- **The privileged helpers** (B1): the network helper, `hayami-network`
  (Linux, with `CAP_BPF`, `CAP_NET_ADMIN`, `CAP_PERFMON`; Windows, as Local
  Service), and the sensor helper, `hayami-sensors` (Windows, as LocalSystem,
  with the PawnIO modules hayami carries), with their sockets, pipes and
  protocol.
- **Installing, upgrading and removing** (B2): the installers on Linux and
  Windows, the polkit and administrator steps they take, `install.sh` and
  `install_windows.ps1`, and the services they register: the helpers, the
  VPN keepalive, and the service other programs read (`hayami-api`, as a
  systemd user unit, a Windows per-user service or a macOS launchd agent).
- **Credentials** (B3): the Claude and Codex tokens, their refresh and the
  shared usage cache; the GitHub and GitLab keys read from `gh`, `glab`,
  `~/.secrets.env` or the preferences.
- **What hayami parses** (B4): device and sensor data, the helpers' lines,
  LibreHardwareMonitor's `data.json`, the usage providers' and forges'
  answers, the settings file, and requests to the service.
- **What hayami does outside itself** (B5): ending processes, the KWin rule
  and scripts, the udev rule, autostart entries, and connecting, bouncing or
  disconnecting a VPN, by hand or through the keepalive.
- **What hayami shows and prints** (B6): the panels, `doctor`, `readings`,
  `detail`, and what the service serves.
- **The supply chain and releases** (B7): the release workflow, its
  artifacts and their provenance.

## Out of scope

- Programs hayami reads from or installs on request but does not ship:
  PawnIO's driver, LibreHardwareMonitor, `gh`, `glab`, Claude Code, Codex,
  NetworkManager and OpenVPN3. Report those to their own projects; how
  hayami uses them is in scope.
- The usage providers' and forges' APIs.
- Anything that needs an administrator or root on the machine already.
- What a person chooses to show: a screenshot or a shared screen of the
  panel shows what the panel shows.

## Supported versions

The latest release. Fixes are not backported.
