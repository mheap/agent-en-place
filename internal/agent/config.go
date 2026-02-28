package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// ImageConfig represents the configuration file structure
type ImageConfig struct {
	Tools               map[string]ToolConfigEntry `yaml:"tools"`
	Agents              map[string]AgentConfig     `yaml:"agents"`
	Image               ImageSettings              `yaml:"image"`
	Mise                MiseSettings               `yaml:"mise"`
	ImageCustomizations ImageCustomizations        `yaml:"image_customizations"`
}

// ToolConfigEntry defines a tool with version and dependencies
type ToolConfigEntry struct {
	Version            string   `yaml:"version"`
	Depends            string   `yaml:"depends"`
	AdditionalPackages []string `yaml:"additionalPackages"`
}

// AgentConfig defines an agent's configuration
type AgentConfig struct {
	PackageName      string   `yaml:"packageName"`
	Command          string   `yaml:"command"`
	ConfigDir        string   `yaml:"configDir"`
	AdditionalMounts []string `yaml:"additionalMounts"`
	EnvVars          []string `yaml:"envVars"`
	Depends          []string `yaml:"depends"`
}

// ImageSettings defines Docker image configuration
type ImageSettings struct {
	Base     string   `yaml:"base"`
	Packages []string `yaml:"packages"`
}

// MiseSettings defines mise installation commands and environment variables
type MiseSettings struct {
	Install []string       `yaml:"install"`
	Env     map[string]any `yaml:"env"`
}

// ImageCustomization represents a single customization operation (JSON patch style)
type ImageCustomization struct {
	Op    string `yaml:"op"`    // "add" or "remove"
	Value string `yaml:"value"` // The value to add or remove
}

// ImageCustomizations defines customization operations for the image
type ImageCustomizations struct {
	Packages []ImageCustomization `yaml:"packages"`
}

// loadDefaultConfig parses the embedded default config
func loadDefaultConfig(data []byte) (*ImageConfig, error) {
	var cfg ImageConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse default config: %w", err)
	}
	if cfg.Tools == nil {
		cfg.Tools = make(map[string]ToolConfigEntry)
	}
	if cfg.Agents == nil {
		cfg.Agents = make(map[string]AgentConfig)
	}
	return &cfg, nil
}

// loadConfigFile loads a config from a specific path
func loadConfigFile(path string) (*ImageConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	var cfg ImageConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return &cfg, nil
}

// getXDGConfigPath returns the path to the XDG config file
// Uses $XDG_CONFIG_HOME if set, otherwise ~/.config
func getXDGConfigPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "agent-en-place.yaml")
}

// LoadMergedConfig loads the default config and merges with user configs
// Config precedence (later configs override earlier):
// 1. Embedded default config
// 2. XDG config ($XDG_CONFIG_HOME/agent-en-place.yaml or ~/.config/agent-en-place.yaml)
// 3. Project-local config (./.agent-en-place.yaml)
// 4. Explicit config path (--config flag)
// After merging, image_customizations are applied to modify packages
func LoadMergedConfig(defaultConfigData []byte, configPath string) (*ImageConfig, error) {
	base, err := loadDefaultConfig(defaultConfigData)
	if err != nil {
		return nil, err
	}

	// Load XDG config
	xdgPath := getXDGConfigPath()
	if xdgPath != "" {
		xdgConfig, err := loadConfigFile(xdgPath)
		if err != nil {
			return nil, err
		}
		if xdgConfig != nil {
			base = mergeConfigs(base, xdgConfig)
		}
	}

	// Load project-local config
	localConfig, err := loadConfigFile(".agent-en-place.yaml")
	if err != nil {
		return nil, err
	}
	if localConfig != nil {
		base = mergeConfigs(base, localConfig)
	}

	// Load explicit config path if provided
	if configPath != "" {
		explicitConfig, err := loadConfigFile(configPath)
		if err != nil {
			return nil, err
		}
		if explicitConfig == nil {
			return nil, fmt.Errorf("config file not found: %s", configPath)
		}
		base = mergeConfigs(base, explicitConfig)
	}

	// Apply image customizations after all configs are merged
	base = applyImageCustomizations(base)

	return base, nil
}

// mergeConfigs deep merges user config into base config
// - Tools: user adds/overrides individual tools
// - Agents: user adds/overrides individual agents
// - Image.Base: user replaces if set
// - Image.Packages: user replaces entirely if set
// - Mise.Install: user replaces entirely if set
// - ImageCustomizations: user customizations are accumulated
func mergeConfigs(base, user *ImageConfig) *ImageConfig {
	result := &ImageConfig{
		Tools:               make(map[string]ToolConfigEntry),
		Agents:              make(map[string]AgentConfig),
		Image:               base.Image,
		Mise:                base.Mise,
		ImageCustomizations: base.ImageCustomizations,
	}

	// Copy base tools
	for k, v := range base.Tools {
		result.Tools[k] = v
	}
	// Merge user tools (override/add)
	for k, v := range user.Tools {
		result.Tools[k] = v
	}

	// Copy base agents
	for k, v := range base.Agents {
		result.Agents[k] = v
	}
	// Merge user agents (override/add)
	for k, v := range user.Agents {
		result.Agents[k] = v
	}

	// Replace image base if user specified
	if user.Image.Base != "" {
		result.Image.Base = user.Image.Base
	}

	// Replace packages entirely if user specified
	if len(user.Image.Packages) > 0 {
		result.Image.Packages = user.Image.Packages
	}

	// Replace mise install commands if user specified
	if len(user.Mise.Install) > 0 {
		result.Mise.Install = user.Mise.Install
	}

	// Merge mise env vars (user adds/overrides individual keys)
	if len(user.Mise.Env) > 0 {
		if result.Mise.Env == nil {
			result.Mise.Env = make(map[string]any)
		}
		for k, v := range user.Mise.Env {
			result.Mise.Env[k] = v
		}
	}

	// Accumulate image customizations from user config
	if len(user.ImageCustomizations.Packages) > 0 {
		result.ImageCustomizations.Packages = append(
			result.ImageCustomizations.Packages,
			user.ImageCustomizations.Packages...,
		)
	}

	return result
}

// GetAgent returns the agent config by name
func (c *ImageConfig) GetAgent(name string) (AgentConfig, bool) {
	agent, ok := c.Agents[name]
	return agent, ok
}

// AgentNames returns a sorted list of available agent names
func (c *ImageConfig) AgentNames() []string {
	names := make([]string, 0, len(c.Agents))
	for name := range c.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ResolveToolDeps resolves all tool dependencies for an agent.
// userTools contains tools explicitly specified by the user - only these get transitive deps resolved.
// When debug is true, logs which transitive dependencies were skipped.
// Returns tools in dependency order (dependencies first)
func (c *ImageConfig) ResolveToolDeps(agentName string, userTools map[string]bool, debug bool) []toolDescriptor {
	agent, ok := c.Agents[agentName]
	if !ok {
		return nil
	}

	var result []toolDescriptor
	seen := make(map[string]bool)

	// Process dependencies using a queue for breadth-first resolution
	queue := make([]string, len(agent.Depends))
	copy(queue, agent.Depends)

	for len(queue) > 0 {
		toolName := queue[0]
		queue = queue[1:]

		if seen[toolName] {
			continue
		}
		seen[toolName] = true

		tool := c.Tools[toolName]
		version := tool.Version
		if version == "" {
			version = "latest"
		}

		result = append(result, toolDescriptor{name: toolName, version: version, source: sourceConfig})

		// Only resolve transitive dependencies if this tool was user-specified
		if tool.Depends != "" {
			if userTools[toolName] {
				queue = append(queue, tool.Depends)
			} else if debug {
				fmt.Fprintf(os.Stderr, "debug: skipping transitive dependency %q of %q (not user-specified)\n", tool.Depends, toolName)
			}
		}
	}

	return result
}

// ToToolSpec converts an AgentConfig to a ToolSpec for backwards compatibility
func (a AgentConfig) ToToolSpec() ToolSpec {
	return ToolSpec{
		MiseToolName:     a.PackageName,
		ConfigKey:        a.PackageName,
		Command:          a.Command,
		ConfigDir:        a.ConfigDir,
		AdditionalMounts: a.AdditionalMounts,
		EnvVars:          a.EnvVars,
	}
}

// ResolveAdditionalPackages resolves all additional apt packages needed for an agent
// by traversing the agent's tool dependencies and collecting their additionalPackages.
// userTools contains tools explicitly specified by the user - only these get transitive deps resolved.
func (c *ImageConfig) ResolveAdditionalPackages(agentName string, userTools map[string]bool) []string {
	agent, ok := c.Agents[agentName]
	if !ok {
		return nil
	}

	var packages []string
	seen := make(map[string]bool)

	// Process dependencies using a queue for breadth-first resolution
	queue := make([]string, len(agent.Depends))
	copy(queue, agent.Depends)

	for len(queue) > 0 {
		toolName := queue[0]
		queue = queue[1:]

		if seen[toolName] {
			continue
		}
		seen[toolName] = true

		tool := c.Tools[toolName]
		packages = append(packages, tool.AdditionalPackages...)

		// Only resolve transitive dependencies if this tool was user-specified
		if tool.Depends != "" && userTools[toolName] {
			queue = append(queue, tool.Depends)
		}
	}

	return packages
}

// applyImageCustomizations applies add/remove operations to image packages
// This is called after all config files have been merged
func applyImageCustomizations(cfg *ImageConfig) *ImageConfig {
	for _, customization := range cfg.ImageCustomizations.Packages {
		switch customization.Op {
		case "add":
			cfg.Image.Packages = append(cfg.Image.Packages, customization.Value)
		case "remove":
			found := false
			newPackages := make([]string, 0, len(cfg.Image.Packages))
			for _, pkg := range cfg.Image.Packages {
				if pkg == customization.Value {
					found = true
				} else {
					newPackages = append(newPackages, pkg)
				}
			}
			cfg.Image.Packages = newPackages
			if !found {
				fmt.Fprintf(os.Stderr, "Warning: package %q not found for removal\n", customization.Value)
			}
		default:
			fmt.Fprintf(os.Stderr, "Warning: unknown image customization operation %q\n", customization.Op)
		}
	}
	return cfg
}
