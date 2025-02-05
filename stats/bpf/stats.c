#include "vmlinux.h"
#include <bpf/bpf_helpers.h>

struct stats_t {
	__u64 context_switches;
	__u64 major_faults;
	__u64 minor_faults;
};

// An eBPF map to store per-process statistics.
struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 1024);
	__type(key, __u32);
	__type(value, struct stats_t);
} stats_map SEC(".maps");

static __always_inline struct stats_t *get_or_init_stats(__u32 pid) {
	struct stats_t *stats, zero = {};

	stats = bpf_map_lookup_elem(&stats_map, &pid);
	if (!stats) {
		bpf_map_update_elem(&stats_map, &pid, &zero, BPF_ANY);
		stats = bpf_map_lookup_elem(&stats_map, &pid);
		if (!stats)
			return NULL;
	}

	return stats;
}

SEC("tracepoint/sched/sched_switch")
int count_context_switches(struct trace_event_raw_sched_switch *ctx) {
	if (!ctx)
		return 0;

	// Count for the process being switched out.
	struct stats_t *prev_stats = get_or_init_stats(ctx->prev_pid);
	if (prev_stats)
		prev_stats->context_switches++;

	// Count for the process being switched in.
	struct stats_t *next_stats = get_or_init_stats(ctx->next_pid);
	if (next_stats)
		next_stats->context_switches++;

#if 0
	bpf_printk("eBPF: Context switch from PID %u to PID %u", prev_pid, next_pid);
#endif
	return 0;
}

char LICENSE[] SEC("license") = "GPL";
