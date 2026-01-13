# Context Groups Feature

**Supergroup:** Feature
**Status:** Active
**Related:** context-groups-v2.md

## Description

Handles the context groups overlay UI - displaying, navigating, and copying documentation-based context groups. This is the user-facing interaction layer for the context groups system.

## Key Files

- internal/app/update_groups.go - Overlay interaction, navigation, and copy logic
- internal/groups/groups.go - Parsing, validation, and registry management
- internal/app/view.go - Groups overlay rendering (renderDocGroupsOverlay, renderAddGroupOverlay)

## Scope

- Groups overlay display and navigation (j/k, scroll)
- Copy group content to clipboard
- Add new context group from markdown files
- Cursor management and scroll visibility

## Out of Scope

- Markdown parsing logic (see internal/groups/groups.go)
- Git staleness detection (see internal/groups/groups.go)
- Main application state (see internal/app/model.go)
