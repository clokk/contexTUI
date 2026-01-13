# Context Groups v2: Documentation-First Context System

**Supergroup:** Meta
**Status:** Active

## Description

This document describes the vision and implementation plan for Context Groups v2—a fundamental shift from "context groups as curated code file lists" to "context groups ARE documentation files with embedded metadata."

## Key Files

- types.go - ContextGroup struct and model definition
- groups.go - Parsing logic and UI interaction
- render.go - Groups overlay rendering
- clipboard.go - Copy functionality

---

## The Problem

Context groups with many code files are counterproductive. Copying 15+ code files blows out the AI context window before the conversation even starts. The original model optimizes for exhaustiveness—"give Claude everything it might need"—but that's backwards:

- More files ≠ better context
- Code shows *what*, not *why*
- Human curation time scales poorly

## The Insight

Markdown documentation has a much higher signal-to-token ratio than raw code. A well-written doc can convey the intent and architecture of a system in 200 lines that would take 2000+ lines of code to infer.

**The shift**: Instead of "here are the 15 files you need to understand feature X", the approach becomes "here's the documentation that explains feature X, and I (Claude) will find the specific code when needed."

## The New Model

### Documentation IS Context

Documentation files aren't pointers to context—they ARE the context. Each documentation markdown file becomes a context group with embedded metadata that helps both humans and AI understand:

1. What this covers (Description)
2. Where to find the code (Key Files)
3. How it relates to other parts (Related, Supergroup)
4. Whether it's current (Status, staleness detection)
5. What it doesn't cover (Out of Scope)

### Architecture

```
.context-groups.md              ← Index/registry + agent primer
    ↓ references
docs/authentication.md          ← Context group (with inline metadata)
docs/api-layer.md              ← Context group
ARCHITECTURE.md                ← Context group
```

## Documentation Structure

Each context group is a markdown file with this structure:

```markdown
# Feature Name

**Supergroup:** Feature
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
| **Supergroup** | Yes | Categorization (Meta, Feature, or custom) |
| **Status** | Yes | Active, Deprecated, Experimental, Planned |
| **Related** | No | Links to other docs for context chaining |
| **Description** | Yes | High-level purpose section |
| **Key Files** | Yes | Code entry points (not exhaustive) |
| **Out of Scope** | No | Boundaries to prevent AI assumptions |

### Default Supergroups

- **Meta** - Project-level docs (vision, standards, architecture, agent instructions)
- **Feature** - Feature-specific documentation
- **Miscellaneous** - Catch-all

Custom supergroups are auto-discovered from markdown files.

## .context-groups.md as Registry

The `.context-groups.md` file becomes an index/registry and agent primer:

```markdown
# Context Groups

This project uses structured documentation as context groups.
Each group is a markdown file with metadata: Supergroup, Status,
Description, Key Files, and optionally Related and Out of Scope.

Supergroups are auto-discovered from markdown files. To create a custom
supergroup, just set `**Supergroup:** YourCategory` in any markdown file.

## Supergroups (auto-discovered)

- Meta
- Feature
- Miscellaneous

## Active Groups

- docs/authentication.md (Feature, Active)
- docs/api-layer.md (Feature, Active)
- ARCHITECTURE.md (Meta, Active)
```

## User Flow in contexTUI

1. Press `g` → Groups view
2. See groups organized by supergroup (gallery navigation at top)
3. Use `h`/`l` or click to switch between supergroup categories
4. Visual indicators:
   - incomplete - Missing required structure
   - broken refs - File references that don't exist
   - stale - Referenced files changed since doc last modified
5. `a` → Add new group (search all .md files in project)
6. Select file → Added to registry, analyzed for structure
7. `c` or click → Copy markdown content for prompting

## Managing Supergroups

Supergroups are managed automatically. There are three defaults (Meta, Feature, Miscellaneous) and custom supergroups are auto-discovered from markdown files.

### For Users and AI Agents

**To create a custom supergroup:**
Simply set `**Supergroup:** YourCategory` in any markdown file. The supergroup will be created automatically when the file is added to the registry.

```markdown
# My Feature Documentation

**Supergroup:** My Custom Category
**Status:** Active
```

**To remove a custom supergroup:**
Change or remove the `**Supergroup:**` line in the markdown files that use it. When no files reference a custom supergroup, it is automatically removed.

**Default supergroups:**
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
- AI agents can read `.context-groups.md` to understand project structure

## Implementation Phases

### Phase 1: Core Implementation

- [ ] New context group discovery (search .md files)
- [ ] Parse markdown for required metadata sections
- [ ] Registry management in .context-groups.md
- [ ] Display groups organized by supergroup
- [ ] Copy markdown content
- [ ] Flag missing structure with visual indicators

### Phase 2: Validation & Assistance

- [ ] Validate key file paths exist
- [ ] Git-based staleness detection
- [ ] Template insertion for missing sections
- [ ] Generate Claude prompt for doc structuring assistance

### Phase 3: Future Vision

- Git heatmap analysis to identify undocumented features
- Proactive suggestions: "These files are hot but have no associated documentation"
- Guide users toward documentation coverage
