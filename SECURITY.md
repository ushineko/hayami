# Security

hayami is a desktop panel, and parts of it hold more than a panel's usual
reach: a network helper that runs with rights the panel does not have (a
Windows service; a Linux service with kernel capabilities), installers that
run as an administrator or under sudo, the OAuth tokens it reads usage
with, and what it shows about the machine and other accounts. What it
defends, against whom, and how is in
[docs/threat-model.md](docs/threat-model.md).

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

- the network helpers and their sockets, pipes and protocol;
- installing, upgrading and removing, on Linux and Windows;
- the Claude and Codex tokens, their refresh and the shared usage cache;
- what hayami parses: device and sensor data, the helper's lines,
  LibreHardwareMonitor's `data.json`, the providers' responses;
- what hayami does outside itself: ending processes, the KWin rule and
  script, the udev rule, autostart entries;
- what hayami shows and prints, including `doctor` and the JSON outputs;
- the release workflow and its artifacts.

## Out of scope

- LibreHardwareMonitor, the providers' APIs and the tools whose credential
  stores hayami reads: report those to their own projects.
- Anything that needs an administrator or root on the machine already.
- What a person chooses to show: a screenshot or a shared screen of the
  panel shows what the panel shows.

## Supported versions

The latest release. Fixes are not backported.
