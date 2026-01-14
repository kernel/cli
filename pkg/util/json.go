package util

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// RawJSONProvider is an interface for SDK types that provide raw JSON responses.
type RawJSONProvider interface {
	RawJSON() string
}

// PrintPrettyJSON prints the raw JSON from an SDK response type with indentation.
// It uses the RawJSON() method to get the original API response, avoiding
// zero-value fields that would appear when re-marshaling the Go struct.
func PrintPrettyJSON(v RawJSONProvider) error {
	raw := v.RawJSON()
	if raw == "" {
		fmt.Println("{}")
		return nil
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "", "  "); err != nil {
		return err
	}
	fmt.Println(buf.String())
	return nil
}

// PrintPrettyJSONSlice prints a slice of SDK response types as a JSON array.
// Each element must implement RawJSONProvider.
func PrintPrettyJSONSlice[T RawJSONProvider](items []T) error {
	if len(items) == 0 {
		fmt.Println("[]")
		return nil
	}

	// Build a JSON array from raw JSON elements
	var buf bytes.Buffer
	buf.WriteString("[\n")
	for i, item := range items {
		raw := item.RawJSON()
		if raw == "" {
			raw = "{}"
		}
		// Indent each element
		var elemBuf bytes.Buffer
		if err := json.Indent(&elemBuf, []byte(raw), "  ", "  "); err != nil {
			// Fallback to raw if indentation fails
			buf.WriteString("  ")
			buf.WriteString(raw)
		} else {
			// json.Indent adds prefix after newlines, not before first line
			buf.WriteString("  ")
			buf.Write(elemBuf.Bytes())
		}
		if i < len(items)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("]")
	fmt.Println(buf.String())
	return nil
}
