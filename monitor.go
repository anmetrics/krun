package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func cmdMonit() error {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	renderMonit()

	for {
		select {
		case <-sig:
			fmt.Print("\033[?25h") // show cursor
			return nil
		case <-ticker.C:
			renderMonit()
		}
	}
}

func renderMonit() {
	// Clear screen and move cursor to top
	fmt.Print("\033[2J\033[H")
	fmt.Print("\033[?25l") // hide cursor

	units := listKrunUnits()

	fmt.Println(colorize(bold+cyan, " krun monit") + colorize(gray, " — press Ctrl+C to exit"))
	fmt.Println()

	if len(units) == 0 {
		fmt.Println(colorize(gray, " no apps running"))
		return
	}

	// Process table
	tbl := &Table{
		Headers: []string{"name", "status", "pid", "cpu", "mem", "uptime", "↺"},
	}

	var totalCPU float64
	var totalMemKB int64
	activeCount := 0

	for _, unit := range units {
		info := getProcessInfo(unit)

		pid := colorize(gray, "-")
		cpu := colorize(gray, "-")
		mem := colorize(gray, "-")
		uptime := colorize(gray, "-")

		if info.HasProcess {
			pid = strconv.Itoa(info.PID)
			cpu = fmt.Sprintf("%.1f%%", info.CPU)
			mem = info.MemoryStr
			totalCPU += info.CPU
			totalMemKB += info.MemoryKB
			activeCount++
		}
		if info.UptimeStr != "" {
			uptime = info.UptimeStr
		}

		tbl.Rows = append(tbl.Rows, []string{
			info.Name,
			statusColor(info.Status),
			pid,
			cpu,
			mem,
			uptime,
			strconv.Itoa(info.Restarts),
		})
	}

	fmt.Print(tbl.Render())
	fmt.Println()

	// Summary
	summary := []string{
		fmt.Sprintf("%s %d/%d online", colorize(bold, "apps:"), activeCount, len(units)),
		fmt.Sprintf("%s %.1f%%", colorize(bold, "cpu:"), totalCPU),
		fmt.Sprintf("%s %s", colorize(bold, "mem:"), formatMemory(totalMemKB)),
	}
	fmt.Println(" " + strings.Join(summary, "   "))
	fmt.Println()
	fmt.Println(colorize(gray, " updated: "+time.Now().Format("15:04:05")))
}
