# contexTUI

**Category:** Meta
**Status:** Active

## Description

User-facing documentation for contexTUI - installation, usage, and key commands.

## Key Files

- main.go - Application entry point
- .context-docs.md - Context docs configuration

---

A context-aware terminal file browser for AI-assisted development.

> See [VISION.md](VISION.md) for the philosophy behind this tool.

## Prerequisites

- [Go](https://go.dev/dl/) 1.21 or later

## Supported Platforms

| Platform | Clipboard Support |
|----------|-------------------|
| macOS | Native (pbcopy) |
| Linux X11 | Requires `xclip` or `xsel` |
| Linux Wayland | Requires `wl-clipboard` |
| Windows | Native |
| WSL | Native (via clip.exe) |

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
- **File management** - Create, rename, and delete files and folders
- **Context docs** - Documentation-first context system
- **Git integration** - Status badges, diff preview, branch display
- **Copy as context** - Copy files as `@filepath` references for AI tools

## Key Commands

| Key | Action |
|-----|--------|
| `j/k` | Move up/down |
| `h/l` | Collapse/expand or switch panes |
| `enter` | Open directory or select file |
| `n` | Create new file |
| `N` | Create new folder |
| `r` | Rename file or folder |
| `d` | Delete file or folder |
| `c` | Copy file path(s) |
| `g` | Open context docs |
| `s` | Toggle git status view |
| `.` | Toggle dotfiles visibility |
| `/` | Search files |
| `?` | Show help |
| `q` | Quit |

## Context Docs

Context docs are the core feature of contexTUI. Instead of copying raw code files into AI prompts (which blows out context windows), you curate documentation that explains your system. The AI can then find specific code when needed.

### Why Documentation Over Code?

Structured markdown has a much higher signal-to-token ratio than raw code:
- 200 lines of documentation can convey what 2000+ lines of code only implies
- Documentation captures intent, architecture, and design decisions
- Code shows *what*, but documentation explains *why*

Think of it like onboarding a new team member. You wouldn't hand them a codebase and say "figure it out." You'd give them documentation, architecture diagrams, and explain the key concepts. Your AI collaborator deserves the same.

### Opening Context Docs

Press `g` to open the context docs overlay. You'll see:
- **Categories** at the top (navigate with `h`/`l` or click)
- **Docs** listed below (navigate with `j`/`k`)
- **Status indicators** showing doc health

### Adding Context Docs

1. Press `a` to open the file picker
2. All markdown files in your project appear (excluding those already registered)
3. Use `j`/`k` to navigate, `space` to multi-select, `enter` to confirm
4. Selected files are added to the registry

**What happens when you add a doc:**
- The file is parsed for required metadata (Category, Status, Description, Key Files)
- If metadata is missing, a `<!-- contexTUI: structure-needed -->` tag is inserted at the top
- The doc appears in the overlay, possibly with an `incomplete` indicator

### Structuring Incomplete Docs

If you add a markdown file that lacks the required structure, use the structuring prompt:

1. Press `p` to copy the structuring prompt to your clipboard
2. Paste it into Claude (or your AI collaborator)
3. The AI will find all files with the `<!-- contexTUI: structure-needed -->` tag and add the required metadata

The structuring prompt asks the AI to add:
- `**Category:**` - Meta, Feature, or a custom category
- `**Status:**` - Active, Deprecated, Experimental, or Planned
- `## Description` section
- `## Key Files` section with entry points

### Required Document Structure

Each context doc should have this structure:

```markdown
# Document Title

**Category:** Feature
**Status:** Active
**Related:** other-doc.md, related-doc.md  (optional)

## Description

High-level explanation of what this covers and why it exists.

## Key Files

- src/feature/main.ts - Primary entry point
- src/feature/utils.ts - Helper functions
- tests/feature.test.ts - Test suite

## Out of Scope (optional)

What this documentation doesn't cover - helps AI understand boundaries.

---

[Rest of your documentation content...]
```

**Important:** Key Files must use list format (starting with `- `), not tables.

### Navigating Context Docs

| Key | Action |
|-----|--------|
| `h`/`l` | Switch between categories |
| `j`/`k` | Move up/down within a category |
| `J`/`K` | Reorder docs within category |
| `space` | Multi-select docs |
| `c` or `enter` | Copy selected doc(s) as `@filepath` reference |
| `a` | Add new context doc |
| `d` or `x` | Remove doc from registry |
| `p` | Copy structuring prompt |
| `esc` | Close overlay |

### Copying Context

Press `c` (or `enter`) to copy the selected doc as an `@filepath` reference:
- Single doc: copies `@path/to/doc.md`
- Multi-select (using `space`): copies all selected paths, one per line

Paste this into your AI prompt to reference the documentation.

### Removing Context Docs

Press `d` or `x` to remove a doc from the registry:
- The file itself is **not deleted**
- Metadata (`**Category:**`, `**Status:**`, etc.) is stripped from the file
- The `<!-- contexTUI: structure-needed -->` tag is removed if present

### Visual Indicators

Docs may show status indicators:
- `incomplete` - Missing required metadata (Category, Status, Description, or Key Files)
- `broken refs` - Key Files reference paths that don't exist
- `stale` - Referenced Key Files have been modified more recently than the doc

### Categories

**Default categories:**
- **Meta** - Project-level docs (architecture, standards, vision)
- **Feature** - Feature-specific documentation

**Custom categories:**
Set `**Category:** YourCategory` in any markdown file. Custom categories are auto-discovered and appear in the overlay.

**Uncategorized:**
If a doc has no category or an unrecognized one, it appears in an auto-generated "Uncategorized" section.

### The Registry File

The `.context-docs.md` file in your project root is the registry:
- Lists all registered context docs
- Groups them by category
- Auto-generated by contexTUI (don't edit manually)
- Commit to git to share with your team

## Environment

```bash
NO_COLOR=1 contexTUI  # Disable colors
```

Respects the [NO_COLOR](https://no-color.org/) standard.

## Configuration

contexTUI stores user preferences in `.contexTUI.json`:
- `splitRatio` - Width ratio between tree and preview panes
- `showDotfiles` - Whether dotfiles are visible in the tree (toggle with `.`)

This file is user-specific and should be added to your project's `.gitignore`:

```bash
echo ".contexTUI.json" >> .gitignore
```

## License

MIT
