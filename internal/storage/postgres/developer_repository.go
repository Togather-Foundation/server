package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DeveloperRepository handles developer-related database operations
type DeveloperRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

// NewDeveloperRepository creates a new DeveloperRepository
func NewDeveloperRepository(pool *pgxpool.Pool) *DeveloperRepository {
	return &DeveloperRepository{pool: pool}
}

// queryer returns the appropriate database interface (transaction or pool)
func (r *DeveloperRepository) queryer() Querier {
	if r.tx != nil {
		return &Queries{db: r.tx}
	}
	return &Queries{db: r.pool}
}

// CreateDeveloper creates a new developer account
func (r *DeveloperRepository) CreateDeveloper(ctx context.Context, params CreateDeveloperParams) (Developer, error) {
	q := r.queryer()
	dev, err := q.CreateDeveloper(ctx, params)
	if err != nil {
		return Developer{}, fmt.Errorf("create developer: %w", err)
	}
	return dev, nil
}

// GetDeveloperByID retrieves a developer by their ID
func (r *DeveloperRepository) GetDeveloperByID(ctx context.Context, id pgtype.UUID) (Developer, error) {
	q := r.queryer()
	dev, err := q.GetDeveloperByID(ctx, id)
	if err == pgx.ErrNoRows {
		return Developer{}, fmt.Errorf("developer not found")
	}
	if err != nil {
		return Developer{}, fmt.Errorf("get developer by id: %w", err)
	}
	return dev, nil
}

// GetDeveloperByEmail retrieves a developer by their email
func (r *DeveloperRepository) GetDeveloperByEmail(ctx context.Context, email string) (Developer, error) {
	q := r.queryer()
	dev, err := q.GetDeveloperByEmail(ctx, email)
	if err == pgx.ErrNoRows {
		return Developer{}, fmt.Errorf("developer not found")
	}
	if err != nil {
		return Developer{}, fmt.Errorf("get developer by email: %w", err)
	}
	return dev, nil
}

// GetDeveloperByGitHubID retrieves a developer by their GitHub ID
func (r *DeveloperRepository) GetDeveloperByGitHubID(ctx context.Context, githubID int64) (Developer, error) {
	q := r.queryer()
	dev, err := q.GetDeveloperByGitHubID(ctx, pgtype.Int8{Int64: githubID, Valid: true})
	if err == pgx.ErrNoRows {
		return Developer{}, fmt.Errorf("developer not found")
	}
	if err != nil {
		return Developer{}, fmt.Errorf("get developer by github id: %w", err)
	}
	return dev, nil
}

// ListDevelopers retrieves a paginated list of developers
func (r *DeveloperRepository) ListDevelopers(ctx context.Context, limit, offset int32) ([]Developer, error) {
	q := r.queryer()
	devs, err := q.ListDevelopers(ctx, ListDevelopersParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list developers: %w", err)
	}
	return devs, nil
}

// UpdateDeveloper updates developer fields (nullable parameters only update if provided)
func (r *DeveloperRepository) UpdateDeveloper(ctx context.Context, params UpdateDeveloperParams) (Developer, error) {
	q := r.queryer()
	dev, err := q.UpdateDeveloper(ctx, params)
	if err == pgx.ErrNoRows {
		return Developer{}, fmt.Errorf("developer not found")
	}
	if err != nil {
		return Developer{}, fmt.Errorf("update developer: %w", err)
	}
	return dev, nil
}

// UpdateDeveloperLastLogin updates the last login timestamp for a developer
func (r *DeveloperRepository) UpdateDeveloperLastLogin(ctx context.Context, id pgtype.UUID) error {
	q := r.queryer()
	if err := q.UpdateDeveloperLastLogin(ctx, id); err != nil {
		return fmt.Errorf("update developer last login: %w", err)
	}
	return nil
}

// DeactivateDeveloper marks a developer account as inactive
func (r *DeveloperRepository) DeactivateDeveloper(ctx context.Context, id pgtype.UUID) error {
	q := r.queryer()
	if err := q.DeactivateDeveloper(ctx, id); err != nil {
		return fmt.Errorf("deactivate developer: %w", err)
	}
	return nil
}

// CountDevelopers returns the total number of developers
func (r *DeveloperRepository) CountDevelopers(ctx context.Context) (int64, error) {
	q := r.queryer()
	count, err := q.CountDevelopers(ctx)
	if err != nil {
		return 0, fmt.Errorf("count developers: %w", err)
	}
	return count, nil
}

// CreateDeveloperInvitation creates a new developer invitation
func (r *DeveloperRepository) CreateDeveloperInvitation(ctx context.Context, params CreateDeveloperInvitationParams) (DeveloperInvitation, error) {
	q := r.queryer()
	inv, err := q.CreateDeveloperInvitation(ctx, params)
	if err != nil {
		return DeveloperInvitation{}, fmt.Errorf("create developer invitation: %w", err)
	}
	return inv, nil
}

// GetDeveloperInvitationByTokenHash retrieves an unaccepted invitation by token hash
func (r *DeveloperRepository) GetDeveloperInvitationByTokenHash(ctx context.Context, tokenHash string) (DeveloperInvitation, error) {
	q := r.queryer()
	inv, err := q.GetDeveloperInvitationByTokenHash(ctx, tokenHash)
	if err == pgx.ErrNoRows {
		return DeveloperInvitation{}, fmt.Errorf("invitation not found or already accepted")
	}
	if err != nil {
		return DeveloperInvitation{}, fmt.Errorf("get developer invitation: %w", err)
	}
	return inv, nil
}

// AcceptDeveloperInvitation marks an invitation as accepted
func (r *DeveloperRepository) AcceptDeveloperInvitation(ctx context.Context, id pgtype.UUID) error {
	q := r.queryer()
	if err := q.AcceptDeveloperInvitation(ctx, id); err != nil {
		return fmt.Errorf("accept developer invitation: %w", err)
	}
	return nil
}

// ListActiveDeveloperInvitations retrieves all active (unaccepted, non-expired) invitations
func (r *DeveloperRepository) ListActiveDeveloperInvitations(ctx context.Context) ([]DeveloperInvitation, error) {
	q := r.queryer()
	invs, err := q.ListActiveDeveloperInvitations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active developer invitations: %w", err)
	}
	return invs, nil
}

// ListDeveloperAPIKeys retrieves all API keys for a developer
func (r *DeveloperRepository) ListDeveloperAPIKeys(ctx context.Context, developerID pgtype.UUID) ([]ApiKey, error) {
	q := r.queryer()
	keys, err := q.ListDeveloperAPIKeys(ctx, developerID)
	if err != nil {
		return nil, fmt.Errorf("list developer api keys: %w", err)
	}
	return keys, nil
}

// CountDeveloperAPIKeys counts active API keys for a developer
func (r *DeveloperRepository) CountDeveloperAPIKeys(ctx context.Context, developerID pgtype.UUID) (int64, error) {
	q := r.queryer()
	count, err := q.CountDeveloperAPIKeys(ctx, developerID)
	if err != nil {
		return 0, fmt.Errorf("count developer api keys: %w", err)
	}
	return count, nil
}

// RevokeAllDeveloperAPIKeys revokes all active API keys for a developer
func (r *DeveloperRepository) RevokeAllDeveloperAPIKeys(ctx context.Context, developerID pgtype.UUID) (int64, error) {
	q := r.queryer()
	count, err := q.RevokeAllDeveloperAPIKeys(ctx, developerID)
	if err != nil {
		return 0, fmt.Errorf("revoke all developer api keys: %w", err)
	}
	return count, nil
}

// CheckAPIKeyOwnership verifies that a specific API key belongs to a developer
func (r *DeveloperRepository) CheckAPIKeyOwnership(ctx context.Context, keyID, developerID pgtype.UUID) (bool, error) {
	q := r.queryer()
	owned, err := q.CheckAPIKeyOwnership(ctx, CheckAPIKeyOwnershipParams{
		ID:          keyID,
		DeveloperID: developerID,
	})
	if err != nil {
		return false, fmt.Errorf("check api key ownership: %w", err)
	}
	return owned, nil
}

// WithTx returns a new repository instance that will use the provided transaction
func (r *DeveloperRepository) WithTx(tx pgx.Tx) *DeveloperRepository {
	return &DeveloperRepository{
		pool: r.pool,
		tx:   tx,
	}
}

// BeginTx starts a new transaction and returns a transaction-scoped repository
func (r *DeveloperRepository) BeginTx(ctx context.Context) (*DeveloperRepository, *developerTxCommitter, error) {
	if r.tx != nil {
		return nil, nil, fmt.Errorf("repository already in transaction")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin transaction: %w", err)
	}

	txRepo := &DeveloperRepository{
		pool: r.pool,
		tx:   tx,
	}

	return txRepo, &developerTxCommitter{tx: tx}, nil
}

// developerTxCommitter implements transaction commit/rollback for developer repository
type developerTxCommitter struct {
	tx pgx.Tx
}

func (tc *developerTxCommitter) Commit(ctx context.Context) error {
	return tc.tx.Commit(ctx)
}

func (tc *developerTxCommitter) Rollback(ctx context.Context) error {
	return tc.tx.Rollback(ctx)
}

// Helper functions for creating parameters with nullable types

// NewUpdateDeveloperParams creates an UpdateDeveloperParams with only the ID set
// Call SetX methods to set optional fields
func NewUpdateDeveloperParams(id pgtype.UUID) UpdateDeveloperParams {
	return UpdateDeveloperParams{ID: id}
}

// SetName sets the name field for update
func (p *UpdateDeveloperParams) SetName(name string) {
	p.Name = pgtype.Text{String: name, Valid: true}
}

// SetGitHubID sets the github_id field for update
func (p *UpdateDeveloperParams) SetGitHubID(id int64) {
	p.GithubID = pgtype.Int8{Int64: id, Valid: true}
}

// SetGitHubUsername sets the github_username field for update
func (p *UpdateDeveloperParams) SetGitHubUsername(username string) {
	p.GithubUsername = pgtype.Text{String: username, Valid: true}
}

// SetMaxKeys sets the max_keys field for update
func (p *UpdateDeveloperParams) SetMaxKeys(maxKeys int32) {
	p.MaxKeys = pgtype.Int4{Int32: maxKeys, Valid: true}
}

// SetIsActive sets the is_active field for update
func (p *UpdateDeveloperParams) SetIsActive(isActive bool) {
	p.IsActive = pgtype.Bool{Bool: isActive, Valid: true}
}

// Helper function for creating a CreateDeveloperParams
func NewCreateDeveloperParams(email, name string, maxKeys int32) CreateDeveloperParams {
	return CreateDeveloperParams{
		Email:   email,
		Name:    name,
		MaxKeys: maxKeys,
	}
}

// SetPasswordHash sets the password hash (for email/password auth)
func (p *CreateDeveloperParams) SetPasswordHash(hash string) {
	p.PasswordHash = pgtype.Text{String: hash, Valid: true}
}

// SetGitHub sets GitHub OAuth details
func (p *CreateDeveloperParams) SetGitHub(id int64, username string) {
	p.GithubID = pgtype.Int8{Int64: id, Valid: true}
	p.GithubUsername = pgtype.Text{String: username, Valid: true}
}

// Helper function for creating developer invitations
func NewCreateDeveloperInvitationParams(email, tokenHash string, invitedBy pgtype.UUID, expiresAt time.Time) CreateDeveloperInvitationParams {
	return CreateDeveloperInvitationParams{
		Email:     email,
		TokenHash: tokenHash,
		InvitedBy: invitedBy,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}
}
