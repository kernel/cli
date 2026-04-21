package cmd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/kernel-go-sdk/packages/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type FakeProjectsService struct {
	NewFunc    func(ctx context.Context, body kernel.ProjectNewParams, opts ...option.RequestOption) (*kernel.Project, error)
	GetFunc    func(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.Project, error)
	UpdateFunc func(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (*kernel.Project, error)
	ListFunc   func(ctx context.Context, query kernel.ProjectListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Project], error)
	DeleteFunc func(ctx context.Context, id string, opts ...option.RequestOption) error
}

func (f *FakeProjectsService) New(ctx context.Context, body kernel.ProjectNewParams, opts ...option.RequestOption) (*kernel.Project, error) {
	if f.NewFunc != nil {
		return f.NewFunc(ctx, body, opts...)
	}
	return &kernel.Project{ID: "proj-new", Name: body.CreateProjectRequest.Name}, nil
}

func (f *FakeProjectsService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*kernel.Project, error) {
	if f.GetFunc != nil {
		return f.GetFunc(ctx, id, opts...)
	}
	return &kernel.Project{ID: id, Name: "project", Status: kernel.ProjectStatusActive}, nil
}

func (f *FakeProjectsService) Update(ctx context.Context, id string, body kernel.ProjectUpdateParams, opts ...option.RequestOption) (*kernel.Project, error) {
	if f.UpdateFunc != nil {
		return f.UpdateFunc(ctx, id, body, opts...)
	}
	return &kernel.Project{ID: id, Name: body.UpdateProjectRequest.Name.Value, Status: kernel.ProjectStatusActive}, nil
}

func (f *FakeProjectsService) List(ctx context.Context, query kernel.ProjectListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Project], error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, query, opts...)
	}
	return &pagination.OffsetPagination[kernel.Project]{Items: []kernel.Project{}}, nil
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

func TestProjectsList_HasMore(t *testing.T) {
	buf := captureProfilesOutput(t)
	created := time.Unix(0, 0)
	items := make([]kernel.Project, 3)
	for i := range items {
		items[i] = kernel.Project{
			ID:        fmt.Sprintf("proj-%d", i),
			Name:      fmt.Sprintf("Project %d", i),
			Status:    kernel.ProjectStatusActive,
			CreatedAt: created,
			UpdatedAt: created,
		}
	}

	fakeProjects := &FakeProjectsService{
		ListFunc: func(ctx context.Context, query kernel.ProjectListParams, opts ...option.RequestOption) (*pagination.OffsetPagination[kernel.Project], error) {
			require.True(t, query.Limit.Valid())
			require.True(t, query.Offset.Valid())
			assert.Equal(t, int64(3), query.Limit.Value)
			assert.Equal(t, int64(0), query.Offset.Value)
			return &pagination.OffsetPagination[kernel.Project]{Items: items}, nil
		},
	}

	p := ProjectsCmd{projects: fakeProjects, limits: &FakeProjectLimitsService{}}
	err := p.List(context.Background(), ProjectsListInput{Page: 1, PerPage: 2})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "proj-0")
	assert.Contains(t, out, "proj-1")
	assert.NotContains(t, out, "proj-2")
	assert.Contains(t, out, "Has more: yes")
	assert.Contains(t, out, "Next: kernel projects list --page 2 --per-page 2")
}

func TestProjectsUpdateLimits_OnlyChangedFields(t *testing.T) {
	fakeLimits := &FakeProjectLimitsService{
		UpdateFunc: func(ctx context.Context, id string, body kernel.ProjectLimitUpdateParams, opts ...option.RequestOption) (*kernel.ProjectLimits, error) {
			assert.Equal(t, "proj_123", id)
			assert.True(t, body.UpdateProjectLimitsRequest.MaxConcurrentInvocations.Valid())
			assert.Equal(t, int64(15), body.UpdateProjectLimitsRequest.MaxConcurrentInvocations.Value)
			assert.False(t, body.UpdateProjectLimitsRequest.MaxConcurrentSessions.Valid())
			assert.False(t, body.UpdateProjectLimitsRequest.MaxPersistentSessions.Valid())
			assert.True(t, body.UpdateProjectLimitsRequest.MaxPooledSessions.Valid())
			assert.Equal(t, int64(0), body.UpdateProjectLimitsRequest.MaxPooledSessions.Value)

			return &kernel.ProjectLimits{
				MaxConcurrentInvocations: 15,
			}, nil
		},
	}

	p := ProjectsCmd{projects: &FakeProjectsService{}, limits: fakeLimits}
	err := p.UpdateLimits(context.Background(), ProjectLimitsUpdateInput{
		ID:                       "proj_123",
		MaxConcurrentInvocations: Int64Flag{Set: true, Value: 15},
		MaxPooledSessions:        Int64Flag{Set: true, Value: 0},
	})
	require.NoError(t, err)
}
