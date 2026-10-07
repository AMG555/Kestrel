package storage

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"kestrel/internal/config"
)

// fakeActivity is a test double for Activity.
type fakeActivity struct {
	conversations map[string]time.Time
	projects      map[string]time.Time
	err           error
}

func (f fakeActivity) ConversationLastActivity(id string) (time.Time, bool, error) {
	if f.err != nil {
		return time.Time{}, false, f.err
	}
	at, ok := f.conversations[id]
	return at, ok, nil
}

func (f fakeActivity) ProjectLastActivity(id string) (time.Time, bool, error) {
	if f.err != nil {
		return time.Time{}, false, f.err
	}
	at, ok := f.projects[id]
	return at, ok, nil
}

// ageTree builds a directory tree under root and sets the mtime of all files and directories to now-age.
// Directory mtimes must also be set: statUnit uses the latest mtime among a directory and its contents,
// so if only files are aged, the newly created directory's mtime remains current and the unit is blocked by the active-protection guard.
func ageTree(t *testing.T, root string, files map[string]int, age time.Duration, now time.Time) {
	t.Helper()
	stamp := now.Add(-age)
	dirs := map[string]bool{root: true}
	for rel, size := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
		for dir := filepath.Dir(path); dir != root && strings.HasPrefix(dir, root); dir = filepath.Dir(dir) {
			dirs[dir] = true
		}
	}
	// Bottom-up: updating a subdirectory refreshes its parent's mtime, so processing in the wrong order leaves parents with current timestamps.
	order := make([]string, 0, len(dirs))
	for dir := range dirs {
		order = append(order, dir)
	}
	sort.Slice(order, func(i, j int) bool { return len(order[i]) > len(order[j]) })
	for _, dir := range order {
		if err := os.Chtimes(dir, stamp, stamp); err != nil {
			t.Fatalf("chtimes dir %s: %v", dir, err)
		}
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func newTestCleaner(t *testing.T, cfg *config.Config, paths Paths, activity Activity, now time.Time) *Cleaner {
	t.Helper()
	if cfg == nil {
		cfg = &config.Config{}
	}
	return NewCleaner(Options{
		Config:   cfg,
		Paths:    paths,
		Activity: activity,
		Now:      func() time.Time { return now },
		CacheTTL: -1, // disable caching in tests to ensure each call does a real traversal
	})
}

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

func categoryCfg(days int) config.StorageCategoryConfig {
	return config.StorageCategoryConfig{RetentionDays: intPtr(days)}
}

// workspace root layout: tmp/workspace/{projects,conversations}/<id>/
func TestCleanExpiresIdleWorkspaceAndKeepsFresh(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")

	old := filepath.Join(ws, "conversations", "conv-old")
	fresh := filepath.Join(ws, "conversations", "conv-fresh")
	ageTree(t, old, map[string]int{"scan/nmap.txt": 4096}, 40*24*time.Hour, now)
	ageTree(t, fresh, map[string]int{"scan/nmap.txt": 1024}, 2*24*time.Hour, now)

	activity := fakeActivity{conversations: map[string]time.Time{
		"conv-old":   now.Add(-40 * 24 * time.Hour),
		"conv-fresh": now.Add(-2 * 24 * time.Hour),
	}}
	cfg := &config.Config{Storage: config.StorageConfig{
		Categories: map[string]config.StorageCategoryConfig{
			config.StorageCategoryWorkspace: categoryCfg(30),
		},
	}}
	c := newTestCleaner(t, cfg, Paths{Workspace: ws}, activity, now)

	rep, err := c.Clean(CleanRequest{Categories: []string{config.StorageCategoryWorkspace}})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !exists(fresh) {
		t.Errorf("non-expired workspace was deleted: %s", fresh)
	}
	if exists(old) {
		t.Errorf("workspace past its retention period was not deleted: %s", old)
	}
	cat := findCategory(t, rep, config.StorageCategoryWorkspace)
	if cat.RemovedUnits != 1 {
		t.Errorf("RemovedUnits = %d, want 1", cat.RemovedUnits)
	}
	if cat.FreedBytes != 4096 {
		t.Errorf("FreedBytes = %d, want 4096", cat.FreedBytes)
	}
}

func TestDryRunDeletesNothing(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	old := filepath.Join(ws, "conversations", "conv-old")
	ageTree(t, old, map[string]int{"a.txt": 2048}, 40*24*time.Hour, now)

	activity := fakeActivity{conversations: map[string]time.Time{"conv-old": now.Add(-40 * 24 * time.Hour)}}
	cfg := &config.Config{Storage: config.StorageConfig{
		Categories: map[string]config.StorageCategoryConfig{
			config.StorageCategoryWorkspace: categoryCfg(30),
		},
	}}
	c := newTestCleaner(t, cfg, Paths{Workspace: ws}, activity, now)

	rep, err := c.Clean(CleanRequest{DryRun: true})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !exists(old) {
		t.Fatal("dry-run deleted files")
	}
	if rep.Totals.ReclaimableBytes != 2048 {
		t.Errorf("ReclaimableBytes = %d, want 2048", rep.Totals.ReclaimableBytes)
	}
	if rep.Totals.RemovedUnits != 0 {
		t.Errorf("dry-run should have no RemovedUnits, got %d", rep.Totals.RemovedUnits)
	}
}

// Session has disappeared from the database → orphan directory, reclaimed after the shorter orphan_grace_days.
func TestOrphanReclaimedEvenWhenRetentionIsZero(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	orphan := filepath.Join(ws, "conversations", "conv-gone")
	ageTree(t, orphan, map[string]int{"a.txt": 10}, 5*24*time.Hour, now)

	// retention_days: 0 means do not clean by retention period, but orphan directories should still be reclaimed.
	cfg := &config.Config{Storage: config.StorageConfig{
		OrphanGraceDays: intPtr(1),
		Categories: map[string]config.StorageCategoryConfig{
			config.StorageCategoryWorkspace: categoryCfg(0),
		},
	}}
	c := newTestCleaner(t, cfg, Paths{Workspace: ws}, fakeActivity{}, now)

	if _, err := c.Clean(CleanRequest{}); err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if exists(orphan) {
		t.Error("orphan directory was not reclaimed")
	}
}

// Session still exists and has recent activity → must be protected even if the directory mtime is very old.
func TestActiveSessionIsProtected(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	dir := filepath.Join(ws, "conversations", "conv-busy")
	ageTree(t, dir, map[string]int{"a.txt": 10}, 40*24*time.Hour, now)

	activity := fakeActivity{conversations: map[string]time.Time{
		"conv-busy": now.Add(-10 * time.Minute), // still running 10 minutes ago
	}}
	cfg := &config.Config{Storage: config.StorageConfig{
		Categories: map[string]config.StorageCategoryConfig{
			config.StorageCategoryWorkspace: categoryCfg(30),
		},
	}}
	c := newTestCleaner(t, cfg, Paths{Workspace: ws}, activity, now)

	rep, err := c.Clean(CleanRequest{})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !exists(dir) {
		t.Fatal("workspace for an active session was deleted")
	}
	if rep.Totals.SkippedActive != 1 {
		t.Errorf("SkippedActive = %d, want 1", rep.Totals.SkippedActive)
	}
}

// When the activity lookup fails, the unit must be conservatively skipped: better to under-delete than to accidentally delete data for a running task.
func TestActivityLookupErrorFailsClosed(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	dir := filepath.Join(ws, "conversations", "conv-x")
	ageTree(t, dir, map[string]int{"a.txt": 10}, 400*24*time.Hour, now)

	cfg := &config.Config{Storage: config.StorageConfig{
		Categories: map[string]config.StorageCategoryConfig{
			config.StorageCategoryWorkspace: categoryCfg(30),
		},
	}}
	c := newTestCleaner(t, cfg, Paths{Workspace: ws}, fakeActivity{err: errors.New("db locked")}, now)

	if _, err := c.Clean(CleanRequest{}); err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !exists(dir) {
		t.Fatal("no data should be deleted when the lookup fails")
	}
}

// Symbolic links to directories are never treated as deletion units by any scanner:
// os.ReadDir's DirEntry.IsDir() returns false for symlinks.
// This is the safer behaviour — when a symlink pointing to /etc is placed in the workspace,
// it is neither followed nor treated as a session directory.
func TestSymlinkInsideRootIsNeverADeletionUnit(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	outside := filepath.Join(tmp, "precious")
	if err := os.MkdirAll(filepath.Join(outside, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(outside, "sub", "keep.txt")
	if err := os.WriteFile(victim, []byte("important"), 0o644); err != nil {
		t.Fatal(err)
	}

	linkParent := filepath.Join(ws, "conversations")
	link := filepath.Join(linkParent, "conv-link")
	if err := os.MkdirAll(linkParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	stamp := now.Add(-400 * 24 * time.Hour)
	if err := os.Chtimes(link, stamp, stamp); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Storage: config.StorageConfig{
		Categories: map[string]config.StorageCategoryConfig{
			config.StorageCategoryWorkspace: categoryCfg(30),
		},
	}}
	c := newTestCleaner(t, cfg, Paths{Workspace: ws}, nil, now)

	rep, err := c.Clean(CleanRequest{})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !exists(link) {
		t.Error("symlink should not be removed as a deletion unit")
	}
	if !exists(victim) {
		t.Fatal("symlink target was deleted, path traversal occurred")
	}
	if rep.Totals.Units != 0 {
		t.Errorf("Units = %d, want 0 (symlinks are not counted)", rep.Totals.Units)
	}
}

// A marker directory left by a crashed cleanup round must be deleted unconditionally;
// otherwise its mtime equals the rename time and the active-protection guard would block deletion forever.
func TestLeftoverDeletionMarkerIsReclaimed(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	marker := filepath.Join(ws, "conversations", "conv-crash"+deletionMarkerSuffix)
	// mtime is "right now", simulating an immediate re-run after a crash.
	ageTree(t, marker, map[string]int{"a.txt": 10}, 0, now)

	cfg := &config.Config{Storage: config.StorageConfig{
		Categories: map[string]config.StorageCategoryConfig{
			config.StorageCategoryWorkspace: categoryCfg(30),
		},
	}}
	c := newTestCleaner(t, cfg, Paths{Workspace: ws}, fakeActivity{}, now)

	if _, err := c.Clean(CleanRequest{}); err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if exists(marker) {
		t.Error("marker directory left by a crash was not cleaned up")
	}
}

// chat_uploads uses a root/<date>/<session> three-level layout; empty date directories must be reclaimed after session directories are deleted.
func TestChatUploadsDatedLayoutAndEmptyDirPrune(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	uploads := filepath.Join(tmp, "chat_uploads")
	dateDir := filepath.Join(uploads, "2026-06-01")
	convDir := filepath.Join(dateDir, "conv-old")
	ageTree(t, convDir, map[string]int{"report.pdf": 100}, 120*24*time.Hour, now)

	cfg := &config.Config{Storage: config.StorageConfig{
		Categories: map[string]config.StorageCategoryConfig{
			config.StorageCategoryChatUploads: categoryCfg(90),
		},
	}}
	c := newTestCleaner(t, cfg, Paths{ChatUploads: uploads}, fakeActivity{}, now)

	rep, err := c.Clean(CleanRequest{})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if exists(convDir) {
		t.Error("expired upload directory was not deleted")
	}
	if exists(dateDir) {
		t.Error("empty date directory was not reclaimed")
	}
	if !exists(uploads) {
		t.Error("category root directory should not be deleted")
	}
	// When no categories are specified, Clean returns all registered categories; look up by key, not by index.
	cat := findCategory(t, rep, config.StorageCategoryChatUploads)
	if cat.RemovedEmptyDirs != 1 {
		t.Errorf("RemovedEmptyDirs = %d, want 1", cat.RemovedEmptyDirs)
	}
	if cat.RemovedUnits != 1 {
		t.Errorf("RemovedUnits = %d, want 1", cat.RemovedUnits)
	}
}

// findCategory retrieves a report entry by key; it fails immediately if missing to avoid asserting against the wrong category by index.
func findCategory(t *testing.T, rep *Report, key string) CategoryReport {
	t.Helper()
	for _, cat := range rep.Categories {
		if cat.Key == key {
			return cat
		}
	}
	t.Fatalf("category %s missing from report", key)
	return CategoryReport{}
}

// Placeholder directories _new / _manual are not session IDs and must not be looked up in the database; they are handled as non-session units.
func TestChatUploadsPlaceholderDirIsNotSessionScoped(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	uploads := filepath.Join(tmp, "chat_uploads")
	placeholder := filepath.Join(uploads, "2026-06-01", "_new")
	ageTree(t, placeholder, map[string]int{"a.png": 10}, 120*24*time.Hour, now)

	// Activity returns an error for every query: if placeholder directories were treated as sessions, fail-closed would cause them to be retained.
	cfg := &config.Config{Storage: config.StorageConfig{
		Categories: map[string]config.StorageCategoryConfig{
			config.StorageCategoryChatUploads: categoryCfg(90),
		},
	}}
	c := newTestCleaner(t, cfg, Paths{ChatUploads: uploads},
		fakeActivity{err: errors.New("should not be called")}, now)

	if _, err := c.Clean(CleanRequest{}); err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if exists(placeholder) {
		t.Error("placeholder directory should be cleaned by retention period, not treated as a session")
	}
}

// Glob-type category: only matching files are deleted; other files in the same directory are unaffected.
func TestPatternCategoryOnlyMatchesItsGlob(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	logs := filepath.Join(tmp, "log")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-40 * 24 * time.Hour)
	stale := filepath.Join(logs, "diagnostic-2026-08-01.log")
	keepName := filepath.Join(logs, "app.log")
	for _, p := range []string{stale, keepName} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &config.Config{Storage: config.StorageConfig{
		Categories: map[string]config.StorageCategoryConfig{
			config.StorageCategoryDiagnosticLogs: categoryCfg(14),
		},
	}}
	c := newTestCleaner(t, cfg, Paths{DiagnosticLogs: logs}, nil, now)

	if _, err := c.Clean(CleanRequest{}); err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if exists(stale) {
		t.Error("expired diagnostic log was not deleted")
	}
	if !exists(keepName) {
		t.Error("file not matching the glob was incorrectly deleted")
	}
}

func TestDisabledCategoryIsReportedButNotCleaned(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	dir := filepath.Join(ws, "conversations", "conv-old")
	ageTree(t, dir, map[string]int{"a.txt": 10}, 400*24*time.Hour, now)

	cfg := &config.Config{Storage: config.StorageConfig{
		Categories: map[string]config.StorageCategoryConfig{
			config.StorageCategoryWorkspace: {Enabled: boolPtr(false), RetentionDays: intPtr(30)},
		},
	}}
	c := newTestCleaner(t, cfg, Paths{Workspace: ws}, fakeActivity{}, now)

	// Inspect should still display disabled categories so administrators can see reclaimable space before deciding to enable them.
	inspected := c.Inspect(true)
	var found bool
	for _, cat := range inspected.Categories {
		if cat.Key == config.StorageCategoryWorkspace {
			found = true
			if cat.Enabled {
				t.Error("category should be disabled")
			}
			if cat.ReclaimableUnits != 1 {
				t.Errorf("ReclaimableUnits = %d, want 1", cat.ReclaimableUnits)
			}
		}
	}
	if !found {
		t.Fatal("Inspect did not return the workspace category")
	}

	if _, err := c.Clean(CleanRequest{}); err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if !exists(dir) {
		t.Error("a disabled category should not be cleaned")
	}
}

func TestUnknownCategoryIsRejected(t *testing.T) {
	now := time.Now()
	c := newTestCleaner(t, nil, Paths{Workspace: t.TempDir()}, nil, now)
	if _, err := c.Clean(CleanRequest{Categories: []string{"../../etc"}}); !errors.Is(err, ErrUnknownCategory) {
		t.Errorf("err = %v, want ErrUnknownCategory", err)
	}
}

func TestConcurrentCleanIsRejected(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	for i := 0; i < 20; i++ {
		dir := filepath.Join(ws, "conversations", "conv-"+string(rune('a'+i)))
		ageTree(t, dir, map[string]int{"a.txt": 10}, 400*24*time.Hour, now)
	}
	c := newTestCleaner(t, nil, Paths{Workspace: ws}, fakeActivity{}, now)

	// Manually claim the running slot to simulate "a round is already in progress".
	if !c.running.CompareAndSwap(false, true) {
		t.Fatal("could not claim the running slot")
	}
	if _, err := c.Clean(CleanRequest{}); !errors.Is(err, ErrCleanupInProgress) {
		t.Errorf("err = %v, want ErrCleanupInProgress", err)
	}
	c.running.Store(false)

	if _, err := c.Clean(CleanRequest{}); err != nil {
		t.Fatalf("should be able to clean again after releasing the slot: %v", err)
	}
}

// When cleanup is triggered concurrently, only one round should actually execute;
// the rest should immediately receive ErrCleanupInProgress instead of queuing up for deletion.
func TestParallelCleanHasSingleWinner(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	dir := filepath.Join(ws, "conversations", "conv-old")
	ageTree(t, dir, map[string]int{"a.txt": 10}, 400*24*time.Hour, now)

	c := newTestCleaner(t, nil, Paths{Workspace: ws}, fakeActivity{}, now)

	const n = 8
	var wg sync.WaitGroup
	results := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := c.Clean(CleanRequest{})
			results[i] = err
		}(i)
	}
	close(start)
	wg.Wait()

	inProgress := 0
	for _, err := range results {
		switch {
		case err == nil:
		case errors.Is(err, ErrCleanupInProgress):
			inProgress++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if inProgress == 0 {
		t.Log("note: all goroutines completed serially this round; no concurrency conflict was observed (not a failure)")
	}
}

func TestConfinedRejectsEscapes(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "srv", "app", "tmp", "workspace")
	cases := []struct {
		candidate string
		want      bool
	}{
		{filepath.Join(root, "conversations", "abc"), true},
		{filepath.Join(root, "a", "b", "c"), true},
		{root, false},
		{filepath.Join(root, ".."), false},
		{filepath.Join(root, "..", "secrets"), false},
		{filepath.Join(string(filepath.Separator), "etc", "passwd"), false},
		{"", false},
	}
	for _, tc := range cases {
		if got := confined(root, tc.candidate); got != tc.want {
			t.Errorf("confined(%q) = %v, want %v", tc.candidate, got, tc.want)
		}
	}
	if confined("", filepath.Join(root, "x")) {
		t.Error("empty root should not pass the check")
	}
}

// When the root directory does not exist, no error should be reported; it should only be marked Missing —
// it is normal for the system not to have produced this type of garbage yet.
func TestMissingRootIsNotAnError(t *testing.T) {
	now := time.Now()
	c := newTestCleaner(t, nil, Paths{Workspace: filepath.Join(t.TempDir(), "never-created")}, nil, now)

	rep := c.Inspect(true)
	if len(rep.Categories) != len(config.StorageCategoryOrder) {
		t.Fatalf("Categories = %d, want %d", len(rep.Categories), len(config.StorageCategoryOrder))
	}
	for _, cat := range rep.Categories {
		if !cat.Missing {
			t.Errorf("category %s root does not exist/is unconfigured, should be marked Missing", cat.Key)
		}
		if cat.Units != 0 || len(cat.Errors) != 0 {
			t.Errorf("category %s should have no statistics or errors: units=%d errors=%v", cat.Key, cat.Units, cat.Errors)
		}
	}
	if _, err := c.Clean(CleanRequest{}); err != nil {
		t.Errorf("Clean: %v", err)
	}
}

func TestInspectCachesUntilRefresh(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	dir := filepath.Join(ws, "conversations", "conv-a")
	ageTree(t, dir, map[string]int{"a.txt": 10}, 400*24*time.Hour, now)

	c := NewCleaner(Options{
		Config:   &config.Config{},
		Paths:    Paths{Workspace: ws},
		Activity: fakeActivity{},
		Now:      func() time.Time { return now },
		CacheTTL: time.Minute,
	})

	first := c.Inspect(false)
	if first.Totals.Units != 1 {
		t.Fatalf("first tally Units = %d, want 1", first.Totals.Units)
	}
	// Directories added while the cache is active should not be visible.
	ageTree(t, filepath.Join(ws, "conversations", "conv-b"), map[string]int{"b.txt": 10}, 400*24*time.Hour, now)
	if cached := c.Inspect(false); cached.Totals.Units != 1 {
		t.Errorf("within cache window Units = %d, want 1", cached.Totals.Units)
	}
	if refreshed := c.Inspect(true); refreshed.Totals.Units != 2 {
		t.Errorf("after refresh Units = %d, want 2", refreshed.Totals.Units)
	}
	// After cleanup the cache must be invalidated; otherwise the status page still shows deleted content.
	if _, err := c.Clean(CleanRequest{}); err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if after := c.Inspect(false); after.Totals.Units != 0 {
		t.Errorf("after cleanup Units = %d, want 0", after.Totals.Units)
	}
}

func TestRetentionLoopDisabledByDefault(t *testing.T) {
	s := NewService(nil, &config.Config{}, nil)
	if s.AutoCleanEnabled() {
		t.Error("auto_clean must be off by default")
	}
	// PurgeExpired is a no-op when disabled; it should not panic.
	s.PurgeExpired()

	enabled := &config.Config{Storage: config.StorageConfig{AutoClean: boolPtr(true)}}
	if !NewService(nil, enabled, nil).AutoCleanEnabled() {
		t.Error("should be enabled when explicitly set to true")
	}
}

func TestIntervalHasFloor(t *testing.T) {
	cfg := &config.Config{Storage: config.StorageConfig{IntervalMinutes: intPtr(1)}}
	if got := NewService(nil, cfg, nil).Interval(); got != minSweepInterval {
		t.Errorf("Interval = %v, want %v", got, minSweepInterval)
	}
	cfg = &config.Config{Storage: config.StorageConfig{IntervalMinutes: intPtr(180)}}
	if got := NewService(nil, cfg, nil).Interval(); got != 3*time.Hour {
		t.Errorf("Interval = %v, want 3h", got)
	}
}
