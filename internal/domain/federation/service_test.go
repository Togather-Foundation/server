package federation

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// TestValidateCreateParams tests validation logic for node creation
func TestValidateCreateParams(t *testing.T) {
	tests := []struct {
		name         string
		params       CreateNodeParams
		requireHTTPS bool
		expectError  bool
		errorType    error
	}{
		{
			name: "valid params with HTTP (dev mode)",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				NodeName:         "Example SEL",
				BaseURL:          "http://example.org",
				TrustLevel:       5,
				FederationStatus: "active",
				SyncDirection:    "bidirectional",
			},
			requireHTTPS: false,
			expectError:  false,
		},
		{
			name: "valid params with HTTPS (production mode)",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				NodeName:         "Example SEL",
				BaseURL:          "https://example.org",
				TrustLevel:       5,
				FederationStatus: "active",
				SyncDirection:    "bidirectional",
			},
			requireHTTPS: true,
			expectError:  false,
		},
		{
			name: "missing node domain",
			params: CreateNodeParams{
				NodeName:         "Example SEL",
				BaseURL:          "https://example.org",
				TrustLevel:       5,
				FederationStatus: "active",
				SyncDirection:    "bidirectional",
			},
			requireHTTPS: false,
			expectError:  true,
			errorType:    ErrInvalidNodeParams,
		},
		{
			name: "missing node name",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				BaseURL:          "https://example.org",
				TrustLevel:       5,
				FederationStatus: "active",
				SyncDirection:    "bidirectional",
			},
			requireHTTPS: false,
			expectError:  true,
			errorType:    ErrInvalidNodeParams,
		},
		{
			name: "missing base URL",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				NodeName:         "Example SEL",
				TrustLevel:       5,
				FederationStatus: "active",
				SyncDirection:    "bidirectional",
			},
			requireHTTPS: false,
			expectError:  true,
			errorType:    ErrInvalidNodeParams,
		},
		{
			name: "HTTP URL in production mode",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				NodeName:         "Example SEL",
				BaseURL:          "http://example.org",
				TrustLevel:       5,
				FederationStatus: "active",
				SyncDirection:    "bidirectional",
			},
			requireHTTPS: true,
			expectError:  true,
		},
		{
			name: "invalid base URL with path",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				NodeName:         "Example SEL",
				BaseURL:          "https://example.org/api",
				TrustLevel:       5,
				FederationStatus: "active",
				SyncDirection:    "bidirectional",
			},
			requireHTTPS: false,
			expectError:  true,
		},
		{
			name: "trust level too low",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				NodeName:         "Example SEL",
				BaseURL:          "https://example.org",
				TrustLevel:       0,
				FederationStatus: "active",
				SyncDirection:    "bidirectional",
			},
			requireHTTPS: false,
			expectError:  true,
			errorType:    ErrInvalidNodeParams,
		},
		{
			name: "trust level too high",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				NodeName:         "Example SEL",
				BaseURL:          "https://example.org",
				TrustLevel:       11,
				FederationStatus: "active",
				SyncDirection:    "bidirectional",
			},
			requireHTTPS: false,
			expectError:  true,
			errorType:    ErrInvalidNodeParams,
		},
		{
			name: "invalid federation status",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				NodeName:         "Example SEL",
				BaseURL:          "https://example.org",
				TrustLevel:       5,
				FederationStatus: "invalid_status",
				SyncDirection:    "bidirectional",
			},
			requireHTTPS: false,
			expectError:  true,
			errorType:    ErrInvalidNodeParams,
		},
		{
			name: "invalid sync direction",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				NodeName:         "Example SEL",
				BaseURL:          "https://example.org",
				TrustLevel:       5,
				FederationStatus: "active",
				SyncDirection:    "invalid_direction",
			},
			requireHTTPS: false,
			expectError:  true,
			errorType:    ErrInvalidNodeParams,
		},
		{
			name: "all valid federation statuses - pending",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				NodeName:         "Example SEL",
				BaseURL:          "https://example.org",
				TrustLevel:       5,
				FederationStatus: "pending",
				SyncDirection:    "bidirectional",
			},
			requireHTTPS: false,
			expectError:  false,
		},
		{
			name: "all valid sync directions - pull_only",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				NodeName:         "Example SEL",
				BaseURL:          "https://example.org",
				TrustLevel:       5,
				FederationStatus: "active",
				SyncDirection:    "pull_only",
			},
			requireHTTPS: false,
			expectError:  false,
		},
		{
			name: "all valid sync directions - push_only",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				NodeName:         "Example SEL",
				BaseURL:          "https://example.org",
				TrustLevel:       5,
				FederationStatus: "active",
				SyncDirection:    "push_only",
			},
			requireHTTPS: false,
			expectError:  false,
		},
		{
			name: "all valid sync directions - disabled",
			params: CreateNodeParams{
				NodeDomain:       "example.org",
				NodeName:         "Example SEL",
				BaseURL:          "https://example.org",
				TrustLevel:       5,
				FederationStatus: "active",
				SyncDirection:    "disabled",
			},
			requireHTTPS: false,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockFederationRepo{}
			service := NewService(repo, tt.requireHTTPS)

			err := service.validateCreateParams(tt.params)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if tt.errorType != nil && !errors.Is(err, tt.errorType) {
				t.Errorf("Expected error type %v but got: %v", tt.errorType, err)
			}
		})
	}
}

// TestValidateUpdateParams tests validation logic for node updates
func TestValidateUpdateParams(t *testing.T) {
	tests := []struct {
		name         string
		params       UpdateNodeParams
		requireHTTPS bool
		expectError  bool
		errorType    error
	}{
		{
			name: "valid update with all fields",
			params: UpdateNodeParams{
				NodeName:         strPtr("Updated Name"),
				BaseURL:          strPtr("https://updated.example.org"),
				TrustLevel:       intPtr(7),
				FederationStatus: strPtr("paused"),
				SyncDirection:    strPtr("pull_only"),
			},
			requireHTTPS: false,
			expectError:  false,
		},
		{
			name: "valid partial update",
			params: UpdateNodeParams{
				TrustLevel: intPtr(3),
			},
			requireHTTPS: false,
			expectError:  false,
		},
		{
			name: "trust level too low",
			params: UpdateNodeParams{
				TrustLevel: intPtr(0),
			},
			requireHTTPS: false,
			expectError:  true,
			errorType:    ErrInvalidNodeParams,
		},
		{
			name: "trust level too high",
			params: UpdateNodeParams{
				TrustLevel: intPtr(11),
			},
			requireHTTPS: false,
			expectError:  true,
			errorType:    ErrInvalidNodeParams,
		},
		{
			name: "invalid base URL with path",
			params: UpdateNodeParams{
				BaseURL: strPtr("https://example.org/path"),
			},
			requireHTTPS: false,
			expectError:  true,
		},
		{
			name: "HTTP URL in production mode",
			params: UpdateNodeParams{
				BaseURL: strPtr("http://example.org"),
			},
			requireHTTPS: true,
			expectError:  true,
		},
		{
			name: "invalid federation status",
			params: UpdateNodeParams{
				FederationStatus: strPtr("invalid_status"),
			},
			requireHTTPS: false,
			expectError:  true,
			errorType:    ErrInvalidNodeParams,
		},
		{
			name: "invalid sync direction",
			params: UpdateNodeParams{
				SyncDirection: strPtr("invalid_direction"),
			},
			requireHTTPS: false,
			expectError:  true,
			errorType:    ErrInvalidNodeParams,
		},
		{
			name: "valid blocked status",
			params: UpdateNodeParams{
				FederationStatus: strPtr("blocked"),
			},
			requireHTTPS: false,
			expectError:  false,
		},
		{
			name: "empty base URL allowed",
			params: UpdateNodeParams{
				BaseURL: strPtr(""),
			},
			requireHTTPS: false,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockFederationRepo{}
			service := NewService(repo, tt.requireHTTPS)

			err := service.validateUpdateParams(tt.params)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if tt.errorType != nil && !errors.Is(err, tt.errorType) {
				t.Errorf("Expected error type %v but got: %v", tt.errorType, err)
			}
		})
	}
}

// TestCreateNode tests node creation logic
func TestCreateNode(t *testing.T) {
	ctx := context.Background()

	t.Run("successful creation", func(t *testing.T) {
		repo := &mockFederationRepo{
			createFunc: func(ctx context.Context, params CreateNodeParams) (*Node, error) {
				return &Node{
					ID:         uuid.New(),
					NodeDomain: params.NodeDomain,
					NodeName:   params.NodeName,
					BaseURL:    params.BaseURL,
				}, nil
			},
			getByDomainFunc: func(ctx context.Context, domain string) (*Node, error) {
				return nil, ErrNodeNotFound
			},
		}
		service := NewService(repo, false)

		params := CreateNodeParams{
			NodeDomain:       "Example.Org",
			NodeName:         "Example SEL",
			BaseURL:          "https://example.org",
			TrustLevel:       5,
			FederationStatus: "active",
			SyncDirection:    "bidirectional",
		}

		node, err := service.CreateNode(ctx, params)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if node.NodeDomain != "example.org" {
			t.Errorf("Expected normalized domain 'example.org', got: %s", node.NodeDomain)
		}
	})

	t.Run("validation error", func(t *testing.T) {
		repo := &mockFederationRepo{}
		service := NewService(repo, false)

		params := CreateNodeParams{
			NodeDomain:       "",
			NodeName:         "Example SEL",
			BaseURL:          "https://example.org",
			TrustLevel:       5,
			FederationStatus: "active",
			SyncDirection:    "bidirectional",
		}

		_, err := service.CreateNode(ctx, params)

		if !errors.Is(err, ErrInvalidNodeParams) {
			t.Errorf("Expected ErrInvalidNodeParams, got: %v", err)
		}
	})

	t.Run("duplicate domain", func(t *testing.T) {
		repo := &mockFederationRepo{
			getByDomainFunc: func(ctx context.Context, domain string) (*Node, error) {
				return &Node{
					ID:         uuid.New(),
					NodeDomain: domain,
				}, nil
			},
		}
		service := NewService(repo, false)

		params := CreateNodeParams{
			NodeDomain:       "example.org",
			NodeName:         "Example SEL",
			BaseURL:          "https://example.org",
			TrustLevel:       5,
			FederationStatus: "active",
			SyncDirection:    "bidirectional",
		}

		_, err := service.CreateNode(ctx, params)

		if !errors.Is(err, ErrDuplicateDomain) {
			t.Errorf("Expected ErrDuplicateDomain, got: %v", err)
		}
	})

	t.Run("domain normalization", func(t *testing.T) {
		var normalizedDomain string
		repo := &mockFederationRepo{
			createFunc: func(ctx context.Context, params CreateNodeParams) (*Node, error) {
				normalizedDomain = params.NodeDomain
				return &Node{
					ID:         uuid.New(),
					NodeDomain: params.NodeDomain,
				}, nil
			},
			getByDomainFunc: func(ctx context.Context, domain string) (*Node, error) {
				return nil, ErrNodeNotFound
			},
		}
		service := NewService(repo, false)

		params := CreateNodeParams{
			NodeDomain:       "  EXAMPLE.ORG  ",
			NodeName:         "Example SEL",
			BaseURL:          "https://example.org",
			TrustLevel:       5,
			FederationStatus: "active",
			SyncDirection:    "bidirectional",
		}

		_, err := service.CreateNode(ctx, params)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if normalizedDomain != "example.org" {
			t.Errorf("Expected normalized domain 'example.org', got: %s", normalizedDomain)
		}
	})
}

// TestUpdateNode tests node update logic
func TestUpdateNode(t *testing.T) {
	ctx := context.Background()
	nodeID := uuid.New()

	t.Run("successful update", func(t *testing.T) {
		repo := &mockFederationRepo{
			getByIDFunc: func(ctx context.Context, id uuid.UUID) (*Node, error) {
				return &Node{ID: id, NodeDomain: "example.org"}, nil
			},
			updateFunc: func(ctx context.Context, id uuid.UUID, params UpdateNodeParams) (*Node, error) {
				return &Node{
					ID:         id,
					NodeName:   *params.NodeName,
					TrustLevel: *params.TrustLevel,
				}, nil
			},
		}
		service := NewService(repo, false)

		params := UpdateNodeParams{
			NodeName:   strPtr("Updated Name"),
			TrustLevel: intPtr(7),
		}

		node, err := service.UpdateNode(ctx, nodeID, params)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if node.NodeName != "Updated Name" {
			t.Errorf("Expected name 'Updated Name', got: %s", node.NodeName)
		}
	})

	t.Run("validation error", func(t *testing.T) {
		repo := &mockFederationRepo{}
		service := NewService(repo, false)

		params := UpdateNodeParams{
			TrustLevel: intPtr(11),
		}

		_, err := service.UpdateNode(ctx, nodeID, params)

		if !errors.Is(err, ErrInvalidNodeParams) {
			t.Errorf("Expected ErrInvalidNodeParams, got: %v", err)
		}
	})

	t.Run("node not found", func(t *testing.T) {
		repo := &mockFederationRepo{
			getByIDFunc: func(ctx context.Context, id uuid.UUID) (*Node, error) {
				return nil, ErrNodeNotFound
			},
		}
		service := NewService(repo, false)

		params := UpdateNodeParams{
			TrustLevel: intPtr(5),
		}

		_, err := service.UpdateNode(ctx, nodeID, params)

		if !errors.Is(err, ErrNodeNotFound) {
			t.Errorf("Expected ErrNodeNotFound, got: %v", err)
		}
	})
}

// TestListNodes tests list filtering and pagination
func TestListNodes(t *testing.T) {
	ctx := context.Background()

	t.Run("default limit applied", func(t *testing.T) {
		var capturedLimit int
		repo := &mockFederationRepo{
			listFunc: func(ctx context.Context, filters ListNodesFilters) ([]*Node, error) {
				capturedLimit = filters.Limit
				return []*Node{}, nil
			},
		}
		service := NewService(repo, false)

		filters := ListNodesFilters{Limit: 0}
		_, err := service.ListNodes(ctx, filters)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if capturedLimit != 50 {
			t.Errorf("Expected default limit 50, got: %d", capturedLimit)
		}
	})

	t.Run("max limit enforced", func(t *testing.T) {
		var capturedLimit int
		repo := &mockFederationRepo{
			listFunc: func(ctx context.Context, filters ListNodesFilters) ([]*Node, error) {
				capturedLimit = filters.Limit
				return []*Node{}, nil
			},
		}
		service := NewService(repo, false)

		filters := ListNodesFilters{Limit: 200}
		_, err := service.ListNodes(ctx, filters)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if capturedLimit != 100 {
			t.Errorf("Expected max limit 100, got: %d", capturedLimit)
		}
	})

	t.Run("custom limit within range", func(t *testing.T) {
		var capturedLimit int
		repo := &mockFederationRepo{
			listFunc: func(ctx context.Context, filters ListNodesFilters) ([]*Node, error) {
				capturedLimit = filters.Limit
				return []*Node{}, nil
			},
		}
		service := NewService(repo, false)

		filters := ListNodesFilters{Limit: 25}
		_, err := service.ListNodes(ctx, filters)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if capturedLimit != 25 {
			t.Errorf("Expected limit 25, got: %d", capturedLimit)
		}
	})
}

// TestGetNodeByDomain tests domain lookup with normalization
func TestGetNodeByDomain(t *testing.T) {
	ctx := context.Background()

	t.Run("domain normalization", func(t *testing.T) {
		var capturedDomain string
		repo := &mockFederationRepo{
			getByDomainFunc: func(ctx context.Context, domain string) (*Node, error) {
				capturedDomain = domain
				return &Node{NodeDomain: domain}, nil
			},
		}
		service := NewService(repo, false)

		_, err := service.GetNodeByDomain(ctx, "  EXAMPLE.ORG  ")

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if capturedDomain != "example.org" {
			t.Errorf("Expected normalized domain 'example.org', got: %s", capturedDomain)
		}
	})
}

// TestDeleteNode tests node deletion
func TestDeleteNode(t *testing.T) {
	ctx := context.Background()
	nodeID := uuid.New()

	t.Run("successful deletion", func(t *testing.T) {
		repo := &mockFederationRepo{
			getByIDFunc: func(ctx context.Context, id uuid.UUID) (*Node, error) {
				return &Node{ID: id}, nil
			},
			deleteFunc: func(ctx context.Context, id uuid.UUID) error {
				return nil
			},
		}
		service := NewService(repo, false)

		err := service.DeleteNode(ctx, nodeID)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("node not found", func(t *testing.T) {
		repo := &mockFederationRepo{
			getByIDFunc: func(ctx context.Context, id uuid.UUID) (*Node, error) {
				return nil, ErrNodeNotFound
			},
		}
		service := NewService(repo, false)

		err := service.DeleteNode(ctx, nodeID)

		if !errors.Is(err, ErrNodeNotFound) {
			t.Errorf("Expected ErrNodeNotFound, got: %v", err)
		}
	})
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

// mockFederationRepo implements Repository for testing
type mockFederationRepo struct {
	createFunc      func(ctx context.Context, params CreateNodeParams) (*Node, error)
	getByIDFunc     func(ctx context.Context, id uuid.UUID) (*Node, error)
	getByDomainFunc func(ctx context.Context, domain string) (*Node, error)
	listFunc        func(ctx context.Context, filters ListNodesFilters) ([]*Node, error)
	updateFunc      func(ctx context.Context, id uuid.UUID, params UpdateNodeParams) (*Node, error)
	deleteFunc      func(ctx context.Context, id uuid.UUID) error
}

func (m *mockFederationRepo) Create(ctx context.Context, params CreateNodeParams) (*Node, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, params)
	}
	return nil, errors.New("not implemented")
}

func (m *mockFederationRepo) GetByID(ctx context.Context, id uuid.UUID) (*Node, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, ErrNodeNotFound
}

func (m *mockFederationRepo) GetByDomain(ctx context.Context, domain string) (*Node, error) {
	if m.getByDomainFunc != nil {
		return m.getByDomainFunc(ctx, domain)
	}
	return nil, ErrNodeNotFound
}

func (m *mockFederationRepo) List(ctx context.Context, filters ListNodesFilters) ([]*Node, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, filters)
	}
	return []*Node{}, nil
}

func (m *mockFederationRepo) Update(ctx context.Context, id uuid.UUID, params UpdateNodeParams) (*Node, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, id, params)
	}
	return nil, errors.New("not implemented")
}

func (m *mockFederationRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return errors.New("not implemented")
}
