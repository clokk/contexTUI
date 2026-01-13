# Context Groups Feature

**Supergroup:** Feature
**Status:** Active
**Related:** context-groups-v2.md

## Description

Handles the context groups overlay UI - displaying, navigating, and copying documentation-based context groups. This is the user-facing interaction layer for the context groups system.

## Key Files

- groups.go - Overlay interaction, navigation, and copy logic
- docgroups.go - Parsing, validation, and registry management
- render.go - Groups overlay rendering (renderDocGroupsOverlay, renderAddGroupOverlay)

## Scope

- Groups overlay display and navigation (j/k, scroll)
- Copy group content to clipboard
- Add new context group from markdown files
- Cursor management and scroll visibility

## Out of Scope

- Markdown parsing logic (see docgroups.go)
- Git staleness detection (see docgroups.go)
- Main application state (see model.go)
