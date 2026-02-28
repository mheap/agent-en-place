# agent-en-place

Build on-demand Docker containers for projects + agentic coding using [`mise`](https://github.com/jdx/mise).

## Prerequisites

- Docker (installed and running)
- Go 1.21+ (for building from source)
- Bash or Zsh shell
- `gh` CLI (required for GitHub Copilot provider only)

## Installation

### Homebrew (macOS/Linux)

```bash
brew install mheap/tap/agent-en-place
```

### Build from source

```bash
git clone https://github.com/mheap/agent-en-place
cd agent-en-place
go build
# Move binary to your PATH
mv agent-en-place /usr/local/bin/
```

### Download binary

Download the latest release for your platform from [GitHub Releases](https://github.com/mheap/agent-en-place/releases).

## Usage

Define a function in your `.bashrc` / `.zshrc` / other shell config file

```bash
function vibe() { bash -lc "$(agent-en-place $1)" }
```

Then in any directory, run `vibe <provider>`

The tool will:

1. Detect tool versions from your project's configuration files
2. Build a Docker image with those tools (or reuse cached image)
3. Generate and execute a `docker run` command
4. Launch the selected AI coding tool in the container

## Configuration

`agent-en-place` can be customized via YAML configuration files. See [docs/config.md](docs/config.md) for the full configuration reference.

### Config File Locations

Configuration files are loaded and merged in order (later files override earlier):

1. **Embedded defaults** - Built into the binary
2. **User config** - `~/.config/agent-en-place.yaml`
3. **Project config** - `./.agent-en-place.yaml`
4. **Explicit config** - `--config <path>`

### Quick Examples

**Add a custom agent** (`~/.config/agent-en-place.yaml`):

```yaml
agents:
  aider:
    packageName: pipx:aider-chat
    command: aider --yes
    configDir: .aider
    envVars:
      - OPENAI_API_KEY
    depends:
      - python
```

### Tool Version Detection

`agent-en-place` automatically detects tool versions from project configuration files:

**`.tool-versions`** (asdf/mise format)

```
node 20.11.0
python 3.12.0
ruby 3.3.0
```

**`mise.toml`** (mise native format)

```toml
[tools]
node = "20.11.0"
python = "3.12.0"
```

When you provide a `mise.toml`, agent-en-place will:
1. Copy your `mise.toml` unchanged into the container
2. Generate a separate `mise.agent.toml` with agent requirements (excluding tools you've already defined)
3. Run both `mise install` (for your tools) and `mise install --env agent` (for agent tools)

This means **your tool versions always take precedence** over agent defaults. If you specify `node = "20.11.0"` in your `mise.toml`, that version will be used instead of the agent's default `latest`.

**Idiomatic version files** are also recognized:

| File               | Language | Example        |
| ------------------ | -------- | -------------- |
| `.nvmrc`           | Node.js  | `20.11.0`      |
| `.node-version`    | Node.js  | `20.11.0`      |
| `.python-version`  | Python   | `3.12.0`       |
| `.ruby-version`    | Ruby     | `3.3.0`        |
| `Gemfile`          | Ruby     | `ruby "3.3.0"` |
| `.go-version`      | Go       | `1.21.0`       |
| `.java-version`    | Java     | `17`           |
| `.sdkmanrc`        | Java     | `java=17.0.2`  |
| `.crystal-version` | Crystal  | `1.10.0`       |
| `.exenv-version`   | Elixir   | `1.15.0`       |
| `.yvmrc`           | Yarn     | `1.22.19`      |
| `.bun-version`     | Bun      | `1.0.0`        |

**Note**: Node.js is automatically included if not specified, as it's required by all supported AI coding tools.

## Supported Providers

Currently supported providers:

### `claude`

- **Package**: `@anthropic-ai/claude-code`
- **Command**: `claude --dangerously-skip-permissions`
- **Requirements**: None
- **Configuration**: Stored in `~/.claude` and `~/.claude.json`
- **Environment**: Uses `ANTHROPIC_API_KEY` if set, or OAuth credentials from config

### `codex`

- **Package**: `@openai/codex`
- **Command**: `codex --dangerously-bypass-approvals-and-sandbox`
- **Requirements**: None
- **Configuration**: Stored in `~/.codex`

### `opencode`

- **Package**: `opencode-ai`
- **Command**: `opencode`
- **Requirements**: None
- **Configuration**: Stored in `~/.config/opencode/` and `~/.local/share/opencode/`

### `copilot`

- **Package**: `@github/copilot`
- **Command**: `copilot --allow-all-tools --allow-all-paths --allow-all-urls`
- **Requirements**: `gh` CLI authenticated with `gh auth login`
- **Configuration**: Stored in `~/.copilot`
- **Environment**: Automatically uses `GH_TOKEN` from `gh` CLI

### `gemini`

- **Package**: `@google/gemini-cli`
- **Command**: `gemini --yolo`
- **Configuration**: Stored in `~/.gemini`


## How It Works

1. **Configuration Detection**: Scans current directory for `.tool-versions`, `mise.toml`, and idiomatic version files
2. **Version Parsing**: Extracts tool names and versions from configuration files
3. **Mise Config Generation**: 
   - If you have a `mise.toml`: copies it unchanged and generates `mise.agent.toml` with only the tools you haven't specified
   - If no `mise.toml`: generates `mise.agent.toml` with all required agent tools
4. **Dockerfile Generation**: Creates a Debian 12-slim based Dockerfile with:
   - mise runtime manager
   - All detected development tools at specified versions
   - Non-root user (UID 1000) for security
5. **Image Building**: Builds Docker image (or reuses cached image if unchanged)
   - Image naming: `mheap/agent-en-place:<tool1>-<version1>-<tool2>-<version2>-...`
6. **Container Execution**: Outputs `docker run` command with:
   - Current directory mounted to `/workdir`
   - Provider config directory mounted (e.g., `~/.copilot`)
   - Appropriate environment variables set
   - `MISE_ENV=agent` to activate the agent environment

## Advanced Usage

### Flags

**`--debug`**

Show Docker build output instead of hiding it. Useful for troubleshooting build failures.

```bash
agent-en-place --debug opencode
```

**`--rebuild`**

Force rebuilding the Docker image even if it already exists. Useful when you want to pull latest tool versions.

```bash
agent-en-place --rebuild copilot
```

**`--dockerfile`**

Print the generated Dockerfile and exit without building. Useful for debugging or customization.

```bash
agent-en-place --dockerfile codex
```

**`--mise-file`**

Print the generated mise configuration files and exit without building. Shows both your `mise.toml` (if present) and the generated `mise.agent.toml`.

```bash
agent-en-place --mise-file claude
```

Example output:
```
# mise.toml (user)
[tools]
node = "20.11.0"

# mise.agent.toml (generated)
[tools]
"npm:@anthropic-ai/claude-code" = "latest"
python = "latest"
```

Note that `node` is not in the generated `mise.agent.toml` because you specified it in your `mise.toml`.

**`--config`**

Use a specific configuration file. See [docs/config.md](docs/config.md) for configuration options.

```bash
agent-en-place --config ./my-config.yaml claude
```

### Combining Flags

```bash
agent-en-place --debug --rebuild opencode
```

### Environment Variable Overrides

You can override tool definitions using environment variables, which is useful for when you want to enforce specific tool versions regardless of project configuration files. I use this when I'm working on something quickly and want to use a cached image.

**`AGENT_EN_PLACE_TOOLS`**

A comma-separated list of `tool@version` pairs. When set, these tools take the highest priority -- overriding versions from `mise.toml`, `.tool-versions`, and idiomatic version files. Tools from config files that are **not** specified in the environment variable are still installed.

```bash
AGENT_EN_PLACE_TOOLS=node@latest,python@3.12 agent-en-place claude
```

npm-style packages (including scoped packages) are supported:

```bash
AGENT_EN_PLACE_TOOLS=node@20,npm:trello-cli@1.5.0 agent-en-place claude
AGENT_EN_PLACE_TOOLS=npm:@my-org/some-package@1.2.3 agent-en-place claude
```

If you omit the `@version`, it defaults to `latest`:

```bash
AGENT_EN_PLACE_TOOLS=node,python agent-en-place claude
```

**`AGENT_EN_PLACE_SPECIFIED_TOOLS_ONLY`**

When set to `1` alongside `AGENT_EN_PLACE_TOOLS`, all file-based tool discovery is skipped. Only the tools listed in `AGENT_EN_PLACE_TOOLS` (plus the agent's own tool) are installed. `.tool-versions`, `mise.toml`, and idiomatic version files are ignored entirely.

```bash
AGENT_EN_PLACE_TOOLS=node@20,python@3.12 \
  AGENT_EN_PLACE_SPECIFIED_TOOLS_ONLY=1 \
  agent-en-place claude
```

This is useful when you want a minimal, deterministic container with only the tools you explicitly specify.

Note: Setting `AGENT_EN_PLACE_SPECIFIED_TOOLS_ONLY=1` without `AGENT_EN_PLACE_TOOLS` has no effect (a warning is printed to stderr).

## License

MIT License
