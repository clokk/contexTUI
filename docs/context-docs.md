# Context Docs: Documentation-First Context System

**Category:** Meta
**Status:** Active

## Description

This document describes the vision and implementation plan for Context Docs—a fundamental shift from "context docs as curated code file lists" to "context docs ARE documentation files with embedded metadata."

## Key Files

- internal/app/types.go - ContextDoc struct and model definition
- internal/groups/groups.go - Parsing logic and registry management
- internal/app/view.go - Docs overlay rendering
- internal/clipboard/clipboard.go - Copy functionality

---

## The Problem

Context docs with many code files are counterproductive. Copying 15+ code files blows out the AI context window before the conversation even starts. The original model optimizes for exhaustiveness—"give Claude everything it might need"—but that's backwards:

- More files ≠ better context
- Code shows *what*, not *why*
- Human curation time scales poorly

## The Insight

Markdown documentation has a much higher signal-to-token ratio than raw code. A well-written doc can convey the intent and architecture of a system in 200 lines that would take 2000+ lines of code to infer.

**The shift**: Instead of "here are the 15 files you need to understand feature X", the approach becomes "here's the documentation that explains feature X, and I (Claude) will find the specific code when needed."

## The New Model

### Documentation IS Context

Documentation files aren't pointers to context—they ARE the context. Each documentation markdown file becomes a context doc with embedded metadata that helps both humans and AI understand:

1. What this covers (Description)
2. Where to find the code (Key Files)
3. How it relates to other parts (Related, Category)
4. Whether it's current (Status, staleness detection)
5. What it doesn't cover (Out of Scope)

### Architecture

```
.context-docs.md                ← Index/registry + agent primer
    ↓ references
docs/authentication.md          ← Context doc (with inline metadata)
docs/api-layer.md              ← Context doc
ARCHITECTURE.md                ← Context doc
```

## Documentation Structure

Each context doc is a markdown file with this structure:

```markdown
# Feature Name

**Category:** Feature
**Status:** Active
**Related:** oauth.md, sessions.md

## Description

High-level purpose and architecture explanation.

## Key Files

- src/auth/provider.ts - OAuth provider abstraction
- src/middleware/auth.ts - Request authentication

## Scope

What this feature covers.

## Out of Scope

What this doesn't cover—directs AI elsewhere.

## [Actual documentation content...]
```

### Metadata Fields

| Field | Required | Purpose |
|-------|----------|---------|
| **Category** | Yes | Categorization (Meta, Feature, or custom) |
| **Status** | Yes | Active, Deprecated, Experimental, Planned |
| **Related** | No | Links to other docs for context chaining |
| **Description** | Yes | High-level purpose section |
| **Key Files** | Yes | Code entry points (not exhaustive) |
| **Out of Scope** | No | Boundaries to prevent AI assumptions |

### Default Categories

- **Meta** - Project-level docs (vision, standards, architecture, agent instructions)
- **Feature** - Feature-specific documentation
- **Miscellaneous** - Catch-all

Custom categories are auto-discovered from markdown files.

## .context-docs.md as Registry

The `.context-docs.md` file becomes an index/registry and agent primer:

```markdown
# Context Docs

This project uses structured documentation as context docs.
Each doc is a markdown file with metadata: Category, Status,
Description, Key Files, and optionally Related and Out of Scope.

Categories are auto-discovered from markdown files. To create a custom
category, just set `**Category:** YourCategory` in any markdown file.

## Categories (auto-discovered)

- Meta
- Feature
- Miscellaneous

## Active Docs

- docs/authentication.md (Feature, Active)
- docs/api-layer.md (Feature, Active)
- ARCHITECTURE.md (Meta, Active)
```

## User Flow in contexTUI

1. Press `g` → Docs view
2. See docs organized by category (gallery navigation at top)
3. Use `h`/`l` or click to switch between categories
4. Visual indicators:
   - incomplete - Missing required structure
   - broken refs - File references that don't exist
   - stale - Referenced files changed since doc last modified
5. `a` → Add new doc (search all .md files in project)
6. Select file → Added to registry, analyzed for structure
7. `c` or click → Copy markdown content for prompting

## Managing Categories

Categories are managed automatically. There are three defaults (Meta, Feature, Miscellaneous) and custom categories are auto-discovered from markdown files.

### For Users and AI Agents

**To create a custom category:**
Simply set `**Category:** YourCategory` in any markdown file. The category will be created automatically when the file is added to the registry.

```markdown
# My Feature Documentation

**Category:** My Custom Category
**Status:** Active
```

**To remove a custom category:**
Change or remove the `**Category:**` line in the markdown files that use it. When no files reference a custom category, it is automatically removed.

**Default categories:**
The three defaults (Meta, Feature, Miscellaneous) are always available and cannot be removed.

The registry auto-syncs when files are modified (file watcher) or when the app loads.

## Staleness Detection

Instead of a manual "Last Updated" field, staleness is detected automatically:

1. Get git history of referenced Key Files
2. Get git history of the documentation file
3. If Key Files have significant changes since doc was last modified → flag as stale

This is meaningful because it indicates the code has evolved but the documentation hasn't caught up.

## Value Proposition

**Human curates documentation once → Claude executes better every time.**

- Documentation is the context—no duplication
- Token-efficient (markdown vs code)
- Encourages good documentation practices
- Staleness tracking built-in
- AI agents can read `.context-docs.md` to understand project structure

## Implementation Phases

### Phase 1: Core Implementation

- [x] New context doc discovery (search .md files)
- [x] Parse markdown for required metadata sections
- [x] Registry management in .context-docs.md
- [x] Display docs organized by category
- [x] Copy markdown content
- [x] Flag missing structure with visual indicators

### Phase 2: Validation & Assistance

- [x] Validate key file paths exist
- [x] Git-based staleness detection
- [x] Template insertion for missing sections
- [x] Generate Claude prompt for doc structuring assistance

### Phase 3: Future Vision

- Git heatmap analysis to identify undocumented features
- Proactive suggestions: "These files are hot but have no associated documentation"
- Guide users toward documentation coverage
