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

			// Basic case: no .tool-versions
			got := buildDockerfile(false, collection, spec, imgCfg, tt.tool)

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

	// hasTool=true
	got := buildDockerfile(true, collection, spec, imgCfg, "claude")

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

	// hasTool=false (mise.toml is always generated now)
	got := buildDockerfile(false, collection, spec, imgCfg, "claude")

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

	// hasTool=false
	got := buildDockerfile(false, collection, spec, imgCfg, "claude")

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

	// hasTool=true
	got := buildDockerfile(true, collection, spec, imgCfg, "claude")

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

	// hasTool=false
	got := buildDockerfile(false, collection, spec, imgCfg, "claude")

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

func TestBuildMiseConfig_NoUserFile(t *testing.T) {
	spec := ToolSpec{
		MiseToolName: "npm:@anthropic-ai/claude-code",
		ConfigKey:    "npm:@anthropic-ai/claude-code",
	}

	collection := collectResult{
		idiomaticInfos: []idiomaticInfo{
			{tool: "node", version: "20.0.0", configKey: "node"},
		},
	}

	data, err := buildMiseConfig(nil, collection, spec)
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

func TestBuildMiseConfig_WithUserFile(t *testing.T) {
	spec := ToolSpec{
		MiseToolName: "npm:@anthropic-ai/claude-code",
		ConfigKey:    "npm:@anthropic-ai/claude-code",
	}

	// User's mise.toml with python
	userMise := []byte(`[tools]
python = "3.12.0"
`)

	collection := collectResult{
		idiomaticInfos: []idiomaticInfo{
			{tool: "node", version: "20.0.0", configKey: "node"},
		},
	}

	data, err := buildMiseConfig(userMise, collection, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := string(data)

	// Should contain user's python
	if !strings.Contains(result, "python") || !strings.Contains(result, "3.12.0") {
		t.Errorf("expected python = 3.12.0, got: %s", result)
	}

	// Should contain node from collection
	if !strings.Contains(result, "node") || !strings.Contains(result, "20.0.0") {
		t.Errorf("expected node = 20.0.0, got: %s", result)
	}

	// Should contain agent's primary tool
	if !strings.Contains(result, "npm:@anthropic-ai/claude-code") {
		t.Errorf("expected agent tool, got: %s", result)
	}
}

func TestBuildMiseConfig_UserVersionPrecedence(t *testing.T) {
	spec := ToolSpec{
		MiseToolName: "npm:@anthropic-ai/claude-code",
		ConfigKey:    "npm:@anthropic-ai/claude-code",
	}

	// User specifies node 18.0.0
	userMise := []byte(`[tools]
node = "18.0.0"
`)

	// Collection has node 20.0.0
	collection := collectResult{
		idiomaticInfos: []idiomaticInfo{
			{tool: "node", version: "20.0.0", configKey: "node"},
		},
	}

	data, err := buildMiseConfig(userMise, collection, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := string(data)

	// User's version should take precedence
	if !strings.Contains(result, "18.0.0") {
		t.Errorf("expected user's node version 18.0.0, got: %s", result)
	}

	// Collection's version should NOT be present
	if strings.Contains(result, "20.0.0") {
		t.Errorf("expected user version to take precedence, but found 20.0.0 in: %s", result)
	}
}

func TestBuildMiseConfig_PreservesOtherSections(t *testing.T) {
	spec := ToolSpec{
		MiseToolName: "npm:@anthropic-ai/claude-code",
		ConfigKey:    "npm:@anthropic-ai/claude-code",
	}

	// User's mise.toml with additional sections
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

	data, err := buildMiseConfig(userMise, collection, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := string(data)

	// Should preserve [settings] section
	if !strings.Contains(result, "[settings]") || !strings.Contains(result, "experimental") {
		t.Errorf("expected [settings] section to be preserved, got: %s", result)
	}

	// Should preserve [env] section
	if !strings.Contains(result, "[env]") || !strings.Contains(result, "MY_VAR") {
		t.Errorf("expected [env] section to be preserved, got: %s", result)
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

// TestBuildMiseConfig_AllAgents tests mise.toml generation for each agent in config.yaml
func TestBuildMiseConfig_AllAgents(t *testing.T) {
	imgCfg := loadTestConfig(t)

	tests := []struct {
		name           string
		expectedTools  []string // Tools that must be present in output
		notExpectTools []string // Tools that must NOT be present
	}{
		{
			name:          "codex",
			expectedTools: []string{"npm:@openai/codex", "node", "python"},
		},
		{
			name:          "opencode",
			expectedTools: []string{"npm:opencode-ai", "node", "python"},
		},
		{
			name:          "copilot",
			expectedTools: []string{"npm:@github/copilot", "node", "python"},
		},
		{
			name:          "claude",
			expectedTools: []string{"npm:@anthropic-ai/claude-code", "node", "python"},
		},
		{
			name:          "gemini",
			expectedTools: []string{"npm:@google/gemini-cli", "node", "python"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := getToolSpec(t, imgCfg, tt.name)

			// Build collection with resolved tool dependencies (simulating real behavior)
			toolDeps := imgCfg.ResolveToolDeps(tt.name)
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

			// Build mise.toml without user file
			data, err := buildMiseConfig(nil, collection, spec)
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

// TestBuildMiseConfig_AllAgents_WithUserMise tests that user mise.toml is correctly merged for each agent
func TestBuildMiseConfig_AllAgents_WithUserMise(t *testing.T) {
	imgCfg := loadTestConfig(t)

	// User mise.toml with custom tools
	userMise := []byte(`[tools]
ruby = "3.2.0"
go = "1.21.0"
`)

	agents := []string{"codex", "opencode", "copilot", "claude", "gemini"}

	for _, agentName := range agents {
		t.Run(agentName, func(t *testing.T) {
			spec := getToolSpec(t, imgCfg, agentName)

			// Build collection with resolved tool dependencies
			toolDeps := imgCfg.ResolveToolDeps(agentName)
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

			// Build mise.toml with user file
			data, err := buildMiseConfig(userMise, collection, spec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			result := string(data)

			// User tools should be preserved
			if !strings.Contains(result, "ruby") || !strings.Contains(result, "3.2.0") {
				t.Errorf("expected user's ruby tool to be preserved, got:\n%s", result)
			}
			if !strings.Contains(result, "go") || !strings.Contains(result, "1.21.0") {
				t.Errorf("expected user's go tool to be preserved, got:\n%s", result)
			}

			// Agent's primary tool should be added
			if !strings.Contains(result, spec.ConfigKey) {
				t.Errorf("expected agent tool %q to be added, got:\n%s", spec.ConfigKey, result)
			}

			// Dependencies should be added
			if !strings.Contains(result, "node") {
				t.Errorf("expected node dependency to be added, got:\n%s", result)
			}
			if !strings.Contains(result, "python") {
				t.Errorf("expected python dependency to be added, got:\n%s", result)
			}
		})
	}
}

// TestBuildMiseConfig_AllAgents_UserOverridesDefaults tests that user versions override agent defaults
func TestBuildMiseConfig_AllAgents_UserOverridesDefaults(t *testing.T) {
	imgCfg := loadTestConfig(t)

	// User mise.toml with custom node version (should override the default "latest")
	userMise := []byte(`[tools]
node = "18.19.0"
python = "3.11.0"
`)

	agents := []string{"codex", "opencode", "copilot", "claude", "gemini"}

	for _, agentName := range agents {
		t.Run(agentName, func(t *testing.T) {
			spec := getToolSpec(t, imgCfg, agentName)

			// Build collection with resolved tool dependencies (these have "latest" versions)
			toolDeps := imgCfg.ResolveToolDeps(agentName)
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

			// Build mise.toml with user file
			data, err := buildMiseConfig(userMise, collection, spec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			result := string(data)

			// User's node version should take precedence
			if !strings.Contains(result, "18.19.0") {
				t.Errorf("expected user's node version 18.19.0, got:\n%s", result)
			}

			// User's python version should take precedence
			if !strings.Contains(result, "3.11.0") {
				t.Errorf("expected user's python version 3.11.0, got:\n%s", result)
			}

			// Default "latest" version for node/python should NOT appear as a separate entry
			// (The agent tool itself may still have "latest")
			nodeLatestCount := strings.Count(result, `node = "latest"`)
			pythonLatestCount := strings.Count(result, `python = "latest"`)
			if nodeLatestCount > 0 {
				t.Errorf("expected user node version to override default, but found 'node = \"latest\"' in:\n%s", result)
			}
			if pythonLatestCount > 0 {
				t.Errorf("expected user python version to override default, but found 'python = \"latest\"' in:\n%s", result)
			}
		})
	}
}

// TestBuildMiseConfig_GoldenFiles tests mise.toml generation against golden files for each agent
func TestBuildMiseConfig_GoldenFiles(t *testing.T) {
	imgCfg := loadTestConfig(t)

	agents := []string{"codex", "opencode", "copilot", "claude", "gemini"}

	for _, agentName := range agents {
		t.Run(agentName, func(t *testing.T) {
			spec := getToolSpec(t, imgCfg, agentName)

			// Build collection with resolved tool dependencies
			toolDeps := imgCfg.ResolveToolDeps(agentName)
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

			// Build mise.toml without user file
			data, err := buildMiseConfig(nil, collection, spec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			goldenTest(t, "mise_"+agentName+".golden", string(data))
		})
	}
}
