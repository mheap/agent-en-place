package agent

import (
	"os"
	"path/filepath"
	"strings"
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

			// Basic case: no .tool-versions, no mise.toml
			got := buildDockerfile(false, false, collection, spec, imgCfg, tt.tool, nil)

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

	// hasTool=true, hasMise=false
	got := buildDockerfile(true, false, collection, spec, imgCfg, "claude", nil)

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

	// hasTool=false, hasMise=true
	got := buildDockerfile(false, true, collection, spec, imgCfg, "claude", nil)

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

	// hasTool=false, hasMise=false (node version comes from .node-version file)
	got := buildDockerfile(false, false, collection, spec, imgCfg, "claude", nil)

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

	// hasTool=true, hasMise=true
	got := buildDockerfile(true, true, collection, spec, imgCfg, "claude", nil)

	goldenTest(t, "dockerfile_claude_with_both_configs.golden", got)
}

func TestDockerfile_Claude_WithoutNode(t *testing.T) {
	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	// Simulate a case with only python (no node) - additionalPackages from node not included
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

	// hasTool=false, hasMise=false
	got := buildDockerfile(false, false, collection, spec, imgCfg, "claude", nil)

	goldenTest(t, "dockerfile_claude_without_node.golden", got)
}

func TestHandleBuildOutput_Success(t *testing.T) {
	// Simulate successful Docker build output
	output := `{"stream":"Step 1/5 : FROM debian:12-slim\n"}
{"stream":"---\u003e abc123\n"}
{"stream":"Step 2/5 : RUN apt-get update\n"}
{"stream":"---\u003e Running in def456\n"}
{"stream":"Successfully built abc123\n"}
{"stream":"Successfully tagged myimage:latest\n"}
`
	reader := strings.NewReader(output)
	err := handleBuildOutput(reader, false, "myimage:latest")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestHandleBuildOutput_Error(t *testing.T) {
	// Simulate Docker build output with an error
	output := `{"stream":"Step 1/5 : FROM debian:12-slim\n"}
{"stream":"---\u003e abc123\n"}
{"stream":"Step 2/5 : RUN apt-get install nonexistent\n"}
{"stream":"Reading package lists...\n"}
{"stream":"E: Unable to locate package nonexistent\n"}
{"error":"The command '/bin/sh -c apt-get install nonexistent' returned a non-zero code: 100"}
`
	reader := strings.NewReader(output)
	err := handleBuildOutput(reader, false, "myimage:latest")

	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	errMsg := err.Error()

	// Check error message format
	if !strings.Contains(errMsg, "Error building docker image myimage:latest") {
		t.Errorf("error message should contain image name, got: %s", errMsg)
	}

	// Check that it contains the last meaningful output lines
	if !strings.Contains(errMsg, "E: Unable to locate package nonexistent") {
		t.Errorf("error message should contain last output line, got: %s", errMsg)
	}
}

func TestHandleBuildOutput_FiltersWhitespace(t *testing.T) {
	// Simulate Docker build output with whitespace-only lines
	output := `{"stream":"Step 1/5 : FROM debian:12-slim\n"}
{"stream":"\n"}
{"stream":"   \n"}
{"stream":"Actual content line 1\n"}
{"stream":"\t\n"}
{"stream":"Actual content line 2\n"}
{"stream":"Actual content line 3\n"}
{"stream":"Actual content line 4\n"}
{"error":"Build failed"}
`
	reader := strings.NewReader(output)
	err := handleBuildOutput(reader, false, "test:image")

	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	errMsg := err.Error()

	// Should contain last 3 non-whitespace lines
	if !strings.Contains(errMsg, "Actual content line 2") {
		t.Errorf("error should contain 'Actual content line 2', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "Actual content line 3") {
		t.Errorf("error should contain 'Actual content line 3', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "Actual content line 4") {
		t.Errorf("error should contain 'Actual content line 4', got: %s", errMsg)
	}

	// Should NOT contain "Step 1/5" as it should have been rotated out
	if strings.Contains(errMsg, "Step 1/5") {
		t.Errorf("error should not contain old lines that were rotated out, got: %s", errMsg)
	}
}

func TestBuildAgentMiseConfig_NoUserFile(t *testing.T) {
	spec := ToolSpec{
		MiseToolName: "npm:@anthropic-ai/claude-code",
		ConfigKey:    "npm:@anthropic-ai/claude-code",
	}

	collection := collectResult{
		idiomaticInfos: []idiomaticInfo{
			{tool: "node", version: "20.0.0", configKey: "node"},
		},
	}

	data, err := buildAgentMiseConfig(nil, collection, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := string(data)

	// Should contain tools section
	if !strings.Contains(result, "[tools]") {
		t.Errorf("expected [tools] section, got: %s", result)
	}

	// Should contain node tool from collection
	if !strings.Contains(result, "node") || !strings.Contains(result, "20.0.0") {
		t.Errorf("expected node = 20.0.0, got: %s", result)
	}

	// Should contain agent's primary tool
	if !strings.Contains(result, "npm:@anthropic-ai/claude-code") {
		t.Errorf("expected agent tool, got: %s", result)
	}
}

func TestBuildAgentMiseConfig_WithUserFile(t *testing.T) {
	spec := ToolSpec{
		MiseToolName: "npm:@anthropic-ai/claude-code",
		ConfigKey:    "npm:@anthropic-ai/claude-code",
	}

	// User's mise.toml with python (this should NOT affect agent config since it's a different tool)
	userMise := []byte(`[tools]
python = "3.12.0"
`)

	collection := collectResult{
		idiomaticInfos: []idiomaticInfo{
			{tool: "node", version: "20.0.0", configKey: "node"},
		},
	}

	data, err := buildAgentMiseConfig(userMise, collection, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := string(data)

	// Should NOT contain user's python (agent config only has tools NOT in user's config)
	if strings.Contains(result, "python") {
		t.Errorf("expected python to NOT be in agent config (it's in user's mise.toml), got: %s", result)
	}

	// Should contain node from collection (user didn't specify node)
	if !strings.Contains(result, "node") || !strings.Contains(result, "20.0.0") {
		t.Errorf("expected node = 20.0.0, got: %s", result)
	}

	// Should contain agent's primary tool
	if !strings.Contains(result, "npm:@anthropic-ai/claude-code") {
		t.Errorf("expected agent tool, got: %s", result)
	}
}

func TestBuildAgentMiseConfig_FiltersUserTools(t *testing.T) {
	spec := ToolSpec{
		MiseToolName: "npm:@anthropic-ai/claude-code",
		ConfigKey:    "npm:@anthropic-ai/claude-code",
	}

	// User specifies node - this should be filtered OUT of agent config
	userMise := []byte(`[tools]
node = "18.0.0"
`)

	// Collection has node 20.0.0 (would normally be added)
	collection := collectResult{
		idiomaticInfos: []idiomaticInfo{
			{tool: "node", version: "20.0.0", configKey: "node"},
		},
	}

	data, err := buildAgentMiseConfig(userMise, collection, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := string(data)

	// Node should NOT be in agent config because user specified it
	if strings.Contains(result, "node") {
		t.Errorf("expected node to be filtered out (user specified it), got: %s", result)
	}

	// Agent tool should still be present
	if !strings.Contains(result, "npm:@anthropic-ai/claude-code") {
		t.Errorf("expected agent tool, got: %s", result)
	}
}

func TestBuildAgentMiseConfig_OnlyToolsSection(t *testing.T) {
	spec := ToolSpec{
		MiseToolName: "npm:@anthropic-ai/claude-code",
		ConfigKey:    "npm:@anthropic-ai/claude-code",
	}

	// User's mise.toml with additional sections (these should NOT appear in agent config)
	userMise := []byte(`[tools]
python = "3.12.0"

[settings]
experimental = true

[env]
MY_VAR = "hello"
`)

	collection := collectResult{
		idiomaticInfos: []idiomaticInfo{},
	}

	data, err := buildAgentMiseConfig(userMise, collection, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := string(data)

	// Should only contain [tools] section - no [settings] or [env]
	if strings.Contains(result, "[settings]") {
		t.Errorf("expected NO [settings] section in agent config, got: %s", result)
	}
	if strings.Contains(result, "[env]") {
		t.Errorf("expected NO [env] section in agent config, got: %s", result)
	}

	// Should contain agent's primary tool
	if !strings.Contains(result, "[tools]") {
		t.Errorf("expected [tools] section, got: %s", result)
	}
	if !strings.Contains(result, "npm:@anthropic-ai/claude-code") {
		t.Errorf("expected agent tool, got: %s", result)
	}
}

func TestParseMiseToml_SimpleFormat(t *testing.T) {
	// Test parsing simple [tools] format
	data := []byte(`[tools]
node = "20.0.0"
python = "3.12.0"
`)

	spec := &fileSpec{data: data}
	specs := parseMiseToml(spec)

	if len(specs) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(specs))
	}

	// Check that both tools were parsed (order may vary due to map iteration)
	foundNode := false
	foundPython := false
	for _, s := range specs {
		if s.name == "node" && s.version == "20.0.0" {
			foundNode = true
		}
		if s.name == "python" && s.version == "3.12.0" {
			foundPython = true
		}
	}

	if !foundNode {
		t.Error("expected to find node = 20.0.0")
	}
	if !foundPython {
		t.Error("expected to find python = 3.12.0")
	}
}

func TestParseMiseToml_NilSpec(t *testing.T) {
	specs := parseMiseToml(nil)
	if specs != nil {
		t.Errorf("expected nil for nil spec, got %v", specs)
	}
}

// TestBuildAgentMiseConfig_AllAgents tests mise.agent.toml generation for each agent in config.yaml
func TestBuildAgentMiseConfig_AllAgents(t *testing.T) {
	imgCfg := loadTestConfig(t)

	tests := []struct {
		name           string
		expectedTools  []string // Tools that must be present in output
		notExpectTools []string // Tools that must NOT be present
	}{
		{
			name:           "codex",
			expectedTools:  []string{"npm:@openai/codex", "node"},
			notExpectTools: []string{"python"}, // python not included - node is config-sourced
		},
		{
			name:           "opencode",
			expectedTools:  []string{"npm:opencode-ai", "node"},
			notExpectTools: []string{"python"},
		},
		{
			name:           "copilot",
			expectedTools:  []string{"npm:@github/copilot", "node"},
			notExpectTools: []string{"python"},
		},
		{
			name:           "claude",
			expectedTools:  []string{"npm:@anthropic-ai/claude-code", "node"},
			notExpectTools: []string{"python"},
		},
		{
			name:           "gemini",
			expectedTools:  []string{"npm:@google/gemini-cli", "node"},
			notExpectTools: []string{"python"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := getToolSpec(t, imgCfg, tt.name)

			// Build collection with resolved tool dependencies (simulating real behavior)
			// No user tools, so transitive deps (python) should not be resolved
			userTools := map[string]bool{}
			toolDeps := imgCfg.ResolveToolDeps(tt.name, userTools, false)
			idiomaticInfos := make([]idiomaticInfo, 0, len(toolDeps))
			for _, dep := range toolDeps {
				idiomaticInfos = append(idiomaticInfos, idiomaticInfo{
					tool:      dep.name,
					version:   dep.version,
					configKey: dep.name,
				})
			}

			collection := collectResult{
				specs:          toolDeps,
				idiomaticInfos: idiomaticInfos,
			}

			// Build mise.agent.toml without user file
			data, err := buildAgentMiseConfig(nil, collection, spec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			result := string(data)

			// Verify [tools] section exists
			if !strings.Contains(result, "[tools]") {
				t.Errorf("expected [tools] section, got:\n%s", result)
			}

			// Verify all expected tools are present
			for _, tool := range tt.expectedTools {
				if !strings.Contains(result, tool) {
					t.Errorf("expected tool %q to be present, got:\n%s", tool, result)
				}
			}

			// Verify no unexpected tools are present
			for _, tool := range tt.notExpectTools {
				if strings.Contains(result, tool) {
					t.Errorf("did not expect tool %q to be present, got:\n%s", tool, result)
				}
			}
		})
	}
}

// TestBuildAgentMiseConfig_AllAgents_WithUserMise tests that user tools are filtered out from agent config
func TestBuildAgentMiseConfig_AllAgents_WithUserMise(t *testing.T) {
	imgCfg := loadTestConfig(t)

	// User mise.toml with custom tools (ruby and go are NOT agent dependencies, so they don't affect filtering)
	userMise := []byte(`[tools]
ruby = "3.2.0"
go = "1.21.0"
`)

	agents := []string{"codex", "opencode", "copilot", "claude", "gemini"}

	for _, agentName := range agents {
		t.Run(agentName, func(t *testing.T) {
			spec := getToolSpec(t, imgCfg, agentName)

			// Build collection with resolved tool dependencies
			// User specified ruby and go, but not node - so python should not be resolved
			userTools := map[string]bool{}
			toolDeps := imgCfg.ResolveToolDeps(agentName, userTools, false)
			idiomaticInfos := make([]idiomaticInfo, 0, len(toolDeps))
			for _, dep := range toolDeps {
				idiomaticInfos = append(idiomaticInfos, idiomaticInfo{
					tool:      dep.name,
					version:   dep.version,
					configKey: dep.name,
				})
			}

			collection := collectResult{
				specs:          toolDeps,
				idiomaticInfos: idiomaticInfos,
			}

			// Build mise.agent.toml with user file
			data, err := buildAgentMiseConfig(userMise, collection, spec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			result := string(data)

			// User tools (ruby, go) should NOT be in agent config - they're not agent dependencies
			if strings.Contains(result, "ruby") {
				t.Errorf("expected user's ruby tool to NOT be in agent config, got:\n%s", result)
			}
			if strings.Contains(result, "go =") {
				t.Errorf("expected user's go tool to NOT be in agent config, got:\n%s", result)
			}

			// Agent's primary tool should be present
			if !strings.Contains(result, spec.ConfigKey) {
				t.Errorf("expected agent tool %q to be present, got:\n%s", spec.ConfigKey, result)
			}

			// Node dependency should be present (user didn't specify it)
			if !strings.Contains(result, "node") {
				t.Errorf("expected node dependency to be present, got:\n%s", result)
			}

			// Python should NOT be present - node is config-sourced, so its transitive deps aren't resolved
			if strings.Contains(result, "python") {
				t.Errorf("expected python to NOT be present (node is config-sourced), got:\n%s", result)
			}
		})
	}
}

// TestBuildAgentMiseConfig_AllAgents_UserOverridesDefaults tests that user-specified tools are filtered out
func TestBuildAgentMiseConfig_AllAgents_UserOverridesDefaults(t *testing.T) {
	imgCfg := loadTestConfig(t)

	// User mise.toml specifies node and python - these should be filtered OUT of agent config
	userMise := []byte(`[tools]
node = "18.19.0"
python = "3.11.0"
`)

	agents := []string{"codex", "opencode", "copilot", "claude", "gemini"}

	for _, agentName := range agents {
		t.Run(agentName, func(t *testing.T) {
			spec := getToolSpec(t, imgCfg, agentName)

			// Build collection with resolved tool dependencies
			// No user tools specified that are agent dependencies, so python should not be resolved
			userTools := map[string]bool{}
			toolDeps := imgCfg.ResolveToolDeps(agentName, userTools, false)
			idiomaticInfos := make([]idiomaticInfo, 0, len(toolDeps))
			for _, dep := range toolDeps {
				idiomaticInfos = append(idiomaticInfos, idiomaticInfo{
					tool:      dep.name,
					version:   dep.version,
					configKey: dep.name,
				})
			}

			collection := collectResult{
				specs:          toolDeps,
				idiomaticInfos: idiomaticInfos,
			}

			// Build mise.agent.toml with user file
			data, err := buildAgentMiseConfig(userMise, collection, spec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			result := string(data)

			// node and python should NOT be in agent config (user specified them)
			if strings.Contains(result, "node") {
				t.Errorf("expected node to be filtered out (user specified it), got:\n%s", result)
			}
			if strings.Contains(result, "python") {
				t.Errorf("expected python to be filtered out (user specified it), got:\n%s", result)
			}

			// Agent tool should still be present
			if !strings.Contains(result, spec.ConfigKey) {
				t.Errorf("expected agent tool %q to be present, got:\n%s", spec.ConfigKey, result)
			}
		})
	}
}

// TestBuildAgentMiseConfig_GoldenFiles tests mise.agent.toml generation against golden files for each agent
func TestBuildAgentMiseConfig_GoldenFiles(t *testing.T) {
	imgCfg := loadTestConfig(t)

	agents := []string{"codex", "opencode", "copilot", "claude", "gemini"}

	for _, agentName := range agents {
		t.Run(agentName, func(t *testing.T) {
			spec := getToolSpec(t, imgCfg, agentName)

			// Build collection with resolved tool dependencies
			// No user tools, so transitive deps (python) should not be resolved
			userTools := map[string]bool{}
			toolDeps := imgCfg.ResolveToolDeps(agentName, userTools, false)
			idiomaticInfos := make([]idiomaticInfo, 0, len(toolDeps))
			for _, dep := range toolDeps {
				idiomaticInfos = append(idiomaticInfos, idiomaticInfo{
					tool:      dep.name,
					version:   dep.version,
					configKey: dep.name,
				})
			}

			collection := collectResult{
				specs:          toolDeps,
				idiomaticInfos: idiomaticInfos,
			}

			// Build mise.agent.toml without user file
			data, err := buildAgentMiseConfig(nil, collection, spec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			goldenTest(t, "mise_agent_"+agentName+".golden", string(data))
		})
	}
}

func TestParseGoModVersion(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantVersion string
		wantOk      bool
	}{
		{
			name: "simple go directive",
			content: `module example.com/myapp

go 1.21.0

require (
	github.com/example/dep v1.0.0
)
`,
			wantVersion: "1.21.0",
			wantOk:      true,
		},
		{
			name: "go directive without patch version",
			content: `module example.com/myapp

go 1.21

require (
	github.com/example/dep v1.0.0
)
`,
			wantVersion: "1.21",
			wantOk:      true,
		},
		{
			name: "go directive with toolchain",
			content: `module example.com/myapp

go 1.24.4

toolchain go1.24.5

require (
	github.com/example/dep v1.0.0
)
`,
			wantVersion: "1.24.4",
			wantOk:      true,
		},
		{
			name: "no go directive",
			content: `module example.com/myapp

require (
	github.com/example/dep v1.0.0
)
`,
			wantVersion: "",
			wantOk:      false,
		},
		{
			name:        "empty file",
			content:     "",
			wantVersion: "",
			wantOk:      false,
		},
		{
			name: "go directive with extra whitespace",
			content: `module example.com/myapp

go   1.22.3  

require (
	github.com/example/dep v1.0.0
)
`,
			wantVersion: "1.22.3",
			wantOk:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			goModPath := filepath.Join(tmpDir, "go.mod")
			if err := os.WriteFile(goModPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			gotVersion, gotOk := parseGoModVersion(goModPath)

			if gotOk != tt.wantOk {
				t.Errorf("parseGoModVersion() ok = %v, want %v", gotOk, tt.wantOk)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("parseGoModVersion() version = %q, want %q", gotVersion, tt.wantVersion)
			}
		})
	}
}

func TestParseGoModVersion_FileNotFound(t *testing.T) {
	version, ok := parseGoModVersion("/nonexistent/path/go.mod")
	if ok {
		t.Error("expected ok=false for nonexistent file")
	}
	if version != "" {
		t.Errorf("expected empty version, got %q", version)
	}
}

func TestReadIdiomaticVersion_GoMod(t *testing.T) {
	// Create temp dir and go.mod
	tmpDir := t.TempDir()
	goModPath := filepath.Join(tmpDir, "go.mod")
	content := `module example.com/myapp

go 1.23.1
`
	if err := os.WriteFile(goModPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Change to temp dir to test readIdiomaticVersion
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	version, ok := readIdiomaticVersion("go", "go.mod")
	if !ok {
		t.Error("expected ok=true")
	}
	if version != "1.23.1" {
		t.Errorf("expected version 1.23.1, got %q", version)
	}
}

func TestIdiomaticFiles_GoVersionTakesPrecedence(t *testing.T) {
	// Create temp dir with both .go-version and go.mod
	tmpDir := t.TempDir()

	// .go-version takes precedence
	goVersionPath := filepath.Join(tmpDir, ".go-version")
	if err := os.WriteFile(goVersionPath, []byte("1.20.0\n"), 0644); err != nil {
		t.Fatalf("failed to write .go-version: %v", err)
	}

	goModPath := filepath.Join(tmpDir, "go.mod")
	goModContent := `module example.com/myapp

go 1.21.0
`
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Change to temp dir
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Parse idiomatic files - should get .go-version (1.20.0), not go.mod (1.21.0)
	infos := parseIdiomaticFiles()

	var goVersion string
	for _, info := range infos {
		if info.tool == "go" {
			goVersion = info.version
			break
		}
	}

	if goVersion != "1.20.0" {
		t.Errorf("expected .go-version to take precedence (1.20.0), got %q", goVersion)
	}
}

func TestIdiomaticFiles_GoModUsedAsFallback(t *testing.T) {
	// Create temp dir with only go.mod (no .go-version)
	tmpDir := t.TempDir()

	goModPath := filepath.Join(tmpDir, "go.mod")
	goModContent := `module example.com/myapp

go 1.22.0
`
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Change to temp dir
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Parse idiomatic files - should get go.mod version since no .go-version
	infos := parseIdiomaticFiles()

	var goVersion string
	for _, info := range infos {
		if info.tool == "go" {
			goVersion = info.version
			break
		}
	}

	if goVersion != "1.22.0" {
		t.Errorf("expected go.mod version (1.22.0) as fallback, got %q", goVersion)
	}
}

func TestBuildAgentMiseConfig_GoFromGoMod(t *testing.T) {
	// Create temp dir with only go.mod
	tmpDir := t.TempDir()

	goModPath := filepath.Join(tmpDir, "go.mod")
	goModContent := `module example.com/myapp

go 1.23.0
`
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Change to temp dir
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	// Parse idiomatic files to get go version from go.mod
	idiomaticInfos := parseIdiomaticFiles()

	collection := collectResult{
		idiomaticInfos: idiomaticInfos,
	}

	// Build with no user mise.toml
	data, err := buildAgentMiseConfig(nil, collection, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := string(data)

	// Should contain go = "1.23.0"
	if !strings.Contains(result, `go = "1.23.0"`) {
		t.Errorf("expected go version from go.mod in output, got:\n%s", result)
	}
}

func TestBuildAgentMiseConfig_GoFromGoMod_NotIncludedWhenMiseTomlHasGo(t *testing.T) {
	// Create temp dir with go.mod
	tmpDir := t.TempDir()

	goModPath := filepath.Join(tmpDir, "go.mod")
	goModContent := `module example.com/myapp

go 1.23.0
`
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Change to temp dir
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	// Parse idiomatic files to get go version from go.mod
	idiomaticInfos := parseIdiomaticFiles()

	collection := collectResult{
		idiomaticInfos: idiomaticInfos,
	}

	// User's mise.toml already has go defined
	userMise := []byte(`[tools]
go = "1.21.0"
`)

	// Build with user mise.toml that has go
	data, err := buildAgentMiseConfig(userMise, collection, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := string(data)

	// Should NOT contain any go version (user's mise.toml takes precedence)
	if strings.Contains(result, "go =") {
		t.Errorf("expected go to be excluded when user mise.toml has it, got:\n%s", result)
	}
}

// TestApplyImageCustomizations_AddPackage tests adding a package via customization
func TestApplyImageCustomizations_AddPackage(t *testing.T) {
	cfg := &ImageConfig{
		Image: ImageSettings{
			Packages: []string{"curl", "git"},
		},
		ImageCustomizations: ImageCustomizations{
			Packages: []ImageCustomization{
				{Op: "add", Value: "vim"},
			},
		},
	}

	result := applyImageCustomizations(cfg)

	expected := []string{"curl", "git", "vim"}
	if !slicesEqual(result.Image.Packages, expected) {
		t.Errorf("expected packages %v, got %v", expected, result.Image.Packages)
	}
}

// TestApplyImageCustomizations_RemovePackage tests removing a package via customization
func TestApplyImageCustomizations_RemovePackage(t *testing.T) {
	cfg := &ImageConfig{
		Image: ImageSettings{
			Packages: []string{"curl", "git", "gnupg"},
		},
		ImageCustomizations: ImageCustomizations{
			Packages: []ImageCustomization{
				{Op: "remove", Value: "git"},
			},
		},
	}

	result := applyImageCustomizations(cfg)

	expected := []string{"curl", "gnupg"}
	if !slicesEqual(result.Image.Packages, expected) {
		t.Errorf("expected packages %v, got %v", expected, result.Image.Packages)
	}
}

// TestApplyImageCustomizations_AddAndRemove tests both add and remove operations together
func TestApplyImageCustomizations_AddAndRemove(t *testing.T) {
	cfg := &ImageConfig{
		Image: ImageSettings{
			Packages: []string{"curl", "git", "gnupg"},
		},
		ImageCustomizations: ImageCustomizations{
			Packages: []ImageCustomization{
				{Op: "add", Value: "build-essential"},
				{Op: "remove", Value: "gnupg"},
				{Op: "add", Value: "vim"},
			},
		},
	}

	result := applyImageCustomizations(cfg)

	expected := []string{"curl", "git", "build-essential", "vim"}
	if !slicesEqual(result.Image.Packages, expected) {
		t.Errorf("expected packages %v, got %v", expected, result.Image.Packages)
	}
}

// TestApplyImageCustomizations_NoCustomizations tests that no customizations leaves packages unchanged
func TestApplyImageCustomizations_NoCustomizations(t *testing.T) {
	cfg := &ImageConfig{
		Image: ImageSettings{
			Packages: []string{"curl", "git"},
		},
		ImageCustomizations: ImageCustomizations{},
	}

	result := applyImageCustomizations(cfg)

	expected := []string{"curl", "git"}
	if !slicesEqual(result.Image.Packages, expected) {
		t.Errorf("expected packages %v, got %v", expected, result.Image.Packages)
	}
}

// TestMergeConfigs_AccumulatesCustomizations tests that customizations are accumulated across config files
func TestMergeConfigs_AccumulatesCustomizations(t *testing.T) {
	base := &ImageConfig{
		Tools:  make(map[string]ToolConfigEntry),
		Agents: make(map[string]AgentConfig),
		Image: ImageSettings{
			Packages: []string{"curl", "git"},
		},
		ImageCustomizations: ImageCustomizations{
			Packages: []ImageCustomization{
				{Op: "add", Value: "vim"},
			},
		},
	}

	user := &ImageConfig{
		Tools:  make(map[string]ToolConfigEntry),
		Agents: make(map[string]AgentConfig),
		ImageCustomizations: ImageCustomizations{
			Packages: []ImageCustomization{
				{Op: "add", Value: "nano"},
				{Op: "remove", Value: "git"},
			},
		},
	}

	result := mergeConfigs(base, user)

	// Should have all customizations accumulated
	if len(result.ImageCustomizations.Packages) != 3 {
		t.Errorf("expected 3 customizations, got %d", len(result.ImageCustomizations.Packages))
	}

	// Check that all customizations are present in order
	if result.ImageCustomizations.Packages[0].Op != "add" || result.ImageCustomizations.Packages[0].Value != "vim" {
		t.Errorf("first customization should be add vim, got %+v", result.ImageCustomizations.Packages[0])
	}
	if result.ImageCustomizations.Packages[1].Op != "add" || result.ImageCustomizations.Packages[1].Value != "nano" {
		t.Errorf("second customization should be add nano, got %+v", result.ImageCustomizations.Packages[1])
	}
	if result.ImageCustomizations.Packages[2].Op != "remove" || result.ImageCustomizations.Packages[2].Value != "git" {
		t.Errorf("third customization should be remove git, got %+v", result.ImageCustomizations.Packages[2])
	}
}

// slicesEqual compares two string slices for equality
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestResolveToolDeps_SkipsTransitiveDepsForConfigTools verifies that transitive
// dependencies are not resolved when tools come from config (agent dependencies)
func TestResolveToolDeps_SkipsTransitiveDepsForConfigTools(t *testing.T) {
	imgCfg := loadTestConfig(t)
	userTools := map[string]bool{} // No user-specified tools

	deps := imgCfg.ResolveToolDeps("claude", userTools, false)

	toolNames := make(map[string]bool)
	for _, d := range deps {
		toolNames[d.name] = true
	}

	if !toolNames["node"] {
		t.Error("expected node to be included (direct agent dependency)")
	}
	if toolNames["python"] {
		t.Error("expected python to NOT be included (node is config-sourced, so its transitive deps are skipped)")
	}
}

// TestResolveToolDeps_IncludesTransitiveDepsForUserTools verifies that transitive
// dependencies ARE resolved when the parent tool is user-specified
func TestResolveToolDeps_IncludesTransitiveDepsForUserTools(t *testing.T) {
	imgCfg := loadTestConfig(t)
	userTools := map[string]bool{"node": true} // User explicitly specified node

	deps := imgCfg.ResolveToolDeps("claude", userTools, false)

	toolNames := make(map[string]bool)
	for _, d := range deps {
		toolNames[d.name] = true
	}

	if !toolNames["node"] {
		t.Error("expected node to be included")
	}
	if !toolNames["python"] {
		t.Error("expected python to be included (node is user-specified, so its transitive deps are resolved)")
	}
}

// TestResolveToolDeps_SourceIsConfig verifies that tools from ResolveToolDeps have sourceConfig
func TestResolveToolDeps_SourceIsConfig(t *testing.T) {
	imgCfg := loadTestConfig(t)
	userTools := map[string]bool{}

	deps := imgCfg.ResolveToolDeps("claude", userTools, false)

	for _, d := range deps {
		if d.source != sourceConfig {
			t.Errorf("expected tool %q to have source %q, got %q", d.name, sourceConfig, d.source)
		}
	}
}

// TestResolveAdditionalPackages_SkipsTransitivePackages verifies that additional packages
// from transitive dependencies are not included when parent tool is config-sourced
func TestResolveAdditionalPackages_SkipsTransitivePackages(t *testing.T) {
	imgCfg := loadTestConfig(t)
	userTools := map[string]bool{} // No user-specified tools

	packages := imgCfg.ResolveAdditionalPackages("claude", userTools)

	// Should have libatomic1 from node (direct agent dependency)
	hasLibatomic := false
	for _, pkg := range packages {
		if pkg == "libatomic1" {
			hasLibatomic = true
			break
		}
	}

	if !hasLibatomic {
		t.Error("expected libatomic1 to be included (from node, which is a direct agent dependency)")
	}
}

// TestResolveAdditionalPackages_IncludesTransitivePackages verifies that additional packages
// from transitive dependencies ARE included when parent tool is user-specified
func TestResolveAdditionalPackages_IncludesTransitivePackages(t *testing.T) {
	imgCfg := loadTestConfig(t)
	userTools := map[string]bool{"node": true} // User explicitly specified node

	packages := imgCfg.ResolveAdditionalPackages("claude", userTools)

	// Should have libatomic1 from node
	hasLibatomic := false
	for _, pkg := range packages {
		if pkg == "libatomic1" {
			hasLibatomic = true
			break
		}
	}

	if !hasLibatomic {
		t.Error("expected libatomic1 to be included (from node)")
	}
}

// TestDedupeToolSpecs_PreservesSource verifies that deduplication preserves the source
// from the first occurrence (which has higher priority)
func TestDedupeToolSpecs_PreservesSource(t *testing.T) {
	specs := []toolDescriptor{
		{name: "node", version: "20.0.0", source: sourceUser},     // User-specified first
		{name: "node", version: "latest", source: sourceConfig},   // Config second (should be ignored)
		{name: "python", version: "latest", source: sourceConfig}, // Only config
	}

	deduped := dedupeToolSpecs(specs)

	if len(deduped) != 2 {
		t.Fatalf("expected 2 tools after dedup, got %d", len(deduped))
	}

	// Find node in deduped
	var nodeSpec *toolDescriptor
	var pythonSpec *toolDescriptor
	for i := range deduped {
		if deduped[i].name == "node" {
			nodeSpec = &deduped[i]
		}
		if deduped[i].name == "python" {
			pythonSpec = &deduped[i]
		}
	}

	if nodeSpec == nil {
		t.Fatal("expected node in deduped specs")
	}
	if nodeSpec.source != sourceUser {
		t.Errorf("expected node to have source %q (first wins), got %q", sourceUser, nodeSpec.source)
	}
	if nodeSpec.version != "20.0.0" {
		t.Errorf("expected node to have version %q (first wins), got %q", "20.0.0", nodeSpec.version)
	}

	if pythonSpec == nil {
		t.Fatal("expected python in deduped specs")
	}
	if pythonSpec.source != sourceConfig {
		t.Errorf("expected python to have source %q, got %q", sourceConfig, pythonSpec.source)
	}
}

// TestParseToolVersions_SetsSourceUser verifies that parseToolVersions sets sourceUser
func TestParseToolVersions_SetsSourceUser(t *testing.T) {
	spec := &fileSpec{
		path: ".tool-versions",
		data: []byte("node 20.0.0\npython 3.11.0"),
	}

	specs := parseToolVersions(spec)

	if len(specs) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(specs))
	}

	for _, s := range specs {
		if s.source != sourceUser {
			t.Errorf("expected tool %q to have source %q, got %q", s.name, sourceUser, s.source)
		}
	}
}

// TestParseMiseToml_SetsSourceUser verifies that parseMiseToml sets sourceUser
func TestParseMiseToml_SetsSourceUser(t *testing.T) {
	spec := &fileSpec{
		path: "mise.toml",
		data: []byte(`[tools]
node = "20.0.0"
python = "3.11.0"
`),
	}

	specs := parseMiseToml(spec)

	if len(specs) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(specs))
	}

	for _, s := range specs {
		if s.source != sourceUser {
			t.Errorf("expected tool %q to have source %q, got %q", s.name, sourceUser, s.source)
		}
	}
}

// --- Tests for environment variable tool overrides ---

func TestSplitToolVersion_Simple(t *testing.T) {
	tests := []struct {
		input       string
		wantName    string
		wantVersion string
	}{
		{"node@latest", "node", "latest"},
		{"python@3.12", "python", "3.12"},
		{"node@20.10.0", "node", "20.10.0"},
		{"npm:trello-cli@1.5.0", "npm:trello-cli", "1.5.0"},
		{"npm:@my-org/some-package@1.2.3", "npm:@my-org/some-package", "1.2.3"},
		{"npm:@anthropic-ai/claude-code@latest", "npm:@anthropic-ai/claude-code", "latest"},
		// No version -> defaults to latest
		{"node", "node", "latest"},
		{"npm:trello-cli", "npm:trello-cli", "latest"},
		// Scoped npm package without version -> entire string is the name
		{"npm:@my-org/some-package", "npm:@my-org/some-package", "latest"},
		// Trailing @ -> defaults to latest
		{"node@", "node", "latest"},
		// @ at the beginning (bare scoped package, unusual but handled)
		{"@org/pkg", "@org/pkg", "latest"},
		{"@org/pkg@2.0.0", "@org/pkg", "2.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, version := splitToolVersion(tt.input)
			if name != tt.wantName {
				t.Errorf("splitToolVersion(%q) name = %q, want %q", tt.input, name, tt.wantName)
			}
			if version != tt.wantVersion {
				t.Errorf("splitToolVersion(%q) version = %q, want %q", tt.input, version, tt.wantVersion)
			}
		})
	}
}

func TestParseEnvTools_NotSet(t *testing.T) {
	t.Setenv("AGENT_EN_PLACE_TOOLS", "")
	specs := parseEnvTools()
	if specs != nil {
		t.Errorf("expected nil when env var is not set, got %v", specs)
	}
}

func TestParseEnvTools_Basic(t *testing.T) {
	t.Setenv("AGENT_EN_PLACE_TOOLS", "node@latest,python@3.12")
	specs := parseEnvTools()

	if len(specs) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(specs))
	}

	if specs[0].name != "node" || specs[0].version != "latest" {
		t.Errorf("expected node@latest, got %s@%s", specs[0].name, specs[0].version)
	}
	if specs[1].name != "python" || specs[1].version != "3.12" {
		t.Errorf("expected python@3.12, got %s@%s", specs[1].name, specs[1].version)
	}

	for _, s := range specs {
		if s.source != sourceEnvVar {
			t.Errorf("expected source %q, got %q", sourceEnvVar, s.source)
		}
	}
}

func TestParseEnvTools_NpmScopedPackage(t *testing.T) {
	t.Setenv("AGENT_EN_PLACE_TOOLS", "npm:@my-org/some-package@1.2.3")
	specs := parseEnvTools()

	if len(specs) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(specs))
	}

	if specs[0].name != "npm:@my-org/some-package" {
		t.Errorf("expected name npm:@my-org/some-package, got %s", specs[0].name)
	}
	if specs[0].version != "1.2.3" {
		t.Errorf("expected version 1.2.3, got %s", specs[0].version)
	}
}

func TestParseEnvTools_NoVersion(t *testing.T) {
	t.Setenv("AGENT_EN_PLACE_TOOLS", "node,python")
	specs := parseEnvTools()

	if len(specs) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(specs))
	}

	for _, s := range specs {
		if s.version != "latest" {
			t.Errorf("expected version latest for %s, got %s", s.name, s.version)
		}
	}
}

func TestParseEnvTools_SkipsEmpty(t *testing.T) {
	t.Setenv("AGENT_EN_PLACE_TOOLS", "node@latest,,python@3.12, ,")
	specs := parseEnvTools()

	if len(specs) != 2 {
		t.Fatalf("expected 2 tools (skipping empty entries), got %d", len(specs))
	}

	if specs[0].name != "node" {
		t.Errorf("expected first tool to be node, got %s", specs[0].name)
	}
	if specs[1].name != "python" {
		t.Errorf("expected second tool to be python, got %s", specs[1].name)
	}
}

func TestParseEnvTools_WhitespaceTrimmed(t *testing.T) {
	t.Setenv("AGENT_EN_PLACE_TOOLS", " node@latest , python@3.12 ")
	specs := parseEnvTools()

	if len(specs) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(specs))
	}

	if specs[0].name != "node" {
		t.Errorf("expected name 'node', got %q", specs[0].name)
	}
	if specs[1].name != "python" {
		t.Errorf("expected name 'python', got %q", specs[1].name)
	}
}

func TestCollectToolSpecs_EnvOverridesUserTools(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Set env var with node@20 — this should override mise.toml's node@18
	t.Setenv("AGENT_EN_PLACE_TOOLS", "node@20")
	t.Setenv("AGENT_EN_PLACE_SPECIFIED_TOOLS_ONLY", "")

	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	// Simulate a mise.toml with node@18
	miseFile := &fileSpec{
		path: "mise.toml",
		data: []byte("[tools]\nnode = \"18\"\n"),
	}

	collection := collectToolSpecs(nil, miseFile, spec, imgCfg, "claude", false)

	// Find node in the deduped specs — should have version "20" from env var
	var nodeSpec *toolDescriptor
	for i := range collection.specs {
		if collection.specs[i].name == "node" {
			nodeSpec = &collection.specs[i]
			break
		}
	}

	if nodeSpec == nil {
		t.Fatal("expected node in collected specs")
	}
	if nodeSpec.version != "20" {
		t.Errorf("expected env var to override node version to 20, got %s", nodeSpec.version)
	}
}

func TestCollectToolSpecs_EnvMergesWithFileTools(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Set env var with ruby — mise.toml has node
	t.Setenv("AGENT_EN_PLACE_TOOLS", "ruby@3.2")
	t.Setenv("AGENT_EN_PLACE_SPECIFIED_TOOLS_ONLY", "")

	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	// Simulate a mise.toml with node
	miseFile := &fileSpec{
		path: "mise.toml",
		data: []byte("[tools]\nnode = \"18\"\n"),
	}

	collection := collectToolSpecs(nil, miseFile, spec, imgCfg, "claude", false)

	// Both ruby (from env) and node (from mise.toml) should be present
	toolNames := make(map[string]string)
	for _, s := range collection.specs {
		toolNames[s.name] = s.version
	}

	if v, ok := toolNames["ruby"]; !ok || v != "3.2" {
		t.Errorf("expected ruby@3.2 from env var, got %v (present=%v)", v, ok)
	}
	if v, ok := toolNames["node"]; !ok || v != "18" {
		t.Errorf("expected node@18 from mise.toml, got %v (present=%v)", v, ok)
	}
}

func TestCollectToolSpecs_SpecifiedToolsOnly(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	t.Setenv("AGENT_EN_PLACE_TOOLS", "python@3.12")
	t.Setenv("AGENT_EN_PLACE_SPECIFIED_TOOLS_ONLY", "1")

	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	// Even though these files are passed, they should be skipped in specifiedOnly mode
	miseFile := &fileSpec{
		path: "mise.toml",
		data: []byte("[tools]\nnode = \"18\"\nruby = \"3.2\"\n"),
	}
	toolFile := &fileSpec{
		path: ".tool-versions",
		data: []byte("go 1.21\n"),
	}

	collection := collectToolSpecs(toolFile, miseFile, spec, imgCfg, "claude", false)

	toolNames := make(map[string]bool)
	for _, s := range collection.specs {
		toolNames[s.name] = true
		// Also index by sanitized name for ensureDefaultTool-added tools
		toolNames[sanitizeTagComponent(s.name)] = true
	}

	// python should be present (from env var)
	if !toolNames["python"] {
		t.Error("expected python from env var to be present")
	}

	// Agent's own tool should be present (ensureDefaultTool)
	agentToolName := sanitizeTagComponent(spec.MiseToolName)
	if !toolNames[agentToolName] {
		t.Errorf("expected agent tool %s to be present", agentToolName)
	}

	// node, ruby, go from file sources should NOT be present
	if toolNames["node"] {
		t.Error("expected node from mise.toml to be skipped in specifiedOnly mode")
	}
	if toolNames["ruby"] {
		t.Error("expected ruby from mise.toml to be skipped in specifiedOnly mode")
	}
	if toolNames["go"] {
		t.Error("expected go from .tool-versions to be skipped in specifiedOnly mode")
	}

	// No idiomatic paths should be collected
	if len(collection.idiomaticPaths) != 0 {
		t.Errorf("expected no idiomatic paths in specifiedOnly mode, got %v", collection.idiomaticPaths)
	}
}

func TestCollectToolSpecs_SpecifiedToolsOnlyWithoutToolsEnv(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Set SPECIFIED_TOOLS_ONLY without TOOLS — should warn and behave as normal
	t.Setenv("AGENT_EN_PLACE_TOOLS", "")
	t.Setenv("AGENT_EN_PLACE_SPECIFIED_TOOLS_ONLY", "1")

	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	// Provide a mise.toml with tools — these should still be collected
	// since SPECIFIED_TOOLS_ONLY is ignored without TOOLS
	miseFile := &fileSpec{
		path: "mise.toml",
		data: []byte("[tools]\nnode = \"18\"\n"),
	}

	collection := collectToolSpecs(nil, miseFile, spec, imgCfg, "claude", false)

	// node should be present because specifiedOnly was ignored
	toolNames := make(map[string]bool)
	for _, s := range collection.specs {
		toolNames[s.name] = true
	}

	if !toolNames["node"] {
		t.Error("expected node from mise.toml to be present when SPECIFIED_TOOLS_ONLY is ignored (no TOOLS set)")
	}
}

func TestCollectToolSpecs_EnvToolsTriggersTransitiveDeps(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Specify node via env var — this should trigger python as a transitive dep
	t.Setenv("AGENT_EN_PLACE_TOOLS", "node@20")
	t.Setenv("AGENT_EN_PLACE_SPECIFIED_TOOLS_ONLY", "")

	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	collection := collectToolSpecs(nil, nil, spec, imgCfg, "claude", false)

	toolNames := make(map[string]bool)
	for _, s := range collection.specs {
		toolNames[s.name] = true
	}

	if !toolNames["node"] {
		t.Error("expected node to be present")
	}
	if !toolNames["python"] {
		t.Error("expected python to be present as transitive dependency of user-specified node (via env var)")
	}
}

func TestCollectToolSpecs_EnvToolsAreInUserToolsSet(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	t.Setenv("AGENT_EN_PLACE_TOOLS", "node@20")
	t.Setenv("AGENT_EN_PLACE_SPECIFIED_TOOLS_ONLY", "")

	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	collection := collectToolSpecs(nil, nil, spec, imgCfg, "claude", false)

	// node should be in userTools (for transitive dep resolution and additional packages)
	if !collection.userTools["node"] {
		t.Error("expected env var tool 'node' to be in userTools set")
	}
}

func TestCollectToolSpecs_EnvToolInMiseAgentConfig(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	t.Setenv("AGENT_EN_PLACE_TOOLS", "ruby@3.2")
	t.Setenv("AGENT_EN_PLACE_SPECIFIED_TOOLS_ONLY", "")

	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	collection := collectToolSpecs(nil, nil, spec, imgCfg, "claude", false)

	// Build mise.agent.toml — ruby should appear since there's no user mise.toml
	data, err := buildAgentMiseConfig(nil, collection, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := string(data)
	if !strings.Contains(result, `ruby = "3.2"`) {
		t.Errorf("expected ruby@3.2 in mise.agent.toml, got:\n%s", result)
	}
}

func TestCollectToolSpecs_EnvToolOverridesInMiseAgentConfig(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Env var says node@20, user mise.toml says node@18
	t.Setenv("AGENT_EN_PLACE_TOOLS", "node@20")
	t.Setenv("AGENT_EN_PLACE_SPECIFIED_TOOLS_ONLY", "")

	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")

	userMise := []byte("[tools]\nnode = \"18\"\n")
	miseFile := &fileSpec{
		path: "mise.toml",
		data: userMise,
	}

	collection := collectToolSpecs(nil, miseFile, spec, imgCfg, "claude", false)

	// Env var tool (node@20) is in idiomaticInfos but the user's mise.toml
	// also has node. Since user mise.toml has node, it should be filtered out
	// of mise.agent.toml — the user's mise.toml takes ownership of that key.
	// BUT the collected spec should have node@20 (env wins in dedup).
	var nodeSpec *toolDescriptor
	for i := range collection.specs {
		if collection.specs[i].name == "node" {
			nodeSpec = &collection.specs[i]
			break
		}
	}
	if nodeSpec == nil {
		t.Fatal("expected node in collected specs")
	}
	if nodeSpec.version != "20" {
		t.Errorf("expected node version 20 (from env), got %s", nodeSpec.version)
	}
}

func TestCollectMiseEnvVars(t *testing.T) {
	tests := []struct {
		name    string
		environ []string
		want    [][2]string
	}{
		{
			name:    "empty environment",
			environ: nil,
			want:    nil,
		},
		{
			name:    "no MISE_ vars",
			environ: []string{"HOME=/home/user", "PATH=/usr/bin", "AGENT_EN_PLACE_TOOLS=node@20"},
			want:    nil,
		},
		{
			name:    "single MISE_ var",
			environ: []string{"MISE_NODE_DEFAULT_PACKAGES_FILE=/path/to/file"},
			want:    [][2]string{{"MISE_NODE_DEFAULT_PACKAGES_FILE", "/path/to/file"}},
		},
		{
			name: "multiple MISE_ vars sorted",
			environ: []string{
				"MISE_PYTHON_DEFAULT_PACKAGES_FILE=/path/python",
				"HOME=/home/user",
				"MISE_NODE_DEFAULT_PACKAGES_FILE=/path/node",
				"MISE_LEGACY_VERSION_FILE=1",
			},
			want: [][2]string{
				{"MISE_LEGACY_VERSION_FILE", "1"},
				{"MISE_NODE_DEFAULT_PACKAGES_FILE", "/path/node"},
				{"MISE_PYTHON_DEFAULT_PACKAGES_FILE", "/path/python"},
			},
		},
		{
			name:    "MISE_ENV is excluded",
			environ: []string{"MISE_ENV=agent", "MISE_NODE_DEFAULT_PACKAGES_FILE=/path"},
			want:    [][2]string{{"MISE_NODE_DEFAULT_PACKAGES_FILE", "/path"}},
		},
		{
			name:    "MISE_ENV alone is excluded",
			environ: []string{"MISE_ENV=production"},
			want:    nil,
		},
		{
			name:    "MISE_SHELL is excluded",
			environ: []string{"MISE_SHELL=zsh", "MISE_NODE_DEFAULT_PACKAGES_FILE=/path"},
			want:    [][2]string{{"MISE_NODE_DEFAULT_PACKAGES_FILE", "/path"}},
		},
		{
			name:    "MISE_ENV and MISE_SHELL both excluded",
			environ: []string{"MISE_ENV=agent", "MISE_SHELL=bash", "MISE_LEGACY_VERSION_FILE=1"},
			want:    [][2]string{{"MISE_LEGACY_VERSION_FILE", "1"}},
		},
		{
			name:    "value with equals sign",
			environ: []string{"MISE_SOME_SETTING=key=value"},
			want:    [][2]string{{"MISE_SOME_SETTING", "key=value"}},
		},
		{
			name:    "empty value",
			environ: []string{"MISE_SOME_FLAG="},
			want:    [][2]string{{"MISE_SOME_FLAG", ""}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectMiseEnvVars(tt.environ)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("collectMiseEnvVars() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDockerfile_Claude_WithMiseEnvVars(t *testing.T) {
	imgCfg := loadTestConfig(t)
	spec := getToolSpec(t, imgCfg, "claude")
	collection := buildDefaultCollection("claude", spec)

	environ := []string{
		"HOME=/home/user",
		"MISE_PYTHON_DEFAULT_PACKAGES_FILE=/home/user/.default-python-packages",
		"MISE_ENV=agent",
		"MISE_NODE_DEFAULT_PACKAGES_FILE=/home/user/.default-npm-packages",
		"PATH=/usr/bin",
	}

	got := buildDockerfile(false, false, collection, spec, imgCfg, "claude", environ)

	goldenTest(t, "dockerfile_claude_with_mise_env_vars.golden", got)
}

func TestConfigMiseEnvVars(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]any
		want [][2]string
	}{
		{
			name: "nil map",
			env:  nil,
			want: nil,
		},
		{
			name: "empty map",
			env:  map[string]any{},
			want: nil,
		},
		{
			name: "string value",
			env:  map[string]any{"node_default_packages_file": "/path/to/file"},
			want: [][2]string{{"MISE_NODE_DEFAULT_PACKAGES_FILE", "/path/to/file"}},
		},
		{
			name: "boolean false",
			env:  map[string]any{"ruby_compile": false},
			want: [][2]string{{"MISE_RUBY_COMPILE", "false"}},
		},
		{
			name: "boolean true",
			env:  map[string]any{"experimental": true},
			want: [][2]string{{"MISE_EXPERIMENTAL", "true"}},
		},
		{
			name: "integer value",
			env:  map[string]any{"jobs": 4},
			want: [][2]string{{"MISE_JOBS", "4"}},
		},
		{
			name: "multiple values sorted",
			env: map[string]any{
				"ruby_compile": false,
				"experimental": true,
				"jobs":         4,
				"color":        "always",
			},
			want: [][2]string{
				{"MISE_COLOR", "always"},
				{"MISE_EXPERIMENTAL", "true"},
				{"MISE_JOBS", "4"},
				{"MISE_RUBY_COMPILE", "false"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configMiseEnvVars(tt.env)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("configMiseEnvVars() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMergeMiseEnvVars(t *testing.T) {
	tests := []struct {
		name       string
		configVars [][2]string
		hostVars   [][2]string
		want       [][2]string
	}{
		{
			name:       "both nil",
			configVars: nil,
			hostVars:   nil,
			want:       nil,
		},
		{
			name:       "config only",
			configVars: [][2]string{{"MISE_RUBY_COMPILE", "false"}},
			hostVars:   nil,
			want:       [][2]string{{"MISE_RUBY_COMPILE", "false"}},
		},
		{
			name:       "host only",
			configVars: nil,
			hostVars:   [][2]string{{"MISE_JOBS", "8"}},
			want:       [][2]string{{"MISE_JOBS", "8"}},
		},
		{
			name:       "host overrides config",
			configVars: [][2]string{{"MISE_RUBY_COMPILE", "false"}},
			hostVars:   [][2]string{{"MISE_RUBY_COMPILE", "true"}},
			want:       [][2]string{{"MISE_RUBY_COMPILE", "true"}},
		},
		{
			name: "merge disjoint sets sorted",
			configVars: [][2]string{
				{"MISE_RUBY_COMPILE", "false"},
			},
			hostVars: [][2]string{
				{"MISE_JOBS", "8"},
			},
			want: [][2]string{
				{"MISE_JOBS", "8"},
				{"MISE_RUBY_COMPILE", "false"},
			},
		},
		{
			name: "host overrides one config key among many",
			configVars: [][2]string{
				{"MISE_COLOR", "always"},
				{"MISE_JOBS", "4"},
				{"MISE_RUBY_COMPILE", "false"},
			},
			hostVars: [][2]string{
				{"MISE_JOBS", "8"},
			},
			want: [][2]string{
				{"MISE_COLOR", "always"},
				{"MISE_JOBS", "8"},
				{"MISE_RUBY_COMPILE", "false"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeMiseEnvVars(tt.configVars, tt.hostVars)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mergeMiseEnvVars() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMergeConfigs_MiseEnv(t *testing.T) {
	base := &ImageConfig{
		Tools:  make(map[string]ToolConfigEntry),
		Agents: make(map[string]AgentConfig),
		Mise: MiseSettings{
			Env: map[string]any{
				"ruby_compile": false,
				"jobs":         4,
			},
		},
	}
	user := &ImageConfig{
		Tools:  make(map[string]ToolConfigEntry),
		Agents: make(map[string]AgentConfig),
		Mise: MiseSettings{
			Env: map[string]any{
				"jobs":         8,
				"experimental": true,
			},
		},
	}

	result := mergeConfigs(base, user)

	if len(result.Mise.Env) != 3 {
		t.Fatalf("expected 3 env vars, got %d: %v", len(result.Mise.Env), result.Mise.Env)
	}
	if result.Mise.Env["ruby_compile"] != false {
		t.Errorf("expected ruby_compile=false, got %v", result.Mise.Env["ruby_compile"])
	}
	if result.Mise.Env["jobs"] != 8 {
		t.Errorf("expected jobs=8 (user override), got %v", result.Mise.Env["jobs"])
	}
	if result.Mise.Env["experimental"] != true {
		t.Errorf("expected experimental=true, got %v", result.Mise.Env["experimental"])
	}
}
