# Context Docs Feature

**Category:** Feature
**Status:** Active
**Related:** context-docs.md

## Description

Handles the context docs overlay UI - displaying, navigating, and copying documentation-based context docs. This is the user-facing interaction layer for the context docs system.

## Key Files

- internal/app/update_groups.go - Overlay interaction, navigation, and copy logic
- internal/groups/groups.go - Parsing, validation, and registry management
- internal/app/view.go - Docs overlay rendering (renderContextDocsOverlay, renderAddDocOverlay)

## Scope

- Docs overlay display and navigation (j/k, scroll)
- Copy doc content to clipboard
- Add new context doc from markdown files
- Cursor management and scroll visibility

## Out of Scope

- Markdown parsing logic (see internal/groups/groups.go)
- Git staleness detection (see internal/groups/groups.go)
- Main application state (see internal/app/model.go)
