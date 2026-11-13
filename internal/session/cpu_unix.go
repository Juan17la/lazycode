//go:build unix

package session

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// TreeCPU returns the total CPU used so far by a process and everything it started.
func TreeCPU(pid int) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,ppid=,time=").Output()
	if err != nil {
		return 0, err
	}
	return sumTree(string(out), pid), nil
}

func sumTree(table string, root int) time.Duration {
	parent := map[int]int{}
	used := map[int]time.Duration{}
	for _, line := range strings.Split(table, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		parent[pid], used[pid] = ppid, parseCPU(fields[2])
	}
	var total time.Duration
	for pid, cpu := range used {
		for at, steps := pid, 0; at != 0 && steps < 4096; at, steps = parent[at], steps+1 {
			if at == root {
				total += cpu
				break
			}
		}
	}
	return total
}

// parseCPU reads ps's [[dd-]hh:]mm:ss[.cc] format.
func parseCPU(text string) time.Duration {
	var days int
	if before, after, ok := strings.Cut(text, "-"); ok {
		days, _ = strconv.Atoi(before)
		text = after
	}
	var clock time.Duration
	for _, part := range strings.Split(text, ":") {
		seconds, _ := strconv.ParseFloat(part, 64)
		clock = clock*60 + time.Duration(seconds*float64(time.Second))
	}
	return time.Duration(days)*24*time.Hour + clock
}
