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

`agent-en-place` automatically detects tool versions from multiple configuration file formats:

### mise/asdf Configuration

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

### Idiomatic Version Files

The tool also recognizes language-specific version files:

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
3. **Dockerfile Generation**: Creates a Debian 12-slim based Dockerfile with:
   - mise runtime manager
   - All detected development tools at specified versions
   - Non-root user (UID 1000) for security
4. **Image Building**: Builds Docker image (or reuses cached image if unchanged)
   - Image naming: `mheap/agent-en-place:<tool1>-<version1>-<tool2>-<version2>-...`
5. **Container Execution**: Outputs `docker run` command with:
   - Current directory mounted to `/workdir`
   - Provider config directory mounted (e.g., `~/.copilot`)
   - Appropriate environment variables set

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

### Combining Flags

```bash
agent-en-place --debug --rebuild opencode
```

## License

MIT License
