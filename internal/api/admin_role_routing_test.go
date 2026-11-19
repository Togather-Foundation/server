package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/api/handlers"
	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/sse"
	"github.com/Togather-Foundation/server/tests/testhelpers"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// TestAdminRoleRouting exercises the production route registrations without a
// database. These HTML shells and the SSE handshake must authorize before any
// data access; the pool refuses connections to keep the test fully offline.
func TestAdminRoleRouting(t *testing.T) {
	t.Setenv("SEL_API_KEY", "")
	t.Setenv("SEL_INGEST_KEY", "")
	poolCfg, err := pgxpool.ParseConfig("postgres://test:test@localhost/test?sslmode=disable")
	require.NoError(t, err)
	poolCfg.BeforeConnect = func(context.Context, *pgx.ConnConfig) error {
		return errors.New("database access is disabled in authorization route tests")
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cfg := testhelpers.TestConfig("")
	router := NewRouter(cfg, zerolog.Nop(), pool, "test", "test", "test", ctx)
	require.NotNil(t, router.RiverClient, "router initialization must succeed")
	key, err := auth.DeriveAdminJWTKey([]byte(cfg.Auth.JWTSecret))
	require.NoError(t, err)
	manager := auth.NewJWTManagerFromKey(key, cfg.Auth.JWTExpiry, "sel.events")

	for _, role := range []string{"viewer", "editor", "admin"} {
		t.Run(role, func(t *testing.T) {
			token, err := manager.Generate("test-user", role)
			require.NoError(t, err)
			for _, path := range []string{
				"/admin", "/admin/", "/admin/dashboard", "/admin/events",
				"/admin/events/consolidate", "/admin/events/test-id", "/admin/duplicates",
				"/admin/review-queue", "/admin/api-keys", "/admin/federation", "/admin/users",
				"/admin/users/test-id/activity", "/admin/places", "/admin/organizations",
				"/admin/developers", "/admin/scraper", "/api/v1/admin/scraper/events",
			} {
				t.Run(path, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodGet, path, nil)
					req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
					// A canceled request lets an authorized SSE handshake return
					// immediately after writing its initial frame.
					requestCtx, requestCancel := context.WithCancel(req.Context())
					requestCancel()
					req = req.WithContext(requestCtx)
					w := httptest.NewRecorder()
					router.Handler.ServeHTTP(w, req)
					if role != "admin" {
						require.Equal(t, http.StatusForbidden, w.Code)
						require.Empty(t, w.Header().Get("Location"), "do not redirect non-admins into a login loop")
						require.NotContains(t, w.Body.String(), "<!DOCTYPE html>")
						return
					}
					switch path {
					case "/api/v1/admin/scraper/events":
						// OpenAPILink currently drops http.Flusher (t_f55d5a51). Its existing
						// streaming error proves the admin reached the SSE handler;
						// non-admins must be rejected before that handler runs.
						require.Equal(t, http.StatusInternalServerError, w.Code)
						require.Contains(t, w.Body.String(), "Streaming not supported")
					case "/admin", "/admin/":
						require.Equal(t, http.StatusFound, w.Code)
						require.Equal(t, "/admin/dashboard", w.Header().Get("Location"))
					default:
						require.Equal(t, http.StatusOK, w.Code, "response: %s", w.Body.String())
					}
				})
			}

			t.Run("bearer", func(t *testing.T) {
				// An invalid ID reaches handler validation without database access.
				req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/not-a-uuid", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				router.Handler.ServeHTTP(w, req)
				if role == "admin" {
					require.Equal(t, http.StatusBadRequest, w.Code)
				} else {
					require.Equal(t, http.StatusForbidden, w.Code)
					require.True(t, strings.HasPrefix(w.Header().Get("Content-Type"), "application/problem+json"))
				}
			})

			t.Run("cookie-authenticated SSE handshake", func(t *testing.T) {
				handler := handlers.NewAdminEventsSSEHandler(sse.NewBroker(), cfg.Environment, zerolog.Nop())
				protected := middleware.AdminAuthCookie(manager, "")(handler)
				req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scraper/events", nil)
				req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
				requestCtx, requestCancel := context.WithCancel(req.Context())
				requestCancel()
				w := httptest.NewRecorder()
				protected.ServeHTTP(w, req.WithContext(requestCtx))
				if role == "admin" {
					require.Equal(t, http.StatusOK, w.Code)
					require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
					require.Contains(t, w.Body.String(), "retry: 5000")
				} else {
					require.Equal(t, http.StatusForbidden, w.Code)
					require.Empty(t, w.Header().Get("Location"))
					require.NotContains(t, w.Body.String(), "retry: 5000")
				}
			})
		})
	}
}
