// Package main provides a simple BPF map reader for the stats
// program.
//
// This program expects the BPF program, map and link to be already
// set up via bpftool:
//
//   - BPF program should be loaded and attached to sched:sched_switch tracepoint.
//   - Map should be pinned at /sys/fs/bpf/stats_map
//   - Link should be created between the program and tracepoint
//
// Use these bpftool commands to set up the environment:
//
//	bpftool prog load bpf/stats.o /sys/fs/bpf/stats_prog autoattach
//	bpftool map pin name stats_map /sys/fs/bpf/stats_map
//
// This program reads the stats map every 3 seconds and reports
// changes in process statistics since the last reading. It shows the
// delta values for context switches and page faults, only displaying
// processes that had activity in the last interval.
//
// Output format:
//
//	PID <pid>: +CS: <context_switches>, +MF: <major_faults>, +mF: <minor_faults>
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/cilium/ebpf"
)

type Stats struct {
	ContextSwitches uint64
	MajorFaults     uint64
	MinorFaults     uint64
}

const mapPath = "/sys/fs/bpf/stats_map"

func main() {
	if os.Geteuid() != 0 {
		fmt.Println("This program must be run as root")
		os.Exit(1)
	}

	statsMap, err := ebpf.LoadPinnedMap(mapPath, nil)
	if err != nil {
		fmt.Printf("Error loading pinned map: %v\n", err)
		fmt.Println("Ensure the BPF program is loaded and map is pinned using bpftool")
		os.Exit(1)
	}
	defer statsMap.Close()

	prevStats := make(map[uint32]Stats)

	fmt.Println("Reading statistics...")
	for {
		currentStats := make(map[uint32]Stats)

		var pid uint32
		var stats Stats
		iter := statsMap.Iterate()
		for iter.Next(&pid, &stats) {
			currentStats[pid] = stats

			if prev, exists := prevStats[pid]; exists {
				delta := Stats{
					ContextSwitches: stats.ContextSwitches - prev.ContextSwitches,
					MajorFaults:     stats.MajorFaults - prev.MajorFaults,
					MinorFaults:     stats.MinorFaults - prev.MinorFaults,
				}

				if delta.ContextSwitches > 0 || delta.MajorFaults > 0 || delta.MinorFaults > 0 {
					fmt.Printf("PID %d: +CS: %d, +MF: %d, +mF: %d\n",
						pid, delta.ContextSwitches, delta.MajorFaults, delta.MinorFaults)
				}
			}
		}
		if err := iter.Err(); err != nil {
			fmt.Printf("Error iterating map: %v\n", err)
		}

		prevStats = currentStats

		fmt.Println("---")
		time.Sleep(3 * time.Second)
	}
}
