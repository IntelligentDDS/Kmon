// Reference https://github.com/iovisor/bcc/blob/master/tools/biosnoop.py and https://github.com/iovisor/bcc/blob/master/tools/biotop.py
// Use part of code --Tianjun Weng 19-Jun-2020
//
// This uses in-kernel eBPF maps to cache process details (PID and comm) by I/O
// request, as well as a starting timestamp for calculating I/O latency.
//
// Copyright (c) 2015 Brendan Gregg.
// Licensed under the Apache License, Version 2.0 (the "License")
//
// 16-Sep-2015   Brendan Gregg   Created this.
// 11-Feb-2016   Allan McAleavy  updated for BPF_PERF_OUTPUT

#include <uapi/linux/ptrace.h>
#include <linux/blkdev.h>

typedef struct disk_key
{
    u32 slot;
    u32 pid;
    u64 flags;
    char disk[DISK_NAME_LEN];
} __attribute__((packed)) disk_key_t;

struct info_t
{
    u32 pid;
    int rwflag;
    char disk[DISK_NAME_LEN];
} __attribute__((packed));

struct val_t
{
    u64 bytes;
    u64 ns;
} __attribute__((packed));

struct req_start
{
    u64 data_len;
    u64 timestamp;
};

struct control
{
    u32 verbose;
    u64 bin_scale;
} __attribute__((packed));

BPF_HASH(control_map, u32, struct control);
BPF_HASH(counts, struct info_t, struct val_t);
BPF_HASH(dist, disk_key_t, u64);

BPF_HASH(start, struct request *, struct req_start);
BPF_HASH(whobyreq, struct request *, u32);

// cache PID
int trace_pid_start(struct pt_regs *ctx, struct request *req)
{
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    struct control *con = control_map.lookup(&pid);
    if (con == NULL)
    {
        return 0;
    }
    bpf_trace_printk("%u\n", pid);

    whobyreq.update(&req, &pid);
    return 0;
}

// time block I/O
int trace_req_start(struct pt_regs *ctx, struct request *req)
{
    struct req_start rs = {
        .timestamp = bpf_ktime_get_ns(),
        .data_len = req->__data_len,
    };
    start.update(&req, &rs);
    return 0;
}

// output
int trace_req_completion(struct pt_regs *ctx, struct request *req)
{
    u32 *pid_ptr = whobyreq.lookup(&req);
    if (pid_ptr == NULL)
    { // missing PID, skip
        start.delete(&req);
        return 0;
    }
    u32 pid = *pid_ptr;

    struct req_start *rs = start.lookup(&req);
    if (rs == NULL)
    { // missed tracing issue
        whobyreq.delete(&req);
        return 0;
    }

    struct control *con = control_map.lookup(&pid);
    if (con == NULL)
    {
        return 0;
    }

    // fetch timestamp and calculate delta
    u64 delta = bpf_ktime_get_ns() - rs->timestamp;
    void *__tmp = (void *)req->rq_disk->disk_name;

    struct info_t info = {
        .pid = pid,
        .rwflag = !!((req->cmd_flags & REQ_OP_MASK) == REQ_OP_WRITE),
    };
    bpf_probe_read_kernel(&info.disk, sizeof(info.disk), __tmp);

    struct val_t *valp;
    struct val_t zero = {
        .ns = delta,
        .bytes = rs->data_len,
    };
    valp = counts.lookup(&info);
    if (valp == NULL)
    {
        valp = counts.lookup_or_try_init(&info, &zero);
    }
    else
    {
        valp->ns += delta;
        valp->bytes += rs->data_len;
        counts.update(&info, valp);
    }

    //dist
    if (con->verbose > 1)
    {
        u64 one = 1;
        u64 bin_scale = con->bin_scale;
        if (bin_scale <= 0)
        {
            bin_scale = 1;
        }

        disk_key_t key = {
            .slot = bpf_log2l(delta / con->bin_scale),
            .pid = pid,
            .flags = req->cmd_flags,
        };
        bpf_probe_read_kernel(&key.disk, sizeof(key.disk), __tmp);

        u64 *counter = dist.lookup(&key);
        if (counter == NULL)
        {
            dist.update(&key, &one);
        }
        else
        {
            (*counter)++;
        }
    }

    start.delete(&req);
    whobyreq.delete(&req);
    return 0;
}