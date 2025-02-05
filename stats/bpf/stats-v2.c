#include "vmlinux.h"
#include <bpf/bpf_helpers.h>

struct stats_t {
	__u64 context_switches;
	__u64 major_faults;
	__u64 minor_faults;
};

// Structure matching the page_fault_user tracepoint format.
struct trace_event_raw_page_fault_user {
	__u16 common_type;
	__u8  common_flags;
	__u8  common_preempt_count;
	__s32 common_pid;
	__u64 address;		// Faulting address
	__u64 ip;		// Instruction pointer
	__u64 error_code;	// Error code indicating fault type
};

// An eBPF map to store per-process statistics.
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 1024);
	__type(key, __u32);
	__type(value, struct stats_t);
} stats_map SEC(".maps");

static __always_inline struct stats_t *get_or_init_stats(__u32 pid)
{
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
int count_context_switches(struct trace_event_raw_sched_switch *ctx)
{
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
#if DEBUG
	bpf_printk("eBPF: Context switch from PID %u to PID %u", prev_pid, next_pid);
#endif
	return 0;
}

// Track page faults
SEC("tracepoint/exceptions/page_fault_user")
int count_page_faults(struct trace_event_raw_page_fault_user *ctx) {
#if DEBUG
	bpf_printk("eBPF: stats-v2: tracepoint/exceptions/page_fault_user");
#endif
	if (!ctx)
		return 0;

	__u32 pid = bpf_get_current_pid_tgid() >> 32;
	struct stats_t *stats = get_or_init_stats(pid);
	if (!stats)
		return 0;

	// error_code bit 0: 0=non-present page (major fault),
	// 1=protection violation (minor fault).
	if (ctx->error_code & 1) {
		stats->minor_faults++;  // Protection violation -> minor fault
	} else {
		stats->major_faults++;  // Non-present page -> major fault
	}

	return 0;
}

char LICENSE[] SEC("license") = "GPL";
