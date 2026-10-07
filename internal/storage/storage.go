package storage

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// Filesystem represents disk usage telemetry for a directory.
type Filesystem struct {
	Path        string  `json:"path"`
	TotalBytes  int64   `json:"total_bytes"`
	FreeBytes   int64   `json:"free_bytes"`
	UsedBytes   int64   `json:"used_bytes"`
	UsedPercent float64 `json:"used_percent"`
	Available   bool    `json:"available"`
}

// CalculateDirUsage traverses a directory and returns total size and file count.
func CalculateDirUsage(dir string) (int64, int, error) {
	var totalBytes int64
	var fileCount int

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0, 0, nil
	}

	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			totalBytes += info.Size()
			fileCount++
		}
		return nil
	})

	return totalBytes, fileCount, err
}

// CleanExpiredFiles deletes files in dir modified before maxAge.
func CleanExpiredFiles(dir string, maxAge time.Duration, logger *zap.Logger) (int, int64, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0, 0, nil
	}

	cutoff := time.Now().Add(-maxAge)
	deletedCount := 0
	var freedBytes int64

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			size := info.Size()
			if removeErr := os.Remove(path); removeErr == nil {
				deletedCount++
				freedBytes += size
			}
		}
		return nil
	})

	if deletedCount > 0 && logger != nil {
		logger.Info("storage cleanup sweep completed",
			zap.String("dir", dir),
			zap.Int("files_removed", deletedCount),
			zap.Int64("bytes_freed", freedBytes),
		)
	}

	return deletedCount, freedBytes, err
}

// StartRetentionWorker launches a background goroutine to sweep expired temp/spill files.
func StartRetentionWorker(ctx context.Context, spillDir string, interval time.Duration, maxAge time.Duration, logger *zap.Logger) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _, _ = CleanExpiredFiles(spillDir, maxAge, logger)
			}
		}
	}()
}
