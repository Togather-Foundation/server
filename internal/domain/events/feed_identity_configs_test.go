package events

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// sharedFeedAllowlist names the feed identities that several source configs may
// share on purpose. Feed identity drops the query string (see feed_identity.go),
// so a config whose URL differs from another only by `?types=` / `?programs=` /
// `?audiences=` / `?entity=` is the SAME feed and its events dedupe together.
//
// That is intentional where the configs are filtered views over one upstream
// that hands out globally unique event ids, and the views really do overlap:
// treating them as separate feeds would duplicate the events they share.
//
// Any other collision is a mistake: either the config was meant to be a separate
// feed and needs a distinct path, or the collapse is deliberate and belongs here
// with a reason.
var sharedFeedAllowlist = map[string]sharedFeed{
	"https://gateway.bibliocommons.com/v2/libraries/tpl/rss/events": {
		Reason: "TPL BiblioCommons: one library catalogue, five filtered views " +
			"(types/programs query params). 5,981 of 6,188 TPL events are returned by " +
			"more than one view; BiblioCommons ids are global.",
		Configs: []string{
			"tpl-artificial-intelligence",
			"tpl-career-coaches-in-residence",
			"tpl-career-job-search",
			"tpl-events",
			"tpl-innovator-in-residence",
		},
	},
	"https://api.luma.com/ics/get": {
		Reason: "Lu.ma: a discovery-place feed and a specific calendar over one API. " +
			"Lu.ma ids are global (evt-<id>@events.lu.ma) and the two views already " +
			"share events.",
		Configs: []string{
			"luma-toronto",
			"trajectory-labs",
		},
	},
}

type sharedFeed struct {
	Reason  string
	Configs []string
}

func eventsRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate the repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// TestSourceConfigs_FeedIdentityCollisions makes feed-identity collapse explicit.
//
// Ingest identifies an event by (feed URL, sourceEventId), and feed URL is the
// validated `scheme://host/path` form — query and fragment are stripped. Two
// configs that differ only by query therefore share one identity, so the same
// event id from both resolves to one event. That is correct for filtered views of
// one catalogue and wrong for genuinely separate feeds. This test is the tripwire:
// it fails when a new config silently joins a feed identity, and when the
// allowlist goes stale.
func TestSourceConfigs_FeedIdentityCollisions(t *testing.T) {
	configDir := filepath.Join(eventsRepoRoot(t), "configs", "sources")
	files, err := filepath.Glob(filepath.Join(configDir, "*.yaml"))
	if err != nil {
		t.Fatalf("glob %s: %v", configDir, err)
	}
	if len(files) == 0 {
		t.Fatalf("no source configs found under %s", configDir)
	}

	type sourceConfig struct {
		Name string `yaml:"name"`
		URL  string `yaml:"url"`
	}

	configsByIdentity := map[string][]string{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		var cfg sourceConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		if strings.TrimSpace(cfg.Name) == "" || strings.TrimSpace(cfg.URL) == "" {
			continue
		}
		// Same normalisation ingest applies to source.url, plus the trailing-slash
		// tolerance of the identity lookup.
		identity := strings.TrimRight(normalizeURL(strings.TrimSpace(cfg.URL)), "/")
		configsByIdentity[identity] = append(configsByIdentity[identity], cfg.Name)
	}

	allowlistUsed := map[string]bool{}
	var problems []string

	for identity, names := range configsByIdentity {
		if len(names) < 2 {
			continue
		}
		sort.Strings(names)
		entry, ok := sharedFeedAllowlist[identity]
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"%s is the feed identity of %d configs (%s) and has no allowlist entry.\n"+
					"     Feed identity drops the query string, so these configs dedupe together.",
				identity, len(names), strings.Join(names, ", ")))
			continue
		}
		allowlistUsed[identity] = true
		sortedAllowed := append([]string(nil), entry.Configs...)
		sort.Strings(sortedAllowed)
		if strings.Join(names, ",") != strings.Join(sortedAllowed, ",") {
			problems = append(problems, fmt.Sprintf(
				"%s is shared by (%s) but the allowlist lists (%s); update one of them.",
				identity, strings.Join(names, ", "), strings.Join(sortedAllowed, ", ")))
			continue
		}
		if strings.TrimSpace(entry.Reason) == "" {
			problems = append(problems, fmt.Sprintf(
				"allowlist entry %s has no Reason; say why the collapse is intentional.", identity))
		}
	}

	for identity := range sharedFeedAllowlist {
		if !allowlistUsed[identity] {
			problems = append(problems, fmt.Sprintf(
				"allowlist entry %s no longer collides; remove it so the list cannot rot.", identity))
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("source config feed identities collide:\n  - %s\n\n"+
			"Fix: give the new config a distinct path if it is a separate feed, or add the pair to "+
			"sharedFeedAllowlist in %s if the collapse is intentional.",
			strings.Join(problems, "\n  - "), "internal/domain/events/feed_identity_configs_test.go")
	}
}
