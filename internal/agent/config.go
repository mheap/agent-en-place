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
	Tools  map[string]ToolConfigEntry `yaml:"tools"`
	Agents map[string]AgentConfig     `yaml:"agents"`
	Image  ImageSettings              `yaml:"image"`
	Mise   MiseSettings               `yaml:"mise"`
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

// MiseSettings defines mise installation commands
type MiseSettings struct {
	Install []string `yaml:"install"`
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

	return base, nil
}

// mergeConfigs deep merges user config into base config
// - Tools: user adds/overrides individual tools
// - Agents: user adds/overrides individual agents
// - Image.Base: user replaces if set
// - Image.Packages: user replaces entirely if set
// - Mise.Install: user replaces entirely if set
func mergeConfigs(base, user *ImageConfig) *ImageConfig {
	result := &ImageConfig{
		Tools:  make(map[string]ToolConfigEntry),
		Agents: make(map[string]AgentConfig),
		Image:  base.Image,
		Mise:   base.Mise,
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

// ResolveToolDeps resolves all tool dependencies for an agent
// Returns tools in dependency order (dependencies first)
func (c *ImageConfig) ResolveToolDeps(agentName string) []toolDescriptor {
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

		result = append(result, toolDescriptor{name: toolName, version: version})

		// Add transitive dependencies to queue
		if tool.Depends != "" {
			queue = append(queue, tool.Depends)
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
// by traversing the agent's tool dependencies and collecting their additionalPackages
func (c *ImageConfig) ResolveAdditionalPackages(agentName string) []string {
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

		// Add transitive dependencies to queue
		if tool.Depends != "" {
			queue = append(queue, tool.Depends)
		}
	}

	return packages
}
