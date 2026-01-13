package groups

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ContextDoc represents a documentation-first context doc (v2)
// The markdown file itself IS the context doc
type ContextDoc struct {
	Name        string   // Derived from filename or H1 heading
	FilePath    string   // Path to the markdown file (relative to root)
	Category    string   // Category: Feature, Documentation, Data Layer, etc.
	Status      string   // Active, Deprecated, Experimental, Planned
	Related     []string // Paths to related documentation files
	Description string   // Content of the Description section
	KeyFiles    []string // Code entry points (relative paths)
	OutOfScope  string   // What this doesn't cover
	RawContent  string   // Full markdown content for copying

	// Metrics
	TokenEstimate int // Approximate token count (len/4)

	// Validation state
	MissingFields    []string // Required fields that are missing
	BrokenKeyFiles   []string // Key files that don't exist
	IsStale          bool     // True if key files changed since doc was modified
	LastDocModified  int64    // Unix timestamp of doc modification
	LastCodeModified int64    // Unix timestamp of most recent key file change
}

// Category represents a category for organizing context docs
type Category struct {
	ID   string // lowercase identifier
	Name string // Display name
}

// DefaultCategories returns the default set of categories
func DefaultCategories() []Category {
	return []Category{
		{ID: "meta", Name: "Meta"},
		{ID: "feature", Name: "Feature"},
		{ID: "miscellaneous", Name: "Miscellaneous"},
	}
}

// ContextDocRegistry holds the v2 context docs system state
type ContextDocRegistry struct {
	Categories []Category              // Available categories (defaults + custom)
	Docs       []ContextDoc            // All registered context docs
	ByCategory map[string][]ContextDoc // Docs organized by category ID
}

// ParseContextDoc parses a markdown file and extracts context doc metadata
func ParseContextDoc(rootPath, filePath string) (*ContextDoc, error) {
	fullPath := filepath.Join(rootPath, filePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}

	doc := &ContextDoc{
		FilePath:      filePath,
		RawContent:    string(content),
		TokenEstimate: len(content) / 4, // Rough approximation for English text
	}

	// Get file modification time
	info, err := os.Stat(fullPath)
	if err == nil {
		doc.LastDocModified = info.ModTime().Unix()
	}

	lines := strings.Split(string(content), "\n")

	// State machine for parsing
	var currentSection string
	var descriptionLines []string
	var outOfScopeLines []string
	var keyFileLines []string
	inCodeBlock := false

	// Regex patterns for inline metadata
	categoryRe := regexp.MustCompile(`(?i)^\*\*Category:\*\*\s*(.+)$`)
	statusRe := regexp.MustCompile(`(?i)^\*\*Status:\*\*\s*(.+)$`)
	relatedRe := regexp.MustCompile(`(?i)^\*\*Related:\*\*\s*(.+)$`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track code blocks - skip content inside them
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}

		// Extract H1 as name (fallback to filename)
		if strings.HasPrefix(trimmed, "# ") && doc.Name == "" {
			doc.Name = strings.TrimPrefix(trimmed, "# ")
			continue
		}

		// Parse inline metadata (bold field format)
		if match := categoryRe.FindStringSubmatch(trimmed); match != nil {
			doc.Category = strings.TrimSpace(match[1])
			continue
		}
		if match := statusRe.FindStringSubmatch(trimmed); match != nil {
			doc.Status = strings.TrimSpace(match[1])
			continue
		}
		if match := relatedRe.FindStringSubmatch(trimmed); match != nil {
			relatedStr := strings.TrimSpace(match[1])
			// Parse comma-separated list
			for _, r := range strings.Split(relatedStr, ",") {
				r = strings.TrimSpace(r)
				if r != "" {
					doc.Related = append(doc.Related, r)
				}
			}
			continue
		}

		// Track section headings
		if strings.HasPrefix(trimmed, "## ") {
			sectionName := strings.ToLower(strings.TrimPrefix(trimmed, "## "))
			switch {
			case strings.Contains(sectionName, "description"):
				currentSection = "description"
			case strings.Contains(sectionName, "key files") || strings.Contains(sectionName, "key-files"):
				currentSection = "keyfiles"
			case strings.Contains(sectionName, "out of scope") || strings.Contains(sectionName, "out-of-scope"):
				currentSection = "outofscope"
			case strings.Contains(sectionName, "scope") && !strings.Contains(sectionName, "out"):
				currentSection = "scope" // Regular scope section, not out of scope
			default:
				currentSection = "other"
			}
			continue
		}

		// Collect section content
		switch currentSection {
		case "description":
			if trimmed != "" {
				descriptionLines = append(descriptionLines, trimmed)
			}
		case "keyfiles":
			// Parse file entries (- path/to/file or - path/to/file - description)
			if strings.HasPrefix(trimmed, "- ") {
				entry := strings.TrimPrefix(trimmed, "- ")
				// Split on " - " to separate path from description
				parts := strings.SplitN(entry, " - ", 2)
				filePath := strings.TrimSpace(parts[0])
				// Remove any backticks
				filePath = strings.Trim(filePath, "`")
				if filePath != "" {
					keyFileLines = append(keyFileLines, filePath)
				}
			}
		case "outofscope":
			if trimmed != "" {
				outOfScopeLines = append(outOfScopeLines, trimmed)
			}
		}
	}

	// Set parsed values
	doc.Description = strings.Join(descriptionLines, " ")
	doc.KeyFiles = keyFileLines
	doc.OutOfScope = strings.Join(outOfScopeLines, " ")

	// Fallback name to filename
	if doc.Name == "" {
		base := filepath.Base(filePath)
		doc.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	// Validate required fields
	doc.MissingFields = validateContextDoc(doc)

	return doc, nil
}

// validateContextDoc checks for missing required fields
func validateContextDoc(doc *ContextDoc) []string {
	var missing []string
	if doc.Category == "" {
		missing = append(missing, "Category")
	}
	if doc.Status == "" {
		missing = append(missing, "Status")
	}
	if doc.Description == "" {
		missing = append(missing, "Description")
	}
	if len(doc.KeyFiles) == 0 {
		missing = append(missing, "Key Files")
	}
	return missing
}

// ValidateKeyFiles checks which key files exist and returns broken paths
func (d *ContextDoc) ValidateKeyFiles(rootPath string) []string {
	var broken []string
	for _, kf := range d.KeyFiles {
		fullPath := filepath.Join(rootPath, kf)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			broken = append(broken, kf)
		}
	}
	d.BrokenKeyFiles = broken
	return broken
}

// LoadContextDocRegistry loads the v2 context docs from .context-docs.md registry
func LoadContextDocRegistry(rootPath string) (*ContextDocRegistry, error) {
	registry := &ContextDocRegistry{
		Categories: DefaultCategories(),
		Docs:       []ContextDoc{},
		ByCategory: make(map[string][]ContextDoc),
	}

	registryPath := filepath.Join(rootPath, ".context-docs.md")
	file, err := os.Open(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No registry yet, return empty
			return registry, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inActiveDocs := false
	// Track per-category doc order from file structure
	categoryDocOrder := make(map[string][]ContextDoc)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Detect sections
		if strings.HasPrefix(line, "## Active Docs") {
			inActiveDocs = true
			continue
		}
		if strings.HasPrefix(line, "## ") && !strings.HasPrefix(line, "### ") {
			inActiveDocs = false
			continue
		}

		// Skip category headers (### CategoryName) - we use doc's own category metadata
		if inActiveDocs && strings.HasPrefix(line, "### ") {
			continue
		}

		// Parse active doc entries: "- path/to/doc.md (Category, Status)"
		if inActiveDocs && strings.HasPrefix(line, "- ") {
			entry := strings.TrimPrefix(line, "- ")
			// Extract path (before parentheses)
			parenIdx := strings.Index(entry, "(")
			var docPath string
			if parenIdx > 0 {
				docPath = strings.TrimSpace(entry[:parenIdx])
			} else {
				docPath = strings.TrimSpace(entry)
			}

			if docPath != "" {
				// Parse the document
				doc, err := ParseContextDoc(rootPath, docPath)
				if err != nil {
					// File doesn't exist or can't be read - create placeholder
					doc = &ContextDoc{
						Name:          filepath.Base(docPath),
						FilePath:      docPath,
						MissingFields: []string{"File not found"},
					}
				} else {
					// Validate key file paths exist
					doc.ValidateKeyFiles(rootPath)
					// Check staleness via git history
					doc.CheckStaleness(rootPath)
				}
				registry.Docs = append(registry.Docs, *doc)

				// Track order per category (from file order, preserves reordering)
				catID := strings.ToLower(strings.ReplaceAll(doc.Category, " ", "-"))
				if catID == "" {
					catID = "miscellaneous"
				}
				categoryDocOrder[catID] = append(categoryDocOrder[catID], *doc)
			}
		}
	}

	// Auto-discover categories from parsed docs
	// Collect unique categories that are not defaults
	usedCategories := make(map[string]string) // ID -> Name
	for _, d := range registry.Docs {
		if d.Category == "" {
			continue
		}
		catID := strings.ToLower(strings.ReplaceAll(d.Category, " ", "-"))
		// Check if it's already a default
		isDefault := false
		for _, dc := range DefaultCategories() {
			if strings.EqualFold(dc.ID, catID) || strings.EqualFold(dc.Name, d.Category) {
				isDefault = true
				break
			}
		}
		if !isDefault {
			usedCategories[catID] = d.Category
		}
	}

	// Add discovered custom categories
	for id, name := range usedCategories {
		registry.Categories = append(registry.Categories, Category{
			ID:   id,
			Name: name,
		})
	}

	// Use the file order for ByCategory (preserves user's reordering)
	registry.ByCategory = categoryDocOrder

	return registry, nil
}

// FindMarkdownFiles searches for all .md files in the project
func FindMarkdownFiles(rootPath string) ([]string, error) {
	var mdFiles []string

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip hidden directories and common non-doc directories
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check for .md extension
		if strings.HasSuffix(strings.ToLower(path), ".md") {
			relPath, err := filepath.Rel(rootPath, path)
			if err == nil {
				mdFiles = append(mdFiles, relPath)
			}
		}

		return nil
	})

	return mdFiles, err
}

// GenerateStructureTemplate returns a template for missing sections
func GenerateStructureTemplate(doc *ContextDoc) string {
	var sb strings.Builder

	// Only add missing sections
	needsCategory := false
	needsStatus := false
	needsDescription := false
	needsKeyFiles := false

	for _, field := range doc.MissingFields {
		switch field {
		case "Category":
			needsCategory = true
		case "Status":
			needsStatus = true
		case "Description":
			needsDescription = true
		case "Key Files":
			needsKeyFiles = true
		}
	}

	if needsCategory || needsStatus {
		sb.WriteString("\n<!-- Add after the H1 title -->\n")
		if needsCategory {
			sb.WriteString("**Category:** Feature\n")
		}
		if needsStatus {
			sb.WriteString("**Status:** Active\n")
		}
		sb.WriteString("\n")
	}

	if needsDescription {
		sb.WriteString("## Description\n\n")
		sb.WriteString("[High-level purpose and architecture explanation]\n\n")
	}

	if needsKeyFiles {
		sb.WriteString("## Key Files\n\n")
		sb.WriteString("- src/example.ts - Description of this entry point\n")
		sb.WriteString("- src/related.ts - Another key file\n\n")
	}

	return sb.String()
}

// GenerateClaudePrompt returns a prompt to help Claude structure the doc
func GenerateClaudePrompt(doc *ContextDoc) string {
	var sb strings.Builder

	sb.WriteString("Please help me structure this documentation file for use as a context doc.\n\n")
	sb.WriteString("The file needs the following sections/metadata:\n\n")

	for _, field := range doc.MissingFields {
		switch field {
		case "Category":
			sb.WriteString("- **Category:** (add after H1 title) - Category like: Feature, Documentation, Data Layer, Architecture, Personal Context, Reference Material\n")
		case "Status":
			sb.WriteString("- **Status:** (add after H1 title) - One of: Active, Deprecated, Experimental, Planned\n")
		case "Description":
			sb.WriteString("- ## Description section - High-level explanation of purpose and architecture\n")
		case "Key Files":
			sb.WriteString("- ## Key Files section - List format required (not tables). Each line: \"- path/to/file - description\"\n")
		}
	}

	sb.WriteString("\nOptionally also add:\n")
	sb.WriteString("- **Related:** comma-separated list of related doc files\n")
	sb.WriteString("- ## Out of Scope section - What this doesn't cover (helps AI know boundaries)\n")

	return sb.String()
}

// CheckStaleness checks if a context doc is stale by comparing git history
// A doc is stale if any of its key files have been modified more recently than the doc
func (d *ContextDoc) CheckStaleness(rootPath string) {
	// Get the git repo root
	cmd := exec.Command("git", "-C", rootPath, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return // Not a git repo or git not available
	}
	gitRoot := strings.TrimSpace(string(output))

	// Get last commit time for the doc file
	docLastCommit := getGitLastCommitTime(gitRoot, d.FilePath)
	if docLastCommit == 0 {
		return // File not tracked or no history
	}
	d.LastDocModified = docLastCommit

	// Check each key file's last commit time
	var latestKeyFileTime int64
	for _, kf := range d.KeyFiles {
		kfTime := getGitLastCommitTime(gitRoot, kf)
		if kfTime > latestKeyFileTime {
			latestKeyFileTime = kfTime
		}
	}
	d.LastCodeModified = latestKeyFileTime

	// Mark as stale if key files changed after doc
	if latestKeyFileTime > docLastCommit {
		d.IsStale = true
	}
}

// getGitLastCommitTime returns the Unix timestamp of the last commit that modified the file
func getGitLastCommitTime(gitRoot, filePath string) int64 {
	// git log -1 --format=%ct -- filepath
	cmd := exec.Command("git", "-C", gitRoot, "log", "-1", "--format=%ct", "--", filePath)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	timestamp, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0
	}
	return timestamp
}

// SaveContextDocRegistry writes the registry back to .context-docs.md
func SaveContextDocRegistry(rootPath string, registry *ContextDocRegistry) error {
	var sb strings.Builder

	sb.WriteString("# Context Docs\n\n")
	sb.WriteString("This project uses structured documentation as context docs.\n")
	sb.WriteString("Each doc is a markdown file with metadata: Category, Status,\n")
	sb.WriteString("Description, Key Files, and optionally Related and Out of Scope.\n\n")
	sb.WriteString("Categories are auto-discovered from markdown files. To create a custom\n")
	sb.WriteString("category, just set `**Category:** YourCategory` in any markdown file.\n\n")

	// Collect categories that are actually in use
	usedCategories := make(map[string]bool)
	for _, d := range registry.Docs {
		if d.Category != "" {
			catID := strings.ToLower(strings.ReplaceAll(d.Category, " ", "-"))
			usedCategories[catID] = true
		}
	}

	sb.WriteString("## Categories (auto-discovered)\n\n")
	for _, cat := range registry.Categories {
		// Only list categories that have docs or are defaults
		isDefault := false
		for _, dc := range DefaultCategories() {
			if dc.ID == cat.ID {
				isDefault = true
				break
			}
		}
		if isDefault || usedCategories[cat.ID] {
			sb.WriteString("- " + cat.Name + "\n")
		}
	}
	sb.WriteString("\n")

	sb.WriteString("## Active Docs\n\n")
	// Write docs grouped by category to preserve per-category ordering
	for _, cat := range registry.Categories {
		catDocs := registry.ByCategory[cat.ID]
		if len(catDocs) == 0 {
			continue
		}
		sb.WriteString("### " + cat.Name + "\n\n")
		for _, d := range catDocs {
			status := d.Status
			if status == "" {
				status = "?"
			}
			sb.WriteString("- " + d.FilePath + " (" + d.Category + ", " + status + ")\n")
		}
		sb.WriteString("\n")
	}

	registryPath := filepath.Join(rootPath, ".context-docs.md")
	return os.WriteFile(registryPath, []byte(sb.String()), 0644)
}
