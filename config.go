package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type AppConfig struct {
	Name        string            `json:"name"`
	Cmd         string            `json:"cmd"`
	Cwd         string            `json:"cwd"`
	Env         map[string]string `json:"env,omitempty"`
	MaxMemory   string            `json:"max_memory,omitempty"`
	Instances   int               `json:"instances,omitempty"`
	LogFile     string            `json:"log_file,omitempty"`
	ErrorFile   string            `json:"error_file,omitempty"`
	LogDir      string            `json:"log_dir,omitempty"`
	Interpreter string            `json:"interpreter,omitempty"`
	CronRestart string            `json:"cron_restart,omitempty"`
	AutoRestart *bool             `json:"auto_restart,omitempty"`
}

func (a *AppConfig) ShouldAutoRestart() bool {
	if a.AutoRestart == nil {
		return true
	}
	return *a.AutoRestart
}

type EcosystemConfig struct {
	Apps []AppConfig `json:"apps"`
}

func krunDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "krun")
}

func savedAppsPath() string {
	return filepath.Join(krunDir(), "apps.json")
}

func loadSavedApps() ([]AppConfig, error) {
	data, err := os.ReadFile(savedAppsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var apps []AppConfig
	if err := json.Unmarshal(data, &apps); err != nil {
		return nil, err
	}
	return apps, nil
}

func saveAppsToFile(apps []AppConfig) error {
	if err := os.MkdirAll(krunDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(savedAppsPath(), data, 0644)
}

func loadEcosystemFile(path string) (*EcosystemConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg EcosystemConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveAppConfig(app AppConfig) error {
	apps, _ := loadSavedApps()

	found := false
	for i, a := range apps {
		if a.Name == app.Name {
			apps[i] = app
			found = true
			break
		}
	}
	if !found {
		apps = append(apps, app)
	}

	return saveAppsToFile(apps)
}

func removeAppConfig(name string) error {
	apps, _ := loadSavedApps()

	var filtered []AppConfig
	for _, a := range apps {
		if a.Name != name {
			filtered = append(filtered, a)
		}
	}

	return saveAppsToFile(filtered)
}

func getAppConfig(name string) *AppConfig {
	apps, _ := loadSavedApps()
	for _, a := range apps {
		if a.Name == name {
			return &a
		}
	}
	return nil
}
