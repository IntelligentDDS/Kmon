#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

typedef struct nest_t
{
    long d[5];
} Nested;

typedef struct test_t
{
    int a;
    Nested nested;
    char b[128];
    unsigned int c[5];
} Test;

struct
{
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 32);
    __type(key, Test);
    __type(value, int);
} test_map SEC(".maps");

SEC("kprobe/test")
int bpf_prog(struct pt_regs *ctx)
{
    Test key = {1, {.d = {9, 8, 7, 6, 5}}, "hello world", {1, 2, 3, 4, 5}};
    int value = 1000;
    bpf_map_update_elem(&test_map, &key, &value, BPF_ANY);
    return 0;
}

char _license[] SEC("license") = "GPL";
