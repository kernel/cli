//go:build !darwin && !linux && !windows

package cmd

import (
	"fmt"
	"os"
)

func tryLockAuditLogsDownloadFile(file *os.File) (bool, error) {
	return false, fmt.Errorf("audit-log download locking is unsupported on this platform")
}

func unlockAuditLogsDownloadFile(file *os.File) error {
	return nil
}
