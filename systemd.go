package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const servicePrefix = "krun-"

const serviceTemplateSrc = `[Unit]
Description=krun: {{.Name}}
After=default.target

[Service]
Type=simple
{{- if .LogDir}}
ExecStart=/bin/bash -c 'mkdir -p {{.LogDir}} && exec {{.ExecCmd}} >>{{.LogDir}}/{{.Name}}-$(date +%%Y-%%m-%%d).log 2>>{{.LogDir}}/{{.Name}}-$(date +%%Y-%%m-%%d)-error.log'
{{- else}}
ExecStart=/bin/bash -c 'exec {{.ExecCmd}}'
{{- end}}
WorkingDirectory={{.Cwd}}
{{- if .AutoRestart}}
Restart=always
RestartSec=3
{{- else}}
Restart=no
{{- end}}
{{- range .EnvLines}}
Environment="{{.}}"
{{- end}}
{{- if .MaxMemory}}
MemoryMax={{.MaxMemory}}
{{- end}}
{{- if and .LogFile (not .LogDir)}}
StandardOutput=append:{{.LogFile}}
{{- end}}
{{- if and .ErrorFile (not .LogDir)}}
StandardError=append:{{.ErrorFile}}
{{- end}}
Environment="PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

[Install]
WantedBy=default.target
`

const timerTemplateSrc = `[Unit]
Description=krun restart timer: {{.Name}}

[Timer]
OnCalendar={{.Schedule}}
Persistent=true

[Install]
WantedBy=timers.target
`

const restartServiceTemplateSrc = `[Unit]
Description=krun restart trigger: {{.Name}}

[Service]
Type=oneshot
ExecStart=/bin/systemctl --user restart {{.ServiceUnit}}
`

type serviceData struct {
	Name        string
	ExecCmd     string
	Cwd         string
	AutoRestart bool
	EnvLines    []string
	MaxMemory   string
	LogFile     string
	ErrorFile   string
	LogDir      string
}

func userServiceDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user")
}

func unitName(name string) string {
	return servicePrefix + name + ".service"
}

func unitFilePath(name string) string {
	return filepath.Join(userServiceDir(), unitName(name))
}

func timerName(name string) string {
	return servicePrefix + name + "-restart.timer"
}

func timerFilePath(name string) string {
	return filepath.Join(userServiceDir(), timerName(name))
}

func restartUnitName(name string) string {
	return servicePrefix + name + "-restart.service"
}

func restartUnitFilePath(name string) string {
	return filepath.Join(userServiceDir(), restartUnitName(name))
}

func systemctl(args ...string) error {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func systemctlQuiet(args ...string) error {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	return cmd.Run()
}

func systemctlOutput(args ...string) (string, error) {
	out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getUnitProperty(unit, prop string) string {
	val, _ := systemctlOutput("show", "-p", prop, "--value", unit)
	return val
}

func ensureLinger() {
	user := os.Getenv("USER")
	if user == "" {
		return
	}
	lingerPath := filepath.Join("/var/lib/systemd/linger", user)
	if _, err := os.Stat(lingerPath); err != nil {
		exec.Command("loginctl", "enable-linger", user).Run()
	}
}

func disableLinger() {
	user := os.Getenv("USER")
	if user != "" {
		exec.Command("loginctl", "disable-linger", user).Run()
	}
}

func daemonReload() error {
	return systemctlQuiet("daemon-reload")
}

func generateServiceFile(app AppConfig) error {
	dir := userServiceDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create service dir: %w", err)
	}

	execCmd := app.Cmd
	if app.Interpreter != "" {
		execCmd = app.Interpreter + " " + app.Cmd
	}
	// Escape single quotes in the command for bash -c 'exec ...'
	execCmd = strings.ReplaceAll(execCmd, "'", "'\"'\"'")

	var envLines []string
	for k, v := range app.Env {
		envLines = append(envLines, k+"="+v)
	}

	data := serviceData{
		Name:        app.Name,
		ExecCmd:     execCmd,
		Cwd:         app.Cwd,
		AutoRestart: app.ShouldAutoRestart(),
		EnvLines:    envLines,
		MaxMemory:   app.MaxMemory,
		LogFile:     app.LogFile,
		ErrorFile:   app.ErrorFile,
		LogDir:      app.LogDir,
	}

	tmpl, err := template.New("service").Parse(serviceTemplateSrc)
	if err != nil {
		return err
	}

	f, err := os.Create(unitFilePath(app.Name))
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

func generateTimerFiles(app AppConfig) error {
	if app.CronRestart == "" {
		return nil
	}

	dir := userServiceDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Generate restart service
	restartTmpl, _ := template.New("restart").Parse(restartServiceTemplateSrc)
	rf, err := os.Create(restartUnitFilePath(app.Name))
	if err != nil {
		return err
	}
	restartTmpl.Execute(rf, struct {
		Name        string
		ServiceUnit string
	}{app.Name, unitName(app.Name)})
	rf.Close()

	// Generate timer
	timerTmpl, _ := template.New("timer").Parse(timerTemplateSrc)
	tf, err := os.Create(timerFilePath(app.Name))
	if err != nil {
		return err
	}
	timerTmpl.Execute(tf, struct {
		Name     string
		Schedule string
	}{app.Name, app.CronRestart})
	tf.Close()

	return nil
}

func removeTimerFiles(name string) {
	systemctlQuiet("stop", timerName(name))
	systemctlQuiet("disable", timerName(name))
	os.Remove(timerFilePath(name))
	os.Remove(restartUnitFilePath(name))
}

func listKrunUnits() []string {
	dir := userServiceDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var units []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, servicePrefix) && strings.HasSuffix(n, ".service") && !strings.Contains(n, "-restart.service") {
			units = append(units, n)
		}
	}
	return units
}

func unitToAppName(unit string) string {
	name := strings.TrimPrefix(unit, servicePrefix)
	name = strings.TrimSuffix(name, ".service")
	return name
}

func parseServiceFileFields(path string) (cmd, cwd string, env map[string]string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "?", "?", nil
	}
	env = make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "ExecStart=/bin/bash -c 'exec "):
			cmd = strings.TrimPrefix(line, "ExecStart=/bin/bash -c 'exec ")
			cmd = strings.TrimSuffix(cmd, "'")
			cmd = strings.ReplaceAll(cmd, "'\"'\"'", "'")
		case strings.HasPrefix(line, "ExecStart="):
			if cmd == "" {
				cmd = strings.TrimPrefix(line, "ExecStart=")
			}
		case strings.HasPrefix(line, "WorkingDirectory="):
			cwd = strings.TrimPrefix(line, "WorkingDirectory=")
		case strings.HasPrefix(line, "Environment=\"") && !strings.Contains(line, "PATH="):
			val := strings.TrimPrefix(line, "Environment=\"")
			val = strings.TrimSuffix(val, "\"")
			if idx := strings.Index(val, "="); idx > 0 {
				env[val[:idx]] = val[idx+1:]
			}
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
