#include <uapi/linux/ptrace.h>
#include <uapi/linux/tcp.h>
#include <uapi/linux/ip.h>
#include <net/sock.h>
#include <bcc/proto.h>
struct control
{
    u32 verbose;
} __attribute__((packed));
BPF_HASH(control_map, u32, struct control);
BPF_STACK_TRACE(stack_traces, 1024);
// separate data structs for ipv4 and ipv6
struct ipv4_data_t
{
    u32 pid;
    u32 stack_id;
    u32 ustack_id;
    u8 state;
    u8 tcpflags;
    u16 sport;
    u16 dport;
    u32 saddr;
    u32 daddr;
} __attribute__((packed));
BPF_PERF_OUTPUT(ipv4_events);
struct ipv6_data_t
{
    u32 pid;
    u32 stack_id;
    u32 ustack_id;
    u8 state;
    u8 tcpflags;
    u16 sport;
    u16 dport;
    char saddr[16];
    char daddr[16];
} __attribute__((packed));
BPF_PERF_OUTPUT(ipv6_events);
static struct tcphdr *skb_to_tcphdr(const struct sk_buff *skb)
{
    // unstable API. verify logic in tcp_hdr() -> skb_transport_header().
    return (struct tcphdr *)(skb->head + skb->transport_header);
}
static inline struct iphdr *skb_to_iphdr(const struct sk_buff *skb)
{
    // unstable API. verify logic in ip_hdr() -> skb_network_header().
    return (struct iphdr *)(skb->head + skb->network_header);
}
// from include/net/tcp.h:
#ifndef tcp_flag_byte
#define tcp_flag_byte(th) (((u_int8_t *)th)[13])
#endif
int trace_tcp_drop(struct pt_regs *ctx, struct sock *sk, struct sk_buff *skb)
{
    if (sk == NULL)
        return 0;
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    struct control *con = control_map.lookup(&pid);
    if (con == NULL)
    {
        return 0;
    }
    bpf_trace_printk("%u %u\n", pid, con->verbose);

    // pull in details from the packet headers and the sock struct
    u16 family = sk->__sk_common.skc_family;
    char state = sk->__sk_common.skc_state;
    u16 sport = 0, dport = 0;
    struct tcphdr *tcp = skb_to_tcphdr(skb);
    struct iphdr *ip = skb_to_iphdr(skb);
    u8 tcpflags = ((u_int8_t *)tcp)[13];
    sport = tcp->source;
    dport = tcp->dest;
    sport = ntohs(sport);
    dport = ntohs(dport);
    if (family == AF_INET)
    {
        bpf_trace_printk("v4\n");
        struct ipv4_data_t data4 = {};
        data4.pid = pid;
        data4.saddr = ip->saddr;
        data4.daddr = ip->daddr;
        data4.dport = dport;
        data4.sport = sport;
        data4.state = state;
        data4.tcpflags = tcpflags;
        if (con->verbose > 1)
        {
            data4.ustack_id = stack_traces.get_stackid(ctx, BPF_F_USER_STACK);
        }
        else
        {
            data4.ustack_id = 0;
        }

        if (con->verbose > 2)
        {
            data4.stack_id = stack_traces.get_stackid(ctx, 0);
        }
        else
        {
            data4.stack_id = -1;
        }

        ipv4_events.perf_submit(ctx, &data4, sizeof(data4));
    }
    else if (family == AF_INET6)
    {
        bpf_trace_printk("v6\n");
        struct ipv6_data_t data6 = {};
        data6.pid = pid;
        // // The remote address (skc_v6_daddr) was the source
        bpf_probe_read_kernel(data6.saddr, sizeof(data6.saddr),
                              sk->__sk_common.skc_v6_daddr.in6_u.u6_addr32);
        // // The local address (skc_v6_rcv_saddr) was the destination
        bpf_probe_read_kernel(data6.daddr, sizeof(data6.daddr),
                              sk->__sk_common.skc_v6_rcv_saddr.in6_u.u6_addr32);

        data6.dport = dport;
        data6.sport = sport;
        data6.state = state;
        data6.tcpflags = tcpflags;
        if (con->verbose > 1)
        {
            data6.ustack_id = stack_traces.get_stackid(ctx, BPF_F_USER_STACK);
        }
        else
        {
            data6.ustack_id = 0;
        }

        if (con->verbose > 2)
        {
            data6.stack_id = stack_traces.get_stackid(ctx, 0);
        }
        else
        {
            data6.stack_id = 0;
        }

        ipv6_events.perf_submit(ctx, &data6, sizeof(data6));
    }
    // else drop
    return 0;
}