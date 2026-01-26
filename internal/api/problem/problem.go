package problem

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/rs/zerolog"
)

const contentType = "application/problem+json"

type ProblemDetails struct {
	Type     string                 `json:"type"`
	Title    string                 `json:"title"`
	Status   int                    `json:"status"`
	Detail   string                 `json:"detail,omitempty"`
	Instance string                 `json:"instance,omitempty"`
	Errors   map[string]interface{} `json:"errors,omitempty"`
}

type Option func(*ProblemDetails)

func WithDetail(detail string) Option {
	return func(p *ProblemDetails) {
		p.Detail = detail
	}
}

func WithInstance(instance string) Option {
	return func(p *ProblemDetails) {
		p.Instance = instance
	}
}

func WithErrors(errs map[string]interface{}) Option {
	return func(p *ProblemDetails) {
		p.Errors = errs
	}
}

func Write(w http.ResponseWriter, r *http.Request, status int, typ, title string, err error, env string, opts ...Option) {
	problem := ProblemDetails{
		Type:   typ,
		Title:  title,
		Status: status,
	}

	for _, opt := range opts {
		opt(&problem)
	}

	if problem.Detail == "" && err != nil {
		if env == "development" || env == "test" {
			problem.Detail = err.Error()
		} else {
			problem.Detail = http.StatusText(status)
		}
	}

	if problem.Instance == "" && r != nil {
		problem.Instance = r.URL.Path
	}

	// Log error with structured logging from context
	if err != nil && status >= 500 {
		// Log server errors (5xx) at error level
		logger := zerolog.Ctx(r.Context())
		logger.Error().
			Err(err).
			Int("status", status).
			Str("type", typ).
			Str("path", r.URL.Path).
			Str("method", r.Method).
			Msg(title)
	} else if err != nil && status >= 400 {
		// Log client errors (4xx) at warn level
		logger := zerolog.Ctx(r.Context())
		logger.Warn().
			Err(err).
			Int("status", status).
			Str("type", typ).
			Str("path", r.URL.Path).
			Str("method", r.Method).
			Msg(title)
	}

	WriteProblem(w, problem)
}

func WriteProblem(w http.ResponseWriter, problem ProblemDetails) {
	payload, err := json.Marshal(problem)
	if err != nil {
		fallback := fmt.Sprintf("{\"type\":\"about:blank\",\"title\":\"%s\",\"status\":500}", http.StatusText(http.StatusInternalServerError))
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fallback))
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(problem.Status)
	_, _ = w.Write(payload)
}

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
)
