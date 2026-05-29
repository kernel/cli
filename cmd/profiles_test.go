package cmd

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
)

// FakeProfilesService implements ProfilesService
type FakeProfilesService struct {
	GetFunc      func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.Profile, error)
	ListFunc     func(ctx context.Context, query kernel.ProfileListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Profile], error)
	DeleteFunc   func(ctx context.Context, idOrName string, opts ...option.RequestOption) error
	NewFunc      func(ctx context.Context, body kernel.ProfileNewParams, opts ...option.RequestOption) (*kernel.Profile, error)
	DownloadFunc func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error)
}

func (f *FakeProfilesService) Get(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.Profile, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, idOrName, opts...)
	}
	return &kernel.Profile{ID: idOrName, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
}
func (f *FakeProfilesService) List(ctx context.Context, query kernel.ProfileListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Profile], error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, query, opts...)
	}
	return &pagination.OffsetPagination[kernel.Profile]{Items: []kernel.Profile{}}, nil
}
func (f *FakeProfilesService) Delete(ctx context.Context, idOrName string, opts ...option.RequestOption) error {
	if f.DeleteFunc != nil {
		return f.DeleteFunc(ctx, idOrName, opts...)
	}
	return nil
}
func (f *FakeProfilesService) Download(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error) {
	if f.DownloadFunc != nil {
		return f.DownloadFunc(ctx, idOrName, opts...)
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
}
func (f *FakeProfilesService) New(ctx context.Context, body kernel.ProfileNewParams, opts ...option.RequestOption) (*kernel.Profile, error) {
	if f.NewFunc != nil {
		return f.NewFunc(ctx, body, opts...)
	}
	return &kernel.Profile{ID: "new", Name: body.Name.Value, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
}

func TestProfilesList_Empty(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeProfilesService{}
	p := ProfilesCmd{profiles: fake}
	_ = p.List(context.Background(), ProfilesListInput{Page: 1, PerPage: 20})
	assert.Contains(t, buf.String(), "No profiles found")
}

func TestProfilesList_WithRows(t *testing.T) {
	buf := capturePtermOutput(t)
	created := time.Unix(0, 0)
	rows := []kernel.Profile{{ID: "p1", Name: "alpha", CreatedAt: created, UpdatedAt: created}, {ID: "p2", Name: "", CreatedAt: created, UpdatedAt: created}}
	fake := &FakeProfilesService{ListFunc: func(ctx context.Context, query kernel.ProfileListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Profile], error) {
		return &pagination.OffsetPagination[kernel.Profile]{Items: rows}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	_ = p.List(context.Background(), ProfilesListInput{Page: 1, PerPage: 20})
	out := buf.String()
	assert.Contains(t, out, "p1")
	assert.Contains(t, out, "alpha")
	assert.Contains(t, out, "p2")
	assert.Contains(t, out, "Has more: no")
}

func TestProfilesList_HasMore(t *testing.T) {
	buf := capturePtermOutput(t)
	created := time.Unix(0, 0)
	perPage := 2
	items := make([]kernel.Profile, perPage+1)
	for i := range items {
		items[i] = kernel.Profile{ID: fmt.Sprintf("p%d", i), CreatedAt: created, UpdatedAt: created}
	}
	fake := &FakeProfilesService{ListFunc: func(ctx context.Context, query kernel.ProfileListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Profile], error) {
		return &pagination.OffsetPagination[kernel.Profile]{Items: items}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	_ = p.List(context.Background(), ProfilesListInput{Page: 1, PerPage: perPage})
	out := buf.String()
	assert.Contains(t, out, "Has more: yes")
	assert.Contains(t, out, "Next: kernel profile list --page 2 --per-page 2")
	assert.Contains(t, out, "p0")
	assert.Contains(t, out, "p1")
	assert.NotContains(t, out, "p2")
}

func TestProfilesList_QueryInNextHint(t *testing.T) {
	buf := capturePtermOutput(t)
	created := time.Unix(0, 0)
	items := make([]kernel.Profile, 3)
	for i := range items {
		items[i] = kernel.Profile{ID: fmt.Sprintf("p%d", i), CreatedAt: created, UpdatedAt: created}
	}
	fake := &FakeProfilesService{ListFunc: func(ctx context.Context, query kernel.ProfileListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Profile], error) {
		return &pagination.OffsetPagination[kernel.Profile]{Items: items}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	_ = p.List(context.Background(), ProfilesListInput{Page: 1, PerPage: 2, Query: "my-bot"})
	out := buf.String()
	assert.Contains(t, out, `--query "my-bot"`)
}

func TestProfilesList_QueryWithSpacesQuoted(t *testing.T) {
	buf := capturePtermOutput(t)
	created := time.Unix(0, 0)
	items := make([]kernel.Profile, 3)
	for i := range items {
		items[i] = kernel.Profile{ID: fmt.Sprintf("p%d", i), CreatedAt: created, UpdatedAt: created}
	}
	fake := &FakeProfilesService{ListFunc: func(ctx context.Context, query kernel.ProfileListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Profile], error) {
		return &pagination.OffsetPagination[kernel.Profile]{Items: items}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	_ = p.List(context.Background(), ProfilesListInput{Page: 1, PerPage: 2, Query: "my bot"})
	out := buf.String()
	assert.Contains(t, out, `--query "my bot"`)
}

func TestProfilesGet_Success(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeProfilesService{GetFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.Profile, error) {
		return &kernel.Profile{ID: "p1", Name: "alpha", CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	_ = p.Get(context.Background(), ProfilesGetInput{Identifier: "p1"})
	out := buf.String()
	assert.Contains(t, out, "ID")
	assert.Contains(t, out, "p1")
	assert.Contains(t, out, "Name")
	assert.Contains(t, out, "alpha")
}

func TestProfilesGet_Error(t *testing.T) {
	fake := &FakeProfilesService{GetFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.Profile, error) {
		return nil, errors.New("boom")
	}}
	p := ProfilesCmd{profiles: fake}
	err := p.Get(context.Background(), ProfilesGetInput{Identifier: "x"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestProfilesCreate_Success(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeProfilesService{NewFunc: func(ctx context.Context, body kernel.ProfileNewParams, opts ...option.RequestOption) (*kernel.Profile, error) {
		return &kernel.Profile{ID: "pnew", Name: body.Name.Value, CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	_ = p.Create(context.Background(), ProfilesCreateInput{Name: "alpha"})
	out := buf.String()
	assert.Contains(t, out, "pnew")
	assert.Contains(t, out, "alpha")
}

func TestProfilesCreate_Error(t *testing.T) {
	fake := &FakeProfilesService{NewFunc: func(ctx context.Context, body kernel.ProfileNewParams, opts ...option.RequestOption) (*kernel.Profile, error) {
		return nil, errors.New("fail")
	}}
	p := ProfilesCmd{profiles: fake}
	err := p.Create(context.Background(), ProfilesCreateInput{Name: "x"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fail")
}

func TestProfilesDelete_ConfirmNotFound(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeProfilesService{GetFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.Profile, error) {
		return nil, &kernel.Error{StatusCode: http.StatusNotFound}
	}}
	p := ProfilesCmd{profiles: fake}
	_ = p.Delete(context.Background(), ProfilesDeleteInput{Identifier: "missing"})
	assert.Contains(t, buf.String(), "not found")
}

func TestProfilesDelete_SkipConfirm(t *testing.T) {
	buf := capturePtermOutput(t)
	fake := &FakeProfilesService{}
	p := ProfilesCmd{profiles: fake}
	_ = p.Delete(context.Background(), ProfilesDeleteInput{Identifier: "a", SkipConfirm: true})
	assert.Contains(t, buf.String(), "Deleted profile: a")
}

// makeProfileArchive builds a zstd-compressed tar archive from a map of file
// paths to contents, for use in download tests.
func makeProfileArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf)
	assert.NoError(t, err)
	tw := tar.NewWriter(zw)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		assert.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write([]byte(content))
		assert.NoError(t, err)
	}
	assert.NoError(t, tw.Close())
	assert.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestProfilesDownload_MissingTo(t *testing.T) {
	fake := &FakeProfilesService{}
	p := ProfilesCmd{profiles: fake}
	err := p.Download(context.Background(), ProfilesDownloadInput{Identifier: "p1", To: ""})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--to is required")
	assert.Contains(t, err.Error(), "add --to <directory>")
}

func TestProfilesDownload_ExtractSuccess(t *testing.T) {
	buf := capturePtermOutput(t)
	dir, err := os.MkdirTemp("", "profile-*")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	archive := makeProfileArchive(t, map[string]string{
		"Default/Preferences": "{\"k\":1}",
		"Local State":         "local",
	})
	fake := &FakeProfilesService{DownloadFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(archive)), Header: http.Header{}}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	err = p.Download(context.Background(), ProfilesDownloadInput{Identifier: "p1", To: dir})
	assert.NoError(t, err)

	b, readErr := os.ReadFile(filepath.Join(dir, "Default", "Preferences"))
	assert.NoError(t, readErr)
	assert.Equal(t, "{\"k\":1}", string(b))

	b2, readErr := os.ReadFile(filepath.Join(dir, "Local State"))
	assert.NoError(t, readErr)
	assert.Equal(t, "local", string(b2))

	assert.Contains(t, buf.String(), "Extracted profile 'p1' to "+dir)
}

func TestProfilesDownload_202NoData(t *testing.T) {
	buf := capturePtermOutput(t)
	dir, err := os.MkdirTemp("", "profile-*")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	fake := &FakeProfilesService{DownloadFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	err = p.Download(context.Background(), ProfilesDownloadInput{Identifier: "fresh", To: dir})
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "no saved data yet")

	entries, _ := os.ReadDir(dir)
	assert.Empty(t, entries)
}

func TestProfilesDownload_PathTraversalRejected(t *testing.T) {
	dir, err := os.MkdirTemp("", "profile-*")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	archive := makeProfileArchive(t, map[string]string{
		"../escape": "nope",
	})
	fake := &FakeProfilesService{DownloadFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(archive)), Header: http.Header{}}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	err = p.Download(context.Background(), ProfilesDownloadInput{Identifier: "p1", To: dir})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "illegal entry path")
}
