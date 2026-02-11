package federation

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	ErrInvalidFederationURI = errors.New("invalid federation URI")
	ErrInvalidNodeDomain    = errors.New("invalid node domain")
	ErrInvalidULID          = errors.New("invalid ULID format")
	ErrInvalidEntityType    = errors.New("invalid entity type")
)

// ValidEntityTypes defines allowed entity types in federation URIs per SEL Core Profile §1.1
var ValidEntityTypes = map[string]bool{
	"events":        true,
	"places":        true,
	"organizations": true,
	"persons":       true,
}

// SEL URI pattern: https://{node-domain}/{entity-type}/{ulid}
// ULID: 26 character Crockford Base32 (0-9A-Z excluding I,L,O,U)
var ulidPattern = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
var domainPattern = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// ValidateFederationURI validates a federation URI follows SEL Core Profile §1.1 pattern
// Pattern: https://{node-domain}/{entity-type}/{ulid}
func ValidateFederationURI(federationURI string) error {
	if federationURI == "" {
		return fmt.Errorf("%w: empty URI", ErrInvalidFederationURI)
	}
	
	// Parse URL
	parsedURL, err := url.Parse(federationURI)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFederationURI, err)
	}
	
	// Validate scheme (must be https or http)
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return fmt.Errorf("%w: must use http or https scheme, got: %s", ErrInvalidFederationURI, parsedURL.Scheme)
	}
	
	// Validate host (node domain)
	if parsedURL.Host == "" {
		return fmt.Errorf("%w: missing host", ErrInvalidFederationURI)
	}
	
	if err := ValidateNodeDomain(parsedURL.Host); err != nil {
		return err
	}
	
	// Parse path: /{entity-type}/{ulid}
	pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if len(pathParts) != 2 {
		return fmt.Errorf("%w: path must be /{entity-type}/{ulid}, got: %s", ErrInvalidFederationURI, parsedURL.Path)
	}
	
	entityType := pathParts[0]
	ulid := pathParts[1]
	
	// Validate entity type
	if !ValidEntityTypes[entityType] {
		return fmt.Errorf("%w: got %q, expected one of: events, places, organizations, persons", ErrInvalidEntityType, entityType)
	}
	
	// Validate ULID format
	if !ulidPattern.MatchString(ulid) {
		return fmt.Errorf("%w: must be 26 character Crockford Base32, got: %s", ErrInvalidULID, ulid)
	}
	
	return nil
}

// ValidateNodeDomain validates a federation node domain per SEL Core Profile §1.1
func ValidateNodeDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("%w: empty domain", ErrInvalidNodeDomain)
	}
	
	// Remove port if present
	if idx := strings.LastIndex(domain, ":"); idx > 0 {
		domain = domain[:idx]
	}
	
	// Validate domain format
	if !domainPattern.MatchString(domain) {
		return fmt.Errorf("%w: invalid format: %s", ErrInvalidNodeDomain, domain)
	}
	
	// Disallow localhost and IP addresses in production federation URIs
	if strings.Contains(domain, "localhost") || strings.Contains(domain, "127.0.0.1") || strings.Contains(domain, "::1") {
		return fmt.Errorf("%w: localhost not allowed in federation URIs: %s", ErrInvalidNodeDomain, domain)
	}
	
	return nil
}

// ExtractOriginDomain extracts the origin domain from a federation URI
func ExtractOriginDomain(federationURI string) (string, error) {
	parsedURL, err := url.Parse(federationURI)
	if err != nil {
		return "", fmt.Errorf("parse federation URI: %w", err)
	}
	
	if parsedURL.Host == "" {
		return "", fmt.Errorf("missing host in federation URI: %s", federationURI)
	}
	
	return parsedURL.Host, nil
}

// ValidateFederatedEventData validates that a federated event has required provenance data
// per SEL Core Profile §1.3 (Federation Rule - Preservation of Origin)
func ValidateFederatedEventData(federationURI string, originNodeID string) error {
	if federationURI == "" {
		return errors.New("federation_uri is required for federated events")
	}
	
	if err := ValidateFederationURI(federationURI); err != nil {
		return fmt.Errorf("invalid federation_uri: %w", err)
	}
	
	if originNodeID == "" {
		return errors.New("origin_node_id is required for federated events")
	}
	
	// Extract domain from federation URI
	originDomain, err := ExtractOriginDomain(federationURI)
	if err != nil {
		return fmt.Errorf("extract origin domain: %w", err)
	}
	
	// Validate domain format
	if err := ValidateNodeDomain(originDomain); err != nil {
		return fmt.Errorf("invalid origin domain in URI: %w", err)
	}
	
	return nil
}

// ValidateSameAsLinks validates sameAs links per SEL Core Profile §1.4
func ValidateSameAsLinks(sameAsLinks []string) error {
	// Define known authority patterns per §1.4
	knownAuthorities := map[string]*regexp.Regexp{
		"artsdata":      regexp.MustCompile(`^http://kg\.artsdata\.ca/resource/K\d+-\d+$`),
		"wikidata":      regexp.MustCompile(`^http://www\.wikidata\.org/entity/Q\d+$`),
		"musicbrainz":   regexp.MustCompile(`^https://musicbrainz\.org/[a-z]+/[a-f0-9-]+$`),
		"isni":          regexp.MustCompile(`^https://isni\.org/isni/\d{16}$`),
		"openstreetmap": regexp.MustCompile(`^https://www\.openstreetmap\.org/(node|way|relation)/\d+$`),
	}
	
	for _, link := range sameAsLinks {
		if link == "" {
			continue // Skip empty links
		}
		
		// Parse URL
		parsedURL, err := url.Parse(link)
		if err != nil {
			return fmt.Errorf("invalid sameAs URL %q: %w", link, err)
		}
		
		// Validate scheme
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return fmt.Errorf("invalid sameAs URL scheme %q: must be http or https", link)
		}
		
		// Check against known authorities
		matched := false
		for _, pattern := range knownAuthorities {
			if pattern.MatchString(link) {
				matched = true
				break
			}
		}
		
		if !matched {
			// Allow other authorities but warn (in production, log this)
			// This is permissive to allow expansion to new knowledge graphs
			continue
		}
	}
	
	return nil
}

// ValidateOriginPreservation ensures federation sync operations preserve origin URIs
// per SEL Core Profile §1.3
func ValidateOriginPreservation(submittedID string, storedFederationURI string) error {
	if submittedID != storedFederationURI {
		return fmt.Errorf("federation URI mismatch: submitted %q, stored %q (URIs must be preserved exactly)", submittedID, storedFederationURI)
	}
	return nil
}
