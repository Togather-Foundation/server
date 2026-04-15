package cmd

import (
	"testing"
)

func TestScrapeFixtureCmd_Flags(t *testing.T) {
	for _, flag := range []string{"extraction-method", "tier", "trust-level", "source-name"} {
		if f := scrapeFixtureCmd.Flags().Lookup(flag); f == nil {
			t.Errorf("expected flag %q on test-fixture command", flag)
		}
	}
}

func TestScrapeFixtureCmd_Args(t *testing.T) {
	if err := scrapeFixtureCmd.Args(scrapeFixtureCmd, nil); err == nil {
		t.Error("expected error for no args")
	}
	if err := scrapeFixtureCmd.Args(scrapeFixtureCmd, []string{"path.ics"}); err != nil {
		t.Errorf("expected no error for single arg, got: %v", err)
	}
	if err := scrapeFixtureCmd.Args(scrapeFixtureCmd, []string{"a", "b"}); err == nil {
		t.Error("expected error for two args")
	}
}
