package scraper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserAgentConstants_NonEmpty(t *testing.T) {
	t.Parallel()

	assert.NotEmpty(t, ScraperUserAgent, "ScraperUserAgent must not be empty")
	assert.NotEmpty(t, ICSUserAgent, "ICSUserAgent must not be empty")
	assert.NotEqual(t, ScraperUserAgent, ICSUserAgent, "ScraperUserAgent and ICSUserAgent must be distinct")
}
