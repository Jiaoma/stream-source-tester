package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Name      string           `json:"name" yaml:"name"`
	Server    ServerConfig     `json:"server" yaml:"server"`
	Inputs    []InputConfig    `json:"inputs" yaml:"inputs"`
	Outputs   []OutputConfig   `json:"outputs" yaml:"outputs"`
	Mutations []MutationConfig `json:"mutations" yaml:"mutations"`
	Profiles  []ProfileConfig  `json:"profiles" yaml:"profiles"`
}

type ServerConfig struct {
	BindAddress string `json:"bindAddress" yaml:"bindAddress"`
	RTSPPort    int    `json:"rtspPort" yaml:"rtspPort"`
	RTPPortMin  int    `json:"rtpPortMin" yaml:"rtpPortMin"`
	RTPPortMax  int    `json:"rtpPortMax" yaml:"rtpPortMax"`
}

type InputConfig struct {
	Name     string            `json:"name" yaml:"name"`
	Kind     string            `json:"kind" yaml:"kind"`
	Codec    string            `json:"codec" yaml:"codec"`
	Location string            `json:"location" yaml:"location"`
	Options  map[string]string `json:"options" yaml:"options"`
}

type OutputConfig struct {
	Name    string            `json:"name" yaml:"name"`
	Kind    string            `json:"kind" yaml:"kind"`
	Target  string            `json:"target" yaml:"target"`
	Options map[string]string `json:"options" yaml:"options"`
}

type MutationConfig struct {
	Name    string            `json:"name" yaml:"name"`
	Kind    string            `json:"kind" yaml:"kind"`
	Enabled bool              `json:"enabled" yaml:"enabled"`
	Options map[string]string `json:"options" yaml:"options"`
}

type ProfileConfig struct {
	Name      string   `json:"name" yaml:"name"`
	Input     string   `json:"input" yaml:"input"`
	Output    string   `json:"output" yaml:"output"`
	Mutations []string `json:"mutations" yaml:"mutations"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("decode yaml config %q: %w", path, err)
		}
	case ".json":
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("decode json config %q: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("unsupported config extension %q", filepath.Ext(path))
	}

	if cfg.Name == "" {
		return nil, fmt.Errorf("config name must not be empty")
	}

	cfg.ApplyDefaults()
	return &cfg, nil
}
