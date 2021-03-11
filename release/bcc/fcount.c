#include <linux/skbuff.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ptrace.h>

struct val
{
    u64 count;
    u64 ret_count;
} __attribute__((packed));

BPF_HASH(data, u32, struct val);

int fcount_begin(struct pt_regs *ctx)
{
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    struct val *val = data.lookup(&pid);

    if (!val)
        return 0;

    val->count++;

    return 0;
}

int fcount_return(struct pt_regs *ctx)
{
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    struct val *val = data.lookup(&pid);
    if (!val)
        return 0;

    val->ret_count++;

    return 0;
}