package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"kestrel/internal/config"

	"go.uber.org/zap"
)

// ErrCleanupInProgress indicates that a cleanup round is already running; handlers should map this to 409.
var ErrCleanupInProgress = errors.New("a storage cleanup round is already in progress")

// ErrUnknownCategory indicates that the request contains an unregistered category key.
var ErrUnknownCategory = errors.New("unknown cleanup category")

// Decision reason constants, used for report and log interpretability.
const (
	reasonExpired  = "expired"  // retention period exceeded
	reasonOrphan   = "orphan"   // session/project deleted, directory left behind
	reasonLeftover = "leftover" // marker directory left by a crashed cleanup round
	reasonActive   = "active"   // recently active, protected
	reasonRecent   = "recent"   // orphan grace period not yet elapsed
	reasonKept     = "kept"     // not expired, or category retention_days is 0
	reasonUnknown  = "unknown"  // activity lookup failed, conservatively skipped
)

// Activity queries the most recent activity time for a session or project. When the implementation
// returns an error, the cleanup conservatively skips the unit.
type Activity interface {
	ConversationLastActivity(id string) (time.Time, bool, error)
	ProjectLastActivity(id string) (time.Time, bool, error)
}

// CleanRequest describes a single cleanup request.
type CleanRequest struct {
	// DryRun, when true, only counts reclaimable space without deleting anything.
	DryRun bool `json:"dry_run"`
	// Categories, when empty, selects all enabled categories.
	Categories []string `json:"categories"`
	// Trigger is one of manual / schedule and is used only for auditing and logging.
	Trigger string `json:"trigger"`
}

// CategoryReport holds the statistics and execution results for a single category.
type CategoryReport struct {
	Key           string `json:"key"`
	Label         string `json:"label"`
	Hint          string `json:"hint"`
	Root          string `json:"root"`
	Enabled       bool   `json:"enabled"`
	RetentionDays int    `json:"retention_days"`
	// Missing indicates the root directory has not been created yet (the system has not yet produced this type of garbage).
	Missing bool `json:"missing"`

	Units int   `json:"units"`
	Bytes int64 `json:"bytes"`

	ReclaimableUnits int   `json:"reclaimable_units"`
	ReclaimableBytes int64 `json:"reclaimable_bytes"`
	OrphanUnits      int   `json:"orphan_units"`
	SkippedActive    int   `json:"skipped_active"`
	SkippedUnsafe    int   `json:"skipped_unsafe"`

	RemovedUnits     int   `json:"removed_units"`
	FreedBytes       int64 `json:"freed_bytes"`
	RemovedEmptyDirs int   `json:"removed_empty_dirs"`

	Errors []string `json:"errors,omitempty"`
}

// Totals is the aggregate across all categories.
type Totals struct {
	Units            int   `json:"units"`
	Bytes            int64 `json:"bytes"`
	ReclaimableUnits int   `json:"reclaimable_units"`
	ReclaimableBytes int64 `json:"reclaimable_bytes"`
	RemovedUnits     int   `json:"removed_units"`
	FreedBytes       int64 `json:"freed_bytes"`
	SkippedActive    int   `json:"skipped_active"`
	Errors           int   `json:"errors"`
}

// Report is the complete result of an inspection or cleanup run.
type Report struct {
	DryRun     bool             `json:"dry_run"`
	Trigger    string           `json:"trigger"`
	StartedAt  time.Time        `json:"started_at"`
	DurationMS int64            `json:"duration_ms"`
	Filesystem Filesystem       `json:"filesystem"`
	Categories []CategoryReport `json:"categories"`
	Totals     Totals           `json:"totals"`
	Note       string           `json:"note,omitempty"`
}

// defaultCacheTTL limits the directory-traversal frequency for the status endpoint;
// a single full walk over a large workspace can take several seconds.
const defaultCacheTTL = time.Minute

// Options holds the dependencies required to construct a Cleaner.
type Options struct {
	Config   *config.Config
	Paths    Paths
	Activity Activity
	Logger   *zap.Logger
	// Now allows tests to inject a fixed clock; defaults to time.Now when omitted.
	Now func() time.Time
	// CacheTTL uses defaultCacheTTL when omitted; <=0 disables caching.
	CacheTTL time.Duration
}

// Cleaner enumerates, evaluates, and deletes workspace garbage.
type Cleaner struct {
	cfg      *config.Config
	paths    Paths
	activity Activity
	logger   *zap.Logger
	now      func() time.Time
	cacheTTL time.Duration

	// running ensures only one cleanup round runs at a time, preventing two administrators
	// from triggering simultaneous "clean now" operations that would interfere with each other.
	running atomic.Bool

	mu       sync.Mutex
	cached   *Report
	cachedAt time.Time
}

// NewCleaner creates a Cleaner. When cfg is nil, a zero-value config is used (equivalent to all default policies).
func NewCleaner(opts Options) *Cleaner {
	c := &Cleaner{
		cfg:      opts.Config,
		paths:    opts.Paths,
		activity: opts.Activity,
		logger:   opts.Logger,
		now:      opts.Now,
		cacheTTL: opts.CacheTTL,
	}
	if c.cfg == nil {
		c.cfg = &config.Config{}
	}
	if c.now == nil {
		c.now = time.Now
	}
	if opts.CacheTTL == 0 {
		c.cacheTTL = defaultCacheTTL
	} else if opts.CacheTTL < 0 {
		c.cacheTTL = 0
	}
	return c
}

// storageConfig returns the currently effective storage policy (read on demand, so PUT /api/config takes effect immediately).
func (c *Cleaner) storageConfig() config.StorageConfig {
	if c.cfg == nil {
		return config.StorageConfig{}
	}
	return c.cfg.Storage
}

// rootOf returns the absolute path of the category root directory; returns an empty string if unconfigured.
func (c *Cleaner) rootOf(cat category) string {
	root := strings.TrimSpace(cat.root(c.paths))
	if root == "" {
		return ""
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	return filepath.Clean(abs)
}

// selectCategories returns categories to process in registry order, ensuring stable report ordering.
// When onlyEnabled is true, explicitly disabled categories are skipped.
func (c *Cleaner) selectCategories(keys []string, onlyEnabled bool) ([]category, error) {
	all := categories()
	if len(keys) == 0 {
		out := make([]category, 0, len(all))
		for _, cat := range all {
			if onlyEnabled && !c.storageConfig().CategoryEnabled(cat.key) {
				continue
			}
			out = append(out, cat)
		}
		return out, nil
	}

	wanted := make(map[string]bool, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		wanted[k] = true
	}
	known := make(map[string]bool, len(all))
	for _, cat := range all {
		known[cat.key] = true
	}
	for k := range wanted {
		if !known[k] {
			return nil, fmt.Errorf("%w: %s", ErrUnknownCategory, k)
		}
	}
	out := make([]category, 0, len(wanted))
	for _, cat := range all {
		if !wanted[cat.key] {
			continue
		}
		if onlyEnabled && !c.storageConfig().CategoryEnabled(cat.key) {
			continue
		}
		out = append(out, cat)
	}
	return out, nil
}

// scanResult is the output of a single category scan: a report plus the list of eligible units.
type scanResult struct {
	report   CategoryReport
	eligible []Unit
}

// scan enumerates all units in a category and evaluates each one, producing both statistics and a deletion list.
func (c *Cleaner) scan(cat category, now time.Time) scanResult {
	st := c.storageConfig()
	rep := CategoryReport{
		Key:           cat.key,
		Label:         cat.label,
		Hint:          cat.hint,
		Root:          c.rootOf(cat),
		Enabled:       st.CategoryEnabled(cat.key),
		RetentionDays: st.CategoryRetentionDays(cat.key),
	}
	res := scanResult{report: rep}
	if rep.Root == "" {
		rep.Missing = true
		res.report = rep
		return res
	}
	if _, err := os.Stat(rep.Root); err != nil {
		rep.Missing = true
		res.report = rep
		return res
	}

	units, err := cat.scan(rep.Root)
	if err != nil {
		rep.Errors = append(rep.Errors, err.Error())
		res.report = rep
		return res
	}

	for _, u := range units {
		rep.Units++
		rep.Bytes += u.Size
		// Defense in depth: scanners only produce paths under root; assert this again here.
		if !confined(rep.Root, u.Path) {
			rep.SkippedUnsafe++
			continue
		}
		d := c.evaluate(u, rep.RetentionDays, now)
		if !d.eligible {
			if d.reason == reasonActive || d.reason == reasonUnknown {
				rep.SkippedActive++
			}
			continue
		}
		rep.ReclaimableUnits++
		rep.ReclaimableBytes += u.Size
		if d.reason == reasonOrphan {
			rep.OrphanUnits++
		}
		res.eligible = append(res.eligible, u)
	}
	res.report = rep
	return res
}

// decision is the evaluation result for a single unit.
type decision struct {
	eligible bool
	reason   string
}

// evaluate determines whether a unit is eligible for deletion.
// Priority: crash-leftover marker > recent-activity protection > session existence > retention period.
func (c *Cleaner) evaluate(u Unit, retentionDays int, now time.Time) decision {
	// A marker directory left by a mid-run crash in the previous cleanup round: always delete unconditionally.
	// Must be checked first; otherwise its mtime equals the rename time and the active-protection guard
	// would block deletion indefinitely.
	if strings.HasSuffix(filepath.Base(u.Path), deletionMarkerSuffix) {
		return decision{eligible: true, reason: reasonLeftover}
	}

	st := c.storageConfig()
	activeGrace := time.Duration(st.ActiveGraceHoursEffective()) * time.Hour
	age := u.Age(now)
	if age < activeGrace {
		return decision{reason: reasonActive}
	}

	retention := time.Duration(retentionDays) * 24 * time.Hour

	if u.Scope != ScopeNone && u.Session != "" && c.activity != nil {
		last, exists, err := c.lastActivity(u)
		if err != nil {
			// Cannot determine active status: conservatively skip — better to under-delete than to
			// accidentally delete data for a running task.
			return decision{reason: reasonUnknown}
		}
		if exists {
			if now.Sub(last) < activeGrace {
				return decision{reason: reasonActive}
			}
			if retentionDays > 0 && age >= retention {
				return decision{eligible: true, reason: reasonExpired}
			}
			return decision{reason: reasonKept}
		}
		// Session/project no longer exists → orphan directory; reclaim after the shorter orphan grace period.
		if age >= time.Duration(st.OrphanGraceDaysEffective())*24*time.Hour {
			return decision{eligible: true, reason: reasonOrphan}
		}
		return decision{reason: reasonRecent}
	}

	// retention_days: 0 means do not clean by retention period (follows the existing project convention).
	if retentionDays <= 0 {
		return decision{reason: reasonKept}
	}
	if age >= retention {
		return decision{eligible: true, reason: reasonExpired}
	}
	return decision{reason: reasonKept}
}

func (c *Cleaner) lastActivity(u Unit) (time.Time, bool, error) {
	switch u.Scope {
	case ScopeConversation:
		return c.activity.ConversationLastActivity(u.Session)
	case ScopeProject:
		return c.activity.ProjectLastActivity(u.Session)
	default:
		return time.Time{}, false, nil
	}
}

// Inspect tallies usage and reclaimable space for all categories without deleting anything.
// Results are cached per CacheTTL; pass refresh=true to force recomputation.
func (c *Cleaner) Inspect(refresh bool) *Report {
	c.mu.Lock()
	if !refresh && c.cacheTTL > 0 && c.cached != nil && c.now().Sub(c.cachedAt) < c.cacheTTL {
		cached := c.cached
		c.mu.Unlock()
		return cached
	}
	c.mu.Unlock()

	rep := c.buildReport(CleanRequest{DryRun: true, Trigger: "inspect"}, true)

	c.mu.Lock()
	c.cached = rep
	c.cachedAt = c.now()
	c.mu.Unlock()
	return rep
}

// invalidateCache forces the next Inspect call to re-traverse the filesystem.
func (c *Cleaner) invalidateCache() {
	c.mu.Lock()
	c.cached = nil
	c.mu.Unlock()
}

// Clean performs a cleanup run; when DryRun is true it only counts.
// Only enabled categories are processed, and only one round may run at a time.
func (c *Cleaner) Clean(req CleanRequest) (*Report, error) {
	if !c.running.CompareAndSwap(false, true) {
		return nil, ErrCleanupInProgress
	}
	defer c.running.Store(false)

	req.Trigger = strings.TrimSpace(req.Trigger)
	if req.Trigger == "" {
		req.Trigger = "manual"
	}
	if _, err := c.selectCategories(req.Categories, false); err != nil {
		return nil, err
	}
	rep := c.buildReport(req, false)
	c.invalidateCache()

	if !req.DryRun {
		c.logClean(rep, req)
	}
	return rep, nil
}

// buildReport is the shared implementation for Inspect and Clean.
// When inspectAll is true, all categories (including disabled ones) are tallied for the status page;
// when false, only enabled categories are processed and deletions are actually performed.
func (c *Cleaner) buildReport(req CleanRequest, inspectAll bool) *Report {
	startedAt := c.now()
	cats, err := c.selectCategories(req.Categories, !inspectAll)
	rep := &Report{
		DryRun:    req.DryRun,
		Trigger:   req.Trigger,
		StartedAt: startedAt,
	}
	if err != nil {
		rep.Note = err.Error()
		rep.Categories = []CategoryReport{}
		return rep
	}

	for _, cat := range cats {
		res := c.scan(cat, startedAt)
		cr := res.report

		if !req.DryRun && cr.Enabled {
			root := cr.Root
			for _, u := range res.eligible {
				if rmErr := removeUnit(u); rmErr != nil {
					cr.Errors = append(cr.Errors, fmt.Sprintf("%s: %v", filepath.Base(u.Path), rmErr))
					continue
				}
				cr.RemovedUnits++
				cr.FreedBytes += u.Size
			}
			if cat.pruneEmpty && cr.RemovedUnits > 0 && root != "" {
				// date layer + session layer, at most two levels deep.
				cr.RemovedEmptyDirs = pruneEmptyDirs(root, 2)
			}
		}

		rep.Categories = append(rep.Categories, cr)
	}
	if rep.Categories == nil {
		rep.Categories = []CategoryReport{}
	}

	for _, cr := range rep.Categories {
		rep.Totals.Units += cr.Units
		rep.Totals.Bytes += cr.Bytes
		rep.Totals.ReclaimableUnits += cr.ReclaimableUnits
		rep.Totals.ReclaimableBytes += cr.ReclaimableBytes
		rep.Totals.RemovedUnits += cr.RemovedUnits
		rep.Totals.FreedBytes += cr.FreedBytes
		rep.Totals.SkippedActive += cr.SkippedActive
		rep.Totals.Errors += len(cr.Errors)
	}

	rep.Filesystem = c.filesystem()
	rep.DurationMS = time.Since(startedAt).Milliseconds()
	return rep
}

// filesystem returns the filesystem for the first available category root, used as the source for the overview capacity card.
func (c *Cleaner) filesystem() Filesystem {
	probe := ""
	for _, cat := range categories() {
		if root := c.rootOf(cat); root != "" {
			probe = root
			break
		}
	}
	if probe == "" {
		probe = "."
	}
	fs, err := FilesystemUsage(probe)
	if err != nil && c.logger != nil {
		c.logger.Debug("filesystem usage query failed", zap.String("path", probe), zap.Error(err))
	}
	return fs
}

// removeUnit deletes a unit. Directories are first renamed atomically and then removed recursively:
// a mid-run crash leaves only a directory with the deletionMarkerSuffix, which the next evaluate will delete unconditionally.
func removeUnit(u Unit) error {
	if u.IsDir {
		marker := u.Path + deletionMarkerSuffix
		if err := os.Rename(u.Path, marker); err == nil {
			return ignoreMissing(os.RemoveAll(marker))
		}
		// If rename fails (cross-device, permissions, name collision), fall back to direct deletion.
	}
	return ignoreMissing(os.RemoveAll(u.Path))
}

func ignoreMissing(err error) error {
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func (c *Cleaner) logClean(rep *Report, req CleanRequest) {
	if c.logger == nil {
		return
	}
	summary := []zap.Field{
		zap.String("trigger", req.Trigger),
		zap.Int("removed_units", rep.Totals.RemovedUnits),
		zap.Int64("freed_bytes", rep.Totals.FreedBytes),
		zap.Int("skipped_active", rep.Totals.SkippedActive),
		zap.Int("errors", rep.Totals.Errors),
	}
	if rep.Totals.RemovedUnits == 0 && rep.Totals.Errors == 0 {
		c.logger.Debug("workspace cleanup complete, nothing reclaimable", summary...)
		return
	}
	c.logger.Info("workspace cleanup complete", summary...)
	for _, cr := range rep.Categories {
		if cr.RemovedUnits == 0 && len(cr.Errors) == 0 {
			continue
		}
		c.logger.Info("cleanup category details",
			zap.String("category", cr.Key),
			zap.Int("removed", cr.RemovedUnits),
			zap.Int64("freed_bytes", cr.FreedBytes),
			zap.Strings("errors", cr.Errors))
	}
}
