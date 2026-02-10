package util

import (
	"fmt"
	"strings"
)

// OrDash returns the string if non-empty, otherwise returns "-".
func OrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// FirstOrDash returns the first non-empty string from the provided items.
// If all items are empty, it returns "-".
func FirstOrDash(items ...string) string {
	for _, item := range items {
		if item != "" {
			return item
		}
	}
	return "-"
}

// JoinOrDash joins the provided strings with ", " as separator.
// If no items are provided, it returns "-".
func JoinOrDash(items ...string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ", ")
}

// FormatBytes formats bytes in a human-readable format
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
