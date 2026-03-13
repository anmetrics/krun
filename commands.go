package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func cmdStart(app AppConfig) error {
	if app.Cmd == "" {
		return fmt.Errorf("--cmd is required")
	}
	if app.Name == "" {
		return fmt.Errorf("app name is required")
	}

	if app.Cwd == "" {
		app.Cwd, _ = os.Getwd()
	}
	absCwd, err := filepath.Abs(app.Cwd)
	if err != nil {
		return fmt.Errorf("invalid cwd: %w", err)
	}
	if _, err := os.Stat(absCwd); err != nil {
		return fmt.Errorf("cwd does not exist: %s", absCwd)
	}
	app.Cwd = absCwd

	instances := app.Instances
	if instances <= 1 {
		instances = 1
	}

	for i := 0; i < instances; i++ {
		appCopy := app
		if instances > 1 {
			appCopy.Name = fmt.Sprintf("%s-%d", app.Name, i)
		}

		if err := generateServiceFile(appCopy); err != nil {
			return fmt.Errorf("generate service: %w", err)
		}

		if appCopy.CronRestart != "" {
			if err := generateTimerFiles(appCopy); err != nil {
				return fmt.Errorf("generate timer: %w", err)
			}
		}
	}

	ensureLinger()

	if err := daemonReload(); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}

	for i := 0; i < instances; i++ {
		name := app.Name
		if instances > 1 {
			name = fmt.Sprintf("%s-%d", app.Name, i)
		}

		if err := systemctl("enable", unitName(name)); err != nil {
			return fmt.Errorf("enable %s: %w", name, err)
		}
		if err := systemctl("start", unitName(name)); err != nil {
			return fmt.Errorf("start %s: %w", name, err)
		}

		if app.CronRestart != "" {
			systemctl("enable", timerName(name))
			systemctl("start", timerName(name))
		}
	}

	// Save config for resurrect
	if err := saveAppConfig(app); err != nil {
		printError("warning: could not save config: %v", err)
	}

	if instances > 1 {
		printSuccess("started %s (%d instances)", app.Name, instances)
	} else {
		printSuccess("started %s", app.Name)
	}
	return nil
}

func cmdStartEcosystem(path string) error {
	cfg, err := loadEcosystemFile(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if len(cfg.Apps) == 0 {
		return fmt.Errorf("no apps defined in %s", path)
	}

	for _, app := range cfg.Apps {
		if app.Cwd != "" && !filepath.IsAbs(app.Cwd) {
			app.Cwd = filepath.Join(filepath.Dir(path), app.Cwd)
		}
		if err := cmdStart(app); err != nil {
			printError("%s: %v", app.Name, err)
		}
	}
	return nil
}

func cmdStop(name string) error {
	if name == "all" {
		return forEachUnit(func(unit string) error {
			systemctlQuiet("stop", unit)
			appName := unitToAppName(unit)
			printSuccess("stopped %s", appName)
			return nil
		})
	}

	units := resolveUnits(name)
	if len(units) == 0 {
		return fmt.Errorf("app %q not found", name)
	}
	for _, u := range units {
		if err := systemctl("stop", u); err != nil {
			printError("stop %s: %v", unitToAppName(u), err)
		} else {
			printSuccess("stopped %s", unitToAppName(u))
		}
	}
	return nil
}

func cmdRestart(name string) error {
	if name == "all" {
		return forEachUnit(func(unit string) error {
			systemctlQuiet("restart", unit)
			appName := unitToAppName(unit)
			printSuccess("restarted %s", appName)
			return nil
		})
	}

	units := resolveUnits(name)
	if len(units) == 0 {
		return fmt.Errorf("app %q not found", name)
	}
	for _, u := range units {
		if err := systemctl("restart", u); err != nil {
			printError("restart %s: %v", unitToAppName(u), err)
		} else {
			printSuccess("restarted %s", unitToAppName(u))
		}
	}
	return nil
}

func cmdReload(name string) error {
	if name == "all" {
		return forEachUnit(func(unit string) error {
			systemctlQuiet("reload-or-restart", unit)
			appName := unitToAppName(unit)
			printSuccess("reloaded %s", appName)
			return nil
		})
	}

	units := resolveUnits(name)
	if len(units) == 0 {
		return fmt.Errorf("app %q not found", name)
	}
	for _, u := range units {
		if err := systemctl("reload-or-restart", u); err != nil {
			printError("reload %s: %v", unitToAppName(u), err)
		} else {
			printSuccess("reloaded %s", unitToAppName(u))
		}
	}
	return nil
}

func cmdRemove(name string) error {
	if name == "all" {
		return forEachUnit(func(unit string) error {
			appName := unitToAppName(unit)
			systemctlQuiet("stop", unit)
			systemctlQuiet("disable", unit)
			removeTimerFiles(appName)
			os.Remove(unitFilePath(appName))
			removeAppConfig(appName)
			printSuccess("removed %s", appName)
			return nil
		})
	}

	units := resolveUnits(name)
	if len(units) == 0 {
		return fmt.Errorf("app %q not found", name)
	}
	for _, u := range units {
		appName := unitToAppName(u)
		systemctlQuiet("stop", u)
		systemctlQuiet("disable", u)
		removeTimerFiles(appName)
		os.Remove(unitFilePath(appName))
		printSuccess("removed %s", appName)
	}
	removeAppConfig(name)
	daemonReload()
	return nil
}

func cmdList() error {
	units := listKrunUnits()
	if len(units) == 0 {
		fmt.Println(colorize(gray, "no apps managed by krun"))
		return nil
	}

	tbl := &Table{
		Headers: []string{"id", "name", "status", "pid", "cpu", "mem", "uptime", "↺"},
	}

	for i, unit := range units {
		info := getProcessInfo(unit)

		pid := colorize(gray, "-")
		cpu := colorize(gray, "-")
		mem := colorize(gray, "-")
		uptime := colorize(gray, "-")

		if info.HasProcess {
			pid = strconv.Itoa(info.PID)
			cpu = fmt.Sprintf("%.1f%%", info.CPU)
			mem = info.MemoryStr
		}
		if info.UptimeStr != "" {
			uptime = info.UptimeStr
		}

		tbl.Rows = append(tbl.Rows, []string{
			strconv.Itoa(i),
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
	return nil
}

func cmdInfo(name string) error {
	units := resolveUnits(name)
	if len(units) == 0 {
		return fmt.Errorf("app %q not found", name)
	}

	for _, unit := range units {
		info := getProcessInfo(unit)

		pid := "-"
		cpu := "-"
		mem := "-"
		uptime := "-"

		if info.HasProcess {
			pid = strconv.Itoa(info.PID)
			cpu = fmt.Sprintf("%.1f%%", info.CPU)
			mem = info.MemoryStr
		}
		if info.UptimeStr != "" {
			uptime = info.UptimeStr
		}

		fmt.Printf("\n %s %s\n", colorize(bold, "Describing process:"), colorize(cyan, info.Name))
		fmt.Println(strings.Repeat("─", 50))

		pairs := [][]string{
			{"status", statusColor(info.Status)},
			{"name", info.Name},
			{"pid", pid},
			{"cpu", cpu},
			{"memory", mem},
			{"uptime", uptime},
			{"restarts", strconv.Itoa(info.Restarts)},
			{"command", info.Cmd},
			{"cwd", info.Cwd},
			{"service", unit},
			{"service file", unitFilePath(info.Name)},
		}

		if len(info.Env) > 0 {
			envStr := ""
			for k, v := range info.Env {
				envStr += k + "=" + v + " "
			}
			pairs = append(pairs, []string{"env", strings.TrimSpace(envStr)})
		}

		// Check for cron restart timer
		timerState := getUnitProperty(timerName(info.Name), "ActiveState")
		if timerState == "active" {
			schedule := getUnitProperty(timerName(info.Name), "TimersCalendar")
			pairs = append(pairs, []string{"cron restart", schedule})
		}

		fmt.Print(renderKeyValue(pairs))
		fmt.Println()
	}
	return nil
}

func cmdLogs(name string, lines int, noStream bool) error {
	args := []string{"--user", "-u", unitName(name), "--no-pager", "-o", "cat"}

	if lines > 0 {
		args = append(args, "-n", strconv.Itoa(lines))
	} else {
		args = append(args, "-n", "50")
	}

	if !noStream {
		args = append(args, "-f")
	}

	cmd := exec.Command("journalctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func cmdFlush(name string) error {
	if name == "" || name == "all" {
		units := listKrunUnits()
		for _, u := range units {
			n := unitToAppName(u)
			flushAppLogs(n)
		}
		printSuccess("flushed all logs")
		return nil
	}

	flushAppLogs(name)
	printSuccess("flushed logs for %s", name)
	return nil
}

func flushAppLogs(name string) {
	exec.Command("journalctl", "--user", "--rotate", "-u", unitName(name)).Run()
	exec.Command("journalctl", "--user", "--vacuum-time=1s", "-u", unitName(name)).Run()
}

func cmdSave() error {
	units := listKrunUnits()
	var apps []AppConfig

	for _, unit := range units {
		name := unitToAppName(unit)
		// Try to get from saved config first
		if saved := getAppConfig(name); saved != nil {
			apps = append(apps, *saved)
			continue
		}
		// Otherwise parse from service file
		cmd, cwd, env := parseServiceFileFields(unitFilePath(name))
		apps = append(apps, AppConfig{
			Name: name,
			Cmd:  cmd,
			Cwd:  cwd,
			Env:  env,
		})
	}

	if err := saveAppsToFile(apps); err != nil {
		return err
	}
	printSuccess("saved %d app(s) to %s", len(apps), savedAppsPath())
	return nil
}

func cmdResurrect() error {
	apps, err := loadSavedApps()
	if err != nil {
		return fmt.Errorf("load saved apps: %w", err)
	}
	if len(apps) == 0 {
		return fmt.Errorf("no saved apps found. Run 'krun save' first")
	}

	printInfo("resurrecting %d app(s)...", len(apps))
	for _, app := range apps {
		if err := cmdStart(app); err != nil {
			printError("%s: %v", app.Name, err)
		}
	}
	return nil
}

func cmdStartup() error {
	ensureLinger()
	printSuccess("startup enabled (linger activated)")
	printInfo("apps with 'enable' will auto-start on boot")
	return nil
}

func cmdUnstartup() error {
	disableLinger()
	printSuccess("startup disabled (linger deactivated)")
	return nil
}

func cmdEnv(name string) error {
	_, _, env := parseServiceFileFields(unitFilePath(name))
	if len(env) == 0 {
		fmt.Println(colorize(gray, "no environment variables set"))
		return nil
	}

	fmt.Printf("\n %s %s\n", colorize(bold, "Environment for"), colorize(cyan, name))
	fmt.Println(strings.Repeat("─", 40))
	for k, v := range env {
		fmt.Printf("  %s = %s\n", colorize(bold, k), v)
	}
	fmt.Println()
	return nil
}

func cmdReset(name string) error {
	if err := systemctl("reset-failed", unitName(name)); err != nil {
		// Not all versions support this, try restart
		return err
	}
	printSuccess("reset counters for %s", name)
	return nil
}

func cmdShowConfig(name string) error {
	app := getAppConfig(name)
	if app == nil {
		// Try to reconstruct from service file
		cmd, cwd, env := parseServiceFileFields(unitFilePath(name))
		app = &AppConfig{
			Name: name,
			Cmd:  cmd,
			Cwd:  cwd,
			Env:  env,
		}
	}

	data, _ := json.MarshalIndent(app, "", "  ")
	fmt.Println(string(data))
	return nil
}

// resolveUnits finds all units matching a name (handles multi-instance)
func resolveUnits(name string) []string {
	// Check exact match first
	unit := unitName(name)
	if _, err := os.Stat(unitFilePath(name)); err == nil {
		return []string{unit}
	}

	// Check for instance pattern: name-0, name-1, ...
	var matches []string
	units := listKrunUnits()
	prefix := name + "-"
	for _, u := range units {
		appName := unitToAppName(u)
		if appName == name || strings.HasPrefix(appName, prefix) {
			// Verify the suffix is a number (instance)
			suffix := strings.TrimPrefix(appName, prefix)
			if _, err := strconv.Atoi(suffix); err == nil || appName == name {
				matches = append(matches, u)
			}
		}
	}
	return matches
}

// resolveName converts an index (e.g. "0", "1") to app name, or returns as-is
func resolveName(nameOrIndex string) string {
	if nameOrIndex == "all" {
		return "all"
	}
	idx, err := strconv.Atoi(nameOrIndex)
	if err != nil {
		return nameOrIndex
	}
	units := listKrunUnits()
	if idx < 0 || idx >= len(units) {
		printError("index %d out of range (0-%d)", idx, len(units)-1)
		os.Exit(1)
	}
	return unitToAppName(units[idx])
}

func forEachUnit(fn func(string) error) error {
	units := listKrunUnits()
	if len(units) == 0 {
		return fmt.Errorf("no apps managed by krun")
	}
	for _, u := range units {
		if err := fn(u); err != nil {
			printError("%v", err)
		}
	}
	daemonReload()
	return nil
}
