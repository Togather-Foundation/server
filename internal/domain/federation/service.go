package federation

import (
	"context"
	"errors"
	"strings"

	"github.com/Togather-Foundation/server/internal/validation"
	"github.com/google/uuid"
)

var (
	ErrNodeNotFound      = errors.New("federation node not found")
	ErrDuplicateDomain   = errors.New("node domain already exists")
	ErrInvalidNodeParams = errors.New("invalid node parameters")
)

// Service provides business logic for federation node management
type Service struct {
	repo         Repository
	requireHTTPS bool
}

func NewService(repo Repository, requireHTTPS bool) *Service {
	return &Service{
		repo:         repo,
		requireHTTPS: requireHTTPS,
	}
}

// CreateNode creates a new federation node with validation
func (s *Service) CreateNode(ctx context.Context, params CreateNodeParams) (*Node, error) {
	// Validate required fields
	if err := s.validateCreateParams(params); err != nil {
		return nil, err
	}

	// Normalize domain
	params.NodeDomain = strings.ToLower(strings.TrimSpace(params.NodeDomain))

	// Check if domain already exists
	existing, err := s.repo.GetByDomain(ctx, params.NodeDomain)
	if err == nil && existing != nil {
		return nil, ErrDuplicateDomain
	}

	return s.repo.Create(ctx, params)
}

// GetNode retrieves a federation node by ID
func (s *Service) GetNode(ctx context.Context, id uuid.UUID) (*Node, error) {
	return s.repo.GetByID(ctx, id)
}

// GetNodeByDomain retrieves a federation node by domain
func (s *Service) GetNodeByDomain(ctx context.Context, domain string) (*Node, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	return s.repo.GetByDomain(ctx, domain)
}

// ListNodes returns a filtered list of federation nodes
func (s *Service) ListNodes(ctx context.Context, filters ListNodesFilters) ([]*Node, error) {
	if filters.Limit <= 0 {
		filters.Limit = 50
	}
	if filters.Limit > 100 {
		filters.Limit = 100
	}

	return s.repo.List(ctx, filters)
}

// UpdateNode updates a federation node
func (s *Service) UpdateNode(ctx context.Context, id uuid.UUID, params UpdateNodeParams) (*Node, error) {
	// Validate update parameters
	if err := s.validateUpdateParams(params); err != nil {
		return nil, err
	}

	// Check if node exists
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.repo.Update(ctx, id, params)
}

// DeleteNode deletes a federation node
func (s *Service) DeleteNode(ctx context.Context, id uuid.UUID) error {
	// Check if node exists
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	return s.repo.Delete(ctx, id)
}

// validateCreateParams validates node creation parameters
func (s *Service) validateCreateParams(params CreateNodeParams) error {
	if params.NodeDomain == "" {
		return ErrInvalidNodeParams
	}
	if params.NodeName == "" {
		return ErrInvalidNodeParams
	}
	if params.BaseURL == "" {
		return ErrInvalidNodeParams
	}

	// Validate BaseURL format
	if err := validation.ValidateBaseURL(params.BaseURL, "base_url", s.requireHTTPS); err != nil {
		return err
	}

	if params.TrustLevel < 1 || params.TrustLevel > 10 {
		return ErrInvalidNodeParams
	}

	// Validate federation status
	validStatuses := map[string]bool{
		"pending": true,
		"active":  true,
		"paused":  true,
		"blocked": true,
	}
	if !validStatuses[params.FederationStatus] {
		return ErrInvalidNodeParams
	}

	// Validate sync direction
	validDirections := map[string]bool{
		"bidirectional": true,
		"pull_only":     true,
		"push_only":     true,
		"disabled":      true,
	}
	if !validDirections[params.SyncDirection] {
		return ErrInvalidNodeParams
	}

	return nil
}

// validateUpdateParams validates node update parameters
func (s *Service) validateUpdateParams(params UpdateNodeParams) error {
	if params.TrustLevel != nil && (*params.TrustLevel < 1 || *params.TrustLevel > 10) {
		return ErrInvalidNodeParams
	}

	// Validate BaseURL if being updated
	if params.BaseURL != nil && *params.BaseURL != "" {
		if err := validation.ValidateBaseURL(*params.BaseURL, "base_url", s.requireHTTPS); err != nil {
			return err
		}
	}

	if params.FederationStatus != nil {
		validStatuses := map[string]bool{
			"pending": true,
			"active":  true,
			"paused":  true,
			"blocked": true,
		}
		if !validStatuses[*params.FederationStatus] {
			return ErrInvalidNodeParams
		}
	}

	if params.SyncDirection != nil {
		validDirections := map[string]bool{
			"bidirectional": true,
			"pull_only":     true,
			"push_only":     true,
			"disabled":      true,
		}
		if !validDirections[*params.SyncDirection] {
			return ErrInvalidNodeParams
		}
	}

	return nil
}
