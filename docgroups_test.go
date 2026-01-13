package main

import (
	"testing"
)

func TestLoadDocGroupRegistry(t *testing.T) {
	registry, err := LoadDocGroupRegistry(".")
	if err != nil {
		t.Fatalf("Failed to load registry: %v", err)
	}

	if len(registry.Groups) == 0 {
		t.Log("No groups found - v2 system not active")
		return
	}

	t.Logf("Found %d groups", len(registry.Groups))
	for _, g := range registry.Groups {
		t.Logf("- %s (%s)", g.Name, g.FilePath)
		t.Logf("  Supergroup: %s", g.Supergroup)
		t.Logf("  Status: %s", g.Status)
		t.Logf("  Key Files: %d", len(g.KeyFiles))
		if len(g.MissingFields) > 0 {
			t.Logf("  Missing: %v", g.MissingFields)
		}
		if len(g.BrokenKeyFiles) > 0 {
			t.Logf("  Broken refs: %v", g.BrokenKeyFiles)
		}
		if g.IsStale {
			t.Log("  STALE: key files changed since doc was updated")
		}
	}
}

func TestParseDocContextGroup(t *testing.T) {
	// Test parsing our vision doc
	group, err := ParseDocContextGroup(".", "docs/context-groups-v2.md")
	if err != nil {
		t.Fatalf("Failed to parse doc: %v", err)
	}

	t.Logf("Name: %s", group.Name)
	t.Logf("Supergroup: %s", group.Supergroup)
	t.Logf("Status: %s", group.Status)
	t.Logf("Description: %.100s...", group.Description)
	t.Logf("Key Files: %v", group.KeyFiles)
	t.Logf("Missing: %v", group.MissingFields)
}
