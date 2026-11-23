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

// PrintCompactJSONLine prints v as a single compact JSON line followed by a
// newline. Use inside SSE loops where downstream tooling (jq -c, log shippers)
// expects newline-delimited JSON.
func PrintCompactJSONLine(v RawJSONProvider) error {
	raw := v.RawJSON()
	if raw == "" {
		return nil
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(raw)); err != nil {
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

// PrintPageJSON prints one page of a page-based list as
// {"items": [...], "page": N, "per_page": N, "has_more": bool}, using each item's raw JSON.
func PrintPageJSON[T RawJSONProvider](items []T, page, perPage int, hasMore bool) error {
	raw := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		r := item.RawJSON()
		if r == "" {
			r = "{}"
		}
		raw = append(raw, json.RawMessage(r))
	}
	data, err := json.MarshalIndent(struct {
		Items   []json.RawMessage `json:"items"`
		Page    int               `json:"page"`
		PerPage int               `json:"per_page"`
		HasMore bool              `json:"has_more"`
	}{raw, page, perPage, hasMore}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
