# Configuration Reference

`agent-en-place` uses a YAML configuration file to define agents, tools, and Docker image settings. This allows you to customize behavior, add new agents, or override defaults.

## Configuration File Locations

Configuration files are loaded and merged in the following order (later files override earlier ones):

1. **Embedded defaults** - Built into the binary
2. **User config** - `~/.config/agent-en-place.yaml` (or `$XDG_CONFIG_HOME/agent-en-place.yaml`)
3. **Project config** - `./.agent-en-place.yaml` in the current directory
4. **Explicit config** - Path specified via `--config` flag

This layered approach allows you to:
- Set personal defaults in your user config
- Override settings per-project with a local config
- Use a specific config file for one-off runs

## Configuration Structure

```yaml
tools:
  <tool-name>:
    version: <version>
    depends: <dependency-tool>
    additionalPackages:
      - <apt-package>

agents:
  <agent-name>:
    packageName: <mise-package-name>
    command: <command-to-run>
    configDir: <config-directory>
    additionalMounts:
      - <path>
    envVars:
      - <ENV_VAR>
    depends:
      - <tool-name>

image:
  base: <docker-base-image>
  packages:
    - <apt-package>

mise:
  install:
    - <shell-command>
```

## Section Reference

### `tools`

Defines runtime tools that can be installed via mise. Tools can have dependencies on other tools and specify additional apt packages they require.

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Version to install (default: `latest`) |
| `depends` | string | Name of another tool this depends on |
| `additionalPackages` | list | Apt packages required by this tool |

**Example:**

```yaml
tools:
  node:
    version: "20"
    depends: python  # node-gyp needs python for native modules
    additionalPackages:
      - libatomic1   # Required by Node.js on some architectures
  python:
    version: "3.12"
  ruby:
    version: "3.3"
    additionalPackages:
      - build-essential
      - libssl-dev
```

### `agents`

Defines AI coding agents that can be launched with `agent-en-place <agent-name>`.

| Field | Type | Description |
|-------|------|-------------|
| `packageName` | string | Mise package name (e.g., `npm:@openai/codex`) |
| `command` | string | Command to run inside the container |
| `configDir` | string | Directory under `$HOME` to mount for agent config |
| `additionalMounts` | list | Additional paths under `$HOME` to mount |
| `envVars` | list | Environment variables to pass to the container |
| `depends` | list | Tools this agent depends on |

**Example:**

```yaml
agents:
  claude:
    packageName: npm:@anthropic-ai/claude-code
    command: claude --dangerously-skip-permissions
    configDir: .claude
    additionalMounts:
      - .claude.json
    envVars:
      - ANTHROPIC_API_KEY
    depends:
      - node

  my-custom-agent:
    packageName: npm:my-custom-package
    command: my-agent --auto-approve
    configDir: .my-agent
    envVars:
      - MY_API_KEY
      - MY_SECRET
    depends:
      - node
      - python
```

### `image`

Configures the Docker base image and system packages.

| Field | Type | Description |
|-------|------|-------------|
| `base` | string | Docker base image (default: `debian:12-slim`) |
| `packages` | list | Apt packages to install in the image |

**Example:**

```yaml
image:
  base: ubuntu:22.04
  packages:
    - curl
    - ca-certificates
    - git
    - gnupg
    - build-essential
```

**Note:** If you specify `packages`, it completely replaces the default list. Make sure to include essential packages like `curl`, `ca-certificates`, and `git`.

### `mise`

Configures how mise (the runtime version manager) is installed.

| Field | Type | Description |
|-------|------|-------------|
| `install` | list | Shell commands to install mise (joined with `&&`) |

**Example:**

```yaml
mise:
  install:
    - curl https://mise.run | sh
    - echo 'eval "$(~/.local/bin/mise activate bash)"' >> ~/.bashrc
```

**Note:** The install commands are joined with `&&` into a single `RUN` statement in the Dockerfile.

## Merge Behavior

When multiple config files are loaded, they are merged with specific rules:

| Section | Merge Behavior |
|---------|---------------|
| `tools` | Individual tools are added or overridden by name |
| `agents` | Individual agents are added or overridden by name |
| `image.base` | Replaced if specified |
| `image.packages` | Replaced entirely if specified (not merged) |
| `mise.install` | Replaced entirely if specified (not merged) |

This means you can:
- Add a new agent without redefining all existing ones
- Override a single tool's version without affecting others
- Completely replace the package list if needed

## Examples

### Adding a Custom Agent

Create `~/.config/agent-en-place.yaml`:

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

Now you can run: `vibe aider`

### Overriding Node Version

> You should use `mise.toml` or `.nvmrc` if your project requires node. This value should only be used if an agent requires a specific version of node, and you do not use node in your project.

Create `.agent-en-place.yaml` in your project:

```yaml
tools:
  node:
    version: "18"
```

This overrides the default Node.js version for this project only.

### Using a Different Base Image

```yaml
image:
  base: ubuntu:24.04
  packages:
    - curl
    - ca-certificates
    - git
    - gnupg
    - apt-transport-https
```

### Adding System Dependencies for a Tool

```yaml
tools:
  puppeteer:
    version: latest
    additionalPackages:
      - chromium
      - libx11-xcb1
      - libxcomposite1
      - libxdamage1
      - libxi6
      - libxtst6
      - libnss3
      - libcups2
      - libxss1
      - libxrandr2
      - libasound2
      - libpangocairo-1.0-0
      - libatk1.0-0
      - libatk-bridge2.0-0
      - libgtk-3-0
```

## Default Configuration

The embedded default configuration is:

```yaml
tools:
  node:
    version: latest
    depends: python
    additionalPackages:
      - libatomic1
  python:
    version: latest

agents:
  codex:
    packageName: npm:@openai/codex
    command: codex --dangerously-bypass-approvals-and-sandbox
    configDir: .codex
    depends:
      - node
  opencode:
    packageName: npm:opencode-ai
    command: opencode
    configDir: .config/opencode/
    additionalMounts:
      - .local/share/opencode
    depends:
      - node
  copilot:
    packageName: npm:@github/copilot
    command: copilot --allow-all-tools --allow-all-paths --allow-all-urls
    configDir: .copilot
    envVars:
      - GH_TOKEN="$(gh auth token -h github.com)"
    depends:
      - node
  claude:
    packageName: npm:@anthropic-ai/claude-code
    command: claude --dangerously-skip-permissions
    configDir: .claude
    additionalMounts:
      - .claude.json
    envVars:
      - ANTHROPIC_API_KEY
    depends:
      - node
  gemini:
    packageName: npm:@google/gemini-cli
    command: gemini --yolo
    configDir: .gemini
    depends:
      - node

image:
  base: debian:12-slim
  packages:
    - curl
    - ca-certificates
    - git
    - gnupg
    - apt-transport-https

mise:
  install:
    - install -dm 755 /etc/apt/keyrings
    - curl -fSs https://mise.jdx.dev/gpg-key.pub | tee /etc/apt/keyrings/mise-archive-keyring.pub >/dev/null
    - arch=$(dpkg --print-architecture)
    - echo "deb [signed-by=/etc/apt/keyrings/mise-archive-keyring.pub arch=$arch] https://mise.jdx.dev/deb stable main" | tee /etc/apt/sources.list.d/mise.list
    - apt-get update
    - apt-get install -y mise
```
