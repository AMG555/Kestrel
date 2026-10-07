// Package storage enumerates, evaluates, and cleans up disk garbage produced by the workspace.
//
// Design constraints (deleting active task data is far more costly than saving disk space,
// so the following are all hard constraints):
//   - Each category enumerates deletion units only within fixed root directories at fixed depths; no unbounded recursive deletion.
//   - Every deletion unit must pass the confined check to prevent path traversal.
//   - Directory cleanup uses "atomic rename + RemoveAll"; a process crash leaves only a marked remnant that the next round cleans up.
//   - Session-scoped units are checked against the database for recent activity before deletion; active sessions are always skipped; lookup failures cause a conservative skip (fail closed).
package storage

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"kestrel/internal/config"
)

// deletionMarkerSuffix marks a directory that has been judged for deletion and is being removed.
// Follows the Prometheus TSDB convention: rename is atomic, so a crash never leaves the original
// directory name in a partially-deleted state.
const deletionMarkerSuffix = ".tmp-for-deletion"

// Scope describes the binding relationship between a deletion unit and a session or project.
type Scope int

const (
	// ScopeNone has no session association; units are judged only by retention period (logs, checkpoints, C2 artifacts).
	ScopeNone Scope = iota
	// ScopeConversation means the directory name is the session ID; the session's existence and last activity time can be queried.
	ScopeConversation
	// ScopeProject means the directory name is the project ID.
	ScopeProject
)

// Paths aggregates root directories for each category.
// Injected by the app layer using existing resolution rules (the same values as those used by database.SetEinoConversationDirs),
// preventing this package from independently deriving paths that could diverge from the actual write locations.
type Paths struct {
	Workspace            string
	Reduction            string
	ConversationArtifact string
	Plantask             string
	C2                   string
	ChatUploads          string
	WorkflowCheckpoints  string
	DiagnosticLogs       string
}

// Unit is an independently deletable cleanup unit (a directory or file).
type Unit struct {
	Path    string
	Session string // the corresponding ID when Scope is not ScopeNone, otherwise empty
	Scope   Scope
	IsDir   bool
	ModTime time.Time
	Size    int64
}

// Age returns the idle duration of the unit relative to now; future timestamps (clock anomalies) are treated as 0.
func (u Unit) Age(now time.Time) time.Duration {
	if u.ModTime.IsZero() {
		return 0
	}
	if age := now.Sub(u.ModTime); age > 0 {
		return age
	}
	return 0
}

// scanner enumerates all deletion units under a given root directory.
type scanner func(root string) ([]Unit, error)

// category is a named cleanup task, analogous to Gitea's [cron.*] subtask model.
type category struct {
	key        string
	label      string
	hint       string
	root       func(Paths) string
	scan       scanner
	pruneEmpty bool // reclaim empty parent directories after deleting units (the date layer for chat_uploads)
}

// CategoryInfo is the static metadata for a cleanup category, used by the API and frontend.
type CategoryInfo struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
}

// DescribeCategories returns metadata for all categories in registry order.
func DescribeCategories() []CategoryInfo {
	all := categories()
	out := make([]CategoryInfo, 0, len(all))
	for _, cat := range all {
		out = append(out, CategoryInfo{Key: cat.key, Label: cat.label, Hint: cat.hint})
	}
	return out
}

// categories returns all categories in a fixed order; categories with an empty root are skipped by the caller.
func categories() []category {
	return []category{
		{
			key:   config.StorageCategoryWorkspace,
			label: "Agent workspace",
			hint:  "Working directory for agent downloads and scan artifacts (tmp/workspace), organised by project/session.",
			root:  func(p Paths) string { return p.Workspace },
			scan:  scanScopedDirs,
		},
		{
			key:   config.StorageCategoryReduction,
			label: "Tool output cache",
			hint:  "Truncated files for oversized tool outputs written to disk (tmp/reduction); one file per execution, purely derived data.",
			root:  func(p Paths) string { return p.Reduction },
			scan:  scanScopedDirs,
		},
		{
			key:   config.StorageCategoryConversationArtifact,
			label: "Conversation artifacts",
			hint:  "Summary records and oversized user-input ledger files (data/conversation_artifacts).",
			root:  func(p Paths) string { return p.ConversationArtifact },
			scan:  func(root string) ([]Unit, error) { return scanSessionDirs(root, ScopeConversation) },
		},
		{
			key:   config.StorageCategoryPlantask,
			label: "Plan-task board",
			hint:  "Eino multi-agent plan board JSON (skills/.eino/plantask); purely derived data.",
			root:  func(p Paths) string { return p.Plantask },
			scan:  func(root string) ([]Unit, error) { return scanSessionDirs(root, ScopeConversation) },
		},
		{
			key:   config.StorageCategoryC2Artifacts,
			label: "C2 artifacts",
			hint:  "C2 callback screenshots, uploads, downstream files, and generated payload binaries (tmp/c2). Cleaning invalidates the download links for the corresponding payloads.",
			root:  func(p Paths) string { return p.C2 },
			scan: func(root string) ([]Unit, error) {
				return scanSubdirFiles(root, "results", "uploads", "downstream", "payloads")
			},
		},
		{
			key:        config.StorageCategoryChatUploads,
			label:      "Chat upload files",
			hint:       "Attachments uploaded by users during conversations (chat_uploads/date/session).",
			root:       func(p Paths) string { return p.ChatUploads },
			scan:       scanDatedSessionDirs,
			pruneEmpty: true,
		},
		{
			key:   config.StorageCategoryWorkflowCheckpoints,
			label: "Workflow checkpoints",
			hint:  "Workflow run checkpoint files (data/workflow-checkpoints), used only to resume interrupted runs.",
			root:  func(p Paths) string { return p.WorkflowCheckpoints },
			scan: func(root string) ([]Unit, error) {
				return scanPatternFiles(root, "*.ckpt", "*.ckpt.tmp")
			},
		},
		{
			key:   config.StorageCategoryDiagnosticLogs,
			label: "Diagnostic logs",
			hint:  "Daily-rotated diagnostic log files (log/diagnostic-*.log).",
			root:  func(p Paths) string { return p.DiagnosticLogs },
			scan: func(root string) ([]Unit, error) {
				return scanPatternFiles(root, "diagnostic-*.log")
			},
		},
	}
}

// chatUploadsPlaceholderConvs contains placeholder directory names used when no session ID is available at upload time.
// They must not be treated as session IDs for database lookups; they are handled as non-session units (retention-period only).
var chatUploadsPlaceholderConvs = map[string]bool{"_new": true, "_manual": true}

// scanScopedDirs enumerates root/projects/<id> and root/conversations/<id>.
func scanScopedDirs(root string) ([]Unit, error) {
	scopes := []struct {
		dir   string
		scope Scope
	}{{"projects", ScopeProject}, {"conversations", ScopeConversation}}

	var units []Unit
	for _, s := range scopes {
		base := filepath.Join(root, s.dir)
		entries, err := os.ReadDir(base)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			u, err := statUnit(filepath.Join(base, e.Name()), e.Name(), s.scope)
			if err != nil {
				continue
			}
			units = append(units, u)
		}
	}
	return units, nil
}

// scanSessionDirs enumerates root/<id>, where the directory name is the session ID.
func scanSessionDirs(root string, scope Scope) ([]Unit, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var units []Unit
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		u, err := statUnit(filepath.Join(root, e.Name()), e.Name(), scope)
		if err != nil {
			continue
		}
		units = append(units, u)
	}
	return units, nil
}

// scanDatedSessionDirs enumerates root/<YYYY-MM-DD>/<session-ID|placeholder>.
func scanDatedSessionDirs(root string) ([]Unit, error) {
	dates, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var units []Unit
	for _, d := range dates {
		if !d.IsDir() {
			continue
		}
		dateDir := filepath.Join(root, d.Name())
		convs, err := os.ReadDir(dateDir)
		if err != nil {
			continue
		}
		for _, cd := range convs {
			if !cd.IsDir() {
				// Loose files directly under the date directory: clean as non-session units.
				if u, err := statUnit(filepath.Join(dateDir, cd.Name()), "", ScopeNone); err == nil {
					units = append(units, u)
				}
				continue
			}
			session, scope := cd.Name(), ScopeConversation
			if chatUploadsPlaceholderConvs[session] {
				session, scope = "", ScopeNone
			}
			if u, err := statUnit(filepath.Join(dateDir, cd.Name()), session, scope); err == nil {
				units = append(units, u)
			}
		}
	}
	return units, nil
}

// scanSubdirFiles enumerates files (non-recursively) under root/<subdir>/, used for C2 artifacts.
func scanSubdirFiles(root string, subdirs ...string) ([]Unit, error) {
	var units []Unit
	for _, sub := range subdirs {
		base := filepath.Join(root, sub)
		entries, err := os.ReadDir(base)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if u, err := statUnit(filepath.Join(base, e.Name()), "", ScopeNone); err == nil {
				units = append(units, u)
			}
		}
	}
	return units, nil
}

// scanPatternFiles enumerates files under root that match any of the given glob patterns.
func scanPatternFiles(root string, patterns ...string) ([]Unit, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var units []Unit
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		matched := false
		for _, p := range patterns {
			if ok, _ := filepath.Match(p, e.Name()); ok {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if u, err := statUnit(filepath.Join(root, e.Name()), "", ScopeNone); err == nil {
			units = append(units, u)
		}
	}
	return units, nil
}

// statUnit collects the size and most-recent modification time of a unit.
// Uses Lstat: symbolic links are measured as the link itself and never followed to the target
// (following would include or even delete data outside the workspace).
func statUnit(path, session string, scope Scope) (Unit, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Unit{}, err
	}
	u := Unit{
		Path:    path,
		Session: session,
		Scope:   scope,
		IsDir:   info.IsDir(),
		ModTime: info.ModTime(),
		Size:    info.Size(),
	}
	if u.IsDir {
		size, newest := dirStats(path)
		u.Size = size
		if newest.After(u.ModTime) {
			u.ModTime = newest
		}
	}
	if scope == ScopeNone {
		u.Session = ""
	}
	return u, nil
}

// dirStats sums the bytes used by a directory and finds its newest modification time.
// filepath.WalkDir does not follow symbolic links, and unreadable subtrees are skipped
// on a best-effort basis without interrupting the overall tally.
func dirStats(root string) (int64, time.Time) {
	var size int64
	var newest time.Time
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		if !d.IsDir() {
			size += info.Size()
		}
		if mt := info.ModTime(); mt.After(newest) {
			newest = mt
		}
		return nil
	})
	return size, newest
}

// confined verifies that candidate is truly inside root, preventing any form of path traversal.
func confined(root, candidate string) bool {
	if root == "" || candidate == "" {
		return false
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." || filepath.IsAbs(rel) {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// pruneEmptyDirs reclaims empty directories bottom-up, descending at most depth levels.
// After deleting session directories under chat_uploads, empty date directories remain
// and must be reclaimed as well. Only genuinely empty descendant directories are removed;
// root itself is never deleted.
func pruneEmptyDirs(root string, depth int) int {
	if depth <= 0 {
		return 0
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	removed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(root, e.Name())
		removed += pruneEmptyDirs(sub, depth-1)
		if isEmptyDir(sub) && os.Remove(sub) == nil {
			removed++
		}
	}
	return removed
}

func isEmptyDir(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	return len(entries) == 0
}
