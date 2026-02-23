package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/Togather-Foundation/server/internal/scraper"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// scrapeSyncCmd upserts all YAML source configs into the scraper_sources DB table.
var scrapeSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync YAML source configs into the scraper_sources DB table",
	Long: `Read all *.yaml source configs from the sources directory and upsert them
into the scraper_sources database table. Reports counts of created and updated rows.

Examples:
  server scrape sync
  server scrape sync --sources configs/sources`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		dir := scrapeSourceDir

		configs, err := scraper.LoadSourceConfigs(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
		if len(configs) == 0 {
			fmt.Printf("No source configs found in %s\n", dir)
			return nil
		}

		dbURL := getDatabaseURL()
		if dbURL == "" {
			return fmt.Errorf("DATABASE_URL is required for sync")
		}
		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			return fmt.Errorf("connect to DB: %w", err)
		}
		defer pool.Close()

		repo := postgres.NewScraperSourceRepository(pool)

		var created, updated int
		for _, cfg := range configs {
			params, encErr := sourceConfigToUpsertParams(cfg)
			if encErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: skipping %q: %v\n", cfg.Name, encErr)
				continue
			}

			result, upsertErr := repo.Upsert(ctx, params)
			if upsertErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: upsert %q: %v\n", cfg.Name, upsertErr)
				continue
			}
			// Infer created vs updated from the RETURNING timestamps.
			// On INSERT both timestamps are equal; on UPDATE updated_at > created_at.
			if result.UpdatedAt.Equal(result.CreatedAt) {
				created++
			} else {
				updated++
			}
		}

		fmt.Printf("Sync complete: %d created, %d updated (total %d sources)\n",
			created, updated, len(configs))
		return nil
	},
}

// scrapeExportCmd dumps DB scraper_sources rows back to YAML files.
var scrapeExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export scraper_sources DB table to YAML files",
	Long: `Read all rows from the scraper_sources database table and write each one as a
YAML file in the sources directory (one file per source, named <name>.yaml).
Existing files are overwritten.

Examples:
  server scrape export
  server scrape export --sources configs/sources`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		dir := scrapeSourceDir

		dbURL := getDatabaseURL()
		if dbURL == "" {
			return fmt.Errorf("DATABASE_URL is required for export")
		}
		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			return fmt.Errorf("connect to DB: %w", err)
		}
		defer pool.Close()

		repo := postgres.NewScraperSourceRepository(pool)

		sources, err := repo.List(ctx, nil)
		if err != nil {
			return fmt.Errorf("list scraper sources: %w", err)
		}
		if len(sources) == 0 {
			fmt.Println("No sources in DB to export.")
			return nil
		}

		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output dir %s: %w", dir, err)
		}

		for _, src := range sources {
			cfg, decErr := scraper.SourceConfigFromDomain(src)
			if decErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: skipping %q: %v\n", src.Name, decErr)
				continue
			}

			data, marshErr := yaml.Marshal(cfg)
			if marshErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: marshal %q: %v\n", src.Name, marshErr)
				continue
			}

			outPath := filepath.Join(dir, src.Name+".yaml")
			if writeErr := os.WriteFile(outPath, data, 0o644); writeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: write %s: %v\n", outPath, writeErr)
				continue
			}
			fmt.Printf("Exported: %s\n", outPath)
		}

		fmt.Printf("Export complete: %d sources written to %s\n", len(sources), dir)
		return nil
	},
}

// sourceConfigToUpsertParams converts a scraper.SourceConfig (from YAML) to
// the domain UpsertParams. Selectors are JSON-encoded for the JSONB column.
func sourceConfigToUpsertParams(cfg scraper.SourceConfig) (domainScraper.UpsertParams, error) {
	var selectorsJSON []byte
	if cfg.Tier == 1 {
		var encErr error
		selectorsJSON, encErr = json.Marshal(cfg.Selectors)
		if encErr != nil {
			return domainScraper.UpsertParams{}, fmt.Errorf("encode selectors: %w", encErr)
		}
	}
	return domainScraper.UpsertParams{
		Name:       cfg.Name,
		URL:        cfg.URL,
		Tier:       cfg.Tier,
		Schedule:   cfg.Schedule,
		TrustLevel: cfg.TrustLevel,
		License:    cfg.License,
		Enabled:    cfg.Enabled,
		MaxPages:   cfg.MaxPages,
		Selectors:  selectorsJSON,
		Notes:      cfg.Notes,
	}, nil
}
