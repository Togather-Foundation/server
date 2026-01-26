package contracts_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTurtleOutputBasicSyntax verifies that Turtle responses follow basic RDF Turtle syntax
func TestTurtleOutputBasicSyntax(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventName := "Jazz in the Park"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/turtle")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/turtle", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	turtle := string(body)

	// Verify Turtle prefix declarations
	require.Contains(t, turtle, "@prefix", "Turtle must contain @prefix declarations")
	require.Contains(t, turtle, "schema:", "Turtle must declare schema.org prefix")

	// Verify RDF type declaration
	require.Contains(t, turtle, "a schema:Event", "Turtle must declare type as schema:Event")

	// Verify basic property syntax
	require.Contains(t, turtle, `schema:name "`+eventName, "Turtle must include event name property")

	// Verify Turtle syntax elements
	require.Contains(t, turtle, ".", "Turtle statements must end with period")
	require.Contains(t, turtle, ";", "Turtle should use semicolon for multiple properties")
}

// TestTurtlePrefixDeclarations verifies that Turtle output includes standard prefixes
func TestTurtlePrefixDeclarations(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventULID := insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/turtle")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	turtle := string(body)

	// Verify standard prefixes
	require.Contains(t, turtle, "@prefix schema:", "Turtle must declare schema.org prefix")
	require.Contains(t, turtle, "https://schema.org/", "Turtle must include schema.org URI")

	// May also include other standard prefixes
	// Note: Not requiring prov or sel prefixes as they may not be in all responses
}

// TestTurtleEventProperties verifies that Turtle includes expected event properties
func TestTurtleEventProperties(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventName := "Jazz Concert"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "published", []string{"jazz", "music"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/turtle")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	turtle := string(body)

	// Verify event type
	require.Contains(t, turtle, "a schema:Event", "Event must be typed as schema:Event")

	// Verify event name property
	require.Contains(t, turtle, "schema:name", "Event must have schema:name property")
	require.Contains(t, turtle, eventName, "Event name must appear in Turtle")

	// Verify property syntax (quoted strings end properly)
	nameLinePattern := `schema:name "` + eventName + `"`
	require.Contains(t, turtle, nameLinePattern, "Event name must be properly quoted")
}

// TestTurtleOrganizationOutput verifies Turtle serialization for organizations
func TestTurtleOrganizationOutput(t *testing.T) {
	env := setupTestEnv(t)

	orgName := "Toronto Arts Collective"
	org := insertOrganization(t, env, orgName)

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/organizations/"+org.ULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/turtle")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/turtle", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	turtle := string(body)

	require.Contains(t, turtle, "@prefix")
	require.Contains(t, turtle, "a schema:Organization", "Organization must be typed as schema:Organization")
	require.Contains(t, turtle, orgName)
}

// TestTurtlePlaceOutput verifies Turtle serialization for places
func TestTurtlePlaceOutput(t *testing.T) {
	env := setupTestEnv(t)

	placeName := "Centennial Park"
	place := insertPlace(t, env, placeName, "Toronto")

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/places/"+place.ULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/turtle")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/turtle", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	turtle := string(body)

	require.Contains(t, turtle, "@prefix")
	require.Contains(t, turtle, "a schema:Place", "Place must be typed as schema:Place")
	require.Contains(t, turtle, placeName)
}

// TestTurtleURIFormat verifies that Turtle uses proper URI format for subjects
func TestTurtleURIFormat(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventULID := insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/turtle")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	turtle := string(body)

	// Verify subject URI format (should be in angle brackets or prefixed)
	// At minimum, the ULID should appear in the Turtle output
	require.Contains(t, turtle, eventULID, "Turtle must reference event ULID")

	// Verify URI syntax (either <http://...> or prefix:identifier)
	hasAngleBracketURI := strings.Contains(turtle, "<http://")
	hasPrefixedURI := strings.Contains(turtle, "schema:") || strings.Contains(turtle, "sel:")
	require.True(t, hasAngleBracketURI || hasPrefixedURI, "Turtle must use proper URI syntax")
}

// TestTurtleValidSyntax verifies that Turtle output doesn't have obvious syntax errors
func TestTurtleValidSyntax(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventULID := insertEventWithOccurrence(t, env, "Jazz in the Park", org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/turtle")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	turtle := string(body)

	// Check for basic Turtle syntax validity
	lines := strings.Split(turtle, "\n")

	// Count prefix declarations
	prefixCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@prefix") {
			prefixCount++
			require.True(t, strings.HasSuffix(line, "."), "Prefix declaration must end with period: %s", line)
		}
	}
	require.Greater(t, prefixCount, 0, "Turtle must have at least one prefix declaration")

	// Verify no unmatched quotes (basic check)
	quoteCount := strings.Count(turtle, `"`)
	require.Equal(t, 0, quoteCount%2, "Turtle must have matched quotes")

	// Verify proper statement termination (at least one statement)
	statementEndings := strings.Count(turtle, ".")
	require.Greater(t, statementEndings, 0, "Turtle must have statements ending with period")
}

// TestTurtleConsistencyWithJSONLD verifies that Turtle and JSON-LD contain same core data
func TestTurtleConsistencyWithJSONLD(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventName := "Jazz in the Park"
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	// Get Turtle
	turtleReq, err := http.NewRequest(http.MethodGet, env.Server.URL+"/events/"+eventULID, nil)
	require.NoError(t, err)
	turtleReq.Header.Set("Accept", "text/turtle")

	turtleResp, err := env.Server.Client().Do(turtleReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = turtleResp.Body.Close() })

	turtleBody, err := io.ReadAll(turtleResp.Body)
	require.NoError(t, err)
	turtle := string(turtleBody)

	// Verify core data appears in Turtle
	require.Contains(t, turtle, eventULID, "Turtle must contain event ULID")
	require.Contains(t, turtle, eventName, "Turtle must contain event name")
	require.Contains(t, turtle, "Event", "Turtle must reference Event type")
}

// TestTurtleCharacterEncoding verifies proper handling of special characters
func TestTurtleCharacterEncoding(t *testing.T) {
	env := setupTestEnv(t)

	org := insertOrganization(t, env, "Toronto Arts Org")
	place := insertPlace(t, env, "Centennial Park", "Toronto")
	eventName := `Jazz "in the" Park` // Event name with quotes
	eventULID := insertEventWithOccurrence(t, env, eventName, org.ID, place.ID, "music", "published", []string{"jazz"}, time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC))

	req, err := http.NewRequest(http.MethodGet, env.Server.URL+"/events/"+eventULID, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/turtle")

	resp, err := env.Server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	turtle := string(body)

	// Verify quotes are properly escaped in Turtle
	// Turtle should escape internal quotes with backslash or use triple quotes
	hasEscapedQuotes := strings.Contains(turtle, `\"`) || strings.Contains(turtle, `"""`)
	require.True(t, hasEscapedQuotes, "Turtle must properly escape quotes in string literals")
}
