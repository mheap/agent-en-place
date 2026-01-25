package agent

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "embed"

	"github.com/moby/moby/client"
)

//go:embed assets/agent-entrypoint.sh
var agentEntrypointScript []byte

//go:embed config.yaml
var defaultConfigYAML []byte

const imageRepository = "mheap/agent-en-place"

type Config struct {
	Debug          bool
	Rebuild        bool
	DockerfileOnly bool
	Tool           string
	ConfigPath     string
}

type ToolSpec struct {
	MiseToolName     string
	ConfigKey        string
	Command          string
	ConfigDir        string
	AdditionalMounts []string
	EnvVars          []string
}

// getLabelName returns a friendly label name for a tool
// It extracts the last component from npm package names (e.g., "npm:@openai/codex" -> "codex")
func getLabelName(toolName string) string {
	// For npm packages like "npm:@openai/codex", extract the last part
	if idx := strings.LastIndex(toolName, "/"); idx >= 0 {
		return toolName[idx+1:]
	}
	// For simple names like "npm:opencode-ai", strip the prefix
	if idx := strings.Index(toolName, ":"); idx >= 0 {
		return toolName[idx+1:]
	}
	return toolName
}

func Run(cfg Config) error {
	imgCfg, err := LoadMergedConfig(defaultConfigYAML, cfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	agentCfg, ok := imgCfg.GetAgent(cfg.Tool)
	if !ok {
		return fmt.Errorf("unknown agent: %s (available: %s)", cfg.Tool, strings.Join(imgCfg.AgentNames(), ", "))
	}
	spec := agentCfg.ToToolSpec()

	toolFile, err := optionalFileSpec(".tool-versions")
	if err != nil {
		return fmt.Errorf("failed to read .tool-versions: %w", err)
	}
	miseFile, err := optionalFileSpec("mise.toml")
	if err != nil {
		return fmt.Errorf("failed to read mise.toml: %w", err)
	}

	collection := collectToolSpecs(toolFile, miseFile, spec, imgCfg, cfg.Tool)
	hasNode := collectionHasNode(toolFile, miseFile, collection)
	if cfg.DockerfileOnly {
		fmt.Print(buildDockerfile(toolFile != nil, miseFile != nil, hasNode, collection, spec, imgCfg))
		return nil
	}
	imageName := buildImageName(collection.specs)

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to connect to docker daemon: %w", err)
	}

	needBuild := !imageExists(ctx, cli, imageName) || cfg.Rebuild

	if needBuild {
		buildCtx, err := makeBuildContext(toolFile, miseFile, collection, hasNode, spec, imgCfg)
		if err != nil {
			return fmt.Errorf("failed to prepare build context: %w", err)
		}

		buildResp, err := cli.ImageBuild(ctx, buildCtx, client.ImageBuildOptions{
			Tags:        []string{imageName},
			Remove:      true,
			PullParent:  true,
			Dockerfile:  "Dockerfile",
			ForceRemove: true,
		})
		if err != nil {
			return fmt.Errorf("failed to build image: %w", err)
		}
		defer buildResp.Body.Close()

		if err := handleBuildOutput(buildResp.Body, cfg.Debug); err != nil {
			return fmt.Errorf("failed to read build output: %w", err)
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "~"
	}
	configMount := filepath.Join(home, spec.ConfigDir)
	containerConfigPath := filepath.Join("/home/agent", spec.ConfigDir)

	envs := []string{}
	for _, env := range spec.EnvVars {
		envs = append(envs, fmt.Sprintf("-e %s", env))
	}

	volumes := []string{
		fmt.Sprintf("-v %s:/workdir", filepath.Clean(cwd)),
		fmt.Sprintf("-v %s:%s", filepath.Clean(configMount), containerConfigPath),
	}
	for _, mount := range spec.AdditionalMounts {
		hostPath := filepath.Join(home, mount)
		containerPath := filepath.Join("/home/agent", mount)
		volumes = append(volumes, fmt.Sprintf("-v %s:%s", filepath.Clean(hostPath), containerPath))
	}

	allArgs := append(envs, volumes...)
	fmt.Printf("docker run --rm -it %s %s %s\n", strings.Join(allArgs, " "), imageName, spec.Command)
	return nil
}

func makeBuildContext(toolFile, miseFile *fileSpec, collection collectResult, needsLibatomic bool, spec ToolSpec, imgCfg *ImageConfig) (io.Reader, error) {

	dockerfile := buildDockerfile(toolFile != nil, miseFile != nil, needsLibatomic, collection, spec, imgCfg)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	if err := writeFileToTar(tw, "Dockerfile", []byte(dockerfile), 0644); err != nil {
		return nil, err
	}

	if toolFile != nil {
		if err := writeFileToTar(tw, toolFile.path, toolFile.data, toolFile.mode); err != nil {
			return nil, err
		}
	}
	if miseFile != nil {
		if err := writeFileToTar(tw, miseFile.path, miseFile.data, miseFile.mode); err != nil {
			return nil, err
		}
	}
	if err := writeIdiomaticFiles(tw, collection.idiomaticPaths); err != nil {
		return nil, err
	}
	if err := writeFileToTar(tw, "assets/agent-entrypoint.sh", agentEntrypointScript, 0755); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}

	return bytes.NewReader(buf.Bytes()), nil
}

func buildDockerfile(hasTool, hasMise, needLibatomic bool, collection collectResult, spec ToolSpec, imgCfg *ImageConfig) string {
	var b strings.Builder

	// Use configured base image
	baseImage := imgCfg.Image.Base
	if baseImage == "" {
		baseImage = "debian:12-slim"
	}

	// Use configured packages (already includes libatomic1 in default config)
	packages := imgCfg.Image.Packages

	b.WriteString(fmt.Sprintf("FROM %s\n\n", baseImage))
	b.WriteString("RUN apt-get update && apt-get install -y --no-install-recommends ")
	b.WriteString(strings.Join(packages, " "))
	b.WriteString("\n")

	// Use configured mise installation commands (joined with && in a single RUN)
	if len(imgCfg.Mise.Install) > 0 {
		b.WriteString("RUN ")
		b.WriteString(strings.Join(imgCfg.Mise.Install, " && "))
		b.WriteString("\n")
	}

	b.WriteString("RUN rm -rf /var/lib/apt/lists/*\n\n")
	b.WriteString("RUN groupadd -r agent && useradd -m -r -u 1000 -g agent -s /bin/bash agent\n")
	b.WriteString("ENV HOME=/home/agent\n")
	b.WriteString("ENV PATH=\"/home/agent/.local/share/mise/shims:/home/agent/.local/bin:${PATH}\"\n\n")
	b.WriteString("RUN mkdir -p /home/agent/.config/mise\n")
	b.WriteString(buildToolLabels(collection.specs))
	b.WriteString("WORKDIR /home/agent\n")

	if hasTool {
		b.WriteString("COPY .tool-versions .tool-versions\n")
	}
	if hasMise {
		b.WriteString("COPY mise.toml /home/agent/.config/mise/config.toml\n")
	} else {
		b.WriteString("RUN printf '%s\\n' \\\n")
		for _, line := range defaultMiseLines(collection, spec) {
			if line == "" {
				b.WriteString("  '' \\\n")
				continue
			}
			b.WriteString(fmt.Sprintf("  '%s' \\\n", escapeForPrintf(line)))
		}
		b.WriteString("  > /home/agent/.config/mise/config.toml\n")
	}
	if hasTool || hasMise {
		b.WriteString("RUN chown agent:agent")
		if hasTool {
			b.WriteString(" .tool-versions")
		}
		b.WriteString(" /home/agent/.config/mise/config.toml\n")
	}

	b.WriteString("COPY assets/agent-entrypoint.sh /usr/local/bin/agent-entrypoint\n")
	b.WriteString("RUN chmod +x /usr/local/bin/agent-entrypoint\n")

	b.WriteString("USER agent\n")
	b.WriteString("RUN mise trust\n")
	b.WriteString("RUN mise install\n")
	b.WriteString("RUN printf 'export PATH=\"/home/agent/.local/share/mise/shims:/home/agent/.local/bin:$PATH\"\\n' > /home/agent/.bashrc\n")
	b.WriteString("RUN printf 'source ~/.bashrc\\n' > /home/agent/.bash_profile\n")
	b.WriteString("WORKDIR /workdir\n")
	b.WriteString("ENTRYPOINT [\"/bin/bash\", \"/usr/local/bin/agent-entrypoint\"]\n")
	return b.String()
}

type fileSpec struct {
	path string
	data []byte
	mode int64
}

func optionalFileSpec(path string) (*fileSpec, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return &fileSpec{
		path: path,
		data: data,
		mode: int64(info.Mode() & os.ModePerm),
	}, nil
}

type toolDescriptor struct {
	name      string
	version   string
	labelName string // friendly name for Docker labels (e.g., "codex" instead of "npm-openai-codex")
}

type collectResult struct {
	specs          []toolDescriptor
	idiomaticPaths []string
	idiomaticInfos []idiomaticInfo
}

type idiomaticInfo struct {
	tool      string
	version   string
	path      string
	configKey string
}

func collectToolSpecs(toolFile, miseFile *fileSpec, spec ToolSpec, imgCfg *ImageConfig, agentName string) collectResult {
	specs := parseToolVersions(toolFile)
	specs = append(specs, parseMiseToml(miseFile)...)
	idiomatic := parseIdiomaticFiles()
	for _, info := range idiomatic {
		if info.version == "" {
			continue
		}
		specs = append(specs, toolDescriptor{name: info.tool, version: info.version})
	}

	// Add tools from config's dependency resolution
	// These come after mise.toml/.tool-versions so they have lower priority
	configTools := imgCfg.ResolveToolDeps(agentName)
	specs = append(specs, configTools...)

	deduped := dedupeToolSpecs(specs)
	deduped = ensureDefaultTool(deduped, spec)
	infos := ensureToolInfo(idiomatic, spec)
	return collectResult{
		specs:          deduped,
		idiomaticPaths: uniquePaths(infos),
		idiomaticInfos: infos,
	}
}

func dedupeToolSpecs(specs []toolDescriptor) []toolDescriptor {
	seen := map[string]bool{}
	var result []toolDescriptor
	for _, spec := range specs {
		key := sanitizeTagComponent(spec.name)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = true
		version := spec.version
		if version == "" {
			version = "latest"
		}
		labelName := spec.labelName
		if labelName == "" {
			labelName = getLabelName(spec.name)
		}
		result = append(result, toolDescriptor{name: key, version: version, labelName: labelName})
	}
	return result
}

func ensureDefaultTool(specs []toolDescriptor, toolSpec ToolSpec) []toolDescriptor {
	sanitizedName := sanitizeTagComponent(toolSpec.MiseToolName)
	for _, spec := range specs {
		if spec.name == sanitizedName {
			return specs
		}
	}
	return append(specs, toolDescriptor{
		name:      toolSpec.MiseToolName,
		version:   "latest",
		labelName: getLabelName(toolSpec.MiseToolName),
	})
}

func ensureToolInfo(infos []idiomaticInfo, spec ToolSpec) []idiomaticInfo {
	for _, info := range infos {
		if info.configKey == spec.ConfigKey {
			return infos
		}
	}
	return append(infos, idiomaticInfo{tool: spec.MiseToolName, version: "latest", configKey: spec.ConfigKey})
}

func collectionHasNode(toolFile, miseFile *fileSpec, collection collectResult) bool {
	if containsNodeSpec(toolFile) || containsNodeSpec(miseFile) {
		return true
	}
	return hasNodeTool(collection.specs)
}

func hasNodeTool(specs []toolDescriptor) bool {
	for _, spec := range specs {
		if spec.name == "node" {
			return true
		}
	}
	return false
}

func uniquePaths(infos []idiomaticInfo) []string {
	seen := map[string]bool{}
	var result []string
	for _, info := range infos {
		if info.path == "" {
			continue
		}
		if seen[info.path] {
			continue
		}
		seen[info.path] = true
		result = append(result, info.path)
	}
	return result
}

func parseToolVersions(spec *fileSpec) []toolDescriptor {
	if spec == nil {
		return nil
	}
	var specs []toolDescriptor
	scanner := bufio.NewScanner(bytes.NewReader(spec.data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		version := "latest"
		if len(fields) > 1 {
			version = fields[1]
		}
		specs = append(specs, toolDescriptor{name: name, version: version})
	}
	return specs
}

func parseMiseToml(spec *fileSpec) []toolDescriptor {
	if spec == nil {
		return nil
	}
	var specs []toolDescriptor
	scanner := bufio.NewScanner(bytes.NewReader(spec.data))
	insideTool := false
	var current toolDescriptor
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[[tool") || strings.HasPrefix(line, "[tool") {
			insideTool = true
			current = toolDescriptor{}
			continue
		}
		if strings.HasPrefix(line, "[") {
			insideTool = false
			continue
		}
		if !insideTool {
			continue
		}
		if idx := strings.Index(line, "="); idx >= 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			value = strings.Trim(value, "\"'")
			switch key {
			case "name":
				current.name = value
			case "version":
				current.version = value
			}
		}
		if current.name != "" && current.version != "" {
			specs = append(specs, current)
			current = toolDescriptor{}
		}
	}
	return specs
}

var idiomaticToolFiles = map[string][]string{
	"crystal": {".crystal-version"},
	"elixir":  {".exenv-version"},
	"go":      {".go-version"},
	"java":    {".java-version", ".sdkmanrc"},
	"node":    {".nvmrc", ".node-version"},
	"python":  {".python-version", ".python-versions"},
	"ruby":    {".ruby-version", "Gemfile"},
	"yarn":    {".yvmrc"},
	"bun":     {".bun-version"},
}

func parseIdiomaticFiles() []idiomaticInfo {
	var infos []idiomaticInfo
	for tool, paths := range idiomaticToolFiles {
		for _, path := range paths {
			version, ok := readIdiomaticVersion(tool, path)
			if !ok || version == "" {
				continue
			}
			configKey := tool
			if strings.Contains(tool, ":") {
				configKey = tool
			}
			infos = append(infos, idiomaticInfo{tool: tool, version: version, path: path, configKey: configKey})
			break
		}
	}
	return infos
}

func readIdiomaticVersion(tool, path string) (string, bool) {
	switch path {
	case "Gemfile":
		return parseGemfileVersion(path)
	case ".sdkmanrc":
		return parseSdkmanVersion(path)
	default:
		line, ok := readFirstLine(path)
		if !ok {
			return "", false
		}
		return line, true
	}
}

func readFirstLine(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false
		}
		return "", false
	}
	line := strings.TrimSpace(strings.Split(string(data), "\n")[0])
	return line, line != ""
}

func parseGemfileVersion(path string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "ruby") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				version := strings.Trim(fields[1], "\"'")
				return version, version != ""
			}
		}
	}
	return "", false
}

func parseSdkmanVersion(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "java=") {
			version := strings.TrimPrefix(line, "java=")
			return version, version != ""
		}
	}
	return "", false
}

func buildImageName(specs []toolDescriptor) string {
	if len(specs) == 0 {
		return fmt.Sprintf("%s:latest", imageRepository)
	}
	var parts []string
	for _, spec := range specs {
		name := sanitizeTagComponent(spec.name)
		if name == "" {
			name = "tool"
		}
		version := sanitizeTagComponent(spec.version)
		if version == "" {
			version = "latest"
		}
		parts = append(parts, fmt.Sprintf("%s-%s", name, version))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%s:latest", imageRepository)
	}
	return fmt.Sprintf("%s:%s", imageRepository, strings.Join(parts, "-"))
}

func buildToolLabels(specs []toolDescriptor) string {
	var b strings.Builder
	for _, spec := range specs {
		name := spec.labelName
		if name == "" {
			name = sanitizeTagComponent(spec.name)
		}
		if name == "" {
			continue
		}
		version := sanitizeTagComponent(spec.version)
		if version == "" {
			version = "latest"
		}
		key := fmt.Sprintf("com.mheap.agent-en-place.%s", name)
		b.WriteString(fmt.Sprintf("LABEL %s=\"%s\"\n", key, version))
	}
	return b.String()
}

func defaultMiseLines(collection collectResult, spec ToolSpec) []string {
	lines := []string{"[tools]"}
	seen := map[string]bool{}
	hasTool := false
	for _, info := range collection.idiomaticInfos {
		version := strings.TrimSpace(info.version)
		if version == "" {
			continue
		}
		key := info.configKey
		if key == "" {
			key = info.tool
		}
		if key == spec.ConfigKey {
			hasTool = true
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		lines = append(lines, fmt.Sprintf("\"%s\" = \"%s\"", escapeDoubleQuote(key), escapeDoubleQuote(version)))
	}
	if !hasTool {
		lines = append(lines, fmt.Sprintf("\"%s\" = \"latest\"", escapeDoubleQuote(spec.ConfigKey)))
	}
	return lines
}

func escapeDoubleQuote(value string) string {
	if value == "" {
		return ""
	}
	return strings.ReplaceAll(value, "\"", "\"\"")
}

func escapeForPrintf(line string) string {
	if line == "" {
		return ""
	}
	return strings.ReplaceAll(line, "'", "'\"'\"'")
}

func sanitizeTagComponent(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastHyphen := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case r == '.':
			b.WriteRune('.')
			lastHyphen = false
		case r == '+' || r == '@' || r == ':' || r == '/' || r == '_' || r == '-':
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		default:
			// skip other characters
		}
	}
	out := strings.Trim(b.String(), "-")
	return out
}

func containsNodeSpec(spec *fileSpec) bool {
	if spec == nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(spec.data)), "node")
}

func writeFileToTar(tw *tar.Writer, name string, data []byte, mode int64) error {
	header := &tar.Header{
		Name: name,
		Mode: mode,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	if _, err := tw.Write(data); err != nil {
		return err
	}
	return nil
}

func writeIdiomaticFiles(tw *tar.Writer, paths []string) error {
	for _, path := range paths {
		spec, err := optionalFileSpec(path)
		if err != nil {
			return err
		}
		if spec == nil {
			continue
		}
		if err := writeFileToTar(tw, spec.path, spec.data, spec.mode); err != nil {
			return err
		}
	}
	return nil
}

func handleBuildOutput(rc io.Reader, debug bool) error {
	if debug {
		_, err := io.Copy(os.Stdout, rc)
		return err
	}
	_, err := io.Copy(io.Discard, rc)
	return err
}

func imageExists(ctx context.Context, cli *client.Client, name string) bool {
	_, err := cli.ImageInspect(ctx, name)
	return err == nil
}
