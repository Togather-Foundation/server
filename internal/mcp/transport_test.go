package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/Togather-Foundation/server/internal/geocoding"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

const legacySessionID = "mcp-session-00000000-0000-4000-8000-000000000000"

func newTransportTestServer() *server.MCPServer {
	return NewServer(Config{Name: "transport-test", Version: "1.0.0"},
		nil, nil, nil, nil, &developers.Service{}, &geocoding.GeocodingService{}, time.UTC, "http://example.test", zerolog.Nop()).MCPServer()
}

func postTransportRequest(handler http.Handler, method, params, sessionID, apiKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":`+params+`}`))
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

// The legacy transport accepted any well-formed ID without storing a session.
// Preserve its tool payload while also accepting missing and arbitrary IDs.
func TestStreamableHTTPHandlerStatelessTools(t *testing.T) {
	srv := newTransportTestServer()
	legacy := server.NewStreamableHTTPServer(srv)
	want := postTransportRequest(legacy, "tools/list", `{}`, legacySessionID, "")
	require.Equal(t, http.StatusOK, want.Code, want.Body.String())
	var result struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(want.Body.Bytes(), &result))
	require.Len(t, result.Result.Tools, 6)

	handler := NewStreamableHTTPHandler(srv)
	for _, sessionID := range []string{"", "not-a-session", legacySessionID} {
		t.Run("session="+sessionID, func(t *testing.T) {
			got := postTransportRequest(handler, "tools/list", `{}`, sessionID, "")
			require.Equal(t, http.StatusOK, got.Code, got.Body.String())
			require.JSONEq(t, want.Body.String(), got.Body.String())
		})
	}
}

func TestStreamableHTTPHandlerInitialize(t *testing.T) {
	params := `{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}`
	srv := newTransportTestServer()
	want := postTransportRequest(server.NewStreamableHTTPServer(srv), "initialize", params, "", "")
	require.Equal(t, http.StatusOK, want.Code, want.Body.String())
	got := postTransportRequest(NewStreamableHTTPHandler(srv), "initialize", params, "", "")
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	require.JSONEq(t, want.Body.String(), got.Body.String())
	require.Empty(t, got.Header().Get("Mcp-Session-Id"), "stateless initialization must not issue a session")
}

type transportTestKeyStore struct{ key *auth.APIKey }

func (s transportTestKeyStore) LookupByPrefix(context.Context, string) (*auth.APIKey, error) {
	return s.key, nil
}

func (transportTestKeyStore) UpdateLastUsed(context.Context, string) error { return nil }

func TestStreamableHTTPHandlerStatelessUsageTracking(t *testing.T) {
	const apiKey = "transport-test-valid-api-key"
	store := transportTestKeyStore{key: &auth.APIKey{
		ID: uuid.NewString(), Hash: auth.HashAPIKeySHA256(apiKey),
		HashVersion: auth.HashVersionSHA256, IsActive: true,
	}}
	// Assert directly on the recorder's buffer; no database or flush loop is needed.
	recorder := developers.NewUsageRecorder(nil, zerolog.Nop(), config.DeveloperConfig{})
	srv := newTransportTestServer()
	tracked := middleware.UsageTracking(recorder, zerolog.Nop(), nil)(NewStreamableHTTPHandler(srv))
	handler, err := WrapHandler(tracked, store, config.RateLimitConfig{})
	require.NoError(t, err)
	legacy, err := WrapHandler(server.NewStreamableHTTPServer(srv), store, config.RateLimitConfig{})
	require.NoError(t, err)
	want := postTransportRequest(legacy, "tools/list", `{}`, legacySessionID, apiKey)
	require.Equal(t, http.StatusOK, want.Code, want.Body.String())
	var result struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(want.Body.Bytes(), &result))
	require.Len(t, result.Result.Tools, 9)
	for i, sessionID := range []string{"", "not-a-session", legacySessionID} {
		got := postTransportRequest(handler, "tools/list", `{}`, sessionID, apiKey)
		require.Equal(t, http.StatusOK, got.Code, got.Body.String())
		require.JSONEq(t, want.Body.String(), got.Body.String())
		_, requests, errors := recorder.Stats()
		require.Equal(t, int64(i+1), requests, "each authenticated MCP request must reach the usage recorder")
		require.Zero(t, errors)
	}
}
