package ids

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oklog/ulid/v2"
)

var (
	ulidRegex = regexp.MustCompile(`(?i)^[0-9A-HJKMNP-TV-Z]{26}$`)

	ErrInvalidULID          = errors.New("invalid ULID")
	ErrInvalidURI           = errors.New("invalid URI")
	ErrMissingOriginNodeID  = errors.New("origin node id required for foreign identifiers")
	ErrUnsupportedSameAs    = errors.New("unsupported sameAs value")
	ErrInvalidNodeDomain    = errors.New("invalid node domain")
	ErrInvalidEntityPath    = errors.New("invalid entity path")
	ErrMismatchedEntityPath = errors.New("entity path does not match")
)

// IdentifierRole describes how a URI should be treated in SEL.
// canonical = local entity minted by this node
// foreign = federated entity with origin_node_id
// alias = sameAs link to another canonical URI
//
// Role is derived from URI host + origin_node_id presence.
type IdentifierRole string

const (
	RoleCanonical IdentifierRole = "canonical"
	RoleForeign   IdentifierRole = "foreign"
	RoleAlias     IdentifierRole = "alias"
)

// EntityURI is a parsed, validated SEL URI.
type EntityURI struct {
	URI  string
	ULID string
	Role IdentifierRole
	Host string
	Path string
}

// NewULID generates a new ULID string.
func NewULID() (string, error) {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id, err := ulid.New(ulid.Timestamp(time.Now()), entropy)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// IsULID returns true when value is a valid ULID (case-insensitive Crockford Base32).
func IsULID(value string) bool {
	return ulidRegex.MatchString(strings.TrimSpace(value))
}

// ValidateULID validates a ULID string.
func ValidateULID(value string) error {
	if !IsULID(value) {
		return ErrInvalidULID
	}
	return nil
}

// BuildCanonicalURI creates a canonical URI for a local entity.
func BuildCanonicalURI(nodeDomain, entityPath, ulid string) (string, error) {
	if err := ValidateULID(ulid); err != nil {
		return "", err
	}

	scheme, host, err := normalizeNodeDomain(nodeDomain)
	if err != nil {
		return "", err
	}

	cleanEntityPath, err := normalizeEntityPath(entityPath)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s://%s/%s/%s", scheme, host, cleanEntityPath, strings.ToUpper(ulid)), nil
}

// ParseEntityURI validates a URI and derives role + ULID.
// originNodeID is required for foreign identifiers.
func ParseEntityURI(nodeDomain, entityPath, uri, originNodeID string) (EntityURI, error) {
	parsed, err := url.Parse(strings.TrimSpace(uri))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return EntityURI{}, ErrInvalidURI
	}

	cleanEntityPath, err := normalizeEntityPath(entityPath)
	if err != nil {
		return EntityURI{}, err
	}

	cleanPath := path.Clean(parsed.Path)
	pathParts := strings.Split(strings.TrimPrefix(cleanPath, "/"), "/")
	if len(pathParts) < 2 {
		return EntityURI{}, ErrInvalidURI
	}
	if pathParts[0] != cleanEntityPath {
		return EntityURI{}, ErrMismatchedEntityPath
	}

	id := strings.ToUpper(pathParts[1])
	if err := ValidateULID(id); err != nil {
		return EntityURI{}, err
	}

	_, host, err := normalizeNodeDomain(nodeDomain)
	if err != nil {
		return EntityURI{}, err
	}

	role := RoleForeign
	if strings.EqualFold(parsed.Host, host) {
		role = RoleCanonical
	} else if originNodeID == "" {
		return EntityURI{}, ErrMissingOriginNodeID
	}

	return EntityURI{
		URI:  parsed.String(),
		ULID: id,
		Role: role,
		Host: parsed.Host,
		Path: cleanPath,
	}, nil
}

// NormalizeSameAs ensures sameAs values resolve to full canonical URIs.
func NormalizeSameAs(nodeDomain, entityPath, value string) (string, error) {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return "", ErrUnsupportedSameAs
	}

	parsed, err := url.Parse(candidate)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return parsed.String(), nil
	}

	cleanEntityPath, err := normalizeEntityPath(entityPath)
	if err != nil {
		return "", err
	}

	if strings.HasPrefix(candidate, "/") {
		return buildAbsoluteURI(nodeDomain, candidate)
	}

	if IsULID(candidate) {
		return BuildCanonicalURI(nodeDomain, cleanEntityPath, candidate)
	}

	prefix := cleanEntityPath + "/"
	if strings.HasPrefix(candidate, prefix) {
		return BuildCanonicalURI(nodeDomain, cleanEntityPath, strings.TrimPrefix(candidate, prefix))
	}

	return "", ErrUnsupportedSameAs
}

func normalizeNodeDomain(nodeDomain string) (string, string, error) {
	value := strings.TrimSpace(nodeDomain)
	if value == "" {
		return "", "", ErrInvalidNodeDomain
	}

	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" {
			return "", "", ErrInvalidNodeDomain
		}
		if parsed.Path != "" && parsed.Path != "/" {
			return "", "", ErrInvalidNodeDomain
		}
		return parsed.Scheme, parsed.Host, nil
	}

	if strings.Contains(value, "/") {
		return "", "", ErrInvalidNodeDomain
	}

	return "https", value, nil
}

func normalizeEntityPath(entityPath string) (string, error) {
	cleaned := strings.Trim(strings.TrimSpace(entityPath), "/")
	if cleaned == "" {
		return "", ErrInvalidEntityPath
	}
	return cleaned, nil
}

func buildAbsoluteURI(nodeDomain, relativePath string) (string, error) {
	scheme, host, err := normalizeNodeDomain(nodeDomain)
	if err != nil {
		return "", err
	}

	cleanPath := path.Clean("/" + strings.TrimSpace(relativePath))
	return fmt.Sprintf("%s://%s%s", scheme, host, cleanPath), nil
}

// UUIDToString converts a pgtype.UUID to a properly formatted UUID string.
// Returns an empty string if the UUID is not valid.
func UUIDToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	parsedUUID, err := uuid.FromBytes(u.Bytes[:])
	if err != nil {
		return ""
	}
	return parsedUUID.String()
}
