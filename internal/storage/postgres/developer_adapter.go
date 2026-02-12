package postgres

import (
	"context"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// DeveloperRepositoryAdapter adapts postgres.DeveloperRepository to implement developers.Repository
// It converts between postgres models and domain models.
type DeveloperRepositoryAdapter struct {
	repo *DeveloperRepository
	pool *pgxpool.Pool
}

// NewDeveloperRepositoryAdapter creates a new adapter for the developer repository
func NewDeveloperRepositoryAdapter(pool *pgxpool.Pool) *DeveloperRepositoryAdapter {
	return &DeveloperRepositoryAdapter{
		repo: NewDeveloperRepository(pool),
		pool: pool,
	}
}

// CreateDeveloper creates a new developer account
func (a *DeveloperRepositoryAdapter) CreateDeveloper(ctx context.Context, params developers.CreateDeveloperDBParams) (*developers.Developer, error) {
	// Convert domain params to postgres params
	pgParams := CreateDeveloperParams{
		Email:   params.Email,
		Name:    params.Name,
		MaxKeys: int32(params.MaxKeys),
	}
	if params.PasswordHash != nil {
		pgParams.PasswordHash = pgtype.Text{String: *params.PasswordHash, Valid: true}
	}
	if params.GitHubID != nil {
		pgParams.GithubID = pgtype.Int8{Int64: *params.GitHubID, Valid: true}
	}
	if params.GitHubUsername != nil {
		pgParams.GithubUsername = pgtype.Text{String: *params.GitHubUsername, Valid: true}
	}

	pgDev, err := a.repo.CreateDeveloper(ctx, pgParams)
	if err != nil {
		return nil, err
	}

	return pgDeveloperToDomain(pgDev), nil
}

// GetDeveloperByID retrieves a developer by their ID
func (a *DeveloperRepositoryAdapter) GetDeveloperByID(ctx context.Context, id uuid.UUID) (*developers.Developer, error) {
	pgUUID := uuidToPgUUID(id)
	pgDev, err := a.repo.GetDeveloperByID(ctx, pgUUID)
	if err != nil {
		return nil, err
	}
	return pgDeveloperToDomain(pgDev), nil
}

// GetDeveloperByEmail retrieves a developer by their email
func (a *DeveloperRepositoryAdapter) GetDeveloperByEmail(ctx context.Context, email string) (*developers.Developer, error) {
	pgDev, err := a.repo.GetDeveloperByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return pgDeveloperToDomain(pgDev), nil
}

// GetDeveloperByGitHubID retrieves a developer by their GitHub ID
func (a *DeveloperRepositoryAdapter) GetDeveloperByGitHubID(ctx context.Context, githubID int64) (*developers.Developer, error) {
	pgDev, err := a.repo.GetDeveloperByGitHubID(ctx, githubID)
	if err != nil {
		return nil, err
	}
	return pgDeveloperToDomain(pgDev), nil
}

// ListDevelopers retrieves a paginated list of developers
func (a *DeveloperRepositoryAdapter) ListDevelopers(ctx context.Context, limit, offset int) ([]*developers.Developer, error) {
	pgDevs, err := a.repo.ListDevelopers(ctx, int32(limit), int32(offset))
	if err != nil {
		return nil, err
	}

	result := make([]*developers.Developer, 0, len(pgDevs))
	for _, pgDev := range pgDevs {
		result = append(result, pgDeveloperToDomain(pgDev))
	}
	return result, nil
}

// UpdateDeveloper updates developer fields
func (a *DeveloperRepositoryAdapter) UpdateDeveloper(ctx context.Context, id uuid.UUID, params developers.UpdateDeveloperParams) (*developers.Developer, error) {
	pgUUID := uuidToPgUUID(id)
	pgParams := NewUpdateDeveloperParams(pgUUID)

	if params.Name != nil {
		pgParams.SetName(*params.Name)
	}
	if params.GitHubID != nil {
		pgParams.SetGitHubID(*params.GitHubID)
	}
	if params.GitHubUsername != nil {
		pgParams.SetGitHubUsername(*params.GitHubUsername)
	}
	if params.MaxKeys != nil {
		pgParams.SetMaxKeys(int32(*params.MaxKeys))
	}
	if params.IsActive != nil {
		pgParams.SetIsActive(*params.IsActive)
	}

	pgDev, err := a.repo.UpdateDeveloper(ctx, pgParams)
	if err != nil {
		return nil, err
	}
	return pgDeveloperToDomain(pgDev), nil
}

// UpdateDeveloperLastLogin updates the last login timestamp
func (a *DeveloperRepositoryAdapter) UpdateDeveloperLastLogin(ctx context.Context, id uuid.UUID) error {
	pgUUID := uuidToPgUUID(id)
	return a.repo.UpdateDeveloperLastLogin(ctx, pgUUID)
}

// DeactivateDeveloper marks a developer account as inactive
func (a *DeveloperRepositoryAdapter) DeactivateDeveloper(ctx context.Context, id uuid.UUID) error {
	pgUUID := uuidToPgUUID(id)
	return a.repo.DeactivateDeveloper(ctx, pgUUID)
}

// CountDevelopers returns the total number of developers
func (a *DeveloperRepositoryAdapter) CountDevelopers(ctx context.Context) (int64, error) {
	return a.repo.CountDevelopers(ctx)
}

// ValidateDeveloperPassword validates a developer's password against the stored hash
func (a *DeveloperRepositoryAdapter) ValidateDeveloperPassword(ctx context.Context, id uuid.UUID, password string) (bool, error) {
	pgUUID := uuidToPgUUID(id)
	pgDev, err := a.repo.GetDeveloperByID(ctx, pgUUID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	if !pgDev.PasswordHash.Valid {
		// No password set (OAuth-only account)
		return false, nil
	}

	// Compare password with bcrypt hash
	err = bcrypt.CompareHashAndPassword([]byte(pgDev.PasswordHash.String), []byte(password))
	return err == nil, nil
}

// CreateInvitation creates a new developer invitation
func (a *DeveloperRepositoryAdapter) CreateInvitation(ctx context.Context, params developers.CreateInvitationDBParams) (*developers.DeveloperInvitation, error) {
	var pgInvitedBy pgtype.UUID
	if params.InvitedBy != nil {
		pgInvitedBy = uuidToPgUUID(*params.InvitedBy)
	}

	pgParams := NewCreateDeveloperInvitationParams(
		params.Email,
		params.TokenHash,
		pgInvitedBy,
		params.ExpiresAt,
	)

	pgInv, err := a.repo.CreateDeveloperInvitation(ctx, pgParams)
	if err != nil {
		return nil, err
	}

	return pgInvitationToDomain(pgInv), nil
}

// GetInvitationByTokenHash retrieves an invitation by token hash
func (a *DeveloperRepositoryAdapter) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*developers.DeveloperInvitation, error) {
	pgInv, err := a.repo.GetDeveloperInvitationByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	return pgInvitationToDomain(pgInv), nil
}

// AcceptInvitation marks an invitation as accepted
func (a *DeveloperRepositoryAdapter) AcceptInvitation(ctx context.Context, id uuid.UUID) error {
	pgUUID := uuidToPgUUID(id)
	return a.repo.AcceptDeveloperInvitation(ctx, pgUUID)
}

// ListActiveInvitations retrieves all active invitations
func (a *DeveloperRepositoryAdapter) ListActiveInvitations(ctx context.Context) ([]*developers.DeveloperInvitation, error) {
	pgInvs, err := a.repo.ListActiveDeveloperInvitations(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*developers.DeveloperInvitation, 0, len(pgInvs))
	for _, pgInv := range pgInvs {
		result = append(result, pgInvitationToDomain(pgInv))
	}
	return result, nil
}

// ListDeveloperAPIKeys retrieves all API keys for a developer
func (a *DeveloperRepositoryAdapter) ListDeveloperAPIKeys(ctx context.Context, developerID uuid.UUID) ([]developers.APIKey, error) {
	pgUUID := uuidToPgUUID(developerID)
	pgKeys, err := a.repo.ListDeveloperAPIKeys(ctx, pgUUID)
	if err != nil {
		return nil, err
	}

	result := make([]developers.APIKey, 0, len(pgKeys))
	for _, pgKey := range pgKeys {
		result = append(result, pgAPIKeyToDomain(pgKey))
	}
	return result, nil
}

// CountDeveloperAPIKeys counts active API keys for a developer
func (a *DeveloperRepositoryAdapter) CountDeveloperAPIKeys(ctx context.Context, developerID uuid.UUID) (int64, error) {
	pgUUID := uuidToPgUUID(developerID)
	return a.repo.CountDeveloperAPIKeys(ctx, pgUUID)
}

// RevokeAllDeveloperAPIKeys revokes all active API keys for a developer
func (a *DeveloperRepositoryAdapter) RevokeAllDeveloperAPIKeys(ctx context.Context, developerID uuid.UUID) (int64, error) {
	pgUUID := uuidToPgUUID(developerID)
	return a.repo.RevokeAllDeveloperAPIKeys(ctx, pgUUID)
}

// CheckAPIKeyOwnership verifies that a specific API key belongs to a developer
func (a *DeveloperRepositoryAdapter) CheckAPIKeyOwnership(ctx context.Context, keyID uuid.UUID, developerID uuid.UUID) (bool, error) {
	pgKeyID := uuidToPgUUID(keyID)
	pgDeveloperID := uuidToPgUUID(developerID)
	return a.repo.CheckAPIKeyOwnership(ctx, pgKeyID, pgDeveloperID)
}

// CreateAPIKey creates a new API key
func (a *DeveloperRepositoryAdapter) CreateAPIKey(ctx context.Context, params developers.CreateAPIKeyDBParams) (*developers.CreateAPIKeyResult, error) {
	queries := New(a.pool)

	var pgExpiresAt pgtype.Timestamptz
	if params.ExpiresAt != nil {
		pgExpiresAt = pgtype.Timestamptz{Time: *params.ExpiresAt, Valid: true}
	}

	pgParams := CreateDeveloperAPIKeyParams{
		Prefix:        params.Prefix,
		KeyHash:       params.KeyHash,
		HashVersion:   int32(params.HashVersion),
		Name:          params.Name,
		DeveloperID:   uuidToPgUUID(params.DeveloperID),
		Role:          params.Role,
		RateLimitTier: params.RateLimitTier,
		IsActive:      params.IsActive,
		ExpiresAt:     pgExpiresAt,
	}

	row, err := queries.CreateDeveloperAPIKey(ctx, pgParams)
	if err != nil {
		return nil, err
	}

	result := &developers.CreateAPIKeyResult{
		ID:            pgUUIDToUUID(row.ID),
		Key:           "", // Will be filled in by the caller
		Prefix:        row.Prefix,
		Name:          row.Name,
		Role:          row.Role,
		RateLimitTier: row.RateLimitTier,
		CreatedAt:     row.CreatedAt.Time,
	}
	if row.ExpiresAt.Valid {
		expiresAt := row.ExpiresAt.Time
		result.ExpiresAt = &expiresAt
	}

	return result, nil
}

// DeactivateAPIKey deactivates an API key
func (a *DeveloperRepositoryAdapter) DeactivateAPIKey(ctx context.Context, id uuid.UUID) error {
	queries := New(a.pool)
	pgUUID := uuidToPgUUID(id)
	return queries.DeactivateAPIKey(ctx, pgUUID)
}

// GetAPIKeyByID retrieves an API key by ID
func (a *DeveloperRepositoryAdapter) GetAPIKeyByID(ctx context.Context, id uuid.UUID) (*developers.APIKey, error) {
	queries := New(a.pool)
	pgUUID := uuidToPgUUID(id)
	pgKey, err := queries.GetAPIKeyByID(ctx, pgUUID)
	if err != nil {
		return nil, err
	}
	domainKey := pgAPIKeyToDomain(pgKey)
	return &domainKey, nil
}

// GetAPIKeyUsageTotal retrieves total usage for an API key in a date range
func (a *DeveloperRepositoryAdapter) GetAPIKeyUsageTotal(ctx context.Context, apiKeyID uuid.UUID, startDate, endDate time.Time) (totalRequests, totalErrors int64, err error) {
	queries := New(a.pool)
	pgUUID := uuidToPgUUID(apiKeyID)

	row, err := queries.GetAPIKeyUsageTotal(ctx, GetAPIKeyUsageTotalParams{
		ApiKeyID: pgUUID,
		Date:     pgtype.Date{Time: startDate, Valid: true},
		Date_2:   pgtype.Date{Time: endDate, Valid: true},
	})
	if err != nil {
		return 0, 0, err
	}

	return row.TotalRequests, row.TotalErrors, nil
}

// GetDeveloperUsageTotal retrieves total usage for all of a developer's keys in a date range
func (a *DeveloperRepositoryAdapter) GetDeveloperUsageTotal(ctx context.Context, developerID uuid.UUID, startDate, endDate time.Time) (totalRequests, totalErrors int64, err error) {
	queries := New(a.pool)
	pgUUID := uuidToPgUUID(developerID)

	row, err := queries.GetDeveloperUsageTotal(ctx, GetDeveloperUsageTotalParams{
		DeveloperID: pgUUID,
		Date:        pgtype.Date{Time: startDate, Valid: true},
		Date_2:      pgtype.Date{Time: endDate, Valid: true},
	})
	if err != nil {
		return 0, 0, err
	}

	return row.TotalRequests, row.TotalErrors, nil
}

// GetAPIKeyUsage retrieves daily usage records for an API key in a date range
func (a *DeveloperRepositoryAdapter) GetAPIKeyUsage(ctx context.Context, apiKeyID uuid.UUID, startDate, endDate time.Time) ([]developers.DailyUsage, error) {
	queries := New(a.pool)
	pgUUID := uuidToPgUUID(apiKeyID)

	records, err := queries.GetAPIKeyUsage(ctx, GetAPIKeyUsageParams{
		ApiKeyID: pgUUID,
		Date:     pgtype.Date{Time: startDate, Valid: true},
		Date_2:   pgtype.Date{Time: endDate, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	// Convert to domain types
	result := make([]developers.DailyUsage, 0, len(records))
	for _, record := range records {
		result = append(result, developers.DailyUsage{
			Date:     record.Date.Time,
			Requests: record.RequestCount,
			Errors:   record.ErrorCount,
		})
	}

	return result, nil
}

// BeginTx starts a new transaction and returns a transaction-scoped repository adapter
func (a *DeveloperRepositoryAdapter) BeginTx(ctx context.Context) (developers.Repository, developers.TxCommitter, error) {
	txRepo, txCommitter, err := a.repo.BeginTx(ctx)
	if err != nil {
		return nil, nil, err
	}

	txAdapter := &DeveloperRepositoryAdapter{
		repo: txRepo,
		pool: a.pool,
	}

	return txAdapter, &developerAdapterTxCommitter{txCommitter: txCommitter}, nil
}

// developerAdapterTxCommitter wraps the postgres developerTxCommitter to implement developers.TxCommitter
type developerAdapterTxCommitter struct {
	txCommitter *developerTxCommitter
}

func (tc *developerAdapterTxCommitter) Commit(ctx context.Context) error {
	return tc.txCommitter.Commit(ctx)
}

func (tc *developerAdapterTxCommitter) Rollback(ctx context.Context) error {
	return tc.txCommitter.Rollback(ctx)
}

// Helper functions for type conversion

// uuidToPgUUID converts uuid.UUID to pgtype.UUID
func uuidToPgUUID(id uuid.UUID) pgtype.UUID {
	var pgUUID pgtype.UUID
	copy(pgUUID.Bytes[:], id[:])
	pgUUID.Valid = true
	return pgUUID
}

// pgUUIDToUUID converts pgtype.UUID to uuid.UUID
func pgUUIDToUUID(pgUUID pgtype.UUID) uuid.UUID {
	var id uuid.UUID
	copy(id[:], pgUUID.Bytes[:])
	return id
}

// pgDeveloperToDomain converts postgres Developer to domain Developer
func pgDeveloperToDomain(pgDev Developer) *developers.Developer {
	dev := &developers.Developer{
		ID:        pgUUIDToUUID(pgDev.ID),
		Email:     pgDev.Email,
		Name:      pgDev.Name,
		MaxKeys:   int(pgDev.MaxKeys),
		IsActive:  pgDev.IsActive,
		CreatedAt: pgDev.CreatedAt.Time,
	}

	if pgDev.GithubID.Valid {
		githubID := pgDev.GithubID.Int64
		dev.GitHubID = &githubID
	}
	if pgDev.GithubUsername.Valid {
		githubUsername := pgDev.GithubUsername.String
		dev.GitHubUsername = &githubUsername
	}
	if pgDev.LastLoginAt.Valid {
		lastLogin := pgDev.LastLoginAt.Time
		dev.LastLoginAt = &lastLogin
	}

	return dev
}

// pgInvitationToDomain converts postgres DeveloperInvitation to domain DeveloperInvitation
func pgInvitationToDomain(pgInv DeveloperInvitation) *developers.DeveloperInvitation {
	inv := &developers.DeveloperInvitation{
		ID:        pgUUIDToUUID(pgInv.ID),
		Email:     pgInv.Email,
		TokenHash: pgInv.TokenHash,
		ExpiresAt: pgInv.ExpiresAt.Time,
		CreatedAt: pgInv.CreatedAt.Time,
	}

	if pgInv.InvitedBy.Valid {
		invitedBy := pgUUIDToUUID(pgInv.InvitedBy)
		inv.InvitedBy = &invitedBy
	}
	if pgInv.AcceptedAt.Valid {
		acceptedAt := pgInv.AcceptedAt.Time
		inv.AcceptedAt = &acceptedAt
	}

	return inv
}

// pgAPIKeyToDomain converts postgres ApiKey to domain APIKey
func pgAPIKeyToDomain(pgKey ApiKey) developers.APIKey {
	key := developers.APIKey{
		ID:            pgUUIDToUUID(pgKey.ID),
		Prefix:        pgKey.Prefix,
		KeyHash:       pgKey.KeyHash,
		HashVersion:   int(pgKey.HashVersion),
		Name:          pgKey.Name,
		SourceID:      pgUUIDToUUID(pgKey.SourceID),
		Role:          pgKey.Role,
		RateLimitTier: pgKey.RateLimitTier,
		IsActive:      pgKey.IsActive,
		CreatedAt:     pgKey.CreatedAt.Time,
		DeveloperID:   pgUUIDToUUID(pgKey.DeveloperID),
	}

	if pgKey.LastUsedAt.Valid {
		lastUsed := pgKey.LastUsedAt.Time
		key.LastUsedAt = &lastUsed
	}
	if pgKey.ExpiresAt.Valid {
		expires := pgKey.ExpiresAt.Time
		key.ExpiresAt = &expires
	}

	return key
}
