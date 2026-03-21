// Package config handles Docker target configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

const defaultDockerHost = "unix:///var/run/docker.sock"

// TargetConfig holds Docker target settings from the forma file.
type TargetConfig struct {
	Type string `json:"Type"`
	Host string `json:"Host"`
}

// ParseTargetConfig deserializes target configuration from JSON.
func ParseTargetConfig(data json.RawMessage) (*TargetConfig, error) {
	var cfg TargetConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid target config: %w", err)
	}
	return &cfg, nil
}

// DockerHost returns the Docker host to connect to.
// Priority: config Host > DOCKER_HOST env var > default socket.
func (c *TargetConfig) DockerHost() string {
	if c.Host != "" {
		return c.Host
	}
	if h := os.Getenv("DOCKER_HOST"); h != "" {
		return h
	}
	return defaultDockerHost
}
