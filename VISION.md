# contexTUI Vision

**Supergroup:** Meta
**Status:** Active

## Description

The product vision and philosophy behind contexTUI - a context-aware terminal file browser for AI-assisted development.

## Key Files

- main.go - Application entry point
- model.go - Core application state and logic
- types.go - Data structures

## The Shift

The job is changing. Features that took a week can now be done in minutes with proper AI collaboration. But this velocity shift creates a new bottleneck: human judgment, not execution.

Current IDEs assume you're the typist. This tool assumes you're the director.

You're not typing less - you're thinking more. And thinking effectively requires seeing and understanding your codebase quickly.

## What It Is

A context-aware terminal file browser designed for AI-assisted development workflows.

## Core Philosophy

### Context is King

The primary purpose is to help developers quickly gather and share file context with AI assistants. Every feature should serve this goal.

### Less is More

- Minimal footprint, maximum utility
- Features earn their place by frequent use
- When in doubt, leave it out
- Help overlay (`?`) instead of cluttered footer

### Keyboard-First

- Vim-style navigation (j/k/h/l)
- Single-key actions where possible
- Mouse supported but not required

### Tracking Over Commands

For integrations like git:
- Show status, don't manage it
- Inform decisions, don't make them
- Read-only by default (fetch is an exception - it's safe)

### Composability

- Context groups are file references, not containers
- Text lives in files, not inline
- Groups can share files across purposes

## What It's Not

- Not an IDE or code editor
- Not a full git client
- Not a replacement for terminal commands
- Not trying to do everything

## Target Users

Developers in AI-augmented workflows who are becoming:
- **Architect** - Making structure decisions
- **Reviewer** - Understanding changes
- **Context Provider** - Feeding relevant info to AI
- **Quality Judge** - Deciding what's good

And who:
- Work in terminal environments
- Value keyboard efficiency
- Need to frequently share file context

## The Value Proposition

**Human curates once → Claude executes better every time.**

Humans are slow at finding and curating context. AI is slow at rediscovering context every session. This tool makes context curation fast so AI can execute better.

Front-load context so Claude can execute, rather than making Claude search repeatedly.

## Design Decisions

### Why Go + Bubbletea?

- Fast startup, small binary
- Cross-platform
- Excellent TUI library ecosystem

### Why .context-groups.md?

- Human-readable, git-trackable
- Shareable with team
- No database or config sync needed

### Why Minimal Git Integration?

- **Status badges**: Instant visual feedback on file changes
- **Diff preview**: See what changed without switching tools
- **Branch display**: Know where you are
- **Manual fetch**: User controls network access
- **No commit/push/pull**: Use your terminal for that
