package federation

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the data access interface for federation nodes
type Repository interface {
	Create(ctx context.Context, params CreateNodeParams) (*Node, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Node, error)
	GetByDomain(ctx context.Context, domain string) (*Node, error)
	List(ctx context.Context, filters ListNodesFilters) ([]*Node, error)
	Update(ctx context.Context, id uuid.UUID, params UpdateNodeParams) (*Node, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
