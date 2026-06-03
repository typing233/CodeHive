package config

import (
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTP     HTTPConfig     `yaml:"http"`
	SSH      SSHConfig      `yaml:"ssh"`
	Database DatabaseConfig `yaml:"database"`
	Git      GitConfig      `yaml:"git"`
	Session  SessionConfig  `yaml:"session"`
	Log      LogConfig      `yaml:"log"`
	Packages PackageConfig  `yaml:"packages"`
}

type HTTPConfig struct {
	Port    int    `yaml:"port"`
	BaseURL string `yaml:"base_url"`
}

type SSHConfig struct {
	Port        int    `yaml:"port"`
	HostKeyPath string `yaml:"host_key"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type GitConfig struct {
	DataDir string `yaml:"data_dir"`
}

type SessionConfig struct {
	Secret string `yaml:"secret"`
	MaxAge int    `yaml:"max_age"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}

type PackageConfig struct {
	DataDir        string `yaml:"data_dir"`
	MaxSizeMB      int    `yaml:"max_size_mb"`
	ProxyEnabled   bool   `yaml:"proxy_enabled"`
	NPMUpstream    string `yaml:"npm_upstream"`
	DockerUpstream string `yaml:"docker_upstream"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		HTTP:    HTTPConfig{Port: 3000, BaseURL: "http://localhost:3000"},
		SSH:     SSHConfig{Port: 2222, HostKeyPath: "./data/ssh_host_ed25519_key"},
		Git:     GitConfig{DataDir: "./data/repos"},
		Session: SessionConfig{MaxAge: 604800},
		Log:     LogConfig{Level: "info"},
		Packages: PackageConfig{
			DataDir:        "./data/packages",
			MaxSizeMB:      500,
			ProxyEnabled:   true,
			NPMUpstream:    "https://registry.npmjs.org",
			DockerUpstream: "https://registry-1.docker.io",
		},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("CODEHIVE_HTTP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.HTTP.Port = p
		}
	}
	if v := os.Getenv("CODEHIVE_HTTP_BASE_URL"); v != "" {
		cfg.HTTP.BaseURL = v
	}
	if v := os.Getenv("CODEHIVE_SSH_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.SSH.Port = p
		}
	}
	if v := os.Getenv("CODEHIVE_SSH_HOST_KEY"); v != "" {
		cfg.SSH.HostKeyPath = v
	}
	if v := os.Getenv("CODEHIVE_DATABASE_DSN"); v != "" {
		cfg.Database.DSN = v
	}
	if v := os.Getenv("CODEHIVE_GIT_DATA_DIR"); v != "" {
		cfg.Git.DataDir = v
	}
	if v := os.Getenv("CODEHIVE_SESSION_SECRET"); v != "" {
		cfg.Session.Secret = v
	}
	if v := os.Getenv("CODEHIVE_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("CODEHIVE_PACKAGES_DATA_DIR"); v != "" {
		cfg.Packages.DataDir = v
	}
	if v := os.Getenv("CODEHIVE_PACKAGES_NPM_UPSTREAM"); v != "" {
		cfg.Packages.NPMUpstream = v
	}
	if v := os.Getenv("CODEHIVE_PACKAGES_DOCKER_UPSTREAM"); v != "" {
		cfg.Packages.DockerUpstream = v
	}
}
