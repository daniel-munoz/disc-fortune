package disc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadMetaMissingFileIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")

	m, err := LoadMeta(path)
	if err != nil {
		t.Fatalf("LoadMeta on a missing file: %v", err)
	}
	if !m.SyncedAt.IsZero() {
		t.Errorf("SyncedAt = %v, want zero", m.SyncedAt)
	}
}

func TestRecordSyncRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	when := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)

	if err := RecordSync(path, when); err != nil {
		t.Fatalf("RecordSync: %v", err)
	}

	m, err := LoadMeta(path)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if !m.SyncedAt.Equal(when) {
		t.Errorf("SyncedAt = %v, want %v", m.SyncedAt, when)
	}
}

func TestRecordSyncOverwritesEarlierTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	second := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)

	if err := RecordSync(path, first); err != nil {
		t.Fatalf("first RecordSync: %v", err)
	}
	if err := RecordSync(path, second); err != nil {
		t.Fatalf("second RecordSync: %v", err)
	}

	m, _ := LoadMeta(path)
	if !m.SyncedAt.Equal(second) {
		t.Errorf("SyncedAt = %v, want the later %v", m.SyncedAt, second)
	}
}

// A corrupt meta.json is advisory data, not collection data. It must not be
// able to fail a sync that otherwise succeeded.
func TestRecordSyncRepairsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	if err := os.WriteFile(path, []byte("{not json"), FilePerms); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	when := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)

	if err := RecordSync(path, when); err != nil {
		t.Fatalf("RecordSync over a corrupt file: %v", err)
	}
	m, err := LoadMeta(path)
	if err != nil {
		t.Fatalf("LoadMeta after repair: %v", err)
	}
	if !m.SyncedAt.Equal(when) {
		t.Errorf("SyncedAt = %v, want %v", m.SyncedAt, when)
	}
}

func TestStaleNoticeSilentWhenFresh(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	m := Meta{SyncedAt: now.Add(-24 * time.Hour)}

	if got := staleNotice(m, now); got != "" {
		t.Errorf("staleNotice = %q, want empty for a one-day-old sync", got)
	}
}

func TestStaleNoticeSilentWhenNeverSynced(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)

	if got := staleNotice(Meta{}, now); got != "" {
		t.Errorf("staleNotice = %q; a user who never synced is already told so elsewhere", got)
	}
}

func TestStaleNoticeNudgesWhenOld(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	m := Meta{SyncedAt: now.Add(-staleAfter - time.Hour)}

	got := staleNotice(m, now)
	if got == "" {
		t.Fatal("staleNotice was empty for a collection older than staleAfter")
	}
	if !strings.Contains(got, "sync") {
		t.Errorf("staleNotice = %q, want it to name the sync command", got)
	}
}

func TestSyncNoticeSuppressedWhenDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	if err := RecordSync(path, now.Add(-staleAfter-time.Hour)); err != nil {
		t.Fatalf("RecordSync: %v", err)
	}

	if got := SyncNotice(path, now, false); got != "" {
		t.Errorf("SyncNotice = %q, want empty when notices are off", got)
	}
}

func TestSyncNoticeReportsStaleCollection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	if err := RecordSync(path, now.Add(-staleAfter-time.Hour)); err != nil {
		t.Fatalf("RecordSync: %v", err)
	}

	if got := SyncNotice(path, now, true); got == "" {
		t.Error("SyncNotice was empty for a stale collection")
	}
}

// A pick must succeed even if meta.json is unreadable garbage.
func TestSyncNoticeSilentOnUnreadableMeta(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	if err := os.WriteFile(path, []byte("{not json"), FilePerms); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if got := SyncNotice(path, time.Now(), true); got != "" {
		t.Errorf("SyncNotice = %q, want empty when metadata cannot be read", got)
	}
}
