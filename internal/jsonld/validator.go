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
// It uses the pyshacl CLI tool for validation (via uvx or direct execution).
type Validator struct {
	shapeFiles  []string
	pyshaclPath string
	useUvx      bool // If true, use uvx to run pyshacl
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

	// Check if pyshacl is available (try uvx first, then pyshacl directly)
	if uvxPath, err := exec.LookPath("uvx"); err == nil {
		// Use uvx to run pyshacl
		v.pyshaclPath = uvxPath
		v.useUvx = true
	} else if pyshaclPath, err := exec.LookPath("pyshacl"); err == nil {
		// Use pyshacl directly
		v.pyshaclPath = pyshaclPath
		v.useUvx = false
	} else {
		// Neither found - validation will be no-op
		fmt.Fprintf(os.Stderr, "WARNING: pyshacl not found in PATH (tried uvx and pyshacl), SHACL validation will be disabled\n")
		fmt.Fprintf(os.Stderr, "  Install with: make install-pyshacl\n")
		v.enabled = false
		return v, nil
	}

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

	// Create merged shapes file (pyshacl doesn't handle multiple -s flags correctly)
	mergedShapesFile, err := v.createMergedShapesFile()
	if err != nil {
		return fmt.Errorf("failed to create merged shapes file: %w", err)
	}
	defer os.Remove(mergedShapesFile)

	// Build pyshacl command arguments
	var cmd *exec.Cmd
	args := []string{}

	if v.useUvx {
		// Use uvx to run pyshacl
		args = append(args, "pyshacl")
	}

	args = append(args, "-s", mergedShapesFile, tmpFile.Name())

	// Execute pyshacl with context timeout
	cmd = exec.CommandContext(ctx, v.pyshaclPath, args...)
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

// createMergedShapesFile creates a temporary file containing all shape files merged.
// This is necessary because pyshacl doesn't correctly handle multiple -s flags.
func (v *Validator) createMergedShapesFile() (string, error) {
	tmpFile, err := os.CreateTemp("", "merged-shapes-*.ttl")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	for _, shapeFile := range v.shapeFiles {
		content, err := os.ReadFile(shapeFile)
		if err != nil {
			os.Remove(tmpFile.Name())
			return "", fmt.Errorf("failed to read shape file %s: %w", shapeFile, err)
		}
		if _, err := tmpFile.Write(content); err != nil {
			os.Remove(tmpFile.Name())
			return "", fmt.Errorf("failed to write to merged file: %w", err)
		}
		if _, err := tmpFile.WriteString("\n"); err != nil {
			os.Remove(tmpFile.Name())
			return "", fmt.Errorf("failed to write separator to merged file: %w", err)
		} // Ensure separation between files
	}

	return tmpFile.Name(), nil
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
