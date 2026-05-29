package util

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rawJSONStub string

func (r rawJSONStub) RawJSON() string {
	return string(r)
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = original
	})

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&buf, reader)
		done <- copyErr
	}()

	fnErr := fn()
	require.NoError(t, writer.Close())
	require.NoError(t, <-done)
	require.NoError(t, reader.Close())
	os.Stdout = original

	return buf.String(), fnErr
}

func TestPrintPrettyJSONPointerSliceTreatsNilAsEmptyList(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return PrintPrettyJSONPointerSlice[rawJSONStub](nil)
	})

	require.NoError(t, err)
	assert.Equal(t, "[]\n", out)
}

func TestPrintPrettyJSONPointerSlicePrintsItems(t *testing.T) {
	items := []rawJSONStub{`{"id":"one"}`}

	out, err := captureStdout(t, func() error {
		return PrintPrettyJSONPointerSlice(&items)
	})

	require.NoError(t, err)
	assert.Equal(t, "[\n  {\n    \"id\": \"one\"\n  }\n]\n", out)
}

func TestPrintPrettyJSONPageItemsTreatsNilAsEmptyList(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return PrintPrettyJSONPageItems[rawJSONStub](nil)
	})

	require.NoError(t, err)
	assert.Equal(t, "[]\n", out)
}

func TestPrintPrettyJSONPageItemsPrintsItems(t *testing.T) {
	page := &pagination.OffsetPagination[rawJSONStub]{
		Items: []rawJSONStub{`{"id":"one"}`},
	}

	out, err := captureStdout(t, func() error {
		return PrintPrettyJSONPageItems(page)
	})

	require.NoError(t, err)
	assert.Equal(t, "[\n  {\n    \"id\": \"one\"\n  }\n]\n", out)
}
