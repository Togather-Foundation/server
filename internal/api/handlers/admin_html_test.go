package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestHTMLHandler builds an AdminHTMLHandler backed by a minimal in-memory
// template set. The named template must be "consolidate.html" so that
// ServeConsolidate's ExecuteTemplate call resolves correctly.
func newTestHTMLHandler(t *testing.T, tmpl *template.Template) *AdminHTMLHandler {
	t.Helper()
	return &AdminHTMLHandler{
		Templates: tmpl,
		Env:       "test",
		// Logger intentionally nil — handler guards against it.
	}
}

// happyConsolidateTmpl renders Title so tests can assert on it.
var happyConsolidateTmpl = template.Must(
	template.New("consolidate.html").Parse(`<!DOCTYPE html><html><body>{{.Title}} {{.ActivePage}}</body></html>`),
)

// brokenConsolidateTmpl always fails at execute time (references missing sub-template).
var brokenConsolidateTmpl = func() *template.Template {
	tmpl, _ := template.New("consolidate.html").Parse(`{{template "nonexistent" .}}`)
	return tmpl
}()

// TestServeConsolidate_200 verifies the happy path: 200 status, correct
// Content-Type header, and body containing the page title.
func TestServeConsolidate_200(t *testing.T) {
	h := newTestHTMLHandler(t, happyConsolidateTmpl)

	req := httptest.NewRequest(http.MethodGet, "/admin/events/consolidate", nil)
	rec := httptest.NewRecorder()

	h.ServeConsolidate(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, rec.Body.String(), "Consolidate Events")
}

// TestServeConsolidate_TemplateError_500 verifies that a template execution
// failure causes ServeConsolidate to respond with 500 Internal Server Error.
func TestServeConsolidate_TemplateError_500(t *testing.T) {
	h := newTestHTMLHandler(t, brokenConsolidateTmpl)

	req := httptest.NewRequest(http.MethodGet, "/admin/events/consolidate", nil)
	rec := httptest.NewRecorder()

	h.ServeConsolidate(rec, req)

	// http.Error appends a trailing newline; trim before comparing.
	body := strings.TrimSpace(rec.Body.String())
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "Template error", body)
}

// TestServeConsolidate_SetsActivePageEvents verifies that the template data
// map includes ActivePage = "events" by rendering it into the response body.
func TestServeConsolidate_SetsActivePageEvents(t *testing.T) {
	// Template that only renders ActivePage so the assertion is unambiguous.
	tmpl := template.Must(
		template.New("consolidate.html").Parse(`active:{{.ActivePage}}`),
	)
	h := newTestHTMLHandler(t, tmpl)

	req := httptest.NewRequest(http.MethodGet, "/admin/events/consolidate", nil)
	rec := httptest.NewRecorder()

	h.ServeConsolidate(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "active:events")
}
