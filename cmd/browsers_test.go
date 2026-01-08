package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/kernel/kernel-go-sdk/packages/ssestream"
	"github.com/kernel/kernel-go-sdk/shared"
	"github.com/pterm/pterm"
	"github.com/stretchr/testify/assert"
)

// outBuf captures pterm output during tests.
var outBuf bytes.Buffer

// setupStdoutCapture sets pterm's default output to an in-memory buffer.
func setupStdoutCapture(t *testing.T) {
	outBuf.Reset()
	pterm.SetDefaultOutput(&outBuf)
	// Prefix printers capture writer at init; set explicitly
	pterm.Info.Writer = &outBuf
	pterm.Error.Writer = &outBuf
	pterm.Success.Writer = &outBuf
	pterm.Warning.Writer = &outBuf
	pterm.Debug.Writer = &outBuf
	pterm.Fatal.Writer = &outBuf
	// Ensure tables render to our buffer
	pterm.DefaultTable = *pterm.DefaultTable.WithWriter(&outBuf)
	// Restore after test completes
	t.Cleanup(func() {
		pterm.SetDefaultOutput(os.Stdout)
		pterm.Info.Writer = os.Stdout
		pterm.Error.Writer = os.Stdout
		pterm.Success.Writer = os.Stdout
		pterm.Warning.Writer = os.Stdout
		pterm.Debug.Writer = os.Stdout
		pterm.Fatal.Writer = os.Stdout
		pterm.DefaultTable = *pterm.DefaultTable.WithWriter(os.Stdout)
		outBuf.Reset()
	})
}

// FakeBrowsersService is a configurable fake implementing BrowsersService.
type FakeBrowsersService struct {
	GetFunc            func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error)
	ListFunc           func(ctx context.Context, query kernel.BrowserListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserListResponse], error)
	NewFunc            func(ctx context.Context, body kernel.BrowserNewParams, opts ...option.RequestOption) (*kernel.BrowserNewResponse, error)
	DeleteFunc         func(ctx context.Context, body kernel.BrowserDeleteParams, opts ...option.RequestOption) error
	DeleteByIDFunc     func(ctx context.Context, id string, opts ...option.RequestOption) error
	LoadExtensionsFunc func(ctx context.Context, id string, body kernel.BrowserLoadExtensionsParams, opts ...option.RequestOption) error
}

func (f *FakeBrowsersService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, id, opts...)
	}
	return nil, errors.New("not found")
}

func (f *FakeBrowsersService) List(ctx context.Context, query kernel.BrowserListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserListResponse], error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, query, opts...)
	}
	return &pagination.OffsetPagination[kernel.BrowserListResponse]{Items: []kernel.BrowserListResponse{}}, nil
}

func (f *FakeBrowsersService) New(ctx context.Context, body kernel.BrowserNewParams, opts ...option.RequestOption) (*kernel.BrowserNewResponse, error) {
	if f.NewFunc != nil {
		return f.NewFunc(ctx, body, opts...)
	}
	return &kernel.BrowserNewResponse{}, nil
}

func (f *FakeBrowsersService) Delete(ctx context.Context, body kernel.BrowserDeleteParams, opts ...option.RequestOption) error {
	if f.DeleteFunc != nil {
		return f.DeleteFunc(ctx, body, opts...)
	}
	return nil
}

func (f *FakeBrowsersService) DeleteByID(ctx context.Context, id string, opts ...option.RequestOption) error {
	if f.DeleteByIDFunc != nil {
		return f.DeleteByIDFunc(ctx, id, opts...)
	}
	return nil
}

func (f *FakeBrowsersService) LoadExtensions(ctx context.Context, id string, body kernel.BrowserLoadExtensionsParams, opts ...option.RequestOption) error {
	if f.LoadExtensionsFunc != nil {
		return f.LoadExtensionsFunc(ctx, id, body, opts...)
	}
	return nil
}

func TestBrowsersList_PrintsEmptyMessage(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowsersService{
		ListFunc: func(ctx context.Context, query kernel.BrowserListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserListResponse], error) {
			empty := []kernel.BrowserListResponse{}
			return &pagination.OffsetPagination[kernel.BrowserListResponse]{Items: empty}, nil
		},
	}
	b := BrowsersCmd{browsers: fake}
	_ = b.List(context.Background(), BrowsersListInput{})

	out := outBuf.String()
	assert.Contains(t, out, "No running browsers found")
}

func TestBrowsersList_PrintsEmptyMessagePageIsNil(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowsersService{
		ListFunc: func(ctx context.Context, query kernel.BrowserListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserListResponse], error) {
			return nil, nil
		},
	}
	b := BrowsersCmd{browsers: fake}
	_ = b.List(context.Background(), BrowsersListInput{})

	out := outBuf.String()
	assert.Contains(t, out, "No running browsers found")
}

func TestBrowsersList_PrintsTableWithRows(t *testing.T) {
	setupStdoutCapture(t)

	created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	rows := []kernel.BrowserListResponse{
		{
			SessionID:          "sess-1",
			CdpWsURL:           "ws://cdp-1",
			BrowserLiveViewURL: "http://view-1",
			CreatedAt:          created,
			Persistence:        kernel.BrowserPersistence{ID: "pid-1"},
		},
		{
			SessionID:          "sess-2",
			CdpWsURL:           "ws://cdp-2",
			BrowserLiveViewURL: "",
			CreatedAt:          created,
			Persistence:        kernel.BrowserPersistence{ID: ""},
		},
	}

	fake := &FakeBrowsersService{
		ListFunc: func(ctx context.Context, query kernel.BrowserListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserListResponse], error) {
			return &pagination.OffsetPagination[kernel.BrowserListResponse]{Items: rows}, nil
		},
	}
	b := BrowsersCmd{browsers: fake}
	_ = b.List(context.Background(), BrowsersListInput{})

	out := outBuf.String()
	assert.Contains(t, out, "sess-1")
	assert.Contains(t, out, "sess-2")
	assert.Contains(t, out, "pid-1")
}

func TestBrowsersList_PrintsErrorOnFailure(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowsersService{
		ListFunc: func(ctx context.Context, query kernel.BrowserListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.BrowserListResponse], error) {
			return nil, errors.New("list failed")
		},
	}
	b := BrowsersCmd{browsers: fake}
	err := b.List(context.Background(), BrowsersListInput{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list failed")
}

func TestBrowsersCreate_PrintsResponse(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowsersService{
		NewFunc: func(ctx context.Context, body kernel.BrowserNewParams, opts ...option.RequestOption) (*kernel.BrowserNewResponse, error) {
			resp := &kernel.BrowserNewResponse{
				SessionID:          "sess-new",
				CdpWsURL:           "ws://cdp-new",
				BrowserLiveViewURL: "http://view-new",
				Persistence:        kernel.BrowserPersistence{ID: "pid-new"},
			}
			return resp, nil
		},
	}

	b := BrowsersCmd{browsers: fake}
	in := BrowsersCreateInput{
		PersistenceID:  "pid-new",
		TimeoutSeconds: 120,
		Stealth:        BoolFlag{Set: true, Value: true},
		Headless:       BoolFlag{Set: true, Value: false},
	}
	_ = b.Create(context.Background(), in)

	out := outBuf.String()
	assert.Contains(t, out, "Session ID")
	assert.Contains(t, out, "sess-new")
	assert.Contains(t, out, "CDP WebSocket URL")
	assert.Contains(t, out, "ws://cdp-new")
	assert.Contains(t, out, "Live View URL")
	assert.Contains(t, out, "http://view-new")
	assert.Contains(t, out, "Persistent ID")
	assert.Contains(t, out, "pid-new")
}

func TestBrowsersCreate_PrintsErrorOnFailure(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowsersService{
		NewFunc: func(ctx context.Context, body kernel.BrowserNewParams, opts ...option.RequestOption) (*kernel.BrowserNewResponse, error) {
			return nil, errors.New("create failed")
		},
	}
	b := BrowsersCmd{browsers: fake}
	err := b.Create(context.Background(), BrowsersCreateInput{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "create failed")
}

func TestBrowsersDelete_SkipConfirm_Success(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowsersService{
		DeleteFunc: func(ctx context.Context, body kernel.BrowserDeleteParams, opts ...option.RequestOption) error {
			return nil
		},
		DeleteByIDFunc: func(ctx context.Context, id string, opts ...option.RequestOption) error {
			return nil
		},
	}
	b := BrowsersCmd{browsers: fake}
	_ = b.Delete(context.Background(), BrowsersDeleteInput{Identifier: "any", SkipConfirm: true})

	out := outBuf.String()
	assert.Contains(t, out, "Successfully deleted (or already absent) browser: any")
}

func TestBrowsersDelete_SkipConfirm_Failure(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowsersService{
		DeleteFunc: func(ctx context.Context, body kernel.BrowserDeleteParams, opts ...option.RequestOption) error {
			return errors.New("left failed")
		},
		DeleteByIDFunc: func(ctx context.Context, id string, opts ...option.RequestOption) error {
			return errors.New("right failed")
		},
	}
	b := BrowsersCmd{browsers: fake}
	err := b.Delete(context.Background(), BrowsersDeleteInput{Identifier: "any", SkipConfirm: true})

	assert.Error(t, err)
	errMsg := err.Error()
	assert.True(t, strings.Contains(errMsg, "right failed") || strings.Contains(errMsg, "left failed"), "expected error message to contain either 'right failed' or 'left failed', got: %s", errMsg)
}

func TestBrowsersDelete_WithConfirm_NotFound(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowsersService{}
	b := BrowsersCmd{browsers: fake}
	err := b.Delete(context.Background(), BrowsersDeleteInput{Identifier: "missing", SkipConfirm: false})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestBrowsersView_ByID_PrintsURL(t *testing.T) {
	// Capture both pterm output and raw stdout
	setupStdoutCapture(t)
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	fake := &FakeBrowsersService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
			return &kernel.BrowserGetResponse{
				SessionID:          "abc",
				BrowserLiveViewURL: "http://live-url",
			}, nil
		},
	}
	b := BrowsersCmd{browsers: fake}
	_ = b.View(context.Background(), BrowsersViewInput{Identifier: "abc"})

	// Capture stdout
	w.Close()
	var stdoutBuf bytes.Buffer
	io.Copy(&stdoutBuf, r)

	assert.Contains(t, stdoutBuf.String(), "http://live-url")
}

func TestBrowsersView_NotFound(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowsersService{}
	b := BrowsersCmd{browsers: fake}
	err := b.View(context.Background(), BrowsersViewInput{Identifier: "missing"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestBrowsersView_HeadlessBrowser_ShowsWarning(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowsersService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
			return &kernel.BrowserGetResponse{
				SessionID:          "abc",
				Headless:           true,
				BrowserLiveViewURL: "",
			}, nil
		},
	}
	b := BrowsersCmd{browsers: fake}
	_ = b.View(context.Background(), BrowsersViewInput{Identifier: "abc"})

	out := outBuf.String()
	assert.Contains(t, out, "headless mode")
}

func TestBrowsersView_PrintsErrorOnGetFailure(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowsersService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
			return nil, errors.New("get error")
		},
	}
	b := BrowsersCmd{browsers: fake}
	err := b.View(context.Background(), BrowsersViewInput{Identifier: "any"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get error")
}

func TestBrowsersGet_PrintsDetails(t *testing.T) {
	setupStdoutCapture(t)

	created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	fake := &FakeBrowsersService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
			return &kernel.BrowserGetResponse{
				SessionID:          "sess-123",
				CdpWsURL:           "ws://cdp-url",
				BrowserLiveViewURL: "http://live-view",
				CreatedAt:          created,
				TimeoutSeconds:     300,
				Headless:           false,
				Stealth:            true,
				KioskMode:          false,
				Viewport:           shared.BrowserViewport{Width: 1920, Height: 1080, RefreshRate: 25},
				Persistence:        kernel.BrowserPersistence{ID: "persist-id"},
				Profile:            kernel.Profile{ID: "prof-id", Name: "my-profile"},
				ProxyID:            "proxy-123",
			}, nil
		},
	}
	b := BrowsersCmd{browsers: fake}
	_ = b.Get(context.Background(), BrowsersGetInput{Identifier: "sess-123"})

	out := outBuf.String()
	assert.Contains(t, out, "sess-123")
	assert.Contains(t, out, "ws://cdp-url")
	assert.Contains(t, out, "http://live-view")
	assert.Contains(t, out, "300")
	assert.Contains(t, out, "false") // Headless
	assert.Contains(t, out, "true")  // Stealth
	assert.Contains(t, out, "1920x1080@25")
	assert.Contains(t, out, "persist-id")
	assert.Contains(t, out, "my-profile")
	assert.Contains(t, out, "proxy-123")
}

func TestBrowsersGet_JSONOutput(t *testing.T) {
	// Capture both pterm output and raw stdout
	setupStdoutCapture(t)
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	fake := &FakeBrowsersService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
			return &kernel.BrowserGetResponse{
				SessionID: "sess-json",
				CdpWsURL:  "ws://cdp",
			}, nil
		},
	}
	b := BrowsersCmd{browsers: fake}
	_ = b.Get(context.Background(), BrowsersGetInput{Identifier: "sess-json", Output: "json"})

	// Capture stdout
	w.Close()
	var stdoutBuf bytes.Buffer
	io.Copy(&stdoutBuf, r)

	out := stdoutBuf.String()
	assert.Contains(t, out, "\"session_id\"")
	assert.Contains(t, out, "sess-json")
}

func TestBrowsersGet_NotFound(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowsersService{}
	b := BrowsersCmd{browsers: fake}
	err := b.Get(context.Background(), BrowsersGetInput{Identifier: "missing"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestBrowsersGet_Error(t *testing.T) {
	setupStdoutCapture(t)

	fake := &FakeBrowsersService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
			return nil, errors.New("get failed")
		},
	}
	b := BrowsersCmd{browsers: fake}
	err := b.Get(context.Background(), BrowsersGetInput{Identifier: "any"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get failed")
}

// --- Fakes for sub-services ---

type FakeReplaysService struct {
	ListFunc     func(ctx context.Context, id string, opts ...option.RequestOption) (*[]kernel.BrowserReplayListResponse, error)
	DownloadFunc func(ctx context.Context, replayID string, query kernel.BrowserReplayDownloadParams, opts ...option.RequestOption) (*http.Response, error)
	StartFunc    func(ctx context.Context, id string, body kernel.BrowserReplayStartParams, opts ...option.RequestOption) (*kernel.BrowserReplayStartResponse, error)
	StopFunc     func(ctx context.Context, replayID string, body kernel.BrowserReplayStopParams, opts ...option.RequestOption) error
}

func (f *FakeReplaysService) List(ctx context.Context, id string, opts ...option.RequestOption) (*[]kernel.BrowserReplayListResponse, error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, id, opts...)
	}
	empty := []kernel.BrowserReplayListResponse{}
	return &empty, nil
}
func (f *FakeReplaysService) Download(ctx context.Context, replayID string, query kernel.BrowserReplayDownloadParams, opts ...option.RequestOption) (*http.Response, error) {
	if f.DownloadFunc != nil {
		return f.DownloadFunc(ctx, replayID, query, opts...)
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
}
func (f *FakeReplaysService) Start(ctx context.Context, id string, body kernel.BrowserReplayStartParams, opts ...option.RequestOption) (*kernel.BrowserReplayStartResponse, error) {
	if f.StartFunc != nil {
		return f.StartFunc(ctx, id, body, opts...)
	}
	return &kernel.BrowserReplayStartResponse{ReplayID: "r-1", ReplayViewURL: "http://view", StartedAt: time.Now()}, nil
}
func (f *FakeReplaysService) Stop(ctx context.Context, replayID string, body kernel.BrowserReplayStopParams, opts ...option.RequestOption) error {
	if f.StopFunc != nil {
		return f.StopFunc(ctx, replayID, body, opts...)
	}
	return nil
}

type FakeFSService struct {
	NewDirectoryFunc       func(ctx context.Context, id string, body kernel.BrowserFNewDirectoryParams, opts ...option.RequestOption) error
	DeleteDirectoryFunc    func(ctx context.Context, id string, body kernel.BrowserFDeleteDirectoryParams, opts ...option.RequestOption) error
	DeleteFileFunc         func(ctx context.Context, id string, body kernel.BrowserFDeleteFileParams, opts ...option.RequestOption) error
	DownloadDirZipFunc     func(ctx context.Context, id string, query kernel.BrowserFDownloadDirZipParams, opts ...option.RequestOption) (*http.Response, error)
	FileInfoFunc           func(ctx context.Context, id string, query kernel.BrowserFFileInfoParams, opts ...option.RequestOption) (*kernel.BrowserFFileInfoResponse, error)
	ListFilesFunc          func(ctx context.Context, id string, query kernel.BrowserFListFilesParams, opts ...option.RequestOption) (*[]kernel.BrowserFListFilesResponse, error)
	MoveFunc               func(ctx context.Context, id string, body kernel.BrowserFMoveParams, opts ...option.RequestOption) error
	ReadFileFunc           func(ctx context.Context, id string, query kernel.BrowserFReadFileParams, opts ...option.RequestOption) (*http.Response, error)
	SetFilePermissionsFunc func(ctx context.Context, id string, body kernel.BrowserFSetFilePermissionsParams, opts ...option.RequestOption) error
	UploadFunc             func(ctx context.Context, id string, body kernel.BrowserFUploadParams, opts ...option.RequestOption) error
	UploadZipFunc          func(ctx context.Context, id string, body kernel.BrowserFUploadZipParams, opts ...option.RequestOption) error
	WriteFileFunc          func(ctx context.Context, id string, contents io.Reader, body kernel.BrowserFWriteFileParams, opts ...option.RequestOption) error
}

func (f *FakeFSService) NewDirectory(ctx context.Context, id string, body kernel.BrowserFNewDirectoryParams, opts ...option.RequestOption) error {
	if f.NewDirectoryFunc != nil {
		return f.NewDirectoryFunc(ctx, id, body, opts...)
	}
	return nil
}
func (f *FakeFSService) DeleteDirectory(ctx context.Context, id string, body kernel.BrowserFDeleteDirectoryParams, opts ...option.RequestOption) error {
	if f.DeleteDirectoryFunc != nil {
		return f.DeleteDirectoryFunc(ctx, id, body, opts...)
	}
	return nil
}
func (f *FakeFSService) DeleteFile(ctx context.Context, id string, body kernel.BrowserFDeleteFileParams, opts ...option.RequestOption) error {
	if f.DeleteFileFunc != nil {
		return f.DeleteFileFunc(ctx, id, body, opts...)
	}
	return nil
}
func (f *FakeFSService) DownloadDirZip(ctx context.Context, id string, query kernel.BrowserFDownloadDirZipParams, opts ...option.RequestOption) (*http.Response, error) {
	if f.DownloadDirZipFunc != nil {
		return f.DownloadDirZipFunc(ctx, id, query, opts...)
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/zip"}}, Body: io.NopCloser(strings.NewReader("zip"))}, nil
}
func (f *FakeFSService) FileInfo(ctx context.Context, id string, query kernel.BrowserFFileInfoParams, opts ...option.RequestOption) (*kernel.BrowserFFileInfoResponse, error) {
	if f.FileInfoFunc != nil {
		return f.FileInfoFunc(ctx, id, query, opts...)
	}
	return &kernel.BrowserFFileInfoResponse{Path: query.Path, Name: "name", Mode: "-rw-r--r--", IsDir: false, SizeBytes: 5, ModTime: time.Unix(0, 0)}, nil
}
func (f *FakeFSService) ListFiles(ctx context.Context, id string, query kernel.BrowserFListFilesParams, opts ...option.RequestOption) (*[]kernel.BrowserFListFilesResponse, error) {
	if f.ListFilesFunc != nil {
		return f.ListFilesFunc(ctx, id, query, opts...)
	}
	files := []kernel.BrowserFListFilesResponse{{Name: "f1", Path: "/f1", Mode: "-rw-r--r--", SizeBytes: 1, ModTime: time.Unix(0, 0)}}
	return &files, nil
}
func (f *FakeFSService) Move(ctx context.Context, id string, body kernel.BrowserFMoveParams, opts ...option.RequestOption) error {
	if f.MoveFunc != nil {
		return f.MoveFunc(ctx, id, body, opts...)
	}
	return nil
}
func (f *FakeFSService) ReadFile(ctx context.Context, id string, query kernel.BrowserFReadFileParams, opts ...option.RequestOption) (*http.Response, error) {
	if f.ReadFileFunc != nil {
		return f.ReadFileFunc(ctx, id, query, opts...)
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/octet-stream"}}, Body: io.NopCloser(strings.NewReader("content"))}, nil
}
func (f *FakeFSService) SetFilePermissions(ctx context.Context, id string, body kernel.BrowserFSetFilePermissionsParams, opts ...option.RequestOption) error {
	if f.SetFilePermissionsFunc != nil {
		return f.SetFilePermissionsFunc(ctx, id, body, opts...)
	}
	return nil
}
func (f *FakeFSService) Upload(ctx context.Context, id string, body kernel.BrowserFUploadParams, opts ...option.RequestOption) error {
	if f.UploadFunc != nil {
		return f.UploadFunc(ctx, id, body, opts...)
	}
	return nil
}
func (f *FakeFSService) UploadZip(ctx context.Context, id string, body kernel.BrowserFUploadZipParams, opts ...option.RequestOption) error {
	if f.UploadZipFunc != nil {
		return f.UploadZipFunc(ctx, id, body, opts...)
	}
	return nil
}
func (f *FakeFSService) WriteFile(ctx context.Context, id string, contents io.Reader, body kernel.BrowserFWriteFileParams, opts ...option.RequestOption) error {
	if f.WriteFileFunc != nil {
		return f.WriteFileFunc(ctx, id, contents, body, opts...)
	}
	return nil
}

type FakeProcessService struct {
	ExecFunc         func(ctx context.Context, id string, body kernel.BrowserProcessExecParams, opts ...option.RequestOption) (*kernel.BrowserProcessExecResponse, error)
	KillFunc         func(ctx context.Context, processID string, params kernel.BrowserProcessKillParams, opts ...option.RequestOption) (*kernel.BrowserProcessKillResponse, error)
	SpawnFunc        func(ctx context.Context, id string, body kernel.BrowserProcessSpawnParams, opts ...option.RequestOption) (*kernel.BrowserProcessSpawnResponse, error)
	StatusFunc       func(ctx context.Context, processID string, query kernel.BrowserProcessStatusParams, opts ...option.RequestOption) (*kernel.BrowserProcessStatusResponse, error)
	StdinFunc        func(ctx context.Context, processID string, params kernel.BrowserProcessStdinParams, opts ...option.RequestOption) (*kernel.BrowserProcessStdinResponse, error)
	StdoutStreamFunc func(ctx context.Context, processID string, query kernel.BrowserProcessStdoutStreamParams, opts ...option.RequestOption) *ssestream.Stream[kernel.BrowserProcessStdoutStreamResponse]
}

func (f *FakeProcessService) Exec(ctx context.Context, id string, body kernel.BrowserProcessExecParams, opts ...option.RequestOption) (*kernel.BrowserProcessExecResponse, error) {
	if f.ExecFunc != nil {
		return f.ExecFunc(ctx, id, body, opts...)
	}
	return &kernel.BrowserProcessExecResponse{ExitCode: 0, DurationMs: 10}, nil
}
func (f *FakeProcessService) Kill(ctx context.Context, processID string, params kernel.BrowserProcessKillParams, opts ...option.RequestOption) (*kernel.BrowserProcessKillResponse, error) {
	if f.KillFunc != nil {
		return f.KillFunc(ctx, processID, params, opts...)
	}
	return &kernel.BrowserProcessKillResponse{Ok: true}, nil
}
func (f *FakeProcessService) Spawn(ctx context.Context, id string, body kernel.BrowserProcessSpawnParams, opts ...option.RequestOption) (*kernel.BrowserProcessSpawnResponse, error) {
	if f.SpawnFunc != nil {
		return f.SpawnFunc(ctx, id, body, opts...)
	}
	return &kernel.BrowserProcessSpawnResponse{ProcessID: "proc-1", Pid: 123, StartedAt: time.Now()}, nil
}
func (f *FakeProcessService) Status(ctx context.Context, processID string, query kernel.BrowserProcessStatusParams, opts ...option.RequestOption) (*kernel.BrowserProcessStatusResponse, error) {
	if f.StatusFunc != nil {
		return f.StatusFunc(ctx, processID, query, opts...)
	}
	return &kernel.BrowserProcessStatusResponse{State: kernel.BrowserProcessStatusResponseStateRunning, CPUPct: 1.5, MemBytes: 2048, ExitCode: 0}, nil
}
func (f *FakeProcessService) Stdin(ctx context.Context, processID string, params kernel.BrowserProcessStdinParams, opts ...option.RequestOption) (*kernel.BrowserProcessStdinResponse, error) {
	if f.StdinFunc != nil {
		return f.StdinFunc(ctx, processID, params, opts...)
	}
	return &kernel.BrowserProcessStdinResponse{WrittenBytes: int64(len(params.DataB64))}, nil
}
func (f *FakeProcessService) StdoutStreamStreaming(ctx context.Context, processID string, query kernel.BrowserProcessStdoutStreamParams, opts ...option.RequestOption) *ssestream.Stream[kernel.BrowserProcessStdoutStreamResponse] {
	if f.StdoutStreamFunc != nil {
		return f.StdoutStreamFunc(ctx, processID, query, opts...)
	}
	return makeStream([]kernel.BrowserProcessStdoutStreamResponse{{Stream: kernel.BrowserProcessStdoutStreamResponseStreamStdout, DataB64: "aGVsbG8=", Event: ""}, {Event: "exit", ExitCode: 0}})
}

type FakeLogService struct {
	StreamFunc func(ctx context.Context, id string, query kernel.BrowserLogStreamParams, opts ...option.RequestOption) *ssestream.Stream[shared.LogEvent]
}

func (f *FakeLogService) StreamStreaming(ctx context.Context, id string, query kernel.BrowserLogStreamParams, opts ...option.RequestOption) *ssestream.Stream[shared.LogEvent] {
	if f.StreamFunc != nil {
		return f.StreamFunc(ctx, id, query, opts...)
	}
	now := time.Now()
	return makeStream([]shared.LogEvent{{Message: "m1", Timestamp: now}, {Message: "m2", Timestamp: now}})
}

// --- Helpers for SSE streams ---

type testDecoder struct {
	data [][]byte
	idx  int
}

func (d *testDecoder) Event() ssestream.Event { return ssestream.Event{Data: d.data[d.idx-1]} }
func (d *testDecoder) Next() bool {
	if d.idx >= len(d.data) {
		return false
	}
	d.idx++
	return true
}
func (d *testDecoder) Close() error { return nil }
func (d *testDecoder) Err() error   { return nil }

func makeStream[T any](vals []T) *ssestream.Stream[T] {
	var events [][]byte
	for _, v := range vals {
		b, _ := json.Marshal(v)
		events = append(events, b)
	}
	return ssestream.NewStream[T](&testDecoder{data: events}, nil)
}

// --- Fake for Computer ---

type FakeComputerService struct {
	ClickMouseFunc          func(ctx context.Context, id string, body kernel.BrowserComputerClickMouseParams, opts ...option.RequestOption) error
	MoveMouseFunc           func(ctx context.Context, id string, body kernel.BrowserComputerMoveMouseParams, opts ...option.RequestOption) error
	CaptureScreenshotFunc   func(ctx context.Context, id string, body kernel.BrowserComputerCaptureScreenshotParams, opts ...option.RequestOption) (*http.Response, error)
	PressKeyFunc            func(ctx context.Context, id string, body kernel.BrowserComputerPressKeyParams, opts ...option.RequestOption) error
	ScrollFunc              func(ctx context.Context, id string, body kernel.BrowserComputerScrollParams, opts ...option.RequestOption) error
	DragMouseFunc           func(ctx context.Context, id string, body kernel.BrowserComputerDragMouseParams, opts ...option.RequestOption) error
	TypeTextFunc            func(ctx context.Context, id string, body kernel.BrowserComputerTypeTextParams, opts ...option.RequestOption) error
	SetCursorVisibilityFunc func(ctx context.Context, id string, body kernel.BrowserComputerSetCursorVisibilityParams, opts ...option.RequestOption) (*kernel.BrowserComputerSetCursorVisibilityResponse, error)
}

func (f *FakeComputerService) ClickMouse(ctx context.Context, id string, body kernel.BrowserComputerClickMouseParams, opts ...option.RequestOption) error {
	if f.ClickMouseFunc != nil {
		return f.ClickMouseFunc(ctx, id, body, opts...)
	}
	return nil
}
func (f *FakeComputerService) MoveMouse(ctx context.Context, id string, body kernel.BrowserComputerMoveMouseParams, opts ...option.RequestOption) error {
	if f.MoveMouseFunc != nil {
		return f.MoveMouseFunc(ctx, id, body, opts...)
	}
	return nil
}
func (f *FakeComputerService) CaptureScreenshot(ctx context.Context, id string, body kernel.BrowserComputerCaptureScreenshotParams, opts ...option.RequestOption) (*http.Response, error) {
	if f.CaptureScreenshotFunc != nil {
		return f.CaptureScreenshotFunc(ctx, id, body, opts...)
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader("pngdata"))}, nil
}

func (f *FakeComputerService) PressKey(ctx context.Context, id string, body kernel.BrowserComputerPressKeyParams, opts ...option.RequestOption) error {
	if f.PressKeyFunc != nil {
		return f.PressKeyFunc(ctx, id, body, opts...)
	}
	return nil
}
func (f *FakeComputerService) Scroll(ctx context.Context, id string, body kernel.BrowserComputerScrollParams, opts ...option.RequestOption) error {
	if f.ScrollFunc != nil {
		return f.ScrollFunc(ctx, id, body, opts...)
	}
	return nil
}
func (f *FakeComputerService) DragMouse(ctx context.Context, id string, body kernel.BrowserComputerDragMouseParams, opts ...option.RequestOption) error {
	if f.DragMouseFunc != nil {
		return f.DragMouseFunc(ctx, id, body, opts...)
	}
	return nil
}
func (f *FakeComputerService) TypeText(ctx context.Context, id string, body kernel.BrowserComputerTypeTextParams, opts ...option.RequestOption) error {
	if f.TypeTextFunc != nil {
		return f.TypeTextFunc(ctx, id, body, opts...)
	}
	return nil
}
func (f *FakeComputerService) SetCursorVisibility(ctx context.Context, id string, body kernel.BrowserComputerSetCursorVisibilityParams, opts ...option.RequestOption) (*kernel.BrowserComputerSetCursorVisibilityResponse, error) {
	if f.SetCursorVisibilityFunc != nil {
		return f.SetCursorVisibilityFunc(ctx, id, body, opts...)
	}
	return &kernel.BrowserComputerSetCursorVisibilityResponse{}, nil
}

// --- Tests for Logs ---

// newFakeBrowsersServiceWithSimpleGet returns a FakeBrowsersService with a GetFunc that returns a browser with SessionID "id".
func newFakeBrowsersServiceWithSimpleGet() *FakeBrowsersService {
	return &FakeBrowsersService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.BrowserGetResponse, error) {
			return &kernel.BrowserGetResponse{SessionID: "id"}, nil
		},
	}
}

func TestBrowsersLogsStream_PrintsEvents(t *testing.T) {
	setupStdoutCapture(t)
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, logs: &FakeLogService{}}
	_ = b.LogsStream(context.Background(), BrowsersLogsStreamInput{Identifier: "id", Source: string(kernel.BrowserLogStreamParamsSourcePath), Follow: BoolFlag{Set: true, Value: true}, Path: "/var/log.txt"})
	out := outBuf.String()
	assert.Contains(t, out, "m1")
	assert.Contains(t, out, "m2")
}

// --- Tests for Replays ---

func TestBrowsersReplaysList_PrintsRows(t *testing.T) {
	setupStdoutCapture(t)
	created := time.Unix(0, 0)
	replays := []kernel.BrowserReplayListResponse{{ReplayID: "r1", StartedAt: created, FinishedAt: created, ReplayViewURL: "http://v"}}
	fake := &FakeReplaysService{ListFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*[]kernel.BrowserReplayListResponse, error) {
		return &replays, nil
	}}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, replays: fake}
	_ = b.ReplaysList(context.Background(), BrowsersReplaysListInput{Identifier: "id"})
	out := outBuf.String()
	assert.Contains(t, out, "r1")
	assert.Contains(t, out, "http://v")
}

func TestBrowsersReplaysStart_PrintsInfo(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeReplaysService{StartFunc: func(ctx context.Context, id string, body kernel.BrowserReplayStartParams, opts ...option.RequestOption) (*kernel.BrowserReplayStartResponse, error) {
		return &kernel.BrowserReplayStartResponse{ReplayID: "rid", ReplayViewURL: "http://view", StartedAt: time.Unix(0, 0)}, nil
	}}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, replays: fake}
	_ = b.ReplaysStart(context.Background(), BrowsersReplaysStartInput{Identifier: "id", Framerate: 30, MaxDurationSeconds: 60})
	out := outBuf.String()
	assert.Contains(t, out, "rid")
	assert.Contains(t, out, "http://view")
}

func TestBrowsersReplaysStop_PrintsSuccess(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeReplaysService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, replays: fake}
	_ = b.ReplaysStop(context.Background(), BrowsersReplaysStopInput{Identifier: "id", ReplayID: "rid"})
	out := outBuf.String()
	assert.Contains(t, out, "Stopped replay rid")
}

func TestBrowsersReplaysDownload_SavesFile(t *testing.T) {
	setupStdoutCapture(t)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "replay.mp4")
	fake := &FakeReplaysService{DownloadFunc: func(ctx context.Context, replayID string, query kernel.BrowserReplayDownloadParams, opts ...option.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"video/mp4"}}, Body: io.NopCloser(strings.NewReader("mp4data"))}, nil
	}}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, replays: fake}
	_ = b.ReplaysDownload(context.Background(), BrowsersReplaysDownloadInput{Identifier: "id", ReplayID: "rid", Output: outPath})
	data, err := os.ReadFile(outPath)
	assert.NoError(t, err)
	assert.Equal(t, "mp4data", string(data))
}

// --- Tests for Process ---

func TestBrowsersProcessExec_PrintsSummary(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeProcessService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, process: fake}
	_ = b.ProcessExec(context.Background(), BrowsersProcessExecInput{Identifier: "id", Command: "echo"})
	out := outBuf.String()
	assert.Contains(t, out, "Exit Code")
	assert.Contains(t, out, "Duration")
}

func TestBrowsersProcessSpawn_PrintsInfo(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeProcessService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, process: fake}
	_ = b.ProcessSpawn(context.Background(), BrowsersProcessSpawnInput{Identifier: "id", Command: "sleep"})
	out := outBuf.String()
	assert.Contains(t, out, "Process ID")
	assert.Contains(t, out, "PID")
}

func TestBrowsersProcessKill_PrintsSuccess(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeProcessService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, process: fake}
	_ = b.ProcessKill(context.Background(), BrowsersProcessKillInput{Identifier: "id", ProcessID: "proc", Signal: "TERM"})
	out := outBuf.String()
	assert.Contains(t, out, "Sent TERM to process proc")
}

func TestBrowsersProcessStatus_PrintsFields(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeProcessService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, process: fake}
	_ = b.ProcessStatus(context.Background(), BrowsersProcessStatusInput{Identifier: "id", ProcessID: "proc"})
	out := outBuf.String()
	assert.Contains(t, out, "State")
	assert.Contains(t, out, "CPU %")
	assert.Contains(t, out, "Mem Bytes")
}

func TestBrowsersProcessStdin_PrintsSuccess(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeProcessService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, process: fake}
	_ = b.ProcessStdin(context.Background(), BrowsersProcessStdinInput{Identifier: "id", ProcessID: "proc", DataB64: "ZGF0YQ=="})
	out := outBuf.String()
	assert.Contains(t, out, "Wrote to stdin")
}

func TestBrowsersProcessStdoutStream_PrintsExit(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeProcessService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, process: fake}
	_ = b.ProcessStdoutStream(context.Background(), BrowsersProcessStdoutStreamInput{Identifier: "id", ProcessID: "proc"})
	out := outBuf.String()
	assert.Contains(t, out, "process exited with code 0")
}

// --- Tests for FS ---

func TestBrowsersFSNewDirectory_PrintsSuccess(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeFSService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, fs: fake}
	_ = b.FSNewDirectory(context.Background(), BrowsersFSNewDirInput{Identifier: "id", Path: "/tmp/x"})
	out := outBuf.String()
	assert.Contains(t, out, "Created directory /tmp/x")
}

func TestBrowsersFSDeleteDirectory_PrintsSuccess(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeFSService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, fs: fake}
	_ = b.FSDeleteDirectory(context.Background(), BrowsersFSDeleteDirInput{Identifier: "id", Path: "/tmp/x"})
	out := outBuf.String()
	assert.Contains(t, out, "Deleted directory /tmp/x")
}

func TestBrowsersFSDeleteFile_PrintsSuccess(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeFSService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, fs: fake}
	_ = b.FSDeleteFile(context.Background(), BrowsersFSDeleteFileInput{Identifier: "id", Path: "/tmp/file"})
	out := outBuf.String()
	assert.Contains(t, out, "Deleted file /tmp/file")
}

func TestBrowsersFSDownloadDirZip_SavesFile(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.zip")
	fake := &FakeFSService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, fs: fake}
	_ = b.FSDownloadDirZip(context.Background(), BrowsersFSDownloadDirZipInput{Identifier: "id", Path: "/tmp", Output: outPath})
	data, err := os.ReadFile(outPath)
	assert.NoError(t, err)
	assert.Equal(t, "zip", string(data))
}

func TestBrowsersFSFileInfo_PrintsFields(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeFSService{FileInfoFunc: func(ctx context.Context, id string, query kernel.BrowserFFileInfoParams, opts ...option.RequestOption) (*kernel.BrowserFFileInfoResponse, error) {
		return &kernel.BrowserFFileInfoResponse{Path: "/tmp/a", Name: "a", Mode: "-rw-r--r--", IsDir: false, SizeBytes: 1, ModTime: time.Unix(0, 0)}, nil
	}}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, fs: fake}
	_ = b.FSFileInfo(context.Background(), BrowsersFSFileInfoInput{Identifier: "id", Path: "/tmp/a"})
	out := outBuf.String()
	assert.Contains(t, out, "Path")
	assert.Contains(t, out, "/tmp/a")
}

func TestBrowsersFSListFiles_PrintsRows(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeFSService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, fs: fake}
	_ = b.FSListFiles(context.Background(), BrowsersFSListFilesInput{Identifier: "id", Path: "/"})
	out := outBuf.String()
	assert.Contains(t, out, "f1")
	assert.Contains(t, out, "/f1")
}

func TestBrowsersFSMove_PrintsSuccess(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeFSService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, fs: fake}
	_ = b.FSMove(context.Background(), BrowsersFSMoveInput{Identifier: "id", SrcPath: "/a", DestPath: "/b"})
	out := outBuf.String()
	assert.Contains(t, out, "Moved /a -> /b")
}

func TestBrowsersFSReadFile_SavesFile(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "file.txt")
	fake := &FakeFSService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, fs: fake}
	_ = b.FSReadFile(context.Background(), BrowsersFSReadFileInput{Identifier: "id", Path: "/tmp/x", Output: outPath})
	data, err := os.ReadFile(outPath)
	assert.NoError(t, err)
	assert.Equal(t, "content", string(data))
}

func TestBrowsersFSSetPermissions_PrintsSuccess(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeFSService{}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, fs: fake}
	_ = b.FSSetPermissions(context.Background(), BrowsersFSSetPermsInput{Identifier: "id", Path: "/tmp/a", Mode: "644"})
	out := outBuf.String()
	assert.Contains(t, out, "Updated permissions for /tmp/a")
}

func TestBrowsersFSUpload_MappingAndDestDir_Success(t *testing.T) {
	setupStdoutCapture(t)
	var captured kernel.BrowserFUploadParams
	fake := &FakeFSService{UploadFunc: func(ctx context.Context, id string, body kernel.BrowserFUploadParams, opts ...option.RequestOption) error {
		captured = body
		return nil
	}}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, fs: fake}
	in := BrowsersFSUploadInput{Identifier: "id", Mappings: []struct {
		Local string
		Dest  string
	}{{Local: __writeTempFile(t, "a"), Dest: "/remote/a"}}, DestDir: "/remote/dir", Paths: []string{__writeTempFile(t, "b")}}
	_ = b.FSUpload(context.Background(), in)
	out := outBuf.String()
	assert.Contains(t, out, "Uploaded")
	assert.Equal(t, 2, len(captured.Files))
}

func TestBrowsersFSUploadZip_Success(t *testing.T) {
	setupStdoutCapture(t)
	z := __writeTempFile(t, "zipdata")
	fake := &FakeFSService{UploadZipFunc: func(ctx context.Context, id string, body kernel.BrowserFUploadZipParams, opts ...option.RequestOption) error {
		return nil
	}}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, fs: fake}
	_ = b.FSUploadZip(context.Background(), BrowsersFSUploadZipInput{Identifier: "id", ZipPath: z, DestDir: "/dst"})
	out := outBuf.String()
	assert.Contains(t, out, "Uploaded zip")
}

func TestBrowsersFSWriteFile_FromBase64_And_FromInput(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeFSService{WriteFileFunc: func(ctx context.Context, id string, contents io.Reader, body kernel.BrowserFWriteFileParams, opts ...option.RequestOption) error {
		return nil
	}}
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	b := BrowsersCmd{browsers: fakeBrowsers, fs: fake}
	// input mode
	p := __writeTempFile(t, "hello")
	_ = b.FSWriteFile(context.Background(), BrowsersFSWriteFileInput{Identifier: "id", DestPath: "/y", SourcePath: p, Mode: "644"})
	out := outBuf.String()
	assert.Contains(t, out, "Wrote file to /y")
}

// helper to create temp file with contents
func __writeTempFile(t *testing.T, data string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "cli-test-*")
	assert.NoError(t, err)
	_, err = f.WriteString(data)
	assert.NoError(t, err)
	_ = f.Close()
	return f.Name()
}

// --- Tests for Computer ---

func TestBrowsersComputerClickMouse_PrintsSuccess(t *testing.T) {
	setupStdoutCapture(t)
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	fakeComp := &FakeComputerService{}
	b := BrowsersCmd{browsers: fakeBrowsers, computer: fakeComp}
	_ = b.ComputerClickMouse(context.Background(), BrowsersComputerClickMouseInput{Identifier: "id", X: 10, Y: 20, NumClicks: 2, Button: string(kernel.BrowserComputerClickMouseParamsButtonLeft), ClickType: string(kernel.BrowserComputerClickMouseParamsClickTypeClick), HoldKeys: []string{"shift"}})
	out := outBuf.String()
	assert.Contains(t, out, "Clicked mouse at (10,20)")
}

func TestBrowsersComputerMoveMouse_PrintsSuccess(t *testing.T) {
	setupStdoutCapture(t)
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	fakeComp := &FakeComputerService{}
	b := BrowsersCmd{browsers: fakeBrowsers, computer: fakeComp}
	_ = b.ComputerMoveMouse(context.Background(), BrowsersComputerMoveMouseInput{Identifier: "id", X: 5, Y: 6})
	out := outBuf.String()
	assert.Contains(t, out, "Moved mouse to (5,6)")
}

func TestBrowsersComputerScreenshot_SavesFile(t *testing.T) {
	setupStdoutCapture(t)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "shot.png")
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	fakeComp := &FakeComputerService{CaptureScreenshotFunc: func(ctx context.Context, id string, body kernel.BrowserComputerCaptureScreenshotParams, opts ...option.RequestOption) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader("pngDATA"))}, nil
	}}
	b := BrowsersCmd{browsers: fakeBrowsers, computer: fakeComp}
	_ = b.ComputerScreenshot(context.Background(), BrowsersComputerScreenshotInput{Identifier: "id", X: 0, Y: 0, Width: 10, Height: 10, To: outPath})
	data, err := os.ReadFile(outPath)
	assert.NoError(t, err)
	assert.Equal(t, "pngDATA", string(data))
}

func TestBrowsersComputerPressKey_PrintsSuccess(t *testing.T) {
	setupStdoutCapture(t)
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	fakeComp := &FakeComputerService{}
	b := BrowsersCmd{browsers: fakeBrowsers, computer: fakeComp}
	_ = b.ComputerPressKey(context.Background(), BrowsersComputerPressKeyInput{Identifier: "id", Keys: []string{"Return", "Shift"}, Duration: 25, HoldKeys: []string{"Ctrl"}})
	out := outBuf.String()
	assert.Contains(t, out, "Pressed keys: Return,Shift")
}

func TestBrowsersComputerScroll_PrintsSuccess(t *testing.T) {
	setupStdoutCapture(t)
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	fakeComp := &FakeComputerService{}
	b := BrowsersCmd{browsers: fakeBrowsers, computer: fakeComp}
	_ = b.ComputerScroll(context.Background(), BrowsersComputerScrollInput{Identifier: "id", X: 100, Y: 200, DeltaY: 120, DeltaYSet: true})
	out := outBuf.String()
	assert.Contains(t, out, "Scrolled at (100,200)")
}

func TestBrowsersComputerDragMouse_PrintsSuccess(t *testing.T) {
	setupStdoutCapture(t)
	fakeBrowsers := newFakeBrowsersServiceWithSimpleGet()
	fakeComp := &FakeComputerService{}
	b := BrowsersCmd{browsers: fakeBrowsers, computer: fakeComp}
	path := [][]int64{{0, 0}, {50, 50}, {100, 100}}
	_ = b.ComputerDragMouse(context.Background(), BrowsersComputerDragMouseInput{Identifier: "id", Path: path, Delay: 50, Button: string(kernel.BrowserComputerDragMouseParamsButtonLeft)})
	out := outBuf.String()
	assert.Contains(t, out, "Dragged mouse over 3 points")
}

func TestParseViewport_ValidFormats(t *testing.T) {
	tests := []struct {
		input       string
		wantWidth   int64
		wantHeight  int64
		wantRefresh int64
	}{
		{"1920x1080@25", 1920, 1080, 25},
		{"2560x1440@10", 2560, 1440, 10},
		{"1024x768@60", 1024, 768, 60},
		{"1920x1080", 1920, 1080, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			w, h, r, err := parseViewport(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantWidth, w)
			assert.Equal(t, tt.wantHeight, h)
			assert.Equal(t, tt.wantRefresh, r)
		})
	}
}

func TestParseViewport_InvalidFormats(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"1920", "missing height"},
		{"1920x", "incomplete dimension"},
		{"x1080", "missing width"},
		{"1920x1080@", "missing refresh rate"},
		{"1920x1080@abc", "non-numeric refresh rate"},
		{"abcxdef", "non-numeric dimensions"},
		{"1920x1080@25@30", "too many @ signs"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			_, _, _, err := parseViewport(tt.input)
			assert.Error(t, err)
		})
	}
}

func TestGetAvailableViewports_ReturnsExpectedOptions(t *testing.T) {
	viewports := getAvailableViewports()
	assert.Len(t, viewports, 6)
	assert.Contains(t, viewports, "2560x1440@10")
	assert.Contains(t, viewports, "1920x1080@25")
	assert.Contains(t, viewports, "1920x1200@25")
	assert.Contains(t, viewports, "1440x900@25")
	assert.Contains(t, viewports, "1200x800@60")
	assert.Contains(t, viewports, "1024x768@60")
}

func TestBrowsersCreate_WithViewport(t *testing.T) {
	setupStdoutCapture(t)
	var captured kernel.BrowserNewParams
	fake := &FakeBrowsersService{NewFunc: func(ctx context.Context, body kernel.BrowserNewParams, opts ...option.RequestOption) (*kernel.BrowserNewResponse, error) {
		captured = body
		return &kernel.BrowserNewResponse{SessionID: "session123", CdpWsURL: "ws://example"}, nil
	}}
	b := BrowsersCmd{browsers: fake}

	err := b.Create(context.Background(), BrowsersCreateInput{
		Viewport: "1920x1080@25",
	})

	assert.NoError(t, err)
	assert.Equal(t, int64(1920), captured.Viewport.Width)
	assert.Equal(t, int64(1080), captured.Viewport.Height)
	assert.True(t, captured.Viewport.RefreshRate.Valid())
	assert.Equal(t, int64(25), captured.Viewport.RefreshRate.Value)
}

func TestBrowsersCreate_WithViewportNoRefreshRate(t *testing.T) {
	setupStdoutCapture(t)
	var captured kernel.BrowserNewParams
	fake := &FakeBrowsersService{NewFunc: func(ctx context.Context, body kernel.BrowserNewParams, opts ...option.RequestOption) (*kernel.BrowserNewResponse, error) {
		captured = body
		return &kernel.BrowserNewResponse{SessionID: "session123", CdpWsURL: "ws://example"}, nil
	}}
	b := BrowsersCmd{browsers: fake}

	err := b.Create(context.Background(), BrowsersCreateInput{
		Viewport: "1920x1080",
	})

	assert.NoError(t, err)
	assert.Equal(t, int64(1920), captured.Viewport.Width)
	assert.Equal(t, int64(1080), captured.Viewport.Height)
	assert.False(t, captured.Viewport.RefreshRate.Valid())
}

func TestBrowsersCreate_WithInvalidViewport(t *testing.T) {
	setupStdoutCapture(t)
	fake := &FakeBrowsersService{}
	b := BrowsersCmd{browsers: fake}

	err := b.Create(context.Background(), BrowsersCreateInput{
		Viewport: "invalid",
	})

	assert.NoError(t, err)
	out := outBuf.String()
	assert.Contains(t, out, "Invalid viewport format")
}
