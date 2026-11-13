package fixture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

func testManifest(t *testing.T) Manifest {
	t.Helper()
	m, err := New("https://staging.example.org/base/", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestManifestFixturesArePrivateDeterministicAndIsolated(t *testing.T) {
	m := testManifest(t)
	other := testManifest(t)
	if m.RunID == other.RunID || m.SourceURL == other.SourceURL {
		t.Fatal("runs share identity")
	}
	for _, n := range []uint64{1, 2, ^uint64(0)} {
		input, err := m.Event(n)
		if err != nil {
			t.Fatal(err)
		}
		if input.Location != nil || input.Organizer != nil || input.VirtualLocation == nil || !strings.HasPrefix(input.Image, m.SourceURL) || input.License != "https://creativecommons.org/publicdomain/zero/1.0/" {
			t.Fatal("fixture affects shared entities or lacks CC0")
		}
		if err := m.Verify(input); err != nil {
			t.Fatal(err)
		}
		if err := m.Verify(events.NormalizeEventInput(input)); err != nil {
			t.Fatal(err)
		}
		if err := other.Verify(input); err == nil {
			t.Fatal("foreign run accepted")
		}
		wire, _ := json.Marshal(input)
		if strings.Contains(string(wire), m.OwnershipKey) {
			t.Fatal("secret sent over HTTP")
		}
		input.Description = "modified by operator"
		if m.Verify(input) == nil {
			t.Fatal("altered payload accepted")
		}
	}
	if _, err := m.Event(0); err == nil {
		t.Fatal("zero sequence accepted")
	}
}

func TestManifestRejectsInvalidSignaturesAndSequences(t *testing.T) {
	m := testManifest(t)
	for _, id := range []string{"loadtest:" + m.RunID + ":0:abc", "loadtest:" + m.RunID + ":01:abc", "loadtest:" + m.RunID + ":+1:abc", "loadtest:" + m.RunID + ":18446744073709551616:abc", "loadtest:" + m.RunID + ":1:abc"} {
		input, _ := m.Event(1)
		input.Source.EventID = id
		if m.Verify(input) == nil {
			t.Fatalf("invalid ID accepted: %s", id)
		}
	}
}

func TestManifestExclusivePrivateStrictFile(t *testing.T) {
	m := testManifest(t)
	path := filepath.Join(t.TempDir(), "private", "run.json")
	if err := Create(path, m); err != nil {
		t.Fatal(err)
	}
	if err := Create(path, m); err == nil {
		t.Fatal("existing manifest overwritten")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("manifest not private")
	}
	got, err := Read(path)
	if err != nil || got.RunID != m.RunID {
		t.Fatalf("read: %v", err)
	}
	raw, _ := json.Marshal(m)
	for name, contents := range map[string]string{"unknown": strings.TrimSuffix(string(raw), "}") + `,"extra":true}`, "trailing": string(raw) + "{}", "oversize": string(raw) + strings.Repeat(" ", 64*1024) + "{}"} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Read(path); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("public manifest accepted")
	}
	link := filepath.Join(t.TempDir(), "link.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(link); err == nil {
		t.Fatal("symlink manifest accepted")
	}
}

func TestManifestRejectsMalformedContract(t *testing.T) {
	for _, target := range []string{"https://user:secret@host.org", "https://host.org/?x=1", "https://host.org/#fragment", "ftp://host.org", "/relative"} {
		if _, err := New(target, time.Now()); err == nil {
			t.Fatal("invalid target accepted")
		}
	}
	m := testManifest(t)
	for _, mutate := range []func(*Manifest){func(m *Manifest) { m.Version = 2 }, func(m *Manifest) { m.RunID = "bad" }, func(m *Manifest) { m.SourceURL += "/else" }, func(m *Manifest) { m.OwnershipKey = "secret" }, func(m *Manifest) { m.AnchorTime = time.Time{} }} {
		altered := m
		mutate(&altered)
		if altered.Validate() == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
}
