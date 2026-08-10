package cmd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/kernel/kernel-go-sdk/packages/respjson"
	"github.com/pterm/pterm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type FakeProjectsService struct {
	ListFunc   func(ctx context.Context, query kernel.ProjectListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Project], error)
	NewFunc    func(ctx context.Context, body kernel.ProjectNewParams, opts ...option.RequestOption) (*kernel.Project, error)
	GetFunc    func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.Project, error)
	UpdateFunc func(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (*kernel.Project, error)
	DeleteFunc func(ctx context.Context, id string, opts ...option.RequestOption) error
}

func (f *FakeProjectsService) Update(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (*kernel.Project, error) {
	if f.UpdateFunc != nil {
		return f.UpdateFunc(ctx, id, body, opts...)
	}
	return &kernel.Project{ID: id}, nil
}

func (f *FakeProjectsService) List(ctx context.Context, query kernel.ProjectListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Project], error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, query, opts...)
	}
	return &pagination.OffsetPagination[kernel.Project]{Items: []kernel.Project{}}, nil
}

func (f *FakeProjectsService) New(ctx context.Context, body kernel.ProjectNewParams, opts ...option.RequestOption) (*kernel.Project, error) {
	if f.NewFunc != nil {
		return f.NewFunc(ctx, body, opts...)
	}
	return &kernel.Project{ID: "proj_default", Name: body.CreateProjectRequest.Name}, nil
}

func (f *FakeProjectsService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.Project, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, id, opts...)
	}
	return &kernel.Project{ID: id, Name: "default"}, nil
}

func (f *FakeProjectsService) Delete(ctx context.Context, id string, opts ...option.RequestOption) error {
	if f.DeleteFunc != nil {
		return f.DeleteFunc(ctx, id, opts...)
	}
	return nil
}

type FakeProjectLimitsService struct {
	GetFunc    func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ProjectLimits, error)
	UpdateFunc func(ctx context.Context, id string, body kernel.ProjectLimitUpdateParams, opts ...option.RequestOption) (*kernel.ProjectLimits, error)
}

func (f *FakeProjectLimitsService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ProjectLimits, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, id, opts...)
	}
	return &kernel.ProjectLimits{}, nil
}

func (f *FakeProjectLimitsService) Update(ctx context.Context, id string, body kernel.ProjectLimitUpdateParams, opts ...option.RequestOption) (*kernel.ProjectLimits, error) {
	if f.UpdateFunc != nil {
		return f.UpdateFunc(ctx, id, body, opts...)
	}
	return &kernel.ProjectLimits{}, nil
}

func TestProjectsList_UsesSDKResponsePaginationMetadata(t *testing.T) {
	const responseBody = `[
		{"id":"project-alpha","name":"alpha","status":"active","created_at":"2026-08-08T12:00:00Z","updated_at":"2026-08-08T12:00:00Z"},
		{"id":"project-beta","name":"beta","status":"archived","created_at":"2026-08-08T12:01:00Z","updated_at":"2026-08-08T12:01:00Z"}
	]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/org/projects", r.URL.Path)
		assert.Equal(t, "2", r.URL.Query().Get("limit"))
		assert.Equal(t, "20", r.URL.Query().Get("offset"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Has-More", "true")
		w.Header().Set("X-Next-Offset", "22")
		_, _ = w.Write([]byte(responseBody))
	}))
	defer server.Close()

	client := kernel.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"))
	c := ProjectsCmd{projects: &client.Projects, limits: &client.Projects.Limits}

	buf := capturePtermOutput(t)
	err := c.List(context.Background(), ProjectsListInput{Limit: 2, Offset: 20})
	require.NoError(t, err)
	out := pterm.RemoveColorFromString(buf.String())
	assert.Contains(t, out, "idx")
	assert.Regexp(t, `(?m)^project-alpha\s+\| alpha\s+\| active\s+\| [^|]+\| 20\s*$`, out)
	assert.Regexp(t, `(?m)^project-beta\s+\| beta\s+\| archived\s+\| [^|]+\| 21\s*$`, out)
	assert.Contains(t, out, "kernel projects list --limit 2 --offset 22")

	jsonOutput := captureStdout(t, func() {
		err = c.List(context.Background(), ProjectsListInput{Limit: 2, Offset: 20, Output: "json"})
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"projects":`+responseBody+`,"next_offset":22}`, jsonOutput)
}

func TestProjectsList_RejectsInvalidPagination(t *testing.T) {
	fakeProjects := &FakeProjectsService{
		ListFunc: func(ctx context.Context, query kernel.ProjectListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Project], error) {
			t.Fatal("List should not be called")
			return nil, nil
		},
	}
	c := ProjectsCmd{projects: fakeProjects, limits: &FakeProjectLimitsService{}}

	for _, in := range []ProjectsListInput{
		{Limit: 0},
		{Limit: 101},
		{Limit: 100, Offset: -1},
		{Limit: 100, Output: "yaml"},
	} {
		assert.Error(t, c.List(context.Background(), in))
	}
}

func TestParseProjectListPagination(t *testing.T) {
	tests := []struct {
		name     string
		response *http.Response
		want     projectListPagination
		wantErr  string
	}{
		{
			name:     "more results",
			response: &http.Response{Header: http.Header{"X-Has-More": []string{"true"}, "X-Next-Offset": []string{"120"}}},
			want:     projectListPagination{HasMore: true, NextOffset: 120},
		},
		{
			name:     "terminal page",
			response: &http.Response{Header: http.Header{"X-Has-More": []string{"false"}, "X-Next-Offset": []string{"0"}}},
			want:     projectListPagination{},
		},
		{name: "missing response", wantErr: "missing pagination headers"},
		{
			name:     "missing has more",
			response: &http.Response{Header: http.Header{"X-Next-Offset": []string{"120"}}},
			wantErr:  "invalid X-Has-More",
		},
		{
			name:     "has more with missing cursor",
			response: &http.Response{Header: http.Header{"X-Has-More": []string{"true"}}},
			wantErr:  "invalid X-Next-Offset",
		},
		{
			name:     "has more with malformed cursor",
			response: &http.Response{Header: http.Header{"X-Has-More": []string{"true"}, "X-Next-Offset": []string{"next"}}},
			wantErr:  "invalid X-Next-Offset",
		},
		{
			name:     "has more with terminal cursor",
			response: &http.Response{Header: http.Header{"X-Has-More": []string{"true"}, "X-Next-Offset": []string{"0"}}},
			wantErr:  "X-Next-Offset is not positive",
		},
		{
			name:     "terminal page with cursor",
			response: &http.Response{Header: http.Header{"X-Has-More": []string{"false"}, "X-Next-Offset": []string{"120"}}},
			wantErr:  "X-Has-More is false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseProjectListPagination(tt.response)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMarshalProjectsListJSON_EmptyPage(t *testing.T) {
	data, err := marshalProjectsListJSON(nil, 0)
	require.NoError(t, err)
	assert.JSONEq(t, `{"projects":[]}`, string(data))
}

func TestProjectsLimitsGet_DefaultOutput(t *testing.T) {
	buf := capturePtermOutput(t)
	limits := &kernel.ProjectLimits{
		MaxConcurrentSessions:    10,
		MaxConcurrentInvocations: 5,
	}
	limits.JSON.MaxConcurrentSessions = respjson.NewField("10")
	limits.JSON.MaxConcurrentInvocations = respjson.NewField("5")

	fakeProjects := &FakeProjectsService{}
	fakeLimits := &FakeProjectLimitsService{
		GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.ProjectLimits, error) {
			return limits, nil
		},
	}
	c := ProjectsCmd{projects: fakeProjects, limits: fakeLimits}

	err := c.LimitsGet(context.Background(), ProjectsLimitsGetInput{
		Identifier: "a12345678901234567890123",
	})
	assert.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "Max Concurrent Sessions")
	assert.Contains(t, out, "10")
	assert.Contains(t, out, "unlimited")
}

func TestProjectsLimitsGet_InvalidOutput(t *testing.T) {
	c := ProjectsCmd{projects: &FakeProjectsService{}, limits: &FakeProjectLimitsService{}}
	err := c.LimitsGet(context.Background(), ProjectsLimitsGetInput{
		Identifier: "a12345678901234567890123",
		Output:     "yaml",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported --output value")
}

func TestProjectsLimitsSet_InvalidOutput(t *testing.T) {
	c := ProjectsCmd{
		projects: &FakeProjectsService{},
		limits: &FakeProjectLimitsService{
			UpdateFunc: func(ctx context.Context, id string, body kernel.ProjectLimitUpdateParams, opts ...option.RequestOption) (*kernel.ProjectLimits, error) {
				t.Fatal("Update should not be called")
				return nil, nil
			},
		},
	}
	err := c.LimitsSet(context.Background(), ProjectsLimitsSetInput{
		Identifier: "a12345678901234567890123",
		Output:     "yaml",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported --output value")
}

func TestProjectsLimitsSet_RejectsNegativeValues(t *testing.T) {
	c := ProjectsCmd{projects: &FakeProjectsService{}, limits: &FakeProjectLimitsService{}}
	err := c.LimitsSet(context.Background(), ProjectsLimitsSetInput{
		Identifier: "a12345678901234567890123",
		MaxConcurrentSessions: Int64Flag{
			Set:   true,
			Value: -1,
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--max-concurrent-sessions must be non-negative")
}

func TestProjectsLimitsSet_Success(t *testing.T) {
	buf := capturePtermOutput(t)
	fakeProjects := &FakeProjectsService{}
	fakeLimits := &FakeProjectLimitsService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.ProjectLimitUpdateParams, opts ...option.RequestOption) (*kernel.ProjectLimits, error) {
			assert.Equal(t, "a12345678901234567890123", id)
			assert.True(t, body.UpdateProjectLimitsRequest.MaxConcurrentSessions.Valid())
			assert.Equal(t, int64(7), body.UpdateProjectLimitsRequest.MaxConcurrentSessions.Value)

			updated := &kernel.ProjectLimits{MaxConcurrentSessions: 7}
			updated.JSON.MaxConcurrentSessions = respjson.NewField("7")
			return updated, nil
		},
	}
	c := ProjectsCmd{projects: fakeProjects, limits: fakeLimits}

	err := c.LimitsSet(context.Background(), ProjectsLimitsSetInput{
		Identifier: "a12345678901234567890123",
		MaxConcurrentSessions: Int64Flag{
			Set:   true,
			Value: 7,
		},
	})
	assert.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "Project limits updated")
	assert.Contains(t, out, "7")
}

func TestResolveProjectByName_PaginatesAcrossResults(t *testing.T) {
	var seenOffsets []int64
	fakeProjects := &FakeProjectsService{
		ListFunc: func(ctx context.Context, query kernel.ProjectListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Project], error) {
			seenOffsets = append(seenOffsets, query.Offset.Value)
			assert.True(t, query.Limit.Valid())
			assert.Equal(t, int64(100), query.Limit.Value)

			if query.Offset.Value == 0 {
				page := make([]kernel.Project, 100)
				for i := range page {
					page[i] = kernel.Project{ID: "proj_a", Name: "first-page"}
				}
				return &pagination.OffsetPagination[kernel.Project]{Items: page}, nil
			}

			if query.Offset.Value == 100 {
				return &pagination.OffsetPagination[kernel.Project]{
					Items: []kernel.Project{{ID: "proj_target", Name: "Target Name"}},
				}, nil
			}

			return nil, errors.New("unexpected offset")
		},
	}

	id, err := resolveProjectByName(context.Background(), fakeProjects, "target name")
	assert.NoError(t, err)
	assert.Equal(t, "proj_target", id)
	assert.Equal(t, []int64{0, 100}, seenOffsets)
}

func TestProjectsUpdate_RenamesByID(t *testing.T) {
	buf := capturePtermOutput(t)
	var capturedID string
	var capturedBody kernel.ProjectUpdateParams
	fakeProjects := &FakeProjectsService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (*kernel.Project, error) {
			capturedID = id
			capturedBody = body
			return &kernel.Project{ID: id, Name: "renamed", Status: kernel.ProjectStatusActive}, nil
		},
	}
	c := ProjectsCmd{projects: fakeProjects, limits: &FakeProjectLimitsService{}}
	err := c.Update(context.Background(), ProjectsUpdateInput{
		Identifier: "cm5xk8n2p0000abcdefghijk",
		Name:       "renamed",
		NameSet:    true,
	})
	assert.NoError(t, err)
	assert.Equal(t, "cm5xk8n2p0000abcdefghijk", capturedID)
	assert.True(t, capturedBody.UpdateProjectRequest.Name.Valid())
	assert.Equal(t, "renamed", capturedBody.UpdateProjectRequest.Name.Value)
	assert.Contains(t, buf.String(), "Updated project: renamed")
}

func TestProjectsUpdate_ResolvesNameToID(t *testing.T) {
	capturePtermOutput(t)
	var capturedID string
	fakeProjects := &FakeProjectsService{
		ListFunc: func(ctx context.Context, query kernel.ProjectListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Project], error) {
			return &pagination.OffsetPagination[kernel.Project]{
				Items: []kernel.Project{{ID: "proj_target", Name: "my-project"}},
			}, nil
		},
		UpdateFunc: func(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (*kernel.Project, error) {
			capturedID = id
			return &kernel.Project{ID: id, Name: "my-project", Status: kernel.ProjectStatusArchived}, nil
		},
	}
	c := ProjectsCmd{projects: fakeProjects, limits: &FakeProjectLimitsService{}}
	err := c.Update(context.Background(), ProjectsUpdateInput{Identifier: "my-project", Status: "archived"})
	assert.NoError(t, err)
	assert.Equal(t, "proj_target", capturedID)
}

func TestProjectsUpdate_SetsStatus(t *testing.T) {
	capturePtermOutput(t)
	var capturedBody kernel.ProjectUpdateParams
	fakeProjects := &FakeProjectsService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (*kernel.Project, error) {
			capturedBody = body
			return &kernel.Project{ID: id, Status: kernel.ProjectStatusArchived}, nil
		},
	}
	c := ProjectsCmd{projects: fakeProjects, limits: &FakeProjectLimitsService{}}
	err := c.Update(context.Background(), ProjectsUpdateInput{
		Identifier: "cm5xk8n2p0000abcdefghijk",
		Status:     "archived",
	})
	assert.NoError(t, err)
	assert.Equal(t, kernel.UpdateProjectRequestStatusArchived, capturedBody.UpdateProjectRequest.Status)
	assert.False(t, capturedBody.UpdateProjectRequest.Name.Valid())
}

func TestProjectsUpdate_RejectsUnknownStatus(t *testing.T) {
	c := ProjectsCmd{projects: &FakeProjectsService{}, limits: &FakeProjectLimitsService{}}
	err := c.Update(context.Background(), ProjectsUpdateInput{Identifier: "p1", Status: "deleted"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --status")
}

func TestProjectsUpdate_RequiresAtLeastOneField(t *testing.T) {
	c := ProjectsCmd{projects: &FakeProjectsService{}, limits: &FakeProjectLimitsService{}}
	err := c.Update(context.Background(), ProjectsUpdateInput{Identifier: "p1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one of --name or --status")
}
