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

image_customizations:
  packages:
    - op: <add|remove>
      value: <apt-package>

mise:
  install:
    - <shell-command>
  env:
    <key>: <value>
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

**Note:** If you specify `packages`, it completely replaces the default list. Make sure to include essential packages like `curl`, `ca-certificates`, and `git`. If you only want to add or remove a few packages without replacing the entire list, use `image_customizations` instead.

### `image_customizations`

Allows you to customize the image packages using JSON patch-style operations. Unlike `image.packages` which replaces the entire list, `image_customizations` lets you incrementally add or remove packages from the defaults.

| Field | Type | Description |
|-------|------|-------------|
| `packages` | list | List of customization operations |

Each operation has:

| Field | Type | Description |
|-------|------|-------------|
| `op` | string | Operation type: `add` or `remove` |
| `value` | string | The package name to add or remove |

**Example:**

```yaml
image_customizations:
  packages:
    - op: add
      value: build-essential
    - op: add
      value: vim
    - op: remove
      value: gnupg
```

This would modify the default packages by adding `build-essential` and `vim`, and removing `gnupg`.

**Notes:**
- Customizations are applied after all config files are merged
- Customizations from multiple config files accumulate (XDG config + project config + explicit config)
- If you try to remove a package that doesn't exist, a warning is printed but the build continues
- Operations are applied in order, so you can add and then remove the same package if needed

### `mise`

Configures how mise (the runtime version manager) is installed and its environment variables.

| Field | Type | Description |
|-------|------|-------------|
| `install` | list | Shell commands to install mise (joined with `&&`) |
| `env` | map | Mise environment variables (keys are uppercased and prefixed with `MISE_`) |

**Example:**

```yaml
mise:
  install:
    - curl https://mise.run | sh
    - echo 'eval "$(~/.local/bin/mise activate bash)"' >> ~/.bashrc
  env:
    ruby_compile: false
```

The `env` keys are converted to environment variables by uppercasing and prepending `MISE_`. For example, `ruby_compile: false` becomes `ENV MISE_RUBY_COMPILE="false"` in the Dockerfile. Boolean values are converted to `"true"`/`"false"` strings.

These are set as `ENV` directives in the Dockerfile before `mise install`, so they are available both at build time and runtime. Host `MISE_*` environment variables take precedence over config values for the same key.

**Note:** The install commands are joined with `&&` into a single `RUN` statement in the Dockerfile.

## Merge Behavior

When multiple config files are loaded, they are merged with specific rules:

| Section | Merge Behavior |
|---------|---------------|
| `tools` | Individual tools are added or overridden by name |
| `agents` | Individual agents are added or overridden by name |
| `image.base` | Replaced if specified |
| `image.packages` | Replaced entirely if specified (not merged) |
| `image_customizations` | Accumulated (all customizations are collected and applied in order) |
| `mise.install` | Replaced entirely if specified (not merged) |
| `mise.env` | Individual keys are added or overridden |

This means you can:
- Add a new agent without redefining all existing ones
- Override a single tool's version without affecting others
- Completely replace the package list if needed
- Incrementally add or remove packages using customizations

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

### Adding or Removing Packages Without Replacing the List

Use `image_customizations` to add or remove packages from the defaults without having to specify the entire list:

```yaml
# Add build tools and remove packages you don't need
image_customizations:
  packages:
    - op: add
      value: build-essential
    - op: add
      value: cmake
    - op: remove
      value: apt-transport-https
```

This is especially useful when you want to keep the default packages but need to add a few extras for your project.

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
  env:
    ruby_compile: false
```
