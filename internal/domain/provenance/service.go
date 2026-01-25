package provenance

import (
	"context"
	"fmt"
	"strings"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetOrCreateSource(ctx context.Context, params CreateSourceParams) (*Source, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("provenance service not configured")
	}

	baseURL := strings.TrimSpace(params.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("source base url required")
	}

	if existing, err := s.repo.GetByBaseURL(ctx, baseURL); err == nil && existing != nil {
		return existing, nil
	}

	params.BaseURL = baseURL
	return s.repo.Create(ctx, params)
}
