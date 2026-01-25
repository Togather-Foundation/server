package postgres

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const DefaultMigrationsPath = "internal/storage/postgres/migrations"

func MigrateUp(databaseURL string, migrationsPath string) error {
	m, err := newMigrator(databaseURL, migrationsPath)
	if err != nil {
		return err
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil {
			_ = sourceErr
		}
		if dbErr != nil {
			_ = dbErr
		}
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func MigrateDown(databaseURL string, migrationsPath string, steps int) error {
	if steps <= 0 {
		return fmt.Errorf("migrate down: steps must be > 0")
	}
	m, err := newMigrator(databaseURL, migrationsPath)
	if err != nil {
		return err
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil {
			_ = sourceErr
		}
		if dbErr != nil {
			_ = dbErr
		}
	}()

	if err := m.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate down: %w", err)
	}
	return nil
}

func newMigrator(databaseURL string, migrationsPath string) (*migrate.Migrate, error) {
	if migrationsPath == "" {
		migrationsPath = DefaultMigrationsPath
	}
	m, err := migrate.New("file://"+migrationsPath, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("init migrator: %w", err)
	}
	return m, nil
}
