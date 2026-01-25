package problem

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWrite_DevIncludesDetail(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/resource", nil)
	res := httptest.NewRecorder()

	Write(res, req, http.StatusBadRequest, "https://example.com/problem", "bad request", errors.New("boom"), "development")

	if got := res.Result().Header.Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("expected content type problem+json, got %s", got)
	}

	var body ProblemDetails
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Detail != "boom" {
		t.Fatalf("expected detail boom, got %s", body.Detail)
	}
	if body.Instance != "/api/v1/resource" {
		t.Fatalf("expected instance /api/v1/resource, got %s", body.Instance)
	}
}

func TestWrite_ProdSanitizesDetail(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/resource", nil)
	res := httptest.NewRecorder()

	Write(res, req, http.StatusBadRequest, "https://example.com/problem", "bad request", errors.New("boom"), "production")

	var body ProblemDetails
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Detail != http.StatusText(http.StatusBadRequest) {
		t.Fatalf("expected sanitized detail, got %s", body.Detail)
	}
}
