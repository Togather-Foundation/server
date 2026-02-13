package resources

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestNewContextResources(t *testing.T) {
	tests := []struct {
		name        string
		baseDir     string
		wantBaseDir string
	}{
		{
			name:        "with custom base dir",
			baseDir:     "/custom/path",
			wantBaseDir: "/custom/path",
		},
		{
			name:        "with empty base dir uses default",
			baseDir:     "",
			wantBaseDir: "contexts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewContextResources(tt.baseDir)
			if r.baseDir != tt.wantBaseDir {
				t.Errorf("NewContextResources() baseDir = %v, want %v", r.baseDir, tt.wantBaseDir)
			}
			if r.cache == nil {
				t.Error("NewContextResources() cache is nil")
			}
		})
	}
}

func TestContextResources_Resource(t *testing.T) {
	r := NewContextResources("test-contexts")

	resource := r.Resource("context://test", "Test Context", "A test context file")

	if resource.URI != "context://test" {
		t.Errorf("Resource() URI = %v, want %v", resource.URI, "context://test")
	}
	if resource.Name != "Test Context" {
		t.Errorf("Resource() Name = %v, want %v", resource.Name, "Test Context")
	}
	if resource.Description != "A test context file" {
		t.Errorf("Resource() Description = %v, want %v", resource.Description, "A test context file")
	}
	if resource.MIMEType != contextMIMEType {
		t.Errorf("Resource() MIMEType = %v, want %v", resource.MIMEType, contextMIMEType)
	}
}

func TestContextResources_ContextPath(t *testing.T) {
	r := NewContextResources("/base")

	tests := []struct {
		name string
		rel  string
		want string
	}{
		{
			name: "relative path",
			rel:  "event.jsonld",
			want: "/base/event.jsonld",
		},
		{
			name: "absolute path unchanged",
			rel:  "/absolute/path/event.jsonld",
			want: "/absolute/path/event.jsonld",
		},
		{
			name: "nested relative path",
			rel:  "subdir/event.jsonld",
			want: "/base/subdir/event.jsonld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.ContextPath(tt.rel)
			if got != tt.want {
				t.Errorf("ContextPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContextURI(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "simple name",
			arg:  "event",
			want: "context://event",
		},
		{
			name: "complex name",
			arg:  "event-with-dashes",
			want: "context://event-with-dashes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContextURI(tt.arg)
			if got != tt.want {
				t.Errorf("ContextURI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContextResources_loadFile_CacheHit(t *testing.T) {
	// Create temp directory and file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.jsonld")
	testContent := `{"@context": "test"}`

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewContextResources(tmpDir)

	// First load - cache miss
	content1, err := r.loadFile(testFile)
	if err != nil {
		t.Fatalf("loadFile() error = %v", err)
	}
	if content1 != testContent {
		t.Errorf("loadFile() = %v, want %v", content1, testContent)
	}

	// Modify the file
	newContent := `{"@context": "modified"}`
	if err := os.WriteFile(testFile, []byte(newContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Second load - cache hit, should return original content
	content2, err := r.loadFile(testFile)
	if err != nil {
		t.Fatalf("loadFile() error = %v", err)
	}
	if content2 != testContent {
		t.Errorf("loadFile() cache hit = %v, want %v (original content)", content2, testContent)
	}
}

func TestContextResources_loadFile_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewContextResources(tmpDir)

	missingFile := filepath.Join(tmpDir, "nonexistent.jsonld")

	_, err := r.loadFile(missingFile)
	if err == nil {
		t.Error("loadFile() expected error for missing file, got nil")
	}
}

func TestContextResources_loadFile_ConcurrentAccess(t *testing.T) {
	// Create temp directory and file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "concurrent.jsonld")
	testContent := `{"@context": "concurrent test"}`

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewContextResources(tmpDir)

	// Launch multiple goroutines to load the same file concurrently
	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)
	contents := make(chan string, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			content, err := r.loadFile(testFile)
			if err != nil {
				errors <- err
				return
			}
			contents <- content
		}()
	}

	wg.Wait()
	close(errors)
	close(contents)

	// Check for errors
	for err := range errors {
		t.Errorf("loadFile() concurrent error = %v", err)
	}

	// Check all contents are correct
	for content := range contents {
		if content != testContent {
			t.Errorf("loadFile() concurrent = %v, want %v", content, testContent)
		}
	}

	// Verify cache has exactly one entry
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.cache) != 1 {
		t.Errorf("loadFile() cache size = %v, want 1", len(r.cache))
	}
}

func TestContextResources_ReadHandler(t *testing.T) {
	// Create temp directory and file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "handler.jsonld")
	testContent := `{"@context": "handler test"}`

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewContextResources(tmpDir)
	uri := "context://handler"

	handler := r.ReadHandler(testFile, uri)

	tests := []struct {
		name        string
		requestURI  string
		wantURI     string
		wantContent string
	}{
		{
			name:        "default URI",
			requestURI:  "",
			wantURI:     uri,
			wantContent: testContent,
		},
		{
			name:        "custom request URI",
			requestURI:  "context://custom",
			wantURI:     "context://custom",
			wantContent: testContent,
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
				t.Fatalf("ReadHandler() error = %v", err)
			}

			if len(contents) != 1 {
				t.Fatalf("ReadHandler() returned %d contents, want 1", len(contents))
			}

			textContent, ok := contents[0].(mcp.TextResourceContents)
			if !ok {
				t.Fatalf("ReadHandler() content is not TextResourceContents")
			}

			if textContent.URI != tt.wantURI {
				t.Errorf("ReadHandler() URI = %v, want %v", textContent.URI, tt.wantURI)
			}
			if textContent.Text != tt.wantContent {
				t.Errorf("ReadHandler() Text = %v, want %v", textContent.Text, tt.wantContent)
			}
			if textContent.MIMEType != contextMIMEType {
				t.Errorf("ReadHandler() MIMEType = %v, want %v", textContent.MIMEType, contextMIMEType)
			}
		})
	}
}

func TestContextResources_ReadHandler_FileError(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewContextResources(tmpDir)

	missingFile := filepath.Join(tmpDir, "missing.jsonld")
	handler := r.ReadHandler(missingFile, "context://missing")

	req := mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{},
	}

	_, err := handler(context.Background(), req)
	if err == nil {
		t.Error("ReadHandler() expected error for missing file, got nil")
	}
}
