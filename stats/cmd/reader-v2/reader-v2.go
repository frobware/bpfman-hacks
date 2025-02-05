package main

import (
	"fmt"
	"os"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

type Stats struct {
	ContextSwitches uint64
	MajorFaults     uint64
	MinorFaults     uint64
}

const (
	mapPath  = "/sys/fs/bpf/stats-v2_map"
	progPath = "/sys/fs/bpf/stats-v2_prog"
	linkPath = "/sys/fs/bpf/stats-v2_link"
)

func main() {
	if os.Geteuid() != 0 {
		fmt.Println("This program must be run as root")
		os.Exit(1)
	}

	// Load the pre-compiled program
	spec, err := ebpf.LoadCollectionSpec("bpf/stats-v2.o")
	if err != nil {
		fmt.Printf("Failed to load collection spec: %v\n", err)
		os.Exit(1)
	}

	// Create a new collection
	var objs struct {
		StatsMap     *ebpf.Map     `ebpf:"stats_map"`
		StatsProgram *ebpf.Program `ebpf:"count_context_switches"`
	}
	if err := spec.LoadAndAssign(&objs, nil); err != nil {
		fmt.Printf("Failed to load objects: %v\n", err)
		os.Exit(1)
	}
	defer objs.StatsProgram.Close()
	defer objs.StatsMap.Close()

	// Pin the map
	if err := objs.StatsMap.Pin(mapPath); err != nil {
		fmt.Printf("Failed to pin map: %v\n", err)
		os.Exit(1)
	}

	// Create tracepoint link
	tp, err := link.Tracepoint("sched", "sched_switch", objs.StatsProgram, nil)
	if err != nil {
		fmt.Printf("Failed to create tracepoint: %v\n", err)
		os.Exit(1)
	}
	defer tp.Close()

	// Pin the link
	if err := tp.Pin(linkPath); err != nil {
		fmt.Printf("Failed to pin link: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Program loaded and running. Collecting statistics...")

	prevStats := make(map[uint32]Stats)

	for {
		currentStats := make(map[uint32]Stats)
		var pid uint32
		var stats Stats

		iter := objs.StatsMap.Iterate()
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
