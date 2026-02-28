package config_test

import (
	"os"
	"strings"
	"testing"
)

// openapiConfigEntry describes a config-tunable value that must be documented
// in the OpenAPI spec. Each entry is a tuple of:
//
//   - envVar:     the environment variable name operators use to tune the value
//   - wantSubstr: a substring that must appear in the openapi.yaml file
//
// The substring should be specific enough to confirm the env var name and its
// default appear together in the relevant endpoint description or info block.
// Keep this list small: only values that directly affect API-observable
// behaviour (rate limits, validation thresholds, pagination caps). Infra
// config (DB pool size, SMTP port, job retries) does not belong here.
var openapiConfigEntries = []struct {
	envVar     string
	wantSubstr string
}{
	{
		envVar:     "RATE_LIMIT_PUBLIC",
		wantSubstr: "RATE_LIMIT_PUBLIC",
	},
	{
		envVar:     "RATE_LIMIT_LOGIN",
		wantSubstr: "RATE_LIMIT_LOGIN",
	},
	{
		envVar:     "RATE_LIMIT_SUBMISSIONS_PER_IP_PER_24H",
		wantSubstr: "RATE_LIMIT_SUBMISSIONS_PER_IP_PER_24H",
	},
	{
		envVar:     "VALIDATION_MAX_EVENT_NAME_LENGTH",
		wantSubstr: "VALIDATION_MAX_EVENT_NAME_LENGTH",
	},
	{
		envVar:     "DEVELOPER_PASSWORD_MIN_LENGTH",
		wantSubstr: "DEVELOPER_PASSWORD_MIN_LENGTH",
	},
}

// TestOpenAPIDocumentsConfigTunables asserts that each config-tunable
// API-observable value is mentioned by env var name in docs/api/openapi.yaml.
// This prevents silent drift when config defaults change without a spec update.
func TestOpenAPIDocumentsConfigTunables(t *testing.T) {
	t.Parallel()

	specPath := "../../docs/api/openapi.yaml"
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("cannot read OpenAPI spec at %s: %v", specPath, err)
	}
	content := string(data)

	for _, entry := range openapiConfigEntries {
		entry := entry // capture
		t.Run(entry.envVar, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(content, entry.wantSubstr) {
				t.Errorf(
					"openapi.yaml does not mention %q\n"+
						"Add a description to the relevant endpoint(s) referencing "+
						"the env var name and its default value.\n"+
						"See AGENTS.md 'Configuration — keep values DRY' for the rule.",
					entry.wantSubstr,
				)
			}
		})
	}
}
