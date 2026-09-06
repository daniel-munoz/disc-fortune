package discogs

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newRetryTestClient returns a client whose sleeps are recorded instead of
// served, so backoff can be asserted without the test actually waiting.
func newRetryTestClient(handler http.Handler) (*Client, *httptest.Server, *[]time.Duration) {
	srv := httptest.NewServer(handler)
	var slept []time.Duration
	c := &Client{
		token:      "test-token",
		httpClient: srv.Client(),
		sleep:      func(d time.Duration) { slept = append(slept, d) },
	}
	return c, srv, &slept
}

// TestUserAgentHeaderMatchesConstructorArgument replaces the deleted
// TestUserAgentCarriesCurrentVersion (which pinned the header against the
// root's `version` constant, no longer visible from this package). It stays
// inside package discogs, asserting the header actually sent on the wire
// against the string the caller handed to New -- the half of the drift guard
// that version_test.go's TestDiscogsUserAgentCarriesTheCurrentVersion cannot
// cover, since that test only inspects Client.UserAgent() and never performs
// a request.
func TestUserAgentHeaderMatchesConstructorArgument(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		fmt.Fprint(w, "{}")
	}))
	defer srv.Close()

	const want = "disc-fortune/test-agent"
	client := &Client{token: "test-token", userAgent: want, httpClient: srv.Client()}
	if _, err := client.get(srv.URL); err != nil {
		t.Fatalf("get: %v", err)
	}

	if got != want {
		t.Errorf("User-Agent header = %q, want %q", got, want)
	}
}

func TestGetRetriesAfterRateLimit(t *testing.T) {
	var calls int32
	srv := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, "you are making requests too quickly")
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	})

	client, s, slept := newRetryTestClient(srv)
	defer s.Close()

	body, err := client.get(s.URL)
	if err != nil {
		t.Fatalf("get after a 429 should succeed, got: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q", body)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
	if len(*slept) != 1 {
		t.Fatalf("slept %d times, want 1", len(*slept))
	}
	if (*slept)[0] < baseBackoff {
		t.Errorf("backoff = %v, want at least %v", (*slept)[0], baseBackoff)
	}
}

func TestGetRetriesAfterServerError(t *testing.T) {
	var calls int32
	srv := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	})

	client, s, _ := newRetryTestClient(srv)
	defer s.Close()

	if _, err := client.get(s.URL); err != nil {
		t.Fatalf("get after a 502 should succeed, got: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestGetGivesUpOnPersistentRateLimit(t *testing.T) {
	var calls int32
	srv := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, "rate limited")
	})

	client, s, slept := newRetryTestClient(srv)
	defer s.Close()

	_, err := client.get(s.URL)
	if err == nil {
		t.Fatal("a permanently rate-limited endpoint must still fail")
	}
	if int(calls) != maxAttempts {
		t.Errorf("calls = %d, want maxAttempts (%d)", calls, maxAttempts)
	}
	var total time.Duration
	for _, d := range *slept {
		total += d
	}
	if total > maxTotalBackoff {
		t.Errorf("total backoff %v exceeds the %v cap", total, maxTotalBackoff)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should name the status it gave up on: %v", err)
	}
}

func TestGetBacksOffExponentially(t *testing.T) {
	srv := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	client, s, slept := newRetryTestClient(srv)
	defer s.Close()
	_, _ = client.get(s.URL)

	if len(*slept) != maxAttempts-1 {
		t.Fatalf("slept %d times, want %d", len(*slept), maxAttempts-1)
	}
	for i := 1; i < len(*slept); i++ {
		if (*slept)[i] <= (*slept)[i-1] {
			t.Errorf("backoff did not grow: %v", *slept)
			break
		}
	}
}

func TestGetHonorsRetryAfter(t *testing.T) {
	var calls int32
	srv := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	})

	client, s, slept := newRetryTestClient(srv)
	defer s.Close()

	if _, err := client.get(s.URL); err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(*slept) != 1 {
		t.Fatalf("slept %d times, want 1", len(*slept))
	}
	if (*slept)[0] < 7*time.Second {
		t.Errorf("Retry-After: 7 ignored; slept %v", (*slept)[0])
	}
}

func TestGetDoesNotRetryClientErrors(t *testing.T) {
	var calls int32
	srv := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
	})

	client, s, slept := newRetryTestClient(srv)
	defer s.Close()

	if _, err := client.get(s.URL); err == nil {
		t.Fatal("a 404 must fail")
	}
	if calls != 1 {
		t.Errorf("calls = %d; a 404 is not worth retrying", calls)
	}
	if len(*slept) != 0 {
		t.Errorf("slept %v on a non-retryable status", *slept)
	}
}

// TestNilProgressIsSilent moved here from the root's progress_test.go: report
// is unexported, so only a package-discogs test can call it directly.
func TestNilProgressIsSilent(t *testing.T) {
	c := &Client{}
	c.report("this must not panic %d\n", 1) // Progress is nil
}
