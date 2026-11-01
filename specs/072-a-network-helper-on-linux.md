# 072 — a network helper on Linux

**Issue**: #238

## Executive Summary

On Linux, an opt-in helper counts the network by process with eBPF, so the
bandwidth drawer lists UDP, connections shorter than a reading and other
accounts' programs, and names forwarded traffic by interface; the
"unmeasured" remainder that was 9–51 % of an interface is a few percent of
headers and multicast. The helper is a socket-activated systemd service
running as a dynamic user with three capabilities. A reviewer should look
first at `internal/netcount/bpf/netcount.bpf.c` (what is counted, and that
every program passes every packet), the unit in
`internal/nethelper/service_linux.go`, and the socket's access checks in
`server_linux.go`.

## Status: COMPLETE

## Context

The bandwidth drawer's Peers breakdown (spec 065) lists programs by the
bytes their TCP sockets moved, read from `sock_diag`, and closes each
interface with an "unmeasured" line: the interface's rate less what was
attributed (`internal/panel/bandwidth.go`). At the Linux desk that line was
between 9 % and 51 % of an interface (spec 065, Verification). It holds:

- **UDP.** QUIC (most of a browser's traffic), WireGuard and Tailscale's
  transport, DNS, games, calls. The kernel keeps no byte counters for a UDP
  socket, and no privilege changes that.
- **TCP that opened and closed between two readings.** Seeing a socket's
  last counters as it closes means the `sock_diag` destroy group, which
  needs CAP_NET_ADMIN.
- **Other accounts' sockets.** Their owners are in fd directories the panel
  may not read, so they are counted under "other" and their peers under no
  program.
- **Headers.** `tcp_info`'s counters are payload; the interface counts
  packets.
- **Forwarded traffic**: container bridges, virtual machines, an exit node.
  No socket on this machine sends or receives it.

Spec 061 gave Windows a helper and left Linux out: "a different mechanism
(eBPF or a netlink capability) and its own spec". This is that spec. The
mechanism is eBPF: programs attached to the cgroup tree count every packet
that reaches or leaves a local socket, of any protocol, in the kernel, and
record which process made the socket when it was made. Packet capture was
the alternative and was rejected (see Alternatives Considered).

The helper serves peer addresses and ports, not only per-process totals as
on Windows (R5 of spec 061), because the Peers drawer is the reason for the
work. What crosses is no more than `ss -tunp` prints to any account, apart
from the byte counts.

## Requirements

### The counting (kernel side)

- R1 **Programs on the cgroup v2 root**, so every process on the machine is
  in scope, containers included:
  - `cgroup/sock_create` records, in a table keyed by the socket's cookie,
    the process (tgid) that made it and its network namespace.
  - `cgroup_skb/ingress` and `cgroup_skb/egress` add each packet's length
    (IP header included) to a flow: the socket's process, the protocol, and
    the near and far address and port, read from the packet. A connected
    socket has one far end; an unconnected UDP socket has one flow per far
    end it exchanged packets with.
  - `fentry` on `tcp_sendmsg`, `tcp_recvmsg`, `udp_sendmsg`, `udp_recvmsg`,
    `udpv6_sendmsg` and `udpv6_recvmsg` updates the owner to the process
    using the socket. This covers a socket made by one process and used by
    another (a fork, a passed descriptor), a socket accepted by a server,
    which `sock_create` never sees, and every socket that existed before the
    programs were attached.
  - Every entry carries the network namespace of its socket. Sockets in
    another namespace than the helper's (containers) are not listed by
    process: their traffic leaves the machine through the forward hook and
    is counted there, and their traffic to each other never crosses a host
    interface.
  - Multicast and broadcast packets received are not counted to a process,
    and stay in the remainder. Each listening socket gets its own copy and
    the interface carried one: counted per socket, mDNS and SSDP read as
    several programs receiving the same bytes.
  - A `netfilter` program on the IPv4 and IPv6 FORWARD hooks counts
    forwarded bytes per pair of input and output interface: a virtual
    machine behind a bridge, containers, a Tailscale exit node or subnet
    router. It accepts every packet.
  - A connection that lives less than a reading is still counted: a flow's
    bytes are added as its packets pass and stay in the table after the
    socket closes, until the table needs the room. (A `sock_release` hook
    was planned for this and is not needed.)
  - A `kprobe` on `tun_chr_write_iter` records the processes that write to
    a tunnel device (R16).
- R2 The tables are bounded (LRU, sized in one place each). A table more
  than nine-tenths full is reported by the helper, since its oldest entries
  are then being let go and their bytes since the last reading lost.
- R3 The BPF C source lives in the repository with the Go generated from it
  by `bpf2go` (`github.com/cilium/ebpf`) and the compiled object committed,
  so building hayami needs no clang. `make bpf` regenerates both, writing
  `vmlinux.h` from the running kernel (not committed; the programs read
  kernel structures through CO-RE). A generated file records a hash of the
  source, and a test fails when the two disagree.
- R4 `hayami-tui` stays `CGO_ENABLED=0`: `cilium/ebpf` is pure Go.

### The helper

- R5 **A systemd system service, `hayami-network`, started by its socket.**
  The socket unit, `hayami-network.socket`, is a Unix socket at
  `/run/hayami-network.sock` owned by the one account it was installed for,
  mode `0600`, and enabled at boot, which costs a listening socket. Nothing
  starts the service but a connection to the socket; the service has no
  `[Install]` section.
- R6 **Not root.** `DynamicUser=yes` with
  `AmbientCapabilities=CAP_BPF CAP_NET_ADMIN CAP_PERFMON` and the matching
  `CapabilityBoundingSet`, plus `NoNewPrivileges`, `ProtectSystem=strict`,
  `ProtectHome=yes`, `PrivateTmp`, `RestrictAddressFamilies=AF_UNIX
  AF_NETLINK` and `SystemCallFilter=@system-service bpf perf_event_open`.
  What each line is for is commented in the unit.
- R7 **Its program is a copy.** `hayami-tui` is copied to
  `/usr/lib/hayami/hayami-network`, owned by root and not writable by the
  account it serves, for the reason spec 061 R2 gives: a service whose
  program its user can replace is a way to become the service. The service
  runs `hayami-network network-helper run`.
- R8 **It runs only while it is read.** The programs are attached when the
  first reader connects and detached when the last leaves; with no reader
  for fifteen seconds the service exits. Nothing is left attached after it
  exits, including after a crash (links are owned by the process, not
  pinned).
- R9 **One way.** The helper writes and reads nothing a panel sends. It
  checks the peer's credentials (`SO_PEERCRED`) against the account it was
  installed for, as well as the socket's mode.
- R10 **What crosses: protocol version 2.** The first line says who is
  speaking (protocol, hayami version, `Counts` = "every protocol"). Each line
  after it, four times a second, is one entry per (process, protocol, near
  address and port, far address and port) that moved bytes since the line
  before, with received and sent bytes; the forwarded bytes by pair of
  interfaces; the tunnel daemons; and R2's full flag. No names: the panel
  finds them itself, as it does now, and works out which near ports are
  listening ones. Windows' helper stays on version 1. A quarter of a second
  and not a second, because the panel sets the bytes against the
  interfaces' counters read at its own moments, and a reading is out by up
  to a tick of traffic.
- R11 A reader that stops reading is dropped when its buffer fills and holds
  no other reader up (as spec 061 R6). Programs that cannot load are said to
  the reader in the helper's words, the kernel's verifier message included,
  and the service ends.

### The panel

- R12 `core`'s network reader on Linux is a chain: the helper first, then
  the `sock_diag` reader of today. Only the helper not being installed sends
  a reading on; a failure from an installed helper is the answer (spec 061
  R7). The helper is looked for on every reading (spec 061 R8).
- R13 The helper's entries go through the same aggregation as `sock_diag`'s
  (`socketNet` in `core/netprocs.go`): by process and interface, then by
  peer, then by socket. Loopback is left out the same way. The owner comes
  from the entry instead of the fd walk.
- R14 With the helper, the list says "every protocol" where it says "TCP
  only" now. Forwarded traffic is listed under each interface by where it
  came from or went to, grouped under one "forwarded" line with "↔ vnet0",
  "↔ br-…" under it (a name with "forwarded" in front ran into its figures
  on the real window: an interface's name is up to fifteen characters). The
  interface's rate the remainder is set against is read by the reader at
  the moment it takes the flows, not the card's from another second. What is
  left after that is "headers / multicast": link-layer headers, multicast
  and broadcast received, non-IP. ("other" is taken: it is the processes
  with no name.) Without the helper, the list says "TCP only" and
  "unmeasured" as now.
- R15 The panel says why the helper is absent when it is: not installed
  (the list is today's, and `network-helper` says what to run), installed
  for another account, its socket stopped, or a kernel without what R1
  needs (in the helper's words, naming the version that has it).
- R16 The overlay double count is said: a tunnel daemon's UDP on the
  physical interface (`tailscaled`, `openvpn`) is the same traffic as its
  tunnel interface's (`tailscale0`, `tun0`). Its row is marked "(tunnel)".
  Done by the process writing to a tunnel device (a kprobe on
  `tun_chr_write_iter`), not by the program's name. Kernel WireGuard has no
  process to mark and is out of scope.

### Installing and removing

- R17 `hayami-tui network-helper` and `status` say on Linux what they say on
  Windows: installed or not, which version, for whom, whether it runs. They
  need no privilege.
- R18 `hayami-tui network-helper install [--allow USER]` copies the program,
  writes the two units and reloads systemd. It needs root and does not ask
  to become it: run without, it says the `sudo` line to run, with the
  caller's account in `--allow`. `install.sh` never runs sudo (its own
  rule), so it prints the same line when asked with `--with-network`.
- R19 `hayami-tui network-helper remove` stops the socket and service,
  removes the units and the copy. `uninstall.sh` says how when the helper is
  there.

## Acceptance Criteria

- [x] With the helper installed, a panel running as the user lists a
      program moving only UDP by rate and by peer, and the list says "every
      protocol". (`tailscaled`, `openvpn`, `dnsmasq` and `avahi-daemon`
      with their peers; the note "every protocol, 67 % of the interfaces'
      traffic". A QUIC download was seen in the prototype runs, not
      separately on this build.)
- [x] A short-lived connection (a loop of `curl` to one host, each under a
      second) is counted to `curl`'s process name or to "other" when it is
      gone, not to the remainder. (Sixty `curl`s to example.com: its two
      addresses under "other" at 45.6 and 30.4 KiB/s.)
- [x] A root-owned process's traffic is listed under its own name.
      (`tailscaled`, `openvpn`, `dnsmasq`.)
- [x] Under a Steam download (~70 MiB/s), "other" is under 5 % of the
      interface, and the programs and the helper together use under 2 % of
      one core. (Measured with the prototype's programs: 1.9 % left over,
      1.5 % of a core at 72 MiB/s, user space under 1 %. Accepted on those
      figures; daily use is the check on this build.)
- [x] A virtual machine's or a container's download is listed as forwarded
      from its interface, not as "other". (`↔ br-63e956e49aa1`, a Docker
      network, and `↔ virbr0`.)
- [x] A burst of mDNS is not counted to every program that listens for it.
      (`skb->pkt_type`; the identical receive rates of run 3 are gone, and
      `avahi-daemon` shows its own sends only.)
- [x] A privileged test (skipped without CAP_BPF) loads the programs,
      exchanges a known number of UDP and TCP bytes between two sockets of
      the test, and reads them back from the counter under the test's own
      process ID, with the header bytes accounted for.
      (`TestTheKernelCountsThisProcesssUDPAndTCP`: 10 × 1028 bytes of UDP
      each way exactly, TCP between 100,000 and 110,000. The socket between
      the counter and the panel is covered by the server's tests.)
- [x] A test shows the programs are attached only while somebody reads, are
      gone after the last reader leaves, and the service ends by itself.
      (`TestTheServerCountsOnlyWhileSomebodyReads`, falsified.)
- [x] A test shows the helper is asked before `sock_diag`, and a helper's
      failure is not followed by the `sock_diag` reading.
      (`TestTheChainAsksTheHelperFirst`, falsified.)
- [x] A test shows a peer that is not the installed account is refused.
      (`TestTheServerServesOneAccount`, falsified.)
- [x] A test fails when the BPF source and the committed object disagree
      (R3); falsified by editing the source. (`TestTheObjectWasBuiltFromThisSource`.)
- [x] `systemctl show hayami-network` reports the dynamic user and the
      capabilities of R6 and no others; `systemd-analyze security` on the
      unit is recorded in Verification.
- [x] `CGO_ENABLED=0 go build ./cmd/hayami-tui` still succeeds.
- [x] Without the helper, the drawer is unchanged: "TCP only" and
      "unmeasured". (`TestTheBreakdownFromTheHelperSaysWhatIsLeft`, its
      partial reader.)
- [x] The real window's drawer, with the helper, read from a screenshot:
      "every protocol" and the renamed remainder line.
- [x] README, `docs/architecture.md` (rule on the network helper and its
      Status table) and `CLAUDE.md`'s helper paragraph say what the Linux
      helper is and how it is installed.

## Risks & Assumptions

- **Minimum kernel 6.4** (the `netfilter` program type; every other hook
  of R1 is older: `sock_release` 5.9, `CAP_BPF` 5.8, `fentry` 5.5) **with
  BTF** (`/sys/kernel/btf/vmlinux`). Older kernels, or ones without BTF, get
  R15's absence and today's TCP list. Accepted.
- **Kernel lockdown in confidentiality mode** refuses BPF that reads kernel
  memory; `fentry` may be refused there. The helper then runs without R1's
  owner update and says so, and a socket is counted to its maker.
- **Addresses and ports of every account's connections** reach the one
  account the helper serves. Any account can already see them with `ss`;
  what is new is the byte counts and the owner of another account's socket.
  The person installing the helper is agreeing to that for their account.
- **A program that takes the socket's path first**: the path is under
  `/run`, which only root can write, and systemd makes it. Not reachable
  without root.
- **Committed BPF object.** clang's output is not reproducible across
  versions, so CI cannot rebuild and diff it; the hash check of R3 catches a
  source edited without regenerating, not an object built from other source
  on purpose. Reviewers see a binary diff in a PR that changes it.
- **New dependency**, `github.com/cilium/ebpf`. Supply-chain review in the
  security pass; `make vuln` covers it.
- **Header bytes** are counted from the IP header; the interface's counter
  includes the link layer too. The remainder keeps that few percent.
- **The copy goes stale on an upgrade**, as on Windows. A protocol change is
  refused by name (spec 061 R10).
- Rollback: `hayami-tui network-helper remove` and the panel is back to
  `sock_diag`; reverting the code leaves version 1 readers that refuse a
  version 2 helper by name.

## Measurements while scoping

A throwaway prototype (not committed) loaded the programs of R1, minus the
peer addresses and the release hook, and printed once a second what they
attributed against `/proc/net/dev` for every interface but loopback,
bridges, veths and VM taps. Linux 6.18 (CachyOS LTS), cilium/ebpf 0.22.0,
built with `CGO_ENABLED=0`. All three runs were 90 seconds at the desk.

| Run | Traffic | Interfaces | Attributed to processes | Forwarded | Left over | No owner | Programs' CPU |
|---|---|---|---|---|---|---|---|
| 1 | Light, containers counted | 51.2 MiB | 101 % | not measured | -1.3 % | 0.5 % | 0.09 % |
| 2 | Light, containers apart | 67.6 MiB | 63 % | not measured | 36.7 % | 0.6 % | 0.11 % |
| 3 | Steam at ~72 MiB/s | 638.5 MiB | 88.9 % | 9.3 % | 1.9 % | 0.1 % | 0.26 % average, 1.5 % at 72 MiB/s |

- Run 1 counted container sockets as the host's; they talk to each other
  over bridges that the comparison left out, so attribution exceeded the
  interfaces on quiet seconds.
- Run 2's remainder was forwarding: the desk has `ip_forward=1`, advertises
  Tailscale routes and is offered as an exit node, and runs a VM behind
  `virbr0`. Run 3's forward counter accounted for it.
- What run 3 left (1.9 %) is about the size of the link-layer header the
  interface counts and the IP-level count does not.
- Quiet seconds still overshot: Vivaldi, Steam's web helper, Teams,
  `avahi-daemon` and `systemd-resolved` received identical rates in the
  same seconds, which is each socket counting its own copy of one multicast
  packet. R1 leaves these out of every process.
- The user-space side of the prototype read under 1 % of one core.

## Alternatives Considered

- Packet capture (`AF_PACKET`, CAP_NET_RAW) matched to `sock_diag` sockets,
  as nethogs does: no BPF toolchain, but every packet crosses into user
  space, it still misses sockets that close between dumps, and owners still
  need the fd walk. Rejected for cost at the rates spec 065 measured.
- systemd's `IPAccounting=`: per unit, not per process, and the user
  manager cannot load BPF. Rejected.

## Verification

Linux 6.18 (CachyOS LTS), at the desk, against live traffic.

- `make test` (race detector), `make lint` (0 issues), `go vet` for
  Windows on the changed packages, `make check-no-binaries` (the BPF object
  is `application/x-object`, which the check allows), `make vuln`: nothing
  reachable; the three it lists are in `x/net` and `x/crypto` and predate
  this.
- Falsified by breaking what each guards: the chain's order, the server
  letting the programs go when the last reader leaves (the test first
  passed on the shutdown path and was tightened until it did not), the
  account check, the source hash, the forwarded bytes in the remainder, the
  label width and the interface rates the remainder is set against.
- `sudo hayami-tui network-helper install --allow 1000` from the worktree:
  `/usr/lib/hayami/hayami-network` root-owned `0755`, the socket `0600` and
  the account's, `systemctl show`: `DynamicUser=yes`,
  `AmbientCapabilities` and `CapabilityBoundingSet` `cap_net_admin
  cap_perfmon cap_bpf`. `systemd-analyze security hayami-network.service`:
  overall exposure 1.9, OK.
- Lifecycle: the service inactive; `hayami-tui detail bandwidth/peers`
  started it; it was deactivated by itself 18 seconds after it started (a
  two-second read and the fifteen-second idle), 185 ms of CPU and 76.6 MB
  at peak; the socket stayed listening.
- The kernel refused the forward hook at priority 0 ("device or resource
  busy", where nftables' filter chains are); the counter sits at -1000.
- The real window, three screenshots (kept out of the repository: they show
  tailnet host names). The first had `forwarded ↔ br-63e956e49aa1` running
  into its figures, and a remainder of 242 KiB/s from setting the flows
  over two seconds against the card's rate over one; after the fix the
  group fits and the remainder read 460 B/s down and 64 B/s up.
- Repeated `detail` readings: the remainder at 1–3 % of the interface,
  with a burst occasionally on the wrong side of a one-second reading.

## E2E Test Plan

At the Linux desk, helper installed with `sudo hayami-tui network-helper
install --allow $USER`:

1. `systemctl status hayami-network` inactive; open the drawer; active;
   close; inactive within thirty seconds; `bpftool prog` shows none of
   hayami's programs. (R5, R8)
2. A QUIC download in a browser: the browser's row by rate and peer, "every
   protocol". (AC 1)
3. `for i in $(seq 200); do curl -s https://example.com >/dev/null; done`:
   counted, not in the remainder. (AC 2)
4. The Steam download of spec 065: remainder and helper CPU. (AC 4)
5. Screenshot of the drawer. (AC 13)
6. `sudo hayami-tui network-helper remove`: drawer back to "TCP only". (AC 12)

### Prod visibility

Not applicable: a desktop program with no production deployment.
