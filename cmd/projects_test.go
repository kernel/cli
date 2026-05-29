package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/kernel/kernel-go-sdk/packages/respjson"
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

func (f *FakeProjectsService) Update(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (*kernel.Project, error) {
	if f.UpdateFunc != nil {
		return f.UpdateFunc(ctx, id, body, opts...)
	}
	return &kernel.Project{ID: id, Name: body.UpdateProjectRequest.Name.Value}, nil
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

func TestProjectsList_JSONOutput(t *testing.T) {
	project := mustProject(t, `{"id":"a12345678901234567890123","name":"Default","status":"active","created_at":"2026-05-29T12:00:00Z"}`)
	c := ProjectsCmd{
		projects: &FakeProjectsService{
			ListFunc: func(ctx context.Context, query kernel.ProjectListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Project], error) {
				return &pagination.OffsetPagination[kernel.Project]{Items: []kernel.Project{project}}, nil
			},
		},
		limits: &FakeProjectLimitsService{},
	}

	var err error
	out := captureStdout(t, func() {
		err = c.List(context.Background(), ProjectsListInput{Output: "json"})
	})

	require.NoError(t, err)
	assert.JSONEq(t, `[{"id":"a12345678901234567890123","name":"Default","status":"active","created_at":"2026-05-29T12:00:00Z"}]`, out)
}

func TestProjectsList_InvalidOutput(t *testing.T) {
	c := ProjectsCmd{
		projects: &FakeProjectsService{
			ListFunc: func(ctx context.Context, query kernel.ProjectListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Project], error) {
				t.Fatal("List should not be called")
				return nil, nil
			},
		},
		limits: &FakeProjectLimitsService{},
	}

	err := c.List(context.Background(), ProjectsListInput{Output: "yaml"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported --output value")
}

func TestProjectsGet_JSONOutput(t *testing.T) {
	project := mustProject(t, `{"id":"a12345678901234567890123","name":"Default","status":"active","created_at":"2026-05-29T12:00:00Z"}`)
	c := ProjectsCmd{
		projects: &FakeProjectsService{
			GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.Project, error) {
				assert.Equal(t, "a12345678901234567890123", id)
				return &project, nil
			},
		},
		limits: &FakeProjectLimitsService{},
	}

	var err error
	out := captureStdout(t, func() {
		err = c.Get(context.Background(), ProjectsGetInput{Identifier: "a12345678901234567890123", Output: "json"})
	})

	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"a12345678901234567890123","name":"Default","status":"active","created_at":"2026-05-29T12:00:00Z"}`, out)
}

func TestProjectsGet_InvalidOutput(t *testing.T) {
	c := ProjectsCmd{
		projects: &FakeProjectsService{
			GetFunc: func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.Project, error) {
				t.Fatal("Get should not be called")
				return nil, nil
			},
		},
		limits: &FakeProjectLimitsService{},
	}

	err := c.Get(context.Background(), ProjectsGetInput{Identifier: "a12345678901234567890123", Output: "yaml"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported --output value")
}

func TestProjectsUpdate_Success(t *testing.T) {
	buf := capturePtermOutput(t)
	c := ProjectsCmd{
		projects: &FakeProjectsService{
			UpdateFunc: func(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (*kernel.Project, error) {
				assert.Equal(t, "a12345678901234567890123", id)
				assert.True(t, body.UpdateProjectRequest.Name.Valid())
				assert.Equal(t, "Renamed", body.UpdateProjectRequest.Name.Value)
				return &kernel.Project{ID: id, Name: body.UpdateProjectRequest.Name.Value}, nil
			},
		},
	}

	err := c.Update(context.Background(), ProjectsUpdateInput{
		Identifier: "a12345678901234567890123",
		Name:       "Renamed",
	})

	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "Updated project: Renamed")
	assert.Contains(t, out, "a12345678901234567890123")
}

func TestProjectsUpdate_JSONOutput(t *testing.T) {
	project := mustProject(t, `{"id":"a12345678901234567890123","name":"Renamed","status":"active","created_at":"2026-05-29T12:00:00Z","updated_at":"2026-05-29T12:30:00Z"}`)
	c := ProjectsCmd{
		projects: &FakeProjectsService{
			UpdateFunc: func(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (*kernel.Project, error) {
				assert.Equal(t, "a12345678901234567890123", id)
				assert.True(t, body.UpdateProjectRequest.Name.Valid())
				assert.Equal(t, "Renamed", body.UpdateProjectRequest.Name.Value)
				return &project, nil
			},
		},
	}

	var err error
	out := captureStdout(t, func() {
		err = c.Update(context.Background(), ProjectsUpdateInput{
			Identifier: "a12345678901234567890123",
			Name:       "Renamed",
			Output:     "json",
		})
	})

	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"a12345678901234567890123","name":"Renamed","status":"active","created_at":"2026-05-29T12:00:00Z","updated_at":"2026-05-29T12:30:00Z"}`, out)
}

func TestProjectsUpdate_ResolvesName(t *testing.T) {
	c := ProjectsCmd{
		projects: &FakeProjectsService{
			ListFunc: func(ctx context.Context, query kernel.ProjectListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Project], error) {
				return &pagination.OffsetPagination[kernel.Project]{
					Items: []kernel.Project{{ID: "a12345678901234567890123", Name: "Default"}},
				}, nil
			},
			UpdateFunc: func(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (*kernel.Project, error) {
				assert.Equal(t, "a12345678901234567890123", id)
				return &kernel.Project{ID: id, Name: body.UpdateProjectRequest.Name.Value}, nil
			},
		},
	}

	err := c.Update(context.Background(), ProjectsUpdateInput{
		Identifier: "default",
		Name:       "Renamed",
	})

	require.NoError(t, err)
}

func TestProjectsUpdate_RequiresName(t *testing.T) {
	c := ProjectsCmd{
		projects: &FakeProjectsService{
			UpdateFunc: func(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (*kernel.Project, error) {
				t.Fatal("Update should not be called")
				return nil, nil
			},
		},
	}

	err := c.Update(context.Background(), ProjectsUpdateInput{Identifier: "a12345678901234567890123"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--name is required")
	assert.Contains(t, err.Error(), "add --name <name>")
}

func TestProjectsUpdate_InvalidOutput(t *testing.T) {
	c := ProjectsCmd{
		projects: &FakeProjectsService{
			UpdateFunc: func(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (*kernel.Project, error) {
				t.Fatal("Update should not be called")
				return nil, nil
			},
		},
	}

	err := c.Update(context.Background(), ProjectsUpdateInput{
		Identifier: "a12345678901234567890123",
		Name:       "Renamed",
		Output:     "yaml",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported --output value")
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

func mustProject(t *testing.T, raw string) kernel.Project {
	t.Helper()

	var project kernel.Project
	require.NoError(t, json.Unmarshal([]byte(raw), &project))
	return project
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	fn()

	require.NoError(t, writer.Close())
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = io.Copy(&buf, reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	return buf.String()
}
