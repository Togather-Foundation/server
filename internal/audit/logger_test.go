package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestLogger_Log(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerWithZerolog(zerolog.New(&buf))

	entry := Entry{
		Action:       "admin.event.update",
		AdminUser:    "testuser",
		ResourceType: "event",
		ResourceID:   "01HX12ABC123",
		IPAddress:    "192.168.1.1",
		Status:       "success",
	}

	logger.Log(entry)

	// Parse the logged JSON
	output := buf.String()
	// Extract JSON from output
	jsonStart := strings.Index(output, "{")
	if jsonStart == -1 {
		t.Fatal("No JSON found in output")
	}
	jsonStr := output[jsonStart:]
	jsonStr = strings.TrimSpace(jsonStr)

	// Parse the zerolog wrapper first
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
		t.Fatalf("Failed to parse logged JSON: %v\nOutput: %s", err, output)
	}

	// Extract the nested "audit" field
	auditData, ok := wrapper["audit"]
	if !ok {
		t.Fatal("No 'audit' field found in logged JSON")
	}

	var logged Entry
	if err := json.Unmarshal(auditData, &logged); err != nil {
		t.Fatalf("Failed to parse audit entry: %v\nOutput: %s", err, output)
	}

	// Verify fields
	if logged.Action != entry.Action {
		t.Errorf("Action mismatch: got %s, want %s", logged.Action, entry.Action)
	}
	if logged.AdminUser != entry.AdminUser {
		t.Errorf("AdminUser mismatch: got %s, want %s", logged.AdminUser, entry.AdminUser)
	}
	if logged.ResourceType != entry.ResourceType {
		t.Errorf("ResourceType mismatch: got %s, want %s", logged.ResourceType, entry.ResourceType)
	}
	if logged.ResourceID != entry.ResourceID {
		t.Errorf("ResourceID mismatch: got %s, want %s", logged.ResourceID, entry.ResourceID)
	}
	if logged.IPAddress != entry.IPAddress {
		t.Errorf("IPAddress mismatch: got %s, want %s", logged.IPAddress, entry.IPAddress)
	}
	if logged.Status != entry.Status {
		t.Errorf("Status mismatch: got %s, want %s", logged.Status, entry.Status)
	}
	if logged.Timestamp.IsZero() {
		t.Error("Timestamp should be set automatically")
	}
}

func TestLogger_LogSuccess(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerWithZerolog(zerolog.New(&buf))

	logger.LogSuccess("admin.event.delete", "alice", "event", "01HX12ABC123", "10.0.0.1", map[string]string{
		"reason": "duplicate",
	})

	output := buf.String()
	if !strings.Contains(output, "admin.event.delete") {
		t.Error("Should contain action")
	}
	if !strings.Contains(output, "alice") {
		t.Error("Should contain admin user")
	}
	if !strings.Contains(output, "success") {
		t.Error("Should contain success status")
	}
	if !strings.Contains(output, "duplicate") {
		t.Error("Should contain details")
	}
}

func TestLogger_LogFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerWithZerolog(zerolog.New(&buf))

	logger.LogFailure("admin.login", "bob", "192.168.1.1", map[string]string{
		"reason": "invalid_password",
	})

	output := buf.String()
	if !strings.Contains(output, "admin.login") {
		t.Error("Should contain action")
	}
	if !strings.Contains(output, "bob") {
		t.Error("Should contain admin user")
	}
	if !strings.Contains(output, "failure") {
		t.Error("Should contain failure status")
	}
	if !strings.Contains(output, "invalid_password") {
		t.Error("Should contain reason")
	}
}

func TestExtractClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.1")

	ip := extractClientIP(req)
	if ip != "203.0.113.1, 198.51.100.1" {
		t.Errorf("Expected X-Forwarded-For value, got %s", ip)
	}
}

func TestExtractClientIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Real-IP", "203.0.113.5")

	ip := extractClientIP(req)
	if ip != "203.0.113.5" {
		t.Errorf("Expected X-Real-IP value, got %s", ip)
	}
}

func TestExtractClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"

	ip := extractClientIP(req)
	if ip != "192.168.1.100:12345" {
		t.Errorf("Expected RemoteAddr value, got %s", ip)
	}
}

func TestExtractClientIP_PreferXForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	req.Header.Set("X-Real-IP", "203.0.113.2")
	req.RemoteAddr = "192.168.1.1:12345"

	ip := extractClientIP(req)
	if ip != "203.0.113.1" {
		t.Errorf("Should prefer X-Forwarded-For, got %s", ip)
	}
}

func TestLogFromRequest_ExtractsUsername(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerWithZerolog(zerolog.New(&buf))

	req := httptest.NewRequest(http.MethodPost, "/admin/events", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	// Add claims to context
	claims := map[string]interface{}{
		"username": "charlie",
		"role":     "admin",
	}
	ctx := context.WithValue(req.Context(), ClaimsKey, claims)
	req = req.WithContext(ctx)

	logger.LogFromRequest(req, "admin.event.create", "event", "01HX12NEW123", "success", nil)

	output := buf.String()
	if !strings.Contains(output, "charlie") {
		t.Error("Should extract username from context")
	}
	if !strings.Contains(output, "admin.event.create") {
		t.Error("Should contain action")
	}
	if !strings.Contains(output, "10.0.0.1:12345") {
		t.Error("Should contain IP address")
	}
}

func TestLogFromRequest_NoClaimsInContext(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerWithZerolog(zerolog.New(&buf))

	req := httptest.NewRequest(http.MethodPost, "/admin/events", nil)
	logger.LogFromRequest(req, "admin.event.create", "event", "01HX12NEW123", "success", nil)

	output := buf.String()
	if !strings.Contains(output, "unknown") {
		t.Error("Should use 'unknown' when no claims in context")
	}
}

func TestEntry_JSONSerialization(t *testing.T) {
	now := time.Now().UTC()
	entry := Entry{
		Timestamp:    now,
		Action:       "admin.event.merge",
		AdminUser:    "admin1",
		ResourceType: "event",
		ResourceID:   "01HX12MERGE1",
		IPAddress:    "192.168.1.1",
		Status:       "success",
		Details: map[string]string{
			"duplicate_id": "01HX12DUP123",
		},
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal entry: %v", err)
	}

	var decoded Entry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal entry: %v", err)
	}

	// Verify all fields
	if decoded.Action != entry.Action {
		t.Errorf("Action mismatch after round-trip")
	}
	if decoded.AdminUser != entry.AdminUser {
		t.Errorf("AdminUser mismatch after round-trip")
	}
	if decoded.Details["duplicate_id"] != "01HX12DUP123" {
		t.Error("Details should be preserved")
	}
}

func TestWithLogger_AndFromContext(t *testing.T) {
	logger := NewLogger()
	ctx := WithLogger(context.Background(), logger)

	retrieved := FromContext(ctx)
	if retrieved == nil {
		t.Fatal("Should retrieve logger from context")
	}

	// Both should be the same instance
	if retrieved != logger {
		t.Error("Retrieved logger should be the same instance")
	}
}

func TestFromContext_NoLogger(t *testing.T) {
	ctx := context.Background()
	logger := FromContext(ctx)

	if logger == nil {
		t.Fatal("Should return a default logger when not found in context")
	}
}

func TestLogger_AutoTimestamp(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerWithZerolog(zerolog.New(&buf))

	entry := Entry{
		Action:    "admin.test",
		AdminUser: "test",
		IPAddress: "127.0.0.1",
		Status:    "success",
		// Timestamp not set
	}

	logger.Log(entry)

	output := buf.String()
	jsonStart := strings.Index(output, "{")
	jsonStr := output[jsonStart:]
	jsonStr = strings.TrimSpace(jsonStr)

	// Parse the zerolog wrapper first
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
		t.Fatalf("Failed to parse logged JSON: %v", err)
	}

	// Extract the nested "audit" field
	auditData, ok := wrapper["audit"]
	if !ok {
		t.Fatal("No 'audit' field found in logged JSON")
	}

	var logged Entry
	if err := json.Unmarshal(auditData, &logged); err != nil {
		t.Fatalf("Failed to parse audit entry: %v", err)
	}

	if logged.Timestamp.IsZero() {
		t.Error("Timestamp should be set automatically when not provided")
	}

	// Verify timestamp is recent (within last second)
	if time.Since(logged.Timestamp) > time.Second {
		t.Error("Auto-generated timestamp should be recent")
	}
}

// Benchmark audit logging performance
func BenchmarkLogger_Log(b *testing.B) {
	var buf bytes.Buffer
	logger := NewLoggerWithZerolog(zerolog.New(&buf))

	entry := Entry{
		Action:       "admin.event.update",
		AdminUser:    "benchuser",
		ResourceType: "event",
		ResourceID:   "01HX12BENCH1",
		IPAddress:    "192.168.1.1",
		Status:       "success",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Log(entry)
	}
}

func BenchmarkLogger_LogFromRequest(b *testing.B) {
	var buf bytes.Buffer
	logger := NewLoggerWithZerolog(zerolog.New(&buf))

	req := httptest.NewRequest(http.MethodPost, "/admin/events", nil)
	claims := map[string]interface{}{
		"username": "benchuser",
	}
	ctx := context.WithValue(req.Context(), ClaimsKey, claims)
	req = req.WithContext(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.LogFromRequest(req, "admin.event.update", "event", "01HX12BENCH1", "success", nil)
	}
}
