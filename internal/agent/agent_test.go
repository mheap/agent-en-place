package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// updateGolden returns true if golden files should be updated
func updateGolden() bool {
	return os.Getenv("UPDATE_GOLDEN_TESTS") == "true"
}

// goldenTest compares got against the golden file, updating it if UPDATE_GOLDEN_TESTS=true
func goldenTest(t *testing.T, goldenFile string, got string) {
	t.Helper()

	golden := filepath.Join("testdata", "golden", goldenFile)

	if updateGolden() {
		if err := os.MkdirAll(filepath.Dir(golden), 0755); err != nil {
			t.Fatalf("failed to create golden directory: %v", err)
		}
		if err := os.WriteFile(golden, []byte(got), 0644); err != nil {
			t.Fatalf("failed to update golden file: %v", err)
		}
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v (run with UPDATE_GOLDEN_TESTS=true to create)", golden, err)
	}

	if diff := cmp.Diff(string(want), got); diff != "" {
		t.Errorf("Dockerfile mismatch (-want +got):\n%s", diff)
	}
}

// loadTestConfig loads the default config for tests
func loadTestConfig(t *testing.T) *ImageConfig {
	t.Helper()
	imgCfg, err := LoadMergedConfig(defaultConfigYAML, "")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	return imgCfg
}

// getToolSpec gets a tool spec from config by agent name
func getToolSpec(t *testing.T, imgCfg *ImageConfig, agentName string) ToolSpec {
	t.Helper()
	agentCfg, ok := imgCfg.GetAgent(agentName)
	if !ok {
		t.Fatalf("unknown agent: %s", agentName)
	}
	return agentCfg.ToToolSpec()
}

// buildDefaultCollection creates a collectResult with the tool spec and node
func buildDefaultCollection(toolName string, spec ToolSpec) collectResult {
	return collectResult{
		specs: []toolDescriptor{
			{name: sanitizeTagComponent(spec.MiseToolName), version: "latest", labelName: toolName},
			{name: "node", version: "latest", labelName: "node"},
		},
		idiomaticInfos: []idiomaticInfo{
			{tool: spec.MiseToolName, version: "latest", configKey: spec.ConfigKey},
			{tool: "node", version: "latest", configKey: "node"},
		},
	}
}

func TestDockerfile_Basic(t *testing.T) {
	imgCfg := loadTestConfig(t)

	tests := []struct {
		name string
		tool string
	}{
		{"codex", "codex"},
		{"opencode", "opencode"},
		{"copilot", "copilot"},
		{"claude", "claude"},
		{"gemini", "gemini"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := getToolSpec(t, imgCfg, tt.tool)
			collection := buildDefaultCollection(tt.tool, spec)

			// Basic case: no .tool-versions, no mise.toml, but needs libatomic (node)
			got := buildDockerfile(false, false, true, collection, spec, imgCfg)

			goldenTest(t, "dockerfile_"+tt.name+"_basic.golden", got)
		})
	}
}

func TestDockerfile_Claude_WithToolVersions(t *testing.T) {
	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	// Simulate .tool-versions with node 20.10.0
	collection := collectResult{
		specs: []toolDescriptor{
			{name: "node", version: "20.10.0", labelName: "node"},
			{name: sanitizeTagComponent(spec.MiseToolName), version: "latest", labelName: "claude"},
		},
		idiomaticInfos: []idiomaticInfo{
			{tool: "node", version: "20.10.0", configKey: "node"},
			{tool: spec.MiseToolName, version: "latest", configKey: spec.ConfigKey},
		},
	}

	// hasTool=true, hasMise=false, needLibatomic=true
	got := buildDockerfile(true, false, true, collection, spec, imgCfg)

	goldenTest(t, "dockerfile_claude_with_tool_versions.golden", got)
}

func TestDockerfile_Claude_WithMiseToml(t *testing.T) {
	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	// Simulate mise.toml with python 3.12.0 and node 20.10.0
	collection := collectResult{
		specs: []toolDescriptor{
			{name: "python", version: "3.12.0", labelName: "python"},
			{name: "node", version: "20.10.0", labelName: "node"},
			{name: sanitizeTagComponent(spec.MiseToolName), version: "latest", labelName: "claude"},
		},
		idiomaticInfos: []idiomaticInfo{
			{tool: "python", version: "3.12.0", configKey: "python"},
			{tool: "node", version: "20.10.0", configKey: "node"},
			{tool: spec.MiseToolName, version: "latest", configKey: spec.ConfigKey},
		},
	}

	// hasTool=false, hasMise=true, needLibatomic=true
	got := buildDockerfile(false, true, true, collection, spec, imgCfg)

	goldenTest(t, "dockerfile_claude_with_mise_toml.golden", got)
}

func TestDockerfile_Claude_WithNodeVersion(t *testing.T) {
	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	// Simulate .node-version file with 18.19.0
	collection := collectResult{
		specs: []toolDescriptor{
			{name: "node", version: "18.19.0", labelName: "node"},
			{name: sanitizeTagComponent(spec.MiseToolName), version: "latest", labelName: "claude"},
		},
		idiomaticInfos: []idiomaticInfo{
			{tool: "node", version: "18.19.0", path: ".node-version", configKey: "node"},
			{tool: spec.MiseToolName, version: "latest", configKey: spec.ConfigKey},
		},
	}

	// hasTool=false, hasMise=false, needLibatomic=true (has node)
	got := buildDockerfile(false, false, true, collection, spec, imgCfg)

	goldenTest(t, "dockerfile_claude_with_node_version.golden", got)
}

func TestDockerfile_Claude_WithBothConfigs(t *testing.T) {
	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	// Simulate both .tool-versions and mise.toml
	collection := collectResult{
		specs: []toolDescriptor{
			{name: "node", version: "20.10.0", labelName: "node"},
			{name: "python", version: "3.11.0", labelName: "python"},
			{name: sanitizeTagComponent(spec.MiseToolName), version: "latest", labelName: "claude"},
		},
		idiomaticInfos: []idiomaticInfo{
			{tool: "node", version: "20.10.0", configKey: "node"},
			{tool: "python", version: "3.11.0", configKey: "python"},
			{tool: spec.MiseToolName, version: "latest", configKey: spec.ConfigKey},
		},
	}

	// hasTool=true, hasMise=true, needLibatomic=true
	got := buildDockerfile(true, true, true, collection, spec, imgCfg)

	goldenTest(t, "dockerfile_claude_with_both_configs.golden", got)
}

func TestDockerfile_Claude_WithoutNode(t *testing.T) {
	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	// Simulate a case with only python (no node) - libatomic not needed
	collection := collectResult{
		specs: []toolDescriptor{
			{name: "python", version: "3.12.0", labelName: "python"},
			{name: sanitizeTagComponent(spec.MiseToolName), version: "latest", labelName: "claude"},
		},
		idiomaticInfos: []idiomaticInfo{
			{tool: "python", version: "3.12.0", configKey: "python"},
			{tool: spec.MiseToolName, version: "latest", configKey: spec.ConfigKey},
		},
	}

	// hasTool=false, hasMise=false, needLibatomic=false (no node)
	got := buildDockerfile(false, false, false, collection, spec, imgCfg)

	goldenTest(t, "dockerfile_claude_without_node.golden", got)
}
