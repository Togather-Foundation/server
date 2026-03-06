package scraper

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestSafeClient_CopiesTransportAndTimeout(t *testing.T) {
	transport := &http.Transport{}
	orig := &http.Client{
		Transport: transport,
		Timeout:   42 * time.Second,
	}

	got := safeClient(orig, blockRedirects)

	if got == orig {
		t.Fatal("safeClient returned the same pointer; expected a copy")
	}
	if got.Transport != transport {
		t.Error("Transport not copied")
	}
	if got.Timeout != 42*time.Second {
		t.Errorf("Timeout not copied: got %v, want %v", got.Timeout, 42*time.Second)
	}
}

func TestSafeClient_CopiesJar(t *testing.T) {
	jar := &fakeJar{}
	orig := &http.Client{Jar: jar}

	got := safeClient(orig, blockRedirects)

	if got.Jar != jar {
		t.Error("Jar not copied")
	}
}

func TestSafeClient_NilClientReturnsValidClient(t *testing.T) {
	got := safeClient(nil, blockRedirects)

	if got == nil {
		t.Fatal("safeClient returned nil for nil input")
	}
	// Verify it has the redirect policy set.
	err := got.CheckRedirect(nil, nil)
	if !errors.Is(err, http.ErrUseLastResponse) {
		t.Errorf("expected ErrUseLastResponse, got %v", err)
	}
}

func TestSafeClient_AppliesCheckRedirect(t *testing.T) {
	orig := &http.Client{Timeout: 5 * time.Second}

	got := safeClient(orig, blockRedirects)

	if got.CheckRedirect == nil {
		t.Fatal("CheckRedirect not set")
	}
	err := got.CheckRedirect(nil, nil)
	if !errors.Is(err, http.ErrUseLastResponse) {
		t.Errorf("blockRedirects: expected ErrUseLastResponse, got %v", err)
	}
}

func TestSafeClient_DoesNotMutateOriginalCheckRedirect(t *testing.T) {
	orig := &http.Client{}
	if orig.CheckRedirect != nil {
		t.Fatal("precondition: orig.CheckRedirect should be nil")
	}

	_ = safeClient(orig, blockRedirects)

	if orig.CheckRedirect != nil {
		t.Error("safeClient mutated the original client's CheckRedirect")
	}
}

func TestLimitRedirects_AllowsUpToMax(t *testing.T) {
	policy := limitRedirects(3)

	// 2 prior requests → below limit → allow.
	via := make([]*http.Request, 2)
	if err := policy(nil, via); err != nil {
		t.Errorf("expected nil err for len(via)=2, max=3; got %v", err)
	}
}

func TestLimitRedirects_BlocksAtMax(t *testing.T) {
	policy := limitRedirects(3)

	// 3 prior requests → at limit → block.
	via := make([]*http.Request, 3)
	err := policy(nil, via)
	if !errors.Is(err, http.ErrUseLastResponse) {
		t.Errorf("expected ErrUseLastResponse for len(via)=3, max=3; got %v", err)
	}
}

func TestSafeClient_WithLimitRedirects(t *testing.T) {
	orig := &http.Client{Timeout: 5 * time.Second}
	got := safeClient(orig, limitRedirects(10))

	if got.CheckRedirect == nil {
		t.Fatal("CheckRedirect not set")
	}
	// Below limit → no error.
	via := make([]*http.Request, 9)
	if err := got.CheckRedirect(nil, via); err != nil {
		t.Errorf("expected nil for 9 prior redirects, got %v", err)
	}
	// At limit → block.
	via = make([]*http.Request, 10)
	if err := got.CheckRedirect(nil, via); !errors.Is(err, http.ErrUseLastResponse) {
		t.Errorf("expected ErrUseLastResponse at limit, got %v", err)
	}
}

// fakeJar is a minimal http.CookieJar implementation for testing.
type fakeJar struct{}

func (f *fakeJar) SetCookies(_ *url.URL, _ []*http.Cookie) {}
func (f *fakeJar) Cookies(_ *url.URL) []*http.Cookie       { return nil }
