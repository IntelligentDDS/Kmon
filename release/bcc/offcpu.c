// From https://github.com/iovisor/bcc/blob/master/tools/offcputime.py
// Use part of code --Tianjun Weng 19-Jun-2020
//
// Copyright 2016 Netflix, Inc.
// Licensed under the Apache License, Version 2.0 (the "License")
//
// 13-Jan-2016	Brendan Gregg	Created this.

#include <uapi/linux/ptrace.h>
#include <linux/sched.h>

struct key_t
{
    u32 pid;
    u32 tgid;
    u32 user_stack_id;
    u32 kernel_stack_id;
    char name[32];
} __packed;
BPF_HASH(counts, struct key_t, u64);
BPF_HASH(start, u32);
BPF_STACK_TRACE(stack_traces, 1024);
struct warn_event_t
{
    u32 pid;
    u32 tgid;
    u32 t_start;
    u32 t_end;
} __packed;
BPF_PERF_OUTPUT(warn_events);

struct control
{
    u32 verbose;
    u32 state;
    u32 start;
    u32 end;
} __packed;
BPF_HASH(control_map, u32, struct control);

int oncpu(struct pt_regs *ctx, struct task_struct *prev)
{
    u32 pid = prev->pid;
    u32 tgid = prev->tgid;
    u64 ts, *tsp;

    struct control *con = control_map.lookup(&pid);
    // record previous thread sleep time
    // bpf_trace_printk("%u\n", pid);
    if (con != NULL)
    {
        // bpf_trace_printk("%u %u\n", con->state, prev->state & con->state);
        if ((con->state == 0 && prev->state == 0) || (prev->state & con->state))
        {
            ts = bpf_ktime_get_ns();
            start.update(&pid, &ts);
        }
    }

    // get the current thread's start time
    pid = bpf_get_current_pid_tgid();
    tgid = bpf_get_current_pid_tgid() >> 32;
    con = control_map.lookup(&pid);
    if (con == NULL)
    {
        return 0;
    }

    tsp = start.lookup(&pid);
    if (tsp == NULL)
    {
        return 0; // missed start or filtered
    }

    // calculate current thread's delta time
    u64 t_start = *tsp;
    u64 t_end = bpf_ktime_get_ns();
    start.delete(&pid);
    if (t_start > t_end)
    {
        bpf_trace_printk("warn\n");
        struct warn_event_t event = {
            .pid = pid,
            .tgid = tgid,
            .t_start = t_start,
            .t_end = t_end,
        };
        warn_events.perf_submit(ctx, &event, sizeof(event));
        return 0;
    }

    u64 delta = t_end - t_start;
    delta = delta / 1000;
    // bpf_trace_printk("%u %u %u\n", delta, con->start, con->end);
    if ((delta < con->start) || (delta > con->end))
    {
        return 0;
    }

    // create map key
    struct key_t key = {};
    key.pid = pid;
    key.tgid = tgid;
    if (con->verbose > 2)
    {
        key.user_stack_id = stack_traces.get_stackid(ctx, BPF_F_USER_STACK);
    }
    else
    {
        key.user_stack_id = -1;
    }

    if (con->verbose > 1)
    {
        key.kernel_stack_id = stack_traces.get_stackid(ctx, 0);
    }
    else
    {
        key.user_stack_id = -1;
    }

    bpf_get_current_comm(&key.name, sizeof(key.name));
    counts.increment(key, delta);
    return 0;
}