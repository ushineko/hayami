// SPDX-License-Identifier: GPL-2.0-only
//
// hayami's network counter (spec 072): every packet that reaches or leaves a
// socket on this machine, counted by the process using the socket and the
// two ends, and every packet the machine forwards, counted by the two
// interfaces. `make bpf` compiles this into netcount_bpfel.o, which is
// committed; building hayami needs no clang.
//
// vmlinux.h is not committed: `make bpf` writes it from the running kernel's
// BTF. Every kernel structure here is read through CO-RE, so the object runs
// on kernels other than the one it was built on.

#include "vmlinux.h"
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_endian.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";

// Macros vmlinux.h cannot carry.
#define ETH_P_IP 0x0800
#define ETH_P_IPV6 0x86DD
#define PACKET_HOST 0
#define NF_ACCEPT 1

// host_netns is the helper's own network namespace. A socket in another one
// is a container's: its traffic out of the machine is forwarded and counted
// at the forward hook, and its traffic to other containers crosses no host
// interface. Set by the loader.
const volatile __u64 host_netns = 0;

// A flow is one process's traffic with one far end, from one near end.
// Addresses are 16 bytes; an IPv4 one fills the first four.
struct flow {
	__u32 tgid; // 0: no owner seen yet
	__u8 proto; // IPPROTO_*
	__u8 family; // 4 or 6
	__u16 lport;
	__u16 rport;
	__u16 pad;
	__u8 laddr[16];
	__u8 raddr[16];
};

struct bytes {
	__u64 rx, tx;
};

struct owner {
	__u64 netns;
	__u32 tgid;
	__u32 pad;
};

// Who uses each socket, by its cookie.
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 65536);
	__type(key, __u64);
	__type(value, struct owner);
} owners SEC(".maps");

// Bytes by flow, since the programs were loaded.
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 65536);
	__type(key, struct flow);
	__type(value, struct bytes);
} flows SEC(".maps");

// Forwarded bytes by pair of interfaces.
struct hop {
	__u32 in, out;
};

struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 1024);
	__type(key, struct hop);
	__type(value, __u64);
} forwarded SEC(".maps");

// Processes that write to a tunnel device: their UDP on a physical interface
// is the tunnel's traffic over again.
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 256);
	__type(key, __u32);
	__type(value, __u8);
} tunnels SEC(".maps");

static __always_inline void own(__u64 cookie, __u64 netns)
{
	struct owner o = { .netns = netns, .tgid = bpf_get_current_pid_tgid() >> 32 };

	if (o.tgid == 0)
		return;
	bpf_map_update_elem(&owners, &cookie, &o, BPF_ANY);
}

SEC("cgroup/sock_create")
int sock_create(struct bpf_sock *sk)
{
	own(bpf_get_socket_cookie(sk), bpf_get_netns_cookie(sk));
	return 1;
}

// A socket is given to whoever sends or receives on it: a server's accepted
// socket, which sock_create never sees, one made before the programs were
// loaded, one passed to another process.
#define OWN(fn)                                                                            \
	SEC("fentry/" #fn)                                                                 \
	int BPF_PROG(own_##fn, struct sock *sk)                                            \
	{                                                                                  \
		own(bpf_get_socket_cookie(sk),                                             \
		    BPF_CORE_READ(sk, __sk_common.skc_net.net, net_cookie));               \
		return 0;                                                                  \
	}

OWN(tcp_sendmsg)
OWN(tcp_recvmsg)
OWN(udp_sendmsg)
OWN(udp_recvmsg)
OWN(udpv6_sendmsg)
OWN(udpv6_recvmsg)

static __always_inline int loopback4(__u32 a)
{
	return (bpf_ntohl(a) >> 24) == 127;
}

static __always_inline int loopback6(const __u32 *a)
{
	if ((a[0] | a[1] | a[2]) == 0 && a[3] == bpf_htonl(1))
		return 1;
	return a[0] == 0 && a[1] == 0 && a[2] == bpf_htonl(0xffff) && loopback4(a[3]);
}

// read fills f from the packet's headers, the near end being the source on
// the way out and the destination on the way in. It reports 0 for a packet
// not worth counting: not IP, or to or from this machine itself.
static __always_inline int read(struct __sk_buff *skb, struct flow *f, int out)
{
	__u32 off;

	if (skb->protocol == bpf_htons(ETH_P_IP)) {
		struct iphdr ip;

		if (bpf_skb_load_bytes(skb, 0, &ip, sizeof(ip)))
			return 0;
		if (loopback4(ip.saddr) || loopback4(ip.daddr))
			return 0;
		f->family = 4;
		f->proto = ip.protocol;
		__builtin_memcpy(out ? f->laddr : f->raddr, &ip.saddr, 4);
		__builtin_memcpy(out ? f->raddr : f->laddr, &ip.daddr, 4);
		off = ip.ihl * 4;
	} else if (skb->protocol == bpf_htons(ETH_P_IPV6)) {
		struct ipv6hdr ip;

		if (bpf_skb_load_bytes(skb, 0, &ip, sizeof(ip)))
			return 0;
		if (loopback6(ip.saddr.in6_u.u6_addr32) || loopback6(ip.daddr.in6_u.u6_addr32))
			return 0;
		f->family = 6;
		f->proto = ip.nexthdr;
		__builtin_memcpy(out ? f->laddr : f->raddr, &ip.saddr, 16);
		__builtin_memcpy(out ? f->raddr : f->laddr, &ip.daddr, 16);
		off = sizeof(ip);
	} else {
		return 0;
	}

	if (f->proto == IPPROTO_TCP || f->proto == IPPROTO_UDP) {
		__be16 ports[2];

		if (bpf_skb_load_bytes(skb, off, ports, sizeof(ports)) == 0) {
			f->lport = bpf_ntohs(ports[out ? 0 : 1]);
			f->rport = bpf_ntohs(ports[out ? 1 : 0]);
		}
	}
	return 1;
}

static __always_inline void count(struct __sk_buff *skb, int out)
{
	struct flow f = {};
	struct owner *o;
	struct bytes *b;
	__u64 cookie;

	// A multicast or broadcast packet is given to every socket listening
	// for it, and the interface carried it once. Counted per socket, one
	// mDNS packet reads as every browser receiving it.
	if (!out && skb->pkt_type != PACKET_HOST)
		return;
	if (!read(skb, &f, out))
		return;

	cookie = bpf_get_socket_cookie(skb);
	o = bpf_map_lookup_elem(&owners, &cookie);
	if (o) {
		if (o->netns && host_netns && o->netns != host_netns)
			return;
		f.tgid = o->tgid;
	}

	b = bpf_map_lookup_elem(&flows, &f);
	if (!b) {
		struct bytes zero = {};

		bpf_map_update_elem(&flows, &f, &zero, BPF_NOEXIST);
		b = bpf_map_lookup_elem(&flows, &f);
		if (!b)
			return;
	}
	if (out)
		__sync_fetch_and_add(&b->tx, skb->len);
	else
		__sync_fetch_and_add(&b->rx, skb->len);
}

// Both let every packet through: they count, and decide nothing.
SEC("cgroup_skb/ingress")
int skb_in(struct __sk_buff *skb)
{
	count(skb, 0);
	return 1;
}

SEC("cgroup_skb/egress")
int skb_out(struct __sk_buff *skb)
{
	count(skb, 1);
	return 1;
}

SEC("netfilter")
int forward(struct bpf_nf_ctx *ctx)
{
	const struct nf_hook_state *st = ctx->state;
	struct hop h = {};
	__u64 len = ctx->skb->len;
	__u64 *v;

	if (st->in)
		h.in = st->in->ifindex;
	if (st->out)
		h.out = st->out->ifindex;
	v = bpf_map_lookup_elem(&forwarded, &h);
	if (v)
		__sync_fetch_and_add(v, len);
	else
		bpf_map_update_elem(&forwarded, &h, &len, BPF_NOEXIST);
	return NF_ACCEPT;
}

// tun_chr_write_iter is a tunnel daemon handing the kernel a packet it took
// off its UDP socket. A kprobe, not fentry: tun is usually a module.
SEC("kprobe/tun_chr_write_iter")
int tunnel(struct pt_regs *ctx)
{
	__u32 tgid = bpf_get_current_pid_tgid() >> 32;
	__u8 one = 1;

	bpf_map_update_elem(&tunnels, &tgid, &one, BPF_ANY);
	return 0;
}
