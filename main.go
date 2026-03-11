package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"text/template"
)

const serviceTemplate = `[Unit]
Description=krun: {{.Name}}
After=default.target

[Service]
Type=simple
ExecStart={{.Cmd}}
WorkingDirectory={{.Cwd}}
Restart=always
RestartSec=3
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

[Install]
WantedBy=default.target
`

const servicePrefix = "krun-"

func serviceDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user")
}

func serviceName(name string) string {
	return servicePrefix + name + ".service"
}

func serviceFilePath(name string) string {
	return filepath.Join(serviceDir(), serviceName(name))
}

func systemctl(args ...string) error {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ensureLingerEnabled() {
	uid := fmt.Sprintf("%d", os.Getuid())
	lingerPath := filepath.Join("/var/lib/systemd/linger", os.Getenv("USER"))
	if _, err := os.Stat(lingerPath); err != nil {
		exec.Command("loginctl", "enable-linger", uid).Run()
	}
}

func cmdStart(name, command, cwd string) error {
	if command == "" {
		return fmt.Errorf("--cmd is required")
	}
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return fmt.Errorf("invalid cwd: %w", err)
	}

	if _, err := os.Stat(absCwd); err != nil {
		return fmt.Errorf("cwd does not exist: %s", absCwd)
	}

	dir := serviceDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create service dir: %w", err)
	}

	data := struct {
		Name string
		Cmd  string
		Cwd  string
	}{
		Name: name,
		Cmd:  command,
		Cwd:  absCwd,
	}

	tmpl, err := template.New("service").Parse(serviceTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(serviceFilePath(name))
	if err != nil {
		return fmt.Errorf("cannot create service file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return err
	}

	ensureLingerEnabled()

	if err := systemctl("daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload failed: %w", err)
	}
	if err := systemctl("enable", serviceName(name)); err != nil {
		return fmt.Errorf("enable failed: %w", err)
	}
	if err := systemctl("start", serviceName(name)); err != nil {
		return fmt.Errorf("start failed: %w", err)
	}

	fmt.Printf("started %s\n", name)
	return nil
}

func cmdStop(name string) error {
	if err := systemctl("stop", serviceName(name)); err != nil {
		return fmt.Errorf("stop failed: %w", err)
	}
	fmt.Printf("stopped %s\n", name)
	return nil
}

func cmdRestart(name string) error {
	if err := systemctl("restart", serviceName(name)); err != nil {
		return fmt.Errorf("restart failed: %w", err)
	}
	fmt.Printf("restarted %s\n", name)
	return nil
}

func cmdRemove(name string) error {
	systemctl("stop", serviceName(name))
	systemctl("disable", serviceName(name))

	path := serviceFilePath(name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove service file: %w", err)
	}

	systemctl("daemon-reload")
	fmt.Printf("removed %s\n", name)
	return nil
}

func cmdLogs(name string) error {
	cmd := exec.Command("journalctl", "--user", "-u", serviceName(name), "-f", "--no-pager", "-o", "cat")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func cmdList() error {
	dir := serviceDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("no apps managed by krun")
			return nil
		}
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tCOMMAND\tCWD")

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), servicePrefix) || !strings.HasSuffix(entry.Name(), ".service") {
			continue
		}

		name := strings.TrimPrefix(entry.Name(), servicePrefix)
		name = strings.TrimSuffix(name, ".service")

		status := getStatus(entry.Name())
		cmd, cwd := parseServiceFile(filepath.Join(dir, entry.Name()))

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, status, cmd, cwd)
	}

	w.Flush()
	return nil
}

func getStatus(unit string) string {
	out, err := exec.Command("systemctl", "--user", "is-active", unit).Output()
	if err != nil {
		return "inactive"
	}
	return strings.TrimSpace(string(out))
}

func parseServiceFile(path string) (cmd, cwd string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "?", "?"
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ExecStart=") {
			cmd = strings.TrimPrefix(line, "ExecStart=")
		}
		if strings.HasPrefix(line, "WorkingDirectory=") {
			cwd = strings.TrimPrefix(line, "WorkingDirectory=")
		}
	}
	if cmd == "" {
		cmd = "?"
	}
	if cwd == "" {
		cwd = "?"
	}
	return
}

func usage() {
	fmt.Println(`krun — minimal process manager via systemd

Usage:
  krun start <name> --cmd "<command>" [--cwd <path>]
  krun stop <name>
  krun restart <name>
  krun remove <name>
  krun logs <name>
  krun list`)
}

func parseFlags(args []string, flags map[string]*string) []string {
	var positional []string
	for i := 0; i < len(args); i++ {
		if ptr, ok := flags[args[i]]; ok {
			if i+1 < len(args) {
				*ptr = args[i+1]
				i++
				continue
			}
		}
		positional = append(positional, args[i])
	}
	return positional
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	subcmd := os.Args[1]
	rest := os.Args[2:]

	switch subcmd {
	case "start":
		var cmdFlag, cwdFlag string
		pos := parseFlags(rest, map[string]*string{
			"--cmd": &cmdFlag,
			"--cwd": &cwdFlag,
		})
		if len(pos) < 1 {
			fmt.Fprintln(os.Stderr, "usage: krun start <name> --cmd \"<command>\" [--cwd <path>]")
			os.Exit(1)
		}
		if err := cmdStart(pos[0], cmdFlag, cwdFlag); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "stop":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: krun stop <name>")
			os.Exit(1)
		}
		if err := cmdStop(rest[0]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "restart":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: krun restart <name>")
			os.Exit(1)
		}
		if err := cmdRestart(rest[0]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "remove":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: krun remove <name>")
			os.Exit(1)
		}
		if err := cmdRemove(rest[0]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "logs":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "usage: krun logs <name>")
			os.Exit(1)
		}
		if err := cmdLogs(rest[0]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "list":
		if err := cmdList(); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", subcmd)
		usage()
		os.Exit(1)
	}
}
