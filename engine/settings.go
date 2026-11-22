package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Options 一次解密任务的配置。
type Options struct {
	InputDir      string   `json:"inputDir"`
	OutputDir     string   `json:"outputDir"`
	Recursive     bool     `json:"recursive"`
	SkipExisting  bool     `json:"skipExisting"`
	KeepStructure bool     `json:"keepStructure"`
	AutoFixExt    bool     `json:"autoFixExt"`
	IncludeIDs    []string `json:"includeIDs,omitempty"`
}

// Settings 应用级设置（记忆上次的选择）。
type Settings struct {
	OutputDir      string `json:"outputDir"`
	Recursive      bool   `json:"recursive"`
	SkipExisting   bool   `json:"skipExisting"`
	KeepStructure  bool   `json:"keepStructure"`
	AutoFixExt     bool   `json:"autoFixExt"`
	OpenOnComplete bool   `json:"openOnComplete"`
	Theme          string `json:"theme"`
}

func defaultSettings() *Settings {
	return &Settings{
		Recursive:     true,
		SkipExisting:  true,
		KeepStructure: true,
		AutoFixExt:    true,
		Theme:         "dark",
	}
}

func settingsPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "MusicDecryptorDesktop", "settings.json")
}

func LoadSettings() *Settings {
	s := defaultSettings()
	path := settingsPath()
	if path == "" {
		return s
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, s)
	}
	return NormalizeSettings(s)
}

func SaveSettings(s *Settings) error {
	path := settingsPath()
	if path == "" {
		return os.ErrNotExist
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func NormalizeSettings(s *Settings) *Settings {
	d := defaultSettings()
	if s == nil {
		return d
	}
	if s.Theme != "dark" && s.Theme != "light" {
		s.Theme = d.Theme
	}
	s.OutputDir = NormalizeUserPath(s.OutputDir)
	return s
}

func NormalizeOptions(o *Options) {
	o.InputDir = NormalizeUserPath(o.InputDir)
	o.OutputDir = NormalizeUserPath(o.OutputDir)
}
