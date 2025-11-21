package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the watch server configuration
type Config struct {
	Resources     []ResourceWatch `yaml:"resources"`
	DiscoverCRDs  bool            `yaml:"discoverCRDs"`
	StoragePath   string          `yaml:"storagePath"`
	RetentionDays int             `yaml:"retentionDays"`
	ServerPort    int             `yaml:"serverPort"`
	MaxQueryLimit int             `yaml:"maxQueryLimit"`
}

// ResourceWatch defines a Kubernetes resource type to watch
type ResourceWatch struct {
	Group      string `yaml:"group"`
	Version    string `yaml:"version"`
	Kind       string `yaml:"kind"`
	Plural     string `yaml:"plural"`
	Namespaced bool   `yaml:"namespaced"`
}

// LoadConfig reads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Set defaults
	if cfg.RetentionDays == 0 {
		cfg.RetentionDays = 14
	}
	if cfg.ServerPort == 0 {
		cfg.ServerPort = 8080
	}
	if cfg.MaxQueryLimit == 0 {
		cfg.MaxQueryLimit = 1000
	}
	if cfg.StoragePath == "" {
		cfg.StoragePath = "/data/watch-events"
	}

	return &cfg, nil
}

// DefaultConfig returns a configuration with common Kubernetes resources
func DefaultConfig() *Config {
	return &Config{
		DiscoverCRDs:  true,
		StoragePath:   "/data/watch-events",
		RetentionDays: 14,
		ServerPort:    8000,
		MaxQueryLimit: 1000,
		Resources: []ResourceWatch{
			{Group: "", Version: "v1", Kind: "Pod", Plural: "pods", Namespaced: true},
			{Group: "", Version: "v1", Kind: "Node", Plural: "nodes", Namespaced: false},
			{Group: "", Version: "v1", Kind: "Service", Plural: "services", Namespaced: true},
			{Group: "", Version: "v1", Kind: "ConfigMap", Plural: "configmaps", Namespaced: true},
			{Group: "", Version: "v1", Kind: "Secret", Plural: "secrets", Namespaced: true},
			{Group: "", Version: "v1", Kind: "PersistentVolumeClaim", Plural: "persistentvolumeclaims", Namespaced: true},
			{Group: "", Version: "v1", Kind: "PersistentVolume", Plural: "persistentvolumes", Namespaced: false},
			{Group: "", Version: "v1", Kind: "Event", Plural: "events", Namespaced: true},
			{Group: "", Version: "v1", Kind: "Namespace", Plural: "namespaces", Namespaced: false},
			{Group: "apps", Version: "v1", Kind: "Deployment", Plural: "deployments", Namespaced: true},
			{Group: "apps", Version: "v1", Kind: "ReplicaSet", Plural: "replicasets", Namespaced: true},
			{Group: "apps", Version: "v1", Kind: "StatefulSet", Plural: "statefulsets", Namespaced: true},
			{Group: "apps", Version: "v1", Kind: "DaemonSet", Plural: "daemonsets", Namespaced: true},
			{Group: "batch", Version: "v1", Kind: "Job", Plural: "jobs", Namespaced: true},
			{Group: "batch", Version: "v1", Kind: "CronJob", Plural: "cronjobs", Namespaced: true},
			{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress", Plural: "ingresses", Namespaced: true},
			{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy", Plural: "networkpolicies", Namespaced: true},
		},
	}
}
