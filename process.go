package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type ProcessInfo struct {
	Name       string
	Status     string
	PID        int
	CPU        float64
	MemoryKB   int64
	MemoryStr  string
	Uptime     time.Duration
	UptimeStr  string
	Restarts   int
	Cmd        string
	Cwd        string
	Env        map[string]string
	HasProcess bool
}

func getProcessInfo(unit string) ProcessInfo {
	name := unitToAppName(unit)

	info := ProcessInfo{
		Name: name,
	}

	// Get status
	state := getUnitProperty(unit, "ActiveState")
	info.Status = state

	// Get PID
	pidStr := getUnitProperty(unit, "MainPID")
	pid, _ := strconv.Atoi(pidStr)
	info.PID = pid

	// Get restart count
	restartStr := getUnitProperty(unit, "NRestarts")
	info.Restarts, _ = strconv.Atoi(restartStr)

	// Get uptime from ActiveEnterTimestamp
	if state == "active" {
		ts := getUnitProperty(unit, "ActiveEnterTimestamp")
		if ts != "" && ts != "n/a" {
			if t, err := parseSystemdTimestamp(ts); err == nil {
				info.Uptime = time.Since(t)
				info.UptimeStr = formatDuration(info.Uptime)
			}
		}
	}

	// Get CPU and memory from ps if PID is valid
	if pid > 0 {
		info.HasProcess = true
		cpu, mem := getProcessStats(pid)
		info.CPU = cpu
		info.MemoryKB = mem
		info.MemoryStr = formatMemory(mem)
	}

	// Get command and cwd from service file
	info.Cmd, info.Cwd, info.Env = parseServiceFileFields(unitFilePath(name))

	return info
}

func getProcessStats(pid int) (cpu float64, memKB int64) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "%cpu=,rss=", "--no-headers").Output()
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) >= 2 {
		cpu, _ = strconv.ParseFloat(fields[0], 64)
		mem, _ := strconv.ParseInt(fields[1], 10, 64)
		memKB = mem
	}
	return
}

func parseSystemdTimestamp(ts string) (time.Time, error) {
	// systemd timestamps look like: "Mon 2024-01-15 10:30:00 UTC"
	// or "Mon 2024-01-15 10:30:00 CET"
	// Try multiple formats
	formats := []string{
		"Mon 2006-01-02 15:04:05 MST",
		"Mon 2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05 MST",
		"2006-01-02 15:04:05 -0700",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, ts); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse timestamp: %s", ts)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm", h, m)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd %dh", days, hours)
}

func formatMemory(kb int64) string {
	if kb == 0 {
		return "-"
	}
	if kb < 1024 {
		return fmt.Sprintf("%d KB", kb)
	}
	mb := float64(kb) / 1024.0
	if mb < 1024 {
		return fmt.Sprintf("%.1f MB", mb)
	}
	gb := mb / 1024.0
	return fmt.Sprintf("%.1f GB", gb)
}
