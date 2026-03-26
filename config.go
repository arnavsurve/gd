package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

type config struct {
	DarkTheme    string  `json:"darkTheme"`
	LightTheme   string  `json:"lightTheme"`
	SidebarRatio float64 `json:"sidebarRatio,omitempty"`
	Semantic     *bool   `json:"semantic,omitempty"`
	TabWidth     int     `json:"tabWidth,omitempty"`
}

var cfg config

func configPath() string {
	if runtime.GOOS == "windows" {
		dir, err := os.UserConfigDir()
		if err != nil {
			dir = "."
		}
		return filepath.Join(dir, "gd", "config.json")
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "gd", "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "gd", "config.json")
}

func loadConfig() {
	cfg = config{
		DarkTheme:  darkPalette.chromaStyle,
		LightTheme: lightPalette.chromaStyle,
	}
	data, err := os.ReadFile(configPath())
	if err != nil {
		return
	}
	var loaded config
	if err := json.Unmarshal(data, &loaded); err != nil {
		return
	}
	if loaded.DarkTheme != "" {
		cfg.DarkTheme = loaded.DarkTheme
		darkPalette.chromaStyle = loaded.DarkTheme
	}
	if loaded.LightTheme != "" {
		cfg.LightTheme = loaded.LightTheme
		lightPalette.chromaStyle = loaded.LightTheme
	}
	if loaded.SidebarRatio > 0 {
		cfg.SidebarRatio = loaded.SidebarRatio
	}
}

func saveConfig() {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(configPath())
	os.MkdirAll(dir, 0o755)
	os.WriteFile(configPath(), data, 0o644)
}
