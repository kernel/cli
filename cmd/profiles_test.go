package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/pterm/pterm"
	"github.com/stretchr/testify/assert"
)

// captureProfilesOutput sets pterm writers for tests in this file
func captureProfilesOutput(t *testing.T) *bytes.Buffer {
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

// FakeProfilesService implements ProfilesService
type FakeProfilesService struct {
	GetFunc      func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.Profile, error)
	ListFunc     func(ctx context.Context, opts ...option.RequestOption) (*[]kernel.Profile, error)
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
func (f *FakeProfilesService) List(ctx context.Context, opts ...option.RequestOption) (*[]kernel.Profile, error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, opts...)
	}
	empty := []kernel.Profile{}
	return &empty, nil
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
	buf := captureProfilesOutput(t)
	fake := &FakeProfilesService{}
	p := ProfilesCmd{profiles: fake}
	_ = p.List(context.Background())
	assert.Contains(t, buf.String(), "No profiles found")
}

func TestProfilesList_WithRows(t *testing.T) {
	buf := captureProfilesOutput(t)
	created := time.Unix(0, 0)
	rows := []kernel.Profile{{ID: "p1", Name: "alpha", CreatedAt: created, UpdatedAt: created}, {ID: "p2", Name: "", CreatedAt: created, UpdatedAt: created}}
	fake := &FakeProfilesService{ListFunc: func(ctx context.Context, opts ...option.RequestOption) (*[]kernel.Profile, error) { return &rows, nil }}
	p := ProfilesCmd{profiles: fake}
	_ = p.List(context.Background())
	out := buf.String()
	assert.Contains(t, out, "p1")
	assert.Contains(t, out, "alpha")
	assert.Contains(t, out, "p2")
}

func TestProfilesGet_Success(t *testing.T) {
	buf := captureProfilesOutput(t)
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
	buf := captureProfilesOutput(t)
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
	buf := captureProfilesOutput(t)
	fake := &FakeProfilesService{GetFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*kernel.Profile, error) {
		return nil, &kernel.Error{StatusCode: http.StatusNotFound}
	}}
	p := ProfilesCmd{profiles: fake}
	_ = p.Delete(context.Background(), ProfilesDeleteInput{Identifier: "missing"})
	assert.Contains(t, buf.String(), "not found")
}

func TestProfilesDelete_SkipConfirm(t *testing.T) {
	buf := captureProfilesOutput(t)
	fake := &FakeProfilesService{}
	p := ProfilesCmd{profiles: fake}
	_ = p.Delete(context.Background(), ProfilesDeleteInput{Identifier: "a", SkipConfirm: true})
	assert.Contains(t, buf.String(), "Deleted profile: a")
}

func TestProfilesDownload_MissingOutput(t *testing.T) {
	buf := captureProfilesOutput(t)
	fake := &FakeProfilesService{DownloadFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("content")), Header: http.Header{}}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	_ = p.Download(context.Background(), ProfilesDownloadInput{Identifier: "p1", Output: "", Pretty: false})
	assert.Contains(t, buf.String(), "Missing --to output file path")
}

func TestProfilesDownload_RawSuccess(t *testing.T) {
	buf := captureProfilesOutput(t)
	f, err := os.CreateTemp("", "profile-*.zip")
	assert.NoError(t, err)
	name := f.Name()
	_ = f.Close()
	defer os.Remove(name)

	content := "hello"
	fake := &FakeProfilesService{DownloadFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(content)), Header: http.Header{}}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	_ = p.Download(context.Background(), ProfilesDownloadInput{Identifier: "p1", Output: name, Pretty: false})

	b, readErr := os.ReadFile(name)
	assert.NoError(t, readErr)
	assert.Equal(t, content, string(b))
	assert.Contains(t, buf.String(), "Saved profile to "+name)
}

func TestProfilesDownload_PrettySuccess(t *testing.T) {
	f, err := os.CreateTemp("", "profile-*.json")
	assert.NoError(t, err)
	name := f.Name()
	_ = f.Close()
	defer os.Remove(name)

	jsonBody := "{\"a\":1}"
	fake := &FakeProfilesService{DownloadFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(jsonBody)), Header: http.Header{}}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	_ = p.Download(context.Background(), ProfilesDownloadInput{Identifier: "p1", Output: name, Pretty: true})

	b, readErr := os.ReadFile(name)
	assert.NoError(t, readErr)
	out := string(b)
	assert.Contains(t, out, "\n")
	assert.Contains(t, out, "\"a\": 1")
}

func TestProfilesDownload_PrettyEmptyBody(t *testing.T) {
	buf := captureProfilesOutput(t)
	f, err := os.CreateTemp("", "profile-*.json")
	assert.NoError(t, err)
	name := f.Name()
	_ = f.Close()
	defer os.Remove(name)

	fake := &FakeProfilesService{DownloadFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	_ = p.Download(context.Background(), ProfilesDownloadInput{Identifier: "p1", Output: name, Pretty: true})
	assert.Contains(t, buf.String(), "Empty response body")
}

func TestProfilesDownload_PrettyInvalidJSON(t *testing.T) {
	buf := captureProfilesOutput(t)
	f, err := os.CreateTemp("", "profile-*.json")
	assert.NoError(t, err)
	name := f.Name()
	_ = f.Close()
	defer os.Remove(name)

	fake := &FakeProfilesService{DownloadFunc: func(ctx context.Context, idOrName string, opts ...option.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("not json")), Header: http.Header{}}, nil
	}}
	p := ProfilesCmd{profiles: fake}
	_ = p.Download(context.Background(), ProfilesDownloadInput{Identifier: "p1", Output: name, Pretty: true})
	assert.Contains(t, buf.String(), "Failed to pretty-print JSON")
}
