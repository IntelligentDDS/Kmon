#include <linux/bpf.h>
// #include <bpf/bpf_helpers.h>
// #include <bpf/bpf_tracing.h>
// #include <bpf/bpf_endian.h>
#include <net/sock.h>

#ifndef __packed
#define __packed __attribute__((packed))
#endif

#define MSG_TYPE_RECV_CLEAN_RBUF 1
#define MSG_TYPE_SEND_ENTRY 2
#define MSG_TYPE_CONNECT 3
#define MSG_TYPE_DISCONNECT 4

// linux v5.4中struct sock结构体里面各个值相关的偏移
struct socket_pair
{
    __be32 peer_addr;
    __be32 my_addr;
    __u32 _the_placeholder;
    __be16 peer_port;
    __u16 my_port;
};

struct msg_event
{
    u64 cur_ts;

    u32 type;
    u32 pid;

    u32 peer_addr;
    u32 my_addr;
    u16 peer_port;
    u16 my_port;
} __packed;

struct conn_pair
{
    u32 peer_addr;
    u32 my_addr;
    u16 peer_port;
    u16 my_port;
};

struct conn_state
{
    u32 type;
    u64 prevTime;
};

struct control
{
    u32 verbose;
} __packed;

BPF_HASH(control_map, u32, struct control);
BPF_HASH(ptr_map, u32, void *);
BPF_HASH(conn_map, struct conn_pair, struct conn_state);
BPF_PERF_OUTPUT(tcp_event);

static inline void emit_event(struct pt_regs *ctx, __u32 pid, struct conn_pair conn, struct conn_state state, u32 type)
{
    struct msg_event cur_record;

    // 获得网络相关参数
    cur_record.peer_addr = conn.peer_addr;
    cur_record.my_addr = conn.my_addr;
    cur_record.peer_port = conn.peer_port;
    cur_record.my_port = conn.my_port;

    // 获得进程/线程上下文与时间戳
    cur_record.cur_ts = bpf_ktime_get_ns();
    cur_record.pid = pid;
    cur_record.type = type;

    tcp_event.perf_submit(ctx, &cur_record, sizeof(cur_record));
}

static inline void check_state(struct pt_regs *ctx, __u32 pid, struct conn_pair conn, u32 type)
{
    struct conn_state *state, zero = {};
    state = conn_map.lookup(&conn);
    if (state)
    {
        state->prevTime = bpf_ktime_get_ns();
        if (state->type != type)
        {
            state->type = type;
            emit_event(ctx, pid, conn, *state, type);
        }
    }
}

int kprobe_security_socket_accept(struct pt_regs *ctx)
{
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;

    struct control *con = control_map.lookup(&pid);
    if (con == NULL)
    {
        return 0;
    }

    struct socket *sockp = (void *)PT_REGS_PARM2(ctx);
    void *skp;
    bpf_probe_read(&skp, sizeof(void *), &sockp->type);
    ptr_map.update(&pid, (void *)&sockp);
    return 0;
}

int kretprobe_move_addr_to_user(struct pt_regs *ctx)
{
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;

    // 过滤掉其他PID的消息
    void **sk = ptr_map.lookup(&pid);
    if (!sk)
    {
        return 0;
    }

    struct socket *sockp = *sk;
    void *skp;
    bpf_probe_read(&skp, sizeof(void *), &sockp->sk);
    ptr_map.delete(&pid);

    struct socket_pair skc;
    bpf_probe_read(&skc, sizeof(struct socket_pair), skp);

    struct conn_pair conn;
    conn.peer_addr = skc.peer_addr;
    conn.my_addr = skc.my_addr;
    conn.peer_port = bpf_ntohs(skc.peer_port);
    conn.my_port = skc.my_port;

    struct conn_state zero = {};
    zero.prevTime = bpf_ktime_get_ns();
    zero.type = MSG_TYPE_CONNECT;
    conn_map.update(&conn, &zero);
    // bpf_trace_printk("enter %d %u %u\n", pid, skc.peer_addr, skc.my_addr);
    // bpf_trace_printk("enter %u %u\n", pid, conn.peer_port, conn.my_port);
    emit_event(ctx, pid, conn, zero, MSG_TYPE_CONNECT);

    return 0;
}

int kprobe_tcp_sendmsg_entry(struct pt_regs *ctx)
{
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;

    // 过滤掉其他PID的消息
    struct control *con = control_map.lookup(&pid);
    if (con == NULL || con->verbose <= 1)
    {
        return 0;
    }

    void *skp = (void *)PT_REGS_PARM1(ctx);
    struct socket_pair sk;
    bpf_probe_read(&sk, sizeof(struct socket_pair), skp);

    struct conn_pair conn;
    conn.peer_addr = sk.peer_addr;
    conn.my_addr = sk.my_addr;
    conn.peer_port = bpf_ntohs(sk.peer_port);
    conn.my_port = sk.my_port;
    check_state(ctx, pid, conn, MSG_TYPE_SEND_ENTRY);

    return 0;
}

int kprobe_tcp_cleanup_rbuf_entry(struct pt_regs *ctx)
{
    // 滤掉没有收到包或者无效的请求
    int copied = PT_REGS_PARM2(ctx);
    if (copied <= 0)
        return 0;

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    // 过滤掉其他PID的消息
    struct control *con = control_map.lookup(&pid);
    if (con == NULL || con->verbose <= 1)
    {
        return 0;
    }

    void *skp = (void *)PT_REGS_PARM1(ctx);
    struct socket_pair sk;
    bpf_probe_read(&sk, sizeof(struct socket_pair), skp);

    struct conn_pair conn;
    conn.peer_addr = sk.peer_addr;
    conn.my_addr = sk.my_addr;
    conn.peer_port = bpf_ntohs(sk.peer_port);
    conn.my_port = sk.my_port;

    check_state(ctx, pid, conn, MSG_TYPE_RECV_CLEAN_RBUF);

    return 0;
}

int kprobe_tcp_close(struct pt_regs *ctx)
{
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;

    // 过滤掉其他PID的消息
    struct control *con = control_map.lookup(&pid);
    if (con == NULL)
    {
        return 0;
    }

    void *skp = (void *)PT_REGS_PARM1(ctx);
    struct socket_pair sk;
    bpf_probe_read(&sk, sizeof(struct socket_pair), skp);

    struct conn_pair conn;
    conn.peer_addr = sk.peer_addr;
    conn.my_addr = sk.my_addr;
    conn.peer_port = bpf_ntohs(sk.peer_port);
    conn.my_port = sk.my_port;
    check_state(ctx, pid, conn, MSG_TYPE_DISCONNECT);
    conn_map.delete(&conn);
    return 0;
}

// char _license[] SEC("license") = "GPL";
