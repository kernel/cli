package util

import "strings"

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
