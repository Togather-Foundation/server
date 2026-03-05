package mcp

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/mcp/tools"
)

// TestLLMsTxtToolsAreRegistered asserts that every tool name documented in
// web/llms.txt is a subset of the tools actually registered in the MCP server.
//
// llms.txt is allowed to omit tools (it focuses on public-facing tools), but
// it must not document tools that do not exist. This prevents documentation
// drift when tools are renamed or removed.
func TestLLMsTxtToolsAreRegistered(t *testing.T) {
	// Build the set of registered tool names by calling each tool definition
	// function directly. This avoids spinning up the full server and sidesteps
	// the lack of a public enumeration API on mcp-go.
	registered := map[string]bool{}

	// Event tools
	et := tools.NewEventTools(nil, nil, "")
	registered[et.EventsTool().Name] = true
	registered[et.AddEventTool().Name] = true

	// Place tools
	pt := tools.NewPlaceTools(nil, "")
	registered[pt.PlacesTool().Name] = true

	// Organization tools
	ot := tools.NewOrganizationTools(nil, "")
	registered[ot.OrganizationsTool().Name] = true

	// Search tools
	st := tools.NewSearchTools(nil, nil, nil, "")
	registered[st.SearchTool().Name] = true

	// Developer tools
	dt := tools.NewDeveloperTools(nil, "")
	registered[dt.APIKeysTool().Name] = true
	registered[dt.ManageAPIKeyTool().Name] = true

	// Geocoding tools
	gt := tools.NewGeocodingTools(nil)
	registered[gt.GeocodeAddressTool().Name] = true
	registered[gt.ReverseGeocodeTool().Name] = true

	// Locate web/llms.txt relative to this source file.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot locate source file")
	}
	// thisFile: .../internal/mcp/server_llmstxt_test.go
	// web/llms.txt is at: .../web/llms.txt  (three levels up: mcp → internal → repo root)
	llmsTxtPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "web", "llms.txt")

	llmsToolNames, err := parseLLMsTxtToolNames(llmsTxtPath)
	if err != nil {
		t.Fatalf("parsing %s: %v", llmsTxtPath, err)
	}

	if len(llmsToolNames) == 0 {
		t.Fatal("no tool names found in llms.txt — check the parser or the file format")
	}

	for _, name := range llmsToolNames {
		if !registered[name] {
			t.Errorf("tool %q is documented in llms.txt but is not registered in the MCP server", name)
		}
	}
}

// parseLLMsTxtToolNames extracts tool names from the "Available MCP Tools"
// section of an llms.txt file.
//
// It scans for the section header "### Available MCP Tools" and then collects
// lines that begin with "- " (after trimming leading whitespace). It stops at
// the next "###" section header. Tool names are the first token on each such
// line (split on whitespace).
//
// Example line:
//
//   - events             — list events with filters ...
//
// yields "events".
func parseLLMsTxtToolNames(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var names []string
	inToolsSection := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "### Available MCP Tools" {
			inToolsSection = true
			continue
		}

		if inToolsSection {
			// Stop at the next section header.
			if strings.HasPrefix(trimmed, "###") {
				break
			}

			// Lines starting with "- " list a tool name.
			if strings.HasPrefix(trimmed, "- ") {
				rest := strings.TrimPrefix(trimmed, "- ")
				// The tool name is the first whitespace-delimited token.
				fields := strings.Fields(rest)
				if len(fields) > 0 {
					names = append(names, fields[0])
				}
			}
		}
	}

	return names, scanner.Err()
}
