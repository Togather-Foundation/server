package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Togather-Foundation/server/internal/api"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logger := config.NewLogger(cfg.Logging)
	logger.Info().Msg("starting SEL server")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := bootstrapAdminUser(ctx, cfg, logger); err != nil {
		logger.Error().Err(err).Msg("admin bootstrap failed")
	}
	cancel()

	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           api.NewRouter(cfg, logger),
		ReadTimeout:       10 * time.Second, // Total time to read request
		WriteTimeout:      30 * time.Second, // Total time to write response
		ReadHeaderTimeout: 5 * time.Second,  // Time to read headers
		MaxHeaderBytes:    1 << 20,          // 1 MB max header size
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("http server error")
		}
	}()

	shutdown(server, logger)
}

func bootstrapAdminUser(ctx context.Context, cfg config.Config, logger zerolog.Logger) error {
	bootstrap := cfg.AdminBootstrap
	if bootstrap.Username == "" || bootstrap.Password == "" || bootstrap.Email == "" {
		logger.Warn().Msg("admin bootstrap env vars not fully set; skipping")
		return nil
	}

	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	const checkQuery = `SELECT id FROM users WHERE email = $1 OR username = $2 LIMIT 1`
	row := pool.QueryRow(ctx, checkQuery, bootstrap.Email, bootstrap.Username)
	var existingID string
	if err := row.Scan(&existingID); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check admin user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(bootstrap.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	const insertQuery = `
INSERT INTO users (id, username, email, password_hash, role, is_active, created_at)
VALUES (gen_random_uuid(), $1, $2, $3, 'admin', true, now())`
	if _, err := pool.Exec(ctx, insertQuery, bootstrap.Username, bootstrap.Email, string(hash)); err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	logger.Info().Str("email", bootstrap.Email).Msg("bootstrapped admin user")
	return nil
}

func shutdown(server *http.Server, logger zerolog.Logger) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	<-stop
	logger.Info().Msg("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("shutdown error")
	}
}
