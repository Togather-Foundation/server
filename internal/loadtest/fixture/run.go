// Package fixture defines private ownership capabilities for disposable load-test runs.
package fixture

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/google/uuid"
)

// Manifest is a private, immutable run capability. Never log its OwnershipKey.
type Manifest struct {
	Version      int       `json:"version"`
	RunID        string    `json:"run_id"`
	TargetURL    string    `json:"target_url"`
	SourceURL    string    `json:"source_url"`
	AnchorTime   time.Time `json:"anchor_time"`
	OwnershipKey string    `json:"ownership_key"`
}

func canonicalTarget(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("run target must be an absolute HTTP URL without credentials, query or fragment")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u.String(), nil
}

// New creates a fresh random run; it does not write the manifest or send requests.
func New(targetURL string, now time.Time) (Manifest, error) {
	target, err := canonicalTarget(targetURL)
	if err != nil {
		return Manifest{}, err
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return Manifest{}, fmt.Errorf("generate run ID: %w", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return Manifest{}, fmt.Errorf("generate ownership key: %w", err)
	}
	m := Manifest{Version: 1, RunID: id.String(), TargetURL: target, SourceURL: target + "/_loadtest/runs/" + id.String(), AnchorTime: now.UTC().Truncate(time.Second), OwnershipKey: base64.StdEncoding.EncodeToString(key)}
	return m, m.Validate()
}

// Validate verifies the manifest contract without exposing its secret in errors.
func (m Manifest) Validate() error {
	if m.Version != 1 {
		return fmt.Errorf("unsupported run manifest version")
	}
	id, err := uuid.Parse(m.RunID)
	if err != nil || id.String() != m.RunID || id.Version() != 4 {
		return fmt.Errorf("invalid run ID")
	}
	target, err := canonicalTarget(m.TargetURL)
	if err != nil || target != m.TargetURL {
		return fmt.Errorf("invalid canonical run target")
	}
	if m.SourceURL != target+"/_loadtest/runs/"+m.RunID || events.NormalizeFeedURL(m.SourceURL) != m.SourceURL {
		return fmt.Errorf("invalid run source URL")
	}
	_, offset := m.AnchorTime.Zone()
	if m.AnchorTime.IsZero() || offset != 0 || m.AnchorTime.Nanosecond() != 0 || m.AnchorTime.Year() < 1 || m.AnchorTime.Year() > 9998 {
		return fmt.Errorf("invalid UTC run anchor")
	}
	key, err := base64.StdEncoding.Strict().DecodeString(m.OwnershipKey)
	if err != nil || len(key) != 32 || base64.StdEncoding.EncodeToString(key) != m.OwnershipKey {
		return fmt.Errorf("invalid run ownership key")
	}
	return nil
}

// Create exclusively creates a private manifest before any fixture can be sent.
func Create(path string, m Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("run manifest path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create manifest directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create private run manifest: %w", err)
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if err := json.NewEncoder(f).Encode(m); err != nil {
		return fmt.Errorf("write run manifest: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync run manifest: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close run manifest: %w", err)
	}
	ok = true
	return nil
}

// Read accepts only a private regular file and strict versioned JSON.
func Read(path string) (Manifest, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("inspect run manifest: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return Manifest{}, fmt.Errorf("run manifest must be a private regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("open run manifest: %w", err)
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm()&0077 != 0 || opened.Size() > 64*1024 {
		return Manifest{}, fmt.Errorf("run manifest changed while opening")
	}
	var m Manifest
	decoder := json.NewDecoder(io.LimitReader(f, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("invalid run manifest JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Manifest{}, fmt.Errorf("run manifest must contain exactly one JSON object")
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func (m Manifest) eventID(sequence uint64) string {
	prefix := "loadtest:" + m.RunID + ":" + strconv.FormatUint(sequence, 10)
	key, _ := base64.StdEncoding.DecodeString(m.OwnershipKey)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(prefix))
	return prefix + ":" + hex.EncodeToString(mac.Sum(nil))
}

// Event deterministically builds an isolated virtual fixture for a positive sequence.
func (m Manifest) Event(sequence uint64) (events.EventInput, error) {
	if err := m.Validate(); err != nil {
		return events.EventInput{}, err
	}
	if sequence == 0 {
		return events.EventInput{}, fmt.Errorf("fixture sequence must be positive")
	}
	suffix := strconv.FormatUint(sequence, 10)
	start := m.AnchorTime.AddDate(0, 0, 7)
	return events.EventInput{
		Name:        "Load test " + m.RunID + " event " + suffix,
		Description: "Disposable virtual event generated for an explicitly owned load-test run. This fixture measures ingestion and must be removed with its private run manifest after testing.",
		StartDate:   start.Format(time.RFC3339), EndDate: start.Add(time.Hour).Format(time.RFC3339),
		VirtualLocation: &events.VirtualLocationInput{Type: "VirtualLocation", Name: "Load-test virtual venue", URL: m.SourceURL + "/virtual/" + suffix},
		Image:           m.SourceURL + "/images/" + suffix + ".jpg",
		URL:             m.SourceURL + "/events/" + suffix, License: "https://creativecommons.org/publicdomain/zero/1.0/",
		Source: &events.SourceInput{URL: m.SourceURL, EventID: m.eventID(sequence), Name: "Load test " + m.RunID},
	}, nil
}

// Verify proves the signature and every normalized input field, refusing altered fixtures.
func (m Manifest) Verify(input events.EventInput) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if input.Source == nil {
		return fmt.Errorf("fixture source is required")
	}
	parts := strings.Split(input.Source.EventID, ":")
	if len(parts) != 4 || parts[0] != "loadtest" || parts[1] != m.RunID {
		return fmt.Errorf("fixture does not belong to this run")
	}
	sequence, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil || sequence == 0 || strconv.FormatUint(sequence, 10) != parts[2] {
		return fmt.Errorf("invalid fixture sequence")
	}
	expectedID := m.eventID(sequence)
	if !hmac.Equal([]byte(expectedID), []byte(input.Source.EventID)) {
		return fmt.Errorf("invalid fixture ownership signature")
	}
	expected, err := m.Event(sequence)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(events.NormalizeEventInput(input), events.NormalizeEventInput(expected)) {
		return fmt.Errorf("fixture payload has been modified")
	}
	return nil
}
