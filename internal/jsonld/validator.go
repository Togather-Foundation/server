package jsonld

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Validator validates JSON-LD data against SHACL shapes.
// It uses the pyshacl CLI tool for validation.
type Validator struct {
	shapeFiles  []string
	pyshaclPath string
	enabled     bool
	mu          sync.RWMutex
}

// ValidationError represents a SHACL validation failure.
type ValidationError struct {
	Message string
	Report  string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("SHACL validation failed: %s", e.Message)
}

// NewValidator creates a new SHACL validator.
// If pyshacl is not found or enabled is false, validation will be a no-op.
func NewValidator(shapesDir string, enabled bool) (*Validator, error) {
	v := &Validator{
		enabled: enabled,
	}

	// If disabled, return early (no-op validator)
	if !enabled {
		return v, nil
	}

	// Load shape files first (check directory exists)
	shapeFiles, err := findShapeFiles(shapesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find shape files: %w", err)
	}
	if len(shapeFiles) == 0 {
		return nil, fmt.Errorf("no shape files found in %s", shapesDir)
	}
	v.shapeFiles = shapeFiles

	// Check if pyshacl is available
	pyshaclPath, err := exec.LookPath("pyshacl")
	if err != nil {
		// Log warning but don't fail - validation will be no-op
		fmt.Fprintf(os.Stderr, "WARNING: pyshacl not found in PATH, SHACL validation will be disabled\n")
		v.enabled = false
		return v, nil
	}
	v.pyshaclPath = pyshaclPath

	return v, nil
}

// ValidateEvent validates a JSON-LD event payload against SHACL shapes.
// Returns nil if validation passes or if validation is disabled.
// Returns ValidationError if validation fails.
func (v *Validator) ValidateEvent(ctx context.Context, jsonldData map[string]any) error {
	v.mu.RLock()
	enabled := v.enabled
	v.mu.RUnlock()

	// Skip validation if disabled
	if !enabled {
		return nil
	}

	// Convert JSON-LD to Turtle format
	turtle, err := SerializeToTurtle(jsonldData)
	if err != nil {
		return fmt.Errorf("failed to convert JSON-LD to Turtle: %w", err)
	}

	// Create temporary file for data
	tmpFile, err := os.CreateTemp("", "shacl-data-*.ttl")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Write Turtle data to temp file
	if _, err := tmpFile.WriteString(turtle); err != nil {
		return fmt.Errorf("failed to write Turtle data: %w", err)
	}
	tmpFile.Close()

	// Build pyshacl command arguments
	args := []string{}
	for _, shapeFile := range v.shapeFiles {
		args = append(args, "-s", shapeFile)
	}
	args = append(args, tmpFile.Name())

	// Execute pyshacl with context timeout
	cmd := exec.CommandContext(ctx, v.pyshaclPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		// pyshacl returns non-zero exit code on validation failure
		output := stdout.String() + stderr.String()
		return &ValidationError{
			Message: extractValidationMessage(output),
			Report:  output,
		}
	}

	return nil
}

// findShapeFiles finds all .ttl files in the shapes directory.
func findShapeFiles(shapesDir string) ([]string, error) {
	// Check if directory exists
	info, err := os.Stat(shapesDir)
	if err != nil {
		return nil, fmt.Errorf("shapes directory not found: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("shapes path is not a directory: %s", shapesDir)
	}

	// Find all .ttl files
	matches, err := filepath.Glob(filepath.Join(shapesDir, "*.ttl"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob shape files: %w", err)
	}

	// Convert to absolute paths
	absMatches := make([]string, len(matches))
	for i, match := range matches {
		absPath, err := filepath.Abs(match)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
		}
		absMatches[i] = absPath
	}

	return absMatches, nil
}

// extractValidationMessage extracts a human-readable message from pyshacl output.
func extractValidationMessage(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Look for common SHACL violation indicators
		if strings.Contains(line, "sh:message") {
			return strings.TrimSpace(line)
		}
		if strings.Contains(line, "Constraint Violation") {
			return strings.TrimSpace(line)
		}
	}
	// Fallback: return first non-empty line
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return "SHACL validation failed (no specific message)"
}
