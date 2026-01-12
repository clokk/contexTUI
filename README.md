# contexTUI

A context-aware terminal file browser for AI-assisted development.

> See [VISION.md](VISION.md) for the philosophy behind this tool.

## Installation

```bash
# Build from source
go build -o contexTUI

# Or install directly
go install github.com/yourusername/contexTUI@latest
```

## Quick Start

```bash
# Run in any project directory
contexTUI

# Or specify a path
contexTUI ~/projects/myapp
```

Press `?` for help at any time.

## Features

- **File tree + preview** - Navigate and preview files in a split pane
- **Context groups** - Pre-defined file sets for quick context loading
- **Git integration** - Status badges, diff preview, branch display
- **Copy as context** - Copy files as `@filepath` references for AI tools

## Key Commands

| Key | Action |
|-----|--------|
| `j/k` | Move up/down |
| `h/l` | Collapse/expand or switch panes |
| `enter` | Open directory or select file |
| `c` | Copy file path(s) |
| `g` | Open context groups |
| `s` | Toggle git status view |
| `/` | Search files |
| `?` | Show help |
| `q` | Quit |

## Context Groups

Define reusable file groups in `.context-groups.md`:

```markdown
## my-feature
layer: feature

Files for the my-feature system.

- src/feature.ts
- src/feature.test.ts
- docs/feature.md
```

Press `g` to open groups, navigate with `h/l/j/k`, press `c` to copy all files.

## Environment

```bash
NO_COLOR=1 contexTUI  # Disable colors
```

Respects the [NO_COLOR](https://no-color.org/) standard.

## License

MIT
