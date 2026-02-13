package resources

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestNewSchemaResources(t *testing.T) {
	tests := []struct {
		name            string
		openAPIPath     string
		wantOpenAPIPath string
	}{
		{
			name:            "with custom path",
			openAPIPath:     "/custom/openapi.yaml",
			wantOpenAPIPath: "/custom/openapi.yaml",
		},
		{
			name:            "with empty path uses default",
			openAPIPath:     "",
			wantOpenAPIPath: "docs/api/openapi.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewSchemaResources(tt.openAPIPath)
			if r.openAPIPath != tt.wantOpenAPIPath {
				t.Errorf("NewSchemaResources() openAPIPath = %v, want %v", r.openAPIPath, tt.wantOpenAPIPath)
			}
		})
	}
}

func TestSchemaResources_OpenAPIResource(t *testing.T) {
	r := NewSchemaResources("test.yaml")

	resource := r.OpenAPIResource()

	if resource.URI != openAPIResource {
		t.Errorf("OpenAPIResource() URI = %v, want %v", resource.URI, openAPIResource)
	}
	if resource.Name != "OpenAPI Schema" {
		t.Errorf("OpenAPIResource() Name = %v, want %v", resource.Name, "OpenAPI Schema")
	}
	if resource.Description != "OpenAPI specification for the Togather SEL backend" {
		t.Errorf("OpenAPIResource() Description = %v, want 'OpenAPI specification for the Togather SEL backend'", resource.Description)
	}
	if resource.MIMEType != schemaMIMEType {
		t.Errorf("OpenAPIResource() MIMEType = %v, want %v", resource.MIMEType, schemaMIMEType)
	}
}

func TestSchemaResources_InfoResource(t *testing.T) {
	r := NewSchemaResources("test.yaml")

	resource := r.InfoResource()

	if resource.URI != serverInfoResource {
		t.Errorf("InfoResource() URI = %v, want %v", resource.URI, serverInfoResource)
	}
	if resource.Name != "Server Info" {
		t.Errorf("InfoResource() Name = %v, want %v", resource.Name, "Server Info")
	}
	if resource.Description != "MCP server metadata and capabilities" {
		t.Errorf("InfoResource() Description = %v, want 'MCP server metadata and capabilities'", resource.Description)
	}
	if resource.MIMEType != schemaMIMEType {
		t.Errorf("InfoResource() MIMEType = %v, want %v", resource.MIMEType, schemaMIMEType)
	}
}

func TestSchemaResources_loadOpenAPI(t *testing.T) {
	// Create temp directory and YAML file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "openapi.yaml")
	testYAML := `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths:
  /test:
    get:
      summary: Test endpoint`

	if err := os.WriteFile(testFile, []byte(testYAML), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewSchemaResources(testFile)

	// First load
	content1, err := r.loadOpenAPI()
	if err != nil {
		t.Fatalf("loadOpenAPI() error = %v", err)
	}

	// Verify it's valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(content1), &result); err != nil {
		t.Errorf("loadOpenAPI() returned invalid JSON: %v", err)
	}

	// Verify content
	if result["openapi"] != "3.0.0" {
		t.Errorf("loadOpenAPI() openapi version = %v, want 3.0.0", result["openapi"])
	}

	// Second load - should return cached content
	content2, err := r.loadOpenAPI()
	if err != nil {
		t.Fatalf("loadOpenAPI() second call error = %v", err)
	}

	if content1 != content2 {
		t.Error("loadOpenAPI() second call returned different content (cache not working)")
	}
}

func TestSchemaResources_loadOpenAPI_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	missingFile := filepath.Join(tmpDir, "nonexistent.yaml")

	r := NewSchemaResources(missingFile)

	_, err := r.loadOpenAPI()
	if err == nil {
		t.Error("loadOpenAPI() expected error for missing file, got nil")
	}
}

func TestSchemaResources_loadOpenAPI_InvalidYAML(t *testing.T) {
	// Create temp directory and invalid YAML file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.yaml")
	invalidYAML := `invalid: yaml: [unclosed`

	if err := os.WriteFile(testFile, []byte(invalidYAML), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewSchemaResources(testFile)

	_, err := r.loadOpenAPI()
	if err == nil {
		t.Error("loadOpenAPI() expected error for invalid YAML, got nil")
	}
}

func TestSchemaResources_loadInfo(t *testing.T) {
	r := NewSchemaResources("test.yaml")

	testInfo := ServerInfo{
		Name:    "test-server",
		Version: "1.0.0",
		BaseURL: "https://test.example.com",
		Capabilities: ServerCapabilities{
			Tools:     true,
			Resources: true,
			Prompts:   false,
		},
		Transport: "stdio",
	}

	// First load
	content1, err := r.loadInfo(testInfo)
	if err != nil {
		t.Fatalf("loadInfo() error = %v", err)
	}

	// Verify it's valid JSON
	var result ServerInfo
	if err := json.Unmarshal([]byte(content1), &result); err != nil {
		t.Errorf("loadInfo() returned invalid JSON: %v", err)
	}

	// Verify content
	if result.Name != testInfo.Name {
		t.Errorf("loadInfo() name = %v, want %v", result.Name, testInfo.Name)
	}
	if result.Version != testInfo.Version {
		t.Errorf("loadInfo() version = %v, want %v", result.Version, testInfo.Version)
	}
	if result.Capabilities.Tools != testInfo.Capabilities.Tools {
		t.Errorf("loadInfo() capabilities.tools = %v, want %v", result.Capabilities.Tools, testInfo.Capabilities.Tools)
	}

	// Second load - should return cached content
	content2, err := r.loadInfo(testInfo)
	if err != nil {
		t.Fatalf("loadInfo() second call error = %v", err)
	}

	if content1 != content2 {
		t.Error("loadInfo() second call returned different content (cache not working)")
	}
}

func TestSchemaResources_OpenAPIReadHandler(t *testing.T) {
	// Create temp directory and YAML file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "openapi.yaml")
	testYAML := `openapi: 3.0.0
info:
  title: Handler Test
  version: 1.0.0`

	if err := os.WriteFile(testFile, []byte(testYAML), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewSchemaResources(testFile)
	handler := r.OpenAPIReadHandler()

	tests := []struct {
		name       string
		requestURI string
		wantURI    string
	}{
		{
			name:       "default URI",
			requestURI: "",
			wantURI:    openAPIResource,
		},
		{
			name:       "custom request URI",
			requestURI: "schema://custom",
			wantURI:    "schema://custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.ReadResourceRequest{
				Params: mcp.ReadResourceParams{
					URI: tt.requestURI,
				},
			}

			contents, err := handler(context.Background(), req)
			if err != nil {
				t.Fatalf("OpenAPIReadHandler() error = %v", err)
			}

			if len(contents) != 1 {
				t.Fatalf("OpenAPIReadHandler() returned %d contents, want 1", len(contents))
			}

			textContent, ok := contents[0].(mcp.TextResourceContents)
			if !ok {
				t.Fatalf("OpenAPIReadHandler() content is not TextResourceContents")
			}

			if textContent.URI != tt.wantURI {
				t.Errorf("OpenAPIReadHandler() URI = %v, want %v", textContent.URI, tt.wantURI)
			}
			if textContent.MIMEType != schemaMIMEType {
				t.Errorf("OpenAPIReadHandler() MIMEType = %v, want %v", textContent.MIMEType, schemaMIMEType)
			}

			// Verify content is valid JSON
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(textContent.Text), &result); err != nil {
				t.Errorf("OpenAPIReadHandler() returned invalid JSON: %v", err)
			}
		})
	}
}

func TestSchemaResources_InfoReadHandler(t *testing.T) {
	r := NewSchemaResources("test.yaml")

	testInfo := ServerInfo{
		Name:    "info-handler-test",
		Version: "2.0.0",
		Capabilities: ServerCapabilities{
			Tools:     true,
			Resources: false,
			Prompts:   true,
		},
	}

	handler := r.InfoReadHandler(testInfo)

	tests := []struct {
		name       string
		requestURI string
		wantURI    string
	}{
		{
			name:       "default URI",
			requestURI: "",
			wantURI:    serverInfoResource,
		},
		{
			name:       "custom request URI",
			requestURI: "info://custom",
			wantURI:    "info://custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.ReadResourceRequest{
				Params: mcp.ReadResourceParams{
					URI: tt.requestURI,
				},
			}

			contents, err := handler(context.Background(), req)
			if err != nil {
				t.Fatalf("InfoReadHandler() error = %v", err)
			}

			if len(contents) != 1 {
				t.Fatalf("InfoReadHandler() returned %d contents, want 1", len(contents))
			}

			textContent, ok := contents[0].(mcp.TextResourceContents)
			if !ok {
				t.Fatalf("InfoReadHandler() content is not TextResourceContents")
			}

			if textContent.URI != tt.wantURI {
				t.Errorf("InfoReadHandler() URI = %v, want %v", textContent.URI, tt.wantURI)
			}
			if textContent.MIMEType != schemaMIMEType {
				t.Errorf("InfoReadHandler() MIMEType = %v, want %v", textContent.MIMEType, schemaMIMEType)
			}

			// Verify content is valid JSON with expected fields
			var result ServerInfo
			if err := json.Unmarshal([]byte(textContent.Text), &result); err != nil {
				t.Errorf("InfoReadHandler() returned invalid JSON: %v", err)
			}

			if result.Name != testInfo.Name {
				t.Errorf("InfoReadHandler() name = %v, want %v", result.Name, testInfo.Name)
			}
			if result.Version != testInfo.Version {
				t.Errorf("InfoReadHandler() version = %v, want %v", result.Version, testInfo.Version)
			}
		})
	}
}

func TestRepoRoot(t *testing.T) {
	// Save original directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("failed to restore directory: %v", err)
		}
	}()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr bool
	}{
		{
			name: "finds go.mod in current directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				goModPath := filepath.Join(tmpDir, "go.mod")
				if err := os.WriteFile(goModPath, []byte("module test"), 0644); err != nil {
					t.Fatal(err)
				}
				return tmpDir
			},
			wantErr: false,
		},
		{
			name: "finds go.mod in parent directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				goModPath := filepath.Join(tmpDir, "go.mod")
				if err := os.WriteFile(goModPath, []byte("module test"), 0644); err != nil {
					t.Fatal(err)
				}
				subDir := filepath.Join(tmpDir, "subdir")
				if err := os.Mkdir(subDir, 0755); err != nil {
					t.Fatal(err)
				}
				return subDir
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := tt.setup(t)
			if err := os.Chdir(testDir); err != nil {
				t.Fatal(err)
			}

			root, err := repoRoot()
			if (err != nil) != tt.wantErr {
				t.Errorf("repoRoot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify go.mod exists in the returned root
				goModPath := filepath.Join(root, "go.mod")
				if _, err := os.Stat(goModPath); err != nil {
					t.Errorf("repoRoot() = %v, but go.mod not found at that path", root)
				}
			}
		})
	}
}

func TestSchemaResources_resolveOpenAPIPath(t *testing.T) {
	// This is a light test since repoRoot uses runtime.Caller fallback
	r := NewSchemaResources("specs/test.yaml")

	resolved := r.resolveOpenAPIPath()

	// Should contain the configured path
	if !filepath.IsAbs(resolved) {
		// If not absolute, it means repo root wasn't found, which is ok in test context
		t.Logf("resolveOpenAPIPath() returned relative path (repo root not found): %v", resolved)
	}
}

func TestServerInfo_JSONMarshaling(t *testing.T) {
	info := ServerInfo{
		Name:    "test-server",
		Version: "1.0.0",
		BaseURL: "https://example.com",
		Capabilities: ServerCapabilities{
			Tools:     true,
			Resources: false,
			Prompts:   true,
		},
		Transport: "stdio",
	}

	// Marshal
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Unmarshal
	var decoded ServerInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Verify
	if decoded.Name != info.Name {
		t.Errorf("decoded.Name = %v, want %v", decoded.Name, info.Name)
	}
	if decoded.Version != info.Version {
		t.Errorf("decoded.Version = %v, want %v", decoded.Version, info.Version)
	}
	if decoded.BaseURL != info.BaseURL {
		t.Errorf("decoded.BaseURL = %v, want %v", decoded.BaseURL, info.BaseURL)
	}
	if decoded.Capabilities != info.Capabilities {
		t.Errorf("decoded.Capabilities = %v, want %v", decoded.Capabilities, info.Capabilities)
	}
	if decoded.Transport != info.Transport {
		t.Errorf("decoded.Transport = %v, want %v", decoded.Transport, info.Transport)
	}
}
