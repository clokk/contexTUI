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

// DocContextGroup represents a documentation-first context group (v2)
// The markdown file itself IS the context group
type DocContextGroup struct {
	Name        string   // Derived from filename or H1 heading
	FilePath    string   // Path to the markdown file (relative to root)
	Supergroup  string   // Category: Feature, Documentation, Data Layer, etc.
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

// Supergroup represents a category for organizing doc groups
type Supergroup struct {
	ID   string // lowercase identifier
	Name string // Display name
}

// DefaultSupergroups returns the default set of supergroups
func DefaultSupergroups() []Supergroup {
	return []Supergroup{
		{ID: "meta", Name: "Meta"},
		{ID: "feature", Name: "Feature"},
		{ID: "miscellaneous", Name: "Miscellaneous"},
	}
}

// DocGroupRegistry holds the v2 context groups system state
type DocGroupRegistry struct {
	Supergroups []Supergroup                 // Available supergroups (defaults + custom)
	Groups      []DocContextGroup            // All registered doc groups
	BySuper     map[string][]DocContextGroup // Groups organized by supergroup ID
}

// ParseDocContextGroup parses a markdown file and extracts context group metadata
func ParseDocContextGroup(rootPath, filePath string) (*DocContextGroup, error) {
	fullPath := filepath.Join(rootPath, filePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}

	group := &DocContextGroup{
		FilePath:      filePath,
		RawContent:    string(content),
		TokenEstimate: len(content) / 4, // Rough approximation for English text
	}

	// Get file modification time
	info, err := os.Stat(fullPath)
	if err == nil {
		group.LastDocModified = info.ModTime().Unix()
	}

	lines := strings.Split(string(content), "\n")

	// State machine for parsing
	var currentSection string
	var descriptionLines []string
	var outOfScopeLines []string
	var keyFileLines []string
	inCodeBlock := false

	// Regex patterns for inline metadata
	supergroupRe := regexp.MustCompile(`(?i)^\*\*Supergroup:\*\*\s*(.+)$`)
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
		if strings.HasPrefix(trimmed, "# ") && group.Name == "" {
			group.Name = strings.TrimPrefix(trimmed, "# ")
			continue
		}

		// Parse inline metadata (bold field format)
		if match := supergroupRe.FindStringSubmatch(trimmed); match != nil {
			group.Supergroup = strings.TrimSpace(match[1])
			continue
		}
		if match := statusRe.FindStringSubmatch(trimmed); match != nil {
			group.Status = strings.TrimSpace(match[1])
			continue
		}
		if match := relatedRe.FindStringSubmatch(trimmed); match != nil {
			relatedStr := strings.TrimSpace(match[1])
			// Parse comma-separated list
			for _, r := range strings.Split(relatedStr, ",") {
				r = strings.TrimSpace(r)
				if r != "" {
					group.Related = append(group.Related, r)
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
	group.Description = strings.Join(descriptionLines, " ")
	group.KeyFiles = keyFileLines
	group.OutOfScope = strings.Join(outOfScopeLines, " ")

	// Fallback name to filename
	if group.Name == "" {
		base := filepath.Base(filePath)
		group.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	// Validate required fields
	group.MissingFields = validateDocGroup(group)

	return group, nil
}

// validateDocGroup checks for missing required fields
func validateDocGroup(group *DocContextGroup) []string {
	var missing []string
	if group.Supergroup == "" {
		missing = append(missing, "Supergroup")
	}
	if group.Status == "" {
		missing = append(missing, "Status")
	}
	if group.Description == "" {
		missing = append(missing, "Description")
	}
	if len(group.KeyFiles) == 0 {
		missing = append(missing, "Key Files")
	}
	return missing
}

// ValidateKeyFiles checks which key files exist and returns broken paths
func (g *DocContextGroup) ValidateKeyFiles(rootPath string) []string {
	var broken []string
	for _, kf := range g.KeyFiles {
		fullPath := filepath.Join(rootPath, kf)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			broken = append(broken, kf)
		}
	}
	g.BrokenKeyFiles = broken
	return broken
}

// LoadDocGroupRegistry loads the v2 context groups from .context-groups.md registry
func LoadDocGroupRegistry(rootPath string) (*DocGroupRegistry, error) {
	registry := &DocGroupRegistry{
		Supergroups: DefaultSupergroups(),
		Groups:      []DocContextGroup{},
		BySuper:     make(map[string][]DocContextGroup),
	}

	registryPath := filepath.Join(rootPath, ".context-groups.md")
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
	inActiveGroups := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Detect sections
		if strings.HasPrefix(line, "## Active Groups") {
			inActiveGroups = true
			continue
		}
		if strings.HasPrefix(line, "## ") {
			inActiveGroups = false
			continue
		}

		// Parse active group entries: "- path/to/doc.md (Supergroup, Status)"
		if inActiveGroups && strings.HasPrefix(line, "- ") {
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
				group, err := ParseDocContextGroup(rootPath, docPath)
				if err != nil {
					// File doesn't exist or can't be read - create placeholder
					group = &DocContextGroup{
						Name:          filepath.Base(docPath),
						FilePath:      docPath,
						MissingFields: []string{"File not found"},
					}
				} else {
					// Validate key file paths exist
					group.ValidateKeyFiles(rootPath)
					// Check staleness via git history
					group.CheckStaleness(rootPath)
				}
				registry.Groups = append(registry.Groups, *group)
			}
		}
	}

	// Auto-discover supergroups from parsed groups
	// Collect unique supergroups that are not defaults
	usedSupergroups := make(map[string]string) // ID -> Name
	for _, g := range registry.Groups {
		if g.Supergroup == "" {
			continue
		}
		sgID := strings.ToLower(strings.ReplaceAll(g.Supergroup, " ", "-"))
		// Check if it's already a default
		isDefault := false
		for _, dsg := range DefaultSupergroups() {
			if strings.EqualFold(dsg.ID, sgID) || strings.EqualFold(dsg.Name, g.Supergroup) {
				isDefault = true
				break
			}
		}
		if !isDefault {
			usedSupergroups[sgID] = g.Supergroup
		}
	}

	// Add discovered custom supergroups
	for id, name := range usedSupergroups {
		registry.Supergroups = append(registry.Supergroups, Supergroup{
			ID:   id,
			Name: name,
		})
	}

	// Organize by supergroup
	for _, g := range registry.Groups {
		sgID := strings.ToLower(strings.ReplaceAll(g.Supergroup, " ", "-"))
		if sgID == "" {
			sgID = "miscellaneous"
		}
		registry.BySuper[sgID] = append(registry.BySuper[sgID], g)
	}

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
func GenerateStructureTemplate(group *DocContextGroup) string {
	var sb strings.Builder

	// Only add missing sections
	needsSupergroup := false
	needsStatus := false
	needsDescription := false
	needsKeyFiles := false

	for _, field := range group.MissingFields {
		switch field {
		case "Supergroup":
			needsSupergroup = true
		case "Status":
			needsStatus = true
		case "Description":
			needsDescription = true
		case "Key Files":
			needsKeyFiles = true
		}
	}

	if needsSupergroup || needsStatus {
		sb.WriteString("\n<!-- Add after the H1 title -->\n")
		if needsSupergroup {
			sb.WriteString("**Supergroup:** Feature\n")
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
func GenerateClaudePrompt(group *DocContextGroup) string {
	var sb strings.Builder

	sb.WriteString("Please help me structure this documentation file for use as a context group.\n\n")
	sb.WriteString("The file needs the following sections/metadata:\n\n")

	for _, field := range group.MissingFields {
		switch field {
		case "Supergroup":
			sb.WriteString("- **Supergroup:** (add after H1 title) - Category like: Feature, Documentation, Data Layer, Architecture, Personal Context, Reference Material\n")
		case "Status":
			sb.WriteString("- **Status:** (add after H1 title) - One of: Active, Deprecated, Experimental, Planned\n")
		case "Description":
			sb.WriteString("- ## Description section - High-level explanation of purpose and architecture\n")
		case "Key Files":
			sb.WriteString("- ## Key Files section - List of code entry points (not exhaustive, just key files)\n")
		}
	}

	sb.WriteString("\nOptionally also add:\n")
	sb.WriteString("- **Related:** comma-separated list of related doc files\n")
	sb.WriteString("- ## Out of Scope section - What this doesn't cover (helps AI know boundaries)\n")

	return sb.String()
}

// CheckStaleness checks if a doc group is stale by comparing git history
// A doc is stale if any of its key files have been modified more recently than the doc
func (g *DocContextGroup) CheckStaleness(rootPath string) {
	// Get the git repo root
	cmd := exec.Command("git", "-C", rootPath, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return // Not a git repo or git not available
	}
	gitRoot := strings.TrimSpace(string(output))

	// Get last commit time for the doc file
	docLastCommit := getGitLastCommitTime(gitRoot, g.FilePath)
	if docLastCommit == 0 {
		return // File not tracked or no history
	}
	g.LastDocModified = docLastCommit

	// Check each key file's last commit time
	var latestKeyFileTime int64
	for _, kf := range g.KeyFiles {
		kfTime := getGitLastCommitTime(gitRoot, kf)
		if kfTime > latestKeyFileTime {
			latestKeyFileTime = kfTime
		}
	}
	g.LastCodeModified = latestKeyFileTime

	// Mark as stale if key files changed after doc
	if latestKeyFileTime > docLastCommit {
		g.IsStale = true
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

// SaveDocGroupRegistry writes the registry back to .context-groups.md
func SaveDocGroupRegistry(rootPath string, registry *DocGroupRegistry) error {
	var sb strings.Builder

	sb.WriteString("# Context Groups\n\n")
	sb.WriteString("This project uses structured documentation as context groups.\n")
	sb.WriteString("Each group is a markdown file with metadata: Supergroup, Status,\n")
	sb.WriteString("Description, Key Files, and optionally Related and Out of Scope.\n\n")
	sb.WriteString("Supergroups are auto-discovered from markdown files. To create a custom\n")
	sb.WriteString("supergroup, just set `**Supergroup:** YourCategory` in any markdown file.\n\n")

	// Collect supergroups that are actually in use
	usedSupergroups := make(map[string]bool)
	for _, g := range registry.Groups {
		if g.Supergroup != "" {
			sgID := strings.ToLower(strings.ReplaceAll(g.Supergroup, " ", "-"))
			usedSupergroups[sgID] = true
		}
	}

	sb.WriteString("## Supergroups (auto-discovered)\n\n")
	for _, sg := range registry.Supergroups {
		// Only list supergroups that have groups or are defaults
		isDefault := false
		for _, dsg := range DefaultSupergroups() {
			if dsg.ID == sg.ID {
				isDefault = true
				break
			}
		}
		if isDefault || usedSupergroups[sg.ID] {
			sb.WriteString("- " + sg.Name + "\n")
		}
	}
	sb.WriteString("\n")

	sb.WriteString("## Active Groups\n\n")
	for _, g := range registry.Groups {
		status := g.Status
		if status == "" {
			status = "?"
		}
		supergroup := g.Supergroup
		if supergroup == "" {
			supergroup = "?"
		}
		sb.WriteString("- " + g.FilePath + " (" + supergroup + ", " + status + ")\n")
	}

	registryPath := filepath.Join(rootPath, ".context-groups.md")
	return os.WriteFile(registryPath, []byte(sb.String()), 0644)
}
