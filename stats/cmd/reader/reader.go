// This program provides a complete BPF stats program that tracks
// process context switches.
//
// This program handles the complete lifecycle of a BPF program:
//   - Loading the pre-compiled BPF program (bpf/stats.o)
//   - Creating and pinning the BPF map
//   - Attaching the program to the sched:sched_switch tracepoint via a
//     link
//   - Continuously monitoring and displaying stats
//
// The program requires root privileges and expects the BPF object
// file to be compiled:
//
//	make build  # Compiles bpf/stats.o
//
// The program will:
// - Pin the map at /sys/fs/bpf/stats_map
// - Pin the program at /sys/fs/bpf/stats_prog
// - Pin the link at /sys/fs/bpf/stats_link
//
// It displays the top 10 processes by context switch rate, updated
// every 5 seconds, showing both total context switches and the rate
// over the last 5 second window.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

// Stats represents the per-process statistics collected by our BPF
// program. This must match the structure defined in the BPF program.
type Stats struct {
	ContextSwitches uint64
	MajorFaults     uint64
	MinorFaults     uint64
}

// ProcessStats combines the raw stats with calculated delta values
// for displaying rates of change.
type ProcessStats struct {
	PID   uint32
	Stats Stats

	// For tracking changes between iterations.
	Delta Stats
}

func main() {
	const (
		mapPath  = "/sys/fs/bpf/stats_map"
		progPath = "/sys/fs/bpf/stats_prog"
		linkPath = "/sys/fs/bpf/stats_link"
	)

	if os.Geteuid() != 0 {
		fmt.Println("This program must be run as root")
		os.Exit(1)
	}

	spec, err := ebpf.LoadCollectionSpec("bpf/stats.o")
	if err != nil {
		fmt.Printf("Failed to load collection spec: %v\n", err)
		os.Exit(1)
	}

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

	if err := objs.StatsMap.Pin(mapPath); err != nil {
		fmt.Printf("Failed to pin map: %v\n", err)
		os.Exit(1)
	}

	tp, err := link.Tracepoint("sched", "sched_switch", objs.StatsProgram, nil)
	if err != nil {
		fmt.Printf("Failed to create tracepoint: %v\n", err)
		os.Exit(1)
	}
	defer tp.Close()

	if err := tp.Pin(linkPath); err != nil {
		fmt.Printf("Failed to pin link: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Program loaded and attached successfully. Press Ctrl+C to exit.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		prevStats := make(map[uint32]Stats)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			// Read all stats
			var allStats []ProcessStats
			iter := objs.StatsMap.Iterate()
			var pid uint32
			var stats Stats

			for iter.Next(&pid, &stats) {
				delta := Stats{
					ContextSwitches: stats.ContextSwitches,
					MajorFaults:     stats.MajorFaults,
					MinorFaults:     stats.MinorFaults,
				}

				// Calculate delta if we have previous stats
				if prev, ok := prevStats[pid]; ok {
					delta.ContextSwitches -= prev.ContextSwitches
					delta.MajorFaults -= prev.MajorFaults
					delta.MinorFaults -= prev.MinorFaults
				}

				allStats = append(allStats, ProcessStats{
					PID:   pid,
					Stats: stats,
					Delta: delta,
				})

				prevStats[pid] = stats
			}

			if err := iter.Err(); err != nil {
				fmt.Printf("Error iterating map: %v\n", err)
				continue
			}

			sort.Slice(allStats, func(i, j int) bool {
				return allStats[i].Delta.ContextSwitches > allStats[j].Delta.ContextSwitches
			})

			fmt.Printf("\033[2J\033[H") // Clear screen and move cursor to top
			fmt.Printf("Top processes by context switches (last 5s):\n")
			fmt.Printf("%-10s %-15s %-15s\n", "PID", "Total CS", "CS/5s")
			fmt.Println("----------------------------------------")

			for i, ps := range allStats {
				if i >= 10 { // top 10
					break
				}
				fmt.Printf("%-10d %-15d %-15d\n",
					ps.PID,
					ps.Stats.ContextSwitches,
					ps.Delta.ContextSwitches)
			}
		}
	}()

	<-sigChan
}
