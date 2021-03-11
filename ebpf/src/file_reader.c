#include <linux/bpf.h>
#include <linux/version.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#include <linux/skbuff.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/ptrace.h>

#define MAX_NB_PACKETS 1000
#define LEGAL_DIFF_TIMESTAMP_PACKETS 1000000
#define MAX_FILE_NAME 64
#define LOG_BLOCK_SIZE 128
#define MAX_LOG_SIZE 8192
#define PID_FILTER_SIZE 32

#ifndef CUR_CPU_IDENTIFIER
#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 8, 0)
#define CUR_CPU_IDENTIFIER BPF_F_CURRENT_CPU
#else
#define CUR_CPU_IDENTIFIER bpf_get_smp_processor_id()
#endif
#endif

struct tmp_map_key_t
{
  char filename[MAX_FILE_NAME];
};

struct map_key_t
{
  u32 pid;
  u32 fd;
};

struct open_event_t
{
  u32 pid;
  u32 fd;
  char filename[MAX_FILE_NAME];
};

struct write_event_t
{
  u32 pid;
  u32 fd;
  __u64 cur_len;
  __u64 total_len;
  char contents[LOG_BLOCK_SIZE];
};

struct
{
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, PID_FILTER_SIZE);
  __type(key, int);
  __type(value, u32);
} pid_filter SEC(".maps");

struct
{
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, 32);
  __type(key, u32);
  __type(value, struct tmp_map_key_t);
} tmp_map SEC(".maps");

struct
{
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, 32);
  __type(key, struct map_key_t);
  __type(value, int);
} listen_map SEC(".maps");

// TODO : find way to change value size to struct
struct bpf_map_def SEC("maps") events_open = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(int),
    .value_size = sizeof(u32),
    .max_entries = 32,
};

struct bpf_map_def SEC("maps") events_write = {
    .type = BPF_MAP_TYPE_PERF_EVENT_ARRAY,
    .key_size = sizeof(int),
    .value_size = sizeof(u32),
    .max_entries = 32,
};

SEC("kprobe/do_sys_open")
int detect_file_open(struct pt_regs *ctx)
{
  __u64 tgid = bpf_get_current_pid_tgid();
  u32 pid = tgid >> 32;

  u32 *filter_pid;
  struct tmp_map_key_t tmp = {};
  for (int i = 0; i < PID_FILTER_SIZE; i++)
  {
    // Use id instead of i to avoid verifier consider this block as infinity loop
    int id = i;
    filter_pid = bpf_map_lookup_elem(&pid_filter, &id);
    if (!filter_pid)
    {
      continue;
    }

    if (pid == *filter_pid)
    {
      goto FILTER_PASS;
    }
  }

  return 0;

FILTER_PASS:

  // bpf_probe_read_kernel_str in 5.5 or latest
  bpf_probe_read(&tmp.filename, sizeof(tmp.filename),
                 (void *)PT_REGS_PARM2(ctx));
  bpf_map_update_elem(&tmp_map, &pid, &tmp, BPF_ANY);
  return 0;
}

SEC("kretprobe/do_sys_open")
int detect_file_open_ret(struct pt_regs *ctx)
{
  __u64 tgid = bpf_get_current_pid_tgid();
  u32 pid = tgid >> 32;

  u32 ret = PT_REGS_RC(ctx);
  struct open_event_t openPackage = {};
  char *filename;

  struct tmp_map_key_t *tmp;
  tmp = bpf_map_lookup_elem(&tmp_map, &pid);
  if (!tmp)
  {
    return 0;
  }

  __builtin_memcpy(openPackage.filename, tmp->filename,
                   sizeof(openPackage.filename));
  openPackage.pid = pid;
  openPackage.fd = ret;
  bpf_perf_event_output(ctx, &events_open, CUR_CPU_IDENTIFIER, &openPackage, sizeof(openPackage));
  bpf_map_delete_elem(&tmp_map, &pid);

  return 0;
}

SEC("kprobe/ksys_write")
int detect_file_write(struct pt_regs *ctx)
{
  __u64 tgid = bpf_get_current_pid_tgid();
  u32 pid = tgid >> 32;

  struct map_key_t key = {};
  key.pid = pid;
  key.fd = (u32)PT_REGS_PARM1(ctx);

  int *keep_listen;
  keep_listen = bpf_map_lookup_elem(&listen_map, &key);
  if (!keep_listen)
  {
    return 0;
  }

  __u64 count = (__u64)PT_REGS_PARM3(ctx);
  if (count <= 0)
  {
    return 0;
  }

  struct write_event_t writePackage = {};
  // bpf_probe_read_kernel_str in 5.5 or latest
  writePackage.fd = key.fd;
  writePackage.pid = key.pid;
  writePackage.total_len = count;
  void *ptr = (void *)PT_REGS_PARM2(ctx);
  for (__u64 i = 0; i < MAX_LOG_SIZE; i += LOG_BLOCK_SIZE)
  {
    if (count <= i)
    {
      break;
    }

    bpf_probe_read(&writePackage.contents, sizeof(char) * LOG_BLOCK_SIZE,
                   ptr + i);
    writePackage.cur_len = i;
    bpf_perf_event_output(ctx, &events_write, CUR_CPU_IDENTIFIER, &writePackage, sizeof(writePackage));
  }

  return 0;
}

char _license[] SEC("license") = "GPL";