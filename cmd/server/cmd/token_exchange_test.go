package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenExchangeSuccess(t *testing.T) {
	expectedToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
	expectedExpires := "2026-06-08T20:00:00Z"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method, "should use POST")
		assert.Equal(t, "/api/v1/auth/token", r.URL.Path, "should call correct path")
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ", "should have Bearer auth")
		assert.Equal(t, "application/json", r.Header.Get("Accept"), "should accept JSON")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token":      expectedToken,
			"expires_at": expectedExpires,
		})
	}))
	defer server.Close()

	t.Run("with --key flag", func(t *testing.T) {
		tokenExchangeKey = "test-admin-api-key"
		tokenExchangeServer = server.URL
		tokenExchangeJSON = false
		defer func() {
			tokenExchangeKey = ""
			tokenExchangeServer = ""
		}()

		cmd := tokenExchangeCmd
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		err := runTokenExchange(cmd, nil)
		require.NoError(t, err)
		assert.Equal(t, expectedToken+"\n", buf.String())
	})

	t.Run("with --json flag", func(t *testing.T) {
		tokenExchangeKey = "test-admin-api-key"
		tokenExchangeServer = server.URL
		tokenExchangeJSON = true
		defer func() {
			tokenExchangeKey = ""
			tokenExchangeServer = ""
			tokenExchangeJSON = false
		}()

		cmd := tokenExchangeCmd
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)

		err := runTokenExchange(cmd, nil)
		require.NoError(t, err)

		var result map[string]string
		require.NoError(t, json.NewDecoder(buf).Decode(&result))
		assert.Equal(t, expectedToken, result["token"])
		assert.Equal(t, expectedExpires, result["expires_at"])
	})

	t.Run("env var fallback", func(t *testing.T) {
		t.Setenv("TOGATHER_ADMIN_API_KEY", "env-admin-api-key")

		tokenExchangeKey = ""
		tokenExchangeServer = server.URL
		tokenExchangeJSON = false
		defer func() {
			tokenExchangeServer = ""
		}()

		cmd := tokenExchangeCmd
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)

		err := runTokenExchange(cmd, nil)
		require.NoError(t, err)
		assert.Equal(t, expectedToken+"\n", buf.String())
	})

	t.Run("TOGATHER_BASE_URL env", func(t *testing.T) {
		t.Setenv("TOGATHER_BASE_URL", server.URL)

		tokenExchangeKey = "test-admin-api-key"
		tokenExchangeServer = ""
		tokenExchangeJSON = false
		defer func() {
			tokenExchangeKey = ""
		}()

		cmd := tokenExchangeCmd
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)

		err := runTokenExchange(cmd, nil)
		require.NoError(t, err)
		assert.Equal(t, expectedToken+"\n", buf.String())
	})

	t.Run("default server URL", func(t *testing.T) {
		tokenExchangeKey = "test-admin-api-key"
		tokenExchangeServer = ""
		tokenExchangeJSON = false
		defer func() {
			tokenExchangeKey = ""
		}()

		cmd := tokenExchangeCmd
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)

		err := runTokenExchange(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "token exchange failed")
	})
}

func TestTokenExchangeErrorHandling(t *testing.T) {
	t.Run("missing key", func(t *testing.T) {
		tokenExchangeKey = ""
		tokenExchangeServer = ""
		defer func() {
			tokenExchangeKey = ""
		}()

		err := runTokenExchange(tokenExchangeCmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--key is required")
	})

	t.Run("non-200 response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"title": "Admin API key required"}`))
		}))
		defer server.Close()

		tokenExchangeKey = "test-admin-api-key"
		tokenExchangeServer = server.URL
		defer func() {
			tokenExchangeKey = ""
			tokenExchangeServer = ""
		}()

		err := runTokenExchange(tokenExchangeCmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "token exchange failed with status 403")
		assert.Contains(t, err.Error(), "Admin API key required")
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`not json`))
		}))
		defer server.Close()

		tokenExchangeKey = "test-admin-api-key"
		tokenExchangeServer = server.URL
		defer func() {
			tokenExchangeKey = ""
			tokenExchangeServer = ""
		}()

		err := runTokenExchange(tokenExchangeCmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response")
	})
}
