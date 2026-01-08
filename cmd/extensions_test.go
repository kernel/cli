package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/stretchr/testify/assert"
)

// captureExtensionsOutput sets pterm writers for tests in this file
func captureExtensionsOutput(t *testing.T) *bytes.Buffer {
	var buf bytes.Buffer
	pterm.SetDefaultOutput(&buf)
	pterm.Info.Writer = &buf
	pterm.Error.Writer = &buf
	pterm.Success.Writer = &buf
	pterm.Warning.Writer = &buf
	pterm.Debug.Writer = &buf
	pterm.Fatal.Writer = &buf
	pterm.DefaultTable = *pterm.DefaultTable.WithWriter(&buf)
	t.Cleanup(func() {
		pterm.SetDefaultOutput(os.Stdout)
		pterm.Info.Writer = os.Stdout
		pterm.Error.Writer = os.Stdout
		pterm.Success.Writer = os.Stdout
		pterm.Warning.Writer = os.Stdout
		pterm.Debug.Writer = os.Stdout
		pterm.Fatal.Writer = os.Stdout
		pterm.DefaultTable = *pterm.DefaultTable.WithWriter(os.Stdout)
	})
	return &buf
}

// FakeExtensionsService implements ExtensionsService
type FakeExtensionsService struct {
	ListFunc                  func(ctx context.Context, opts ...option.RequestOption) (*[]kernel.ExtensionListResponse, error)
	DeleteFunc                func(ctx context.Context, idOrName string, opts ...option.RequestOption) error
	DownloadFunc              func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error)
	DownloadFromChromeStoreFn func(ctx context.Context, query kernel.ExtensionDownloadFromChromeStoreParams, opts ...option.RequestOption) (*http.Response, error)
	UploadFunc                func(ctx context.Context, body kernel.ExtensionUploadParams, opts ...option.RequestOption) (*kernel.ExtensionUploadResponse, error)
}

func (f *FakeExtensionsService) List(ctx context.Context, opts ...option.RequestOption) (*[]kernel.ExtensionListResponse, error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, opts...)
	}
	empty := []kernel.ExtensionListResponse{}
	return &empty, nil
}
func (f *FakeExtensionsService) Delete(ctx context.Context, idOrName string, opts ...option.RequestOption) error {
	if f.DeleteFunc != nil {
		return f.DeleteFunc(ctx, idOrName, opts...)
	}
	return nil
}
func (f *FakeExtensionsService) Download(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error) {
	if f.DownloadFunc != nil {
		return f.DownloadFunc(ctx, idOrName, opts...)
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
}
func (f *FakeExtensionsService) DownloadFromChromeStore(ctx context.Context, query kernel.ExtensionDownloadFromChromeStoreParams, opts ...option.RequestOption) (*http.Response, error) {
	if f.DownloadFromChromeStoreFn != nil {
		return f.DownloadFromChromeStoreFn(ctx, query, opts...)
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
}
func (f *FakeExtensionsService) Upload(ctx context.Context, body kernel.ExtensionUploadParams, opts ...option.RequestOption) (*kernel.ExtensionUploadResponse, error) {
	if f.UploadFunc != nil {
		return f.UploadFunc(ctx, body, opts...)
	}
	return &kernel.ExtensionUploadResponse{ID: "e-new", Name: body.Name.Value, CreatedAt: time.Unix(0, 0), SizeBytes: 1}, nil
}

func TestExtensionsList_Empty(t *testing.T) {
	buf := captureExtensionsOutput(t)
	fake := &FakeExtensionsService{}
	e := ExtensionsCmd{extensions: fake}
	_ = e.List(context.Background(), ExtensionsListInput{})
	assert.Contains(t, buf.String(), "No extensions found")
}

func TestExtensionsList_WithRows(t *testing.T) {
	buf := captureExtensionsOutput(t)
	created := time.Unix(0, 0)
	rows := []kernel.ExtensionListResponse{{ID: "e1", Name: "alpha", CreatedAt: created, SizeBytes: 10}, {ID: "e2", Name: "", CreatedAt: created, SizeBytes: 20}}
	fake := &FakeExtensionsService{ListFunc: func(ctx context.Context, opts ...option.RequestOption) (*[]kernel.ExtensionListResponse, error) {
		return &rows, nil
	}}
	e := ExtensionsCmd{extensions: fake}
	_ = e.List(context.Background(), ExtensionsListInput{})
	out := buf.String()
	assert.Contains(t, out, "e1")
	assert.Contains(t, out, "alpha")
	assert.Contains(t, out, "e2")
}

func TestExtensionsDelete_SkipConfirm(t *testing.T) {
	buf := captureExtensionsOutput(t)
	fake := &FakeExtensionsService{}
	e := ExtensionsCmd{extensions: fake}
	_ = e.Delete(context.Background(), ExtensionsDeleteInput{Identifier: "e1", SkipConfirm: true})
	assert.Contains(t, buf.String(), "Deleted extension: e1")
}

func TestExtensionsDelete_NotFound(t *testing.T) {
	buf := captureExtensionsOutput(t)
	fake := &FakeExtensionsService{DeleteFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) error {
		return &kernel.Error{StatusCode: http.StatusNotFound}
	}}
	e := ExtensionsCmd{extensions: fake}
	_ = e.Delete(context.Background(), ExtensionsDeleteInput{Identifier: "missing", SkipConfirm: true})
	assert.Contains(t, buf.String(), "not found")
}

func TestExtensionsDownload_MissingOutput(t *testing.T) {
	buf := captureExtensionsOutput(t)
	fake := &FakeExtensionsService{DownloadFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("content")), Header: http.Header{}}, nil
	}}
	e := ExtensionsCmd{extensions: fake}
	_ = e.Download(context.Background(), ExtensionsDownloadInput{Identifier: "e1", Output: ""})
	assert.Contains(t, buf.String(), "Missing --to output directory")
}

func TestExtensionsDownload_ExtractsToDir(t *testing.T) {
	buf := captureExtensionsOutput(t)
	// Create a small in-memory zip
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	w, _ := zw.Create("manifest.json")
	_, _ = w.Write([]byte("{}"))
	_ = zw.Close()

	fake := &FakeExtensionsService{DownloadFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(zbuf.Bytes())), Header: http.Header{}}, nil
	}}
	e := ExtensionsCmd{extensions: fake}

	outDir := filepath.Join(os.TempDir(), "extdl-test")
	_ = os.RemoveAll(outDir)
	_ = e.Download(context.Background(), ExtensionsDownloadInput{Identifier: "e1", Output: outDir})

	// Ensure extracted
	_, statErr := os.Stat(filepath.Join(outDir, "manifest.json"))
	assert.NoError(t, statErr)
	assert.Contains(t, buf.String(), "Extracted extension to "+outDir)
	_ = os.RemoveAll(outDir)
}

func TestExtensionsDownloadWebStore_ExtractsToDir(t *testing.T) {
	buf := captureExtensionsOutput(t)
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	w, _ := zw.Create("manifest.json")
	_, _ = w.Write([]byte("{}"))
	_ = zw.Close()

	fake := &FakeExtensionsService{DownloadFromChromeStoreFn: func(ctx context.Context, query kernel.ExtensionDownloadFromChromeStoreParams, opts ...option.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(zbuf.Bytes())), Header: http.Header{}}, nil
	}}
	e := ExtensionsCmd{extensions: fake}

	outDir := filepath.Join(os.TempDir(), "webstoredl-test")
	_ = os.RemoveAll(outDir)
	_ = e.DownloadWebStore(context.Background(), ExtensionsDownloadWebStoreInput{URL: "https://store/link", Output: outDir, OS: "linux"})

	_, statErr := os.Stat(filepath.Join(outDir, "manifest.json"))
	assert.NoError(t, statErr)
	assert.Contains(t, buf.String(), "Extracted extension to "+outDir)
	_ = os.RemoveAll(outDir)
}

func TestExtensionsDownloadWebStore_InvalidOS(t *testing.T) {
	buf := captureExtensionsOutput(t)
	fake := &FakeExtensionsService{}
	e := ExtensionsCmd{extensions: fake}
	_ = e.DownloadWebStore(context.Background(), ExtensionsDownloadWebStoreInput{URL: "https://store/link", Output: "x", OS: "freebsd"})
	assert.Contains(t, buf.String(), "--os must be one of mac, win, linux")
}

func TestExtensionsUpload_Success(t *testing.T) {
	buf := captureExtensionsOutput(t)
	dir := t.TempDir()
	// create a sample file inside dir
	err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0644)
	assert.NoError(t, err)

	fake := &FakeExtensionsService{UploadFunc: func(ctx context.Context, body kernel.ExtensionUploadParams, opts ...option.RequestOption) (*kernel.ExtensionUploadResponse, error) {
		return &kernel.ExtensionUploadResponse{ID: "e1", Name: "myext", CreatedAt: time.Unix(0, 0), SizeBytes: 10}, nil
	}}
	e := ExtensionsCmd{extensions: fake}
	_ = e.Upload(context.Background(), ExtensionsUploadInput{Dir: dir, Name: "myext"})
	out := buf.String()
	assert.Contains(t, out, "ID")
	assert.Contains(t, out, "e1")
	assert.Contains(t, out, "Name")
	assert.Contains(t, out, "myext")
}

func TestExtensionsUpload_InvalidDir(t *testing.T) {
	fake := &FakeExtensionsService{}
	e := ExtensionsCmd{extensions: fake}
	err := e.Upload(context.Background(), ExtensionsUploadInput{Dir: "/does/not/exist"})
	assert.Error(t, err)
}
