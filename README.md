<div align="center">

# SprintMate

**Connect Jira to your AI coding agents — straight from the terminal.**

[![CI](https://github.com/GustavoMinelli/sprintmate/actions/workflows/ci.yml/badge.svg)](https://github.com/GustavoMinelli/sprintmate/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](go.mod)
[![Platforms](https://img.shields.io/badge/platforms-macOS%20%C2%B7%20Linux%20%C2%B7%20Windows-lightgrey.svg)](#install)

</div>

---

SprintMate is a fast, keyboard-driven TUI that bridges your Jira board and the
AI development agents you already use (Claude Code, Codex, and more). It doesn't
replace Jira or your agent — it removes the friction between them:

```
Jira  →  SprintMate  →  pick an issue  →  pick an agent  →  prepared context  →  agent session  →  code
```

Pick an issue from your sprint, hit Enter, and SprintMate opens your project,
creates (or reuses) a feature branch, generates a rich `.issue-context.md`, and
launches your agent — already loaded with the task context.

## Contents

- [Features](#features)
- [Install](#install)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [Keybindings](#keybindings-dashboard)
- [How launching works](#how-launching-works)
- [Adding a new agent](#adding-a-new-agent)
- [Architecture](#architecture)
- [Development](#development)
- [Roadmap](#roadmap)
- [License](#license)

## Features

- **Modern TUI** built on the Charm stack (Bubble Tea / Bubbles / Lip Gloss v2).
- **Guided setup wizard** — configure everything from a screen, no YAML editing.
  Connect to Jira and pick your **board, columns and sprint from live lists**.
- **Configurable issue source** — board, columns/statuses, sprint, assignee, or a
  raw JQL override.
- **Pluggable agents** — Claude Code and Codex out of the box; add new ones
  without touching the core.
- **Automatic context** — generates `.issue-context.md` from the issue, the
  project (README, `docs/`, layout) and git (branch, recent commits).
- **Git automation** — creates `{issue-key}-{slug}` branches (and reuses them).
- **Smart launch** — opens the agent in a tmux window, a new terminal window, or
  in-place, auto-detected and configurable. Windowed/tmux launches **keep the
  dashboard open** so you can fire off several issues back to back.
- **Single workspace** — one working directory where every issue's agent launches.
- No database, no daemon. A single static binary. macOS, Linux and Windows.

## Install

### Homebrew

```sh
brew install GustavoMinelli/tap/sprintmate
```

### Go install

```sh
go install github.com/GustavoMinelli/sprintmate/cmd/sprintmate@latest
```

### From source

```sh
git clone https://github.com/GustavoMinelli/sprintmate
cd sprintmate
make build      # produces ./sprintmate
```

## Quick start

```sh
sprintmate
```

On first run (no config yet) SprintMate opens the **setup wizard**:

1. **Jira API** — enter your host, email and [API token](https://id.atlassian.com/manage-profile/security/api-tokens), then **Test connection**.
2. **Board** — pick from your boards (fetched live).
3. **Columns** — multi-select the columns/statuses to pull.
4. **Sprint** — active, future, all, or a specific one.
5. **Agent** — choose your default agent (installed ones are highlighted).
6. **Working directory** — pick the folder where agents run. Every issue launches here.

Re-open it anytime with `sprintmate config`, or press <kbd>s</kbd> in the dashboard.

## Configuration

The config lives in your user config dir and is normally written by the wizard,
but it's plain YAML and safe to edit:

| OS      | Path                                                  |
|---------|-------------------------------------------------------|
| Linux   | `~/.config/sprintmate/config.yaml`                    |
| macOS   | `~/Library/Application Support/sprintmate/config.yaml`|
| Windows | `%AppData%\sprintmate\config.yaml`                    |

> **Tip:** set `SPRINTMATE_JIRA_TOKEN` in your environment to keep the API token
> out of the file. The env var always wins.

See [`configs/config.example.yaml`](configs/config.example.yaml) for a fully
documented example, and [`docs/configuration.md`](docs/configuration.md) for the
reference.

## Keybindings (dashboard)

| Key            | Action                       |
|----------------|------------------------------|
| <kbd>↑</kbd>/<kbd>k</kbd>, <kbd>↓</kbd>/<kbd>j</kbd> | Navigate issues |
| <kbd>Enter</kbd> | Launch agent for the selected issue |
| <kbd>Tab</kbd>   | Switch agent                 |
| <kbd>r</kbd>     | Refresh from Jira            |
| <kbd>o</kbd>     | Open the issue in the browser|
| <kbd>/</kbd>     | Search / filter              |
| <kbd>s</kbd>     | Open settings                |
| <kbd>q</kbd>     | Quit                         |

All bindings are configurable under `keys:` in the config.

## How launching works

When you press Enter on an issue, SprintMate:

1. Uses the configured `workdir` as the working directory.
2. Creates or reuses the `{key}-{slug}` git branch.
3. Generates `.issue-context.md` in that directory.
4. Launches the configured agent using the chosen **launch strategy**:
   - `auto` (default): a **tmux** window if you're in tmux → a **new terminal
     window** (osascript / Windows Terminal / gnome-terminal·konsole·…) →
     **in-place** handoff as a universal fallback.
   - or force `tmux` / `window` / `inplace`.

With the `tmux` and `window` strategies the agent opens in its **own pane or
window** and the **dashboard stays open** — you'll see a "launched" confirmation
and can immediately pick another issue to launch alongside it. Only `inplace`
(which replaces the current terminal with the agent) closes the dashboard, since
the two can't share one terminal.

> Add `.issue-context.md` to your project's `.gitignore`.

## Adding a new agent

Agents are plugins. Create a small package that registers itself:

```go
package gemini

import "github.com/GustavoMinelli/sprintmate/internal/agents"

func init() {
    agents.Register("gemini", func() agents.Agent {
        return agents.NewBuiltin("gemini", "gemini", []string{"{context_file}"})
    })
}
```

Then blank-import it in `cmd/sprintmate`. The core never changes. Args support
the `{context_file}`, `{key}`, `{branch}` and `{dir}` placeholders, and any agent
can be fully customized in config under `agents:`.

## Architecture

```
cmd/sprintmate/        # entrypoint + launch flow
internal/
  config/              # YAML load/save/validate, env override, keymap
  jira/                # REST + Agile client, board/columns/sprint, ADF→md
  agents/              # Agent interface + registry (claude/, codex/)
  git/                 # branch create/reuse, slug, recent commits
  context/             # .issue-context.md builder
  terminal/            # launch strategies (tmux/window/inplace)
  tui/                 # Bubble Tea v2: wizard + dashboard
```

## Development

```sh
make test     # run tests
make vet      # go vet
make lint     # golangci-lint
make build    # build ./sprintmate
make run      # run from source
```

## Roadmap

GitHub/Linear/Azure/GitLab issue sources · concurrent agents · MCP integration ·
AI sprint summaries · time-per-issue & productivity dashboard · session history ·
prompt templates · per-issue worktrees · automatic PR creation.

## License

[MIT](LICENSE) © Gustavo Dias Minelli
