package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateRobotsTxt(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()
	webfilesOutput = tempDir
	webfilesDomain = "example.com"

	// Generate robots.txt
	err := generateRobotsTxt()
	if err != nil {
		t.Fatalf("generateRobotsTxt() failed: %v", err)
	}

	// Read generated file
	content, err := os.ReadFile(filepath.Join(tempDir, "robots.txt"))
	if err != nil {
		t.Fatalf("Failed to read generated robots.txt: %v", err)
	}

	contentStr := string(content)

	// Verify content
	tests := []struct {
		name     string
		contains string
	}{
		{"User-agent directive", "User-agent: *"},
		{"Allow directive", "Allow: /"},
		{"Disallow admin", "Disallow: /admin"},
		{"Sitemap URL", "Sitemap: https://example.com/sitemap.xml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(contentStr, tt.contains) {
				t.Errorf("robots.txt missing expected content: %q", tt.contains)
			}
		})
	}
}

func TestGenerateSitemapXML(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()
	webfilesOutput = tempDir
	webfilesDomain = "example.com"

	// Generate sitemap.xml
	err := generateSitemapXML()
	if err != nil {
		t.Fatalf("generateSitemapXML() failed: %v", err)
	}

	// Read generated file
	content, err := os.ReadFile(filepath.Join(tempDir, "sitemap.xml"))
	if err != nil {
		t.Fatalf("Failed to read generated sitemap.xml: %v", err)
	}

	contentStr := string(content)

	// Verify content
	tests := []struct {
		name     string
		contains string
	}{
		{"XML declaration", "<?xml version=\"1.0\" encoding=\"UTF-8\"?>"},
		{"Urlset namespace", "xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\""},
		{"Landing page", "<loc>https://example.com/</loc>"},
		{"API docs", "<loc>https://example.com/api/docs</loc>"},
		{"OpenAPI JSON", "<loc>https://example.com/api/v1/openapi.json</loc>"},
		{"OpenAPI YAML", "<loc>https://example.com/api/v1/openapi.yaml</loc>"},
		{"SEL profile", "<loc>https://example.com/.well-known/sel-profile</loc>"},
		{"Lastmod", "<lastmod>"},
		{"Changefreq", "<changefreq>weekly</changefreq>"},
		{"Priority", "<priority>1.0</priority>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(contentStr, tt.contains) {
				t.Errorf("sitemap.xml missing expected content: %q", tt.contains)
			}
		})
	}

	// Verify no placeholder domains
	if strings.Contains(contentStr, "togather.foundation") {
		t.Error("sitemap.xml should not contain hardcoded togather.foundation domain")
	}
}

func TestRunWebfiles(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()
	webfilesOutput = tempDir
	webfilesDomain = "test.example.com"

	// Run webfiles command
	err := runWebfiles()
	if err != nil {
		t.Fatalf("runWebfiles() failed: %v", err)
	}

	// Verify both files were created
	files := []string{"robots.txt", "sitemap.xml"}
	for _, file := range files {
		path := filepath.Join(tempDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not created: %s", file)
		}
	}
}

func TestRunWebfilesMissingDomain(t *testing.T) {
	// Reset domain to empty
	webfilesDomain = ""

	// Should fail without domain
	err := runWebfiles()
	if err == nil {
		t.Error("runWebfiles() should fail when domain is not set")
	}
}
