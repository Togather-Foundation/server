package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

var (
	webfilesDomain string
	webfilesOutput string
)

// webfilesCmd generates robots.txt and sitemap.xml
var webfilesCmd = &cobra.Command{
	Use:   "webfiles",
	Short: "Generate robots.txt and sitemap.xml for deployment",
	Long: `Generate robots.txt and sitemap.xml with environment-specific domain names.

This command is designed to run during deployment to ensure URLs match the
target environment (staging vs production). The generated files are embedded
into the server binary at build time.

Examples:
  # Generate for production
  server webfiles --domain togather.foundation

  # Generate for staging
  server webfiles --domain staging.toronto.togather.foundation

  # Custom output directory
  server webfiles --domain example.com --output ./build/web`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWebfiles()
	},
}

func init() {
	rootCmd.AddCommand(webfilesCmd)

	webfilesCmd.Flags().StringVar(&webfilesDomain, "domain", "", "domain name for URLs (required)")
	webfilesCmd.Flags().StringVar(&webfilesOutput, "output", "./web", "output directory for generated files")
	_ = webfilesCmd.MarkFlagRequired("domain")
}

func runWebfiles() error {
	if webfilesDomain == "" {
		return fmt.Errorf("--domain is required")
	}

	// Ensure output directory exists
	if err := os.MkdirAll(webfilesOutput, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Generate robots.txt
	if err := generateRobotsTxt(); err != nil {
		return fmt.Errorf("generate robots.txt: %w", err)
	}

	// Generate sitemap.xml
	if err := generateSitemapXML(); err != nil {
		return fmt.Errorf("generate sitemap.xml: %w", err)
	}

	fmt.Printf("✓ Generated web files for domain: %s\n", webfilesDomain)
	fmt.Printf("  robots.txt: %s\n", filepath.Join(webfilesOutput, "robots.txt"))
	fmt.Printf("  sitemap.xml: %s\n", filepath.Join(webfilesOutput, "sitemap.xml"))
	fmt.Println()
	fmt.Println("These files are embedded into the server binary at build time.")
	fmt.Println("Rebuild the server to include the updated files:")
	fmt.Println("  make build")

	return nil
}

func generateRobotsTxt() error {
	content := fmt.Sprintf(`# robots.txt for Shared Events Library (Toronto Togather)
# Allow all bots to crawl public content

User-agent: *
Allow: /
Crawl-delay: 1

# Disallow admin routes
Disallow: /admin
Disallow: /api/v1/admin/

# Sitemap
Sitemap: https://%s/sitemap.xml
`, webfilesDomain)

	outputPath := filepath.Join(webfilesOutput, "robots.txt")
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

func generateSitemapXML() error {
	lastmod := time.Now().UTC().Format("2006-01-02")

	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
    <!-- Landing page -->
    <url>
        <loc>https://%s/</loc>
        <lastmod>%s</lastmod>
        <changefreq>weekly</changefreq>
        <priority>1.0</priority>
    </url>
    
    <!-- API documentation -->
    <url>
        <loc>https://%s/api/docs</loc>
        <lastmod>%s</lastmod>
        <changefreq>weekly</changefreq>
        <priority>0.8</priority>
    </url>
    
    <!-- OpenAPI specifications -->
    <url>
        <loc>https://%s/api/v1/openapi.json</loc>
        <lastmod>%s</lastmod>
        <changefreq>monthly</changefreq>
        <priority>0.7</priority>
    </url>
    
    <url>
        <loc>https://%s/api/v1/openapi.yaml</loc>
        <lastmod>%s</lastmod>
        <changefreq>monthly</changefreq>
        <priority>0.7</priority>
    </url>
    
    <!-- Well-known endpoints -->
    <url>
        <loc>https://%s/.well-known/sel-profile</loc>
        <lastmod>%s</lastmod>
        <changefreq>monthly</changefreq>
        <priority>0.6</priority>
    </url>
    
    <!-- Note: Dynamic event/place/org URLs will be added in future iterations -->
</urlset>
`, webfilesDomain, lastmod,
		webfilesDomain, lastmod,
		webfilesDomain, lastmod,
		webfilesDomain, lastmod,
		webfilesDomain, lastmod)

	outputPath := filepath.Join(webfilesOutput, "sitemap.xml")
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
