package main_test

import (
	"testing"

	"github.com/connorleisz/contexTUI/internal/groups"
)

func TestLoadContextDocRegistry(t *testing.T) {
	registry, err := groups.LoadContextDocRegistry(".")
	if err != nil {
		t.Fatalf("Failed to load registry: %v", err)
	}

	if len(registry.Docs) == 0 {
		t.Log("No docs found - context docs system not active")
		return
	}

	t.Logf("Found %d docs", len(registry.Docs))
	for _, d := range registry.Docs {
		t.Logf("- %s (%s)", d.Name, d.FilePath)
		t.Logf("  Category: %s", d.Category)
		t.Logf("  Status: %s", d.Status)
		t.Logf("  Key Files: %d", len(d.KeyFiles))
		if len(d.MissingFields) > 0 {
			t.Logf("  Missing: %v", d.MissingFields)
		}
		if len(d.BrokenKeyFiles) > 0 {
			t.Logf("  Broken refs: %v", d.BrokenKeyFiles)
		}
		if d.IsStale {
			t.Log("  STALE: key files changed since doc was updated")
		}
	}
}

func TestParseContextDoc(t *testing.T) {
	// Test parsing our context docs doc
	doc, err := groups.ParseContextDoc(".", "docs/context-docs.md")
	if err != nil {
		t.Fatalf("Failed to parse doc: %v", err)
	}

	t.Logf("Name: %s", doc.Name)
	t.Logf("Category: %s", doc.Category)
	t.Logf("Status: %s", doc.Status)
	t.Logf("Description: %.100s...", doc.Description)
	t.Logf("Key Files: %v", doc.KeyFiles)
	t.Logf("Missing: %v", doc.MissingFields)
}
