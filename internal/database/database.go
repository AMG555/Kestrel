package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

const (
	sqliteMaxOpenConns              = 20
	sqliteMaxIdleConns              = 5
	sqliteWALAutoCheckpointPages    = 1000
	sqliteJournalSizeLimitBytes     = 256 * 1024 * 1024
	sqlitePassiveCheckpointInterval = 300 * time.Second
)

// DB wraps sql.DB with application-specific helpers.
type DB struct {
	*sql.DB
	logger         *zap.Logger
	checkpointStop chan struct{}
	checkpointDone chan struct{}
	closeOnce      sync.Once
}

// New opens (or creates) a SQLite database at the given path
// and initialises all tables.
func New(path string, logger *zap.Logger) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	dsn := path + "?_journal_mode=WAL&_foreign_keys=1&_busy_timeout=5000&_synchronous=NORMAL"
	raw, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	raw.SetMaxOpenConns(sqliteMaxOpenConns)
	raw.SetMaxIdleConns(sqliteMaxIdleConns)
	raw.SetConnMaxLifetime(30 * time.Minute)

	if err := raw.Ping(); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	if _, err := raw.Exec(fmt.Sprintf("PRAGMA wal_autocheckpoint=%d", sqliteWALAutoCheckpointPages)); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("setting wal_autocheckpoint: %w", err)
	}
	if _, err := raw.Exec(fmt.Sprintf("PRAGMA journal_size_limit=%d", sqliteJournalSizeLimitBytes)); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("setting journal_size_limit: %w", err)
	}

	db := &DB{
		DB:             raw,
		logger:         logger,
		checkpointStop: make(chan struct{}),
		checkpointDone: make(chan struct{}),
	}

	if err := db.initSchema(); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("initialising schema: %w", err)
	}

	go db.checkpointLoop()
	return db, nil
}

// Close shuts down the checkpoint goroutine and closes the underlying connection.
func (db *DB) Close() error {
	var err error
	db.closeOnce.Do(func() {
		close(db.checkpointStop)
		<-db.checkpointDone
		err = db.DB.Close()
	})
	return err
}

// checkpointLoop runs a periodic PASSIVE WAL checkpoint.
func (db *DB) checkpointLoop() {
	defer close(db.checkpointDone)
	ticker := time.NewTicker(sqlitePassiveCheckpointInterval)
	defer ticker.Stop()
	db.runCheckpoint("startup")
	for {
		select {
		case <-db.checkpointStop:
			return
		case <-ticker.C:
			db.runCheckpoint("ticker")
		}
	}
}

func (db *DB) runCheckpoint(trigger string) {
	var busy, log, checkpointed int
	if err := db.QueryRow("PRAGMA wal_checkpoint(PASSIVE)").Scan(&busy, &log, &checkpointed); err != nil {
		if db.logger != nil {
			db.logger.Warn("WAL checkpoint failed", zap.String("trigger", trigger), zap.Error(err))
		}
		return
	}
	if db.logger != nil {
		db.logger.Debug("WAL checkpoint",
			zap.String("trigger", trigger),
			zap.Int("busy", busy),
			zap.Int("log", log),
			zap.Int("checkpointed", checkpointed),
		)
	}
}

// addColumnIfMissing runs an ALTER TABLE … ADD COLUMN statement only if the
// column does not yet exist, making the migration idempotent.
func (db *DB) addColumnIfMissing(table, column, ddl string) error {
	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?", table, column,
	).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec(ddl); err != nil {
			return err
		}
	}
	return nil
}
