//go:build !darwin && !linux && !windows

package cmd

import (
	"fmt"
	"os"
)

func tryLockAuditLogsDownloadFile(file *os.File) (bool, error) {
	return false, fmt.Errorf("audit-log download locking is unsupported on this platform")
}

func commitAuditLogsDownloadStateFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func syncAuditLogsDownloadDir(dir string) error {
	return nil
}
