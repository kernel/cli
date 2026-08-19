package cmd

import (
	"context"
	"sync"
	"testing"
	"time"

	localbrowser "github.com/kernel/cli/internal/browserimport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type blockingProfileImportClient struct {
	mu         sync.Mutex
	calls      []string
	waitCalled chan struct{}
	release    chan struct{}
}

func (c *blockingProfileImportClient) record(call string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, call)
}

func (c *blockingProfileImportClient) SubmitInventory(context.Context, string, string, localbrowser.Inventory) (localbrowser.Status, error) {
	c.record("inventory")
	return localbrowser.Status{Phase: "awaiting_selection"}, nil
}

func (c *blockingProfileImportClient) SubmitSelection(context.Context, string, localbrowser.Selection) (localbrowser.Status, error) {
	c.record("selection")
	return localbrowser.Status{Phase: "awaiting_upload"}, nil
}

func (c *blockingProfileImportClient) Upload(context.Context, string, string, []byte) (localbrowser.Status, error) {
	c.record("upload")
	return localbrowser.Status{Phase: "applying"}, nil
}

func (c *blockingProfileImportClient) Wait(context.Context, string, time.Duration) (localbrowser.Status, error) {
	return c.wait()
}

func (c *blockingProfileImportClient) WaitForProfile(context.Context, string, time.Duration) (localbrowser.Status, error) {
	return c.wait()
}

func (c *blockingProfileImportClient) wait() (localbrowser.Status, error) {
	c.record("wait")
	close(c.waitCalled)
	<-c.release
	return localbrowser.Status{Phase: "awaiting_client_completion", Applied: &localbrowser.Applied{Profiles: []localbrowser.AppliedProfile{{ProfileID: "prof_1"}}}}, nil
}

func TestProfileImportRunsWhileManagedAuthIsSelected(t *testing.T) {
	client := &blockingProfileImportClient{waitCalled: make(chan struct{}), release: make(chan struct{})}
	job := startProfileImport(t.Context(), client, profileImportRequest{
		importID: "bri_1", helperToken: "grant", dashboardHandoff: true,
		inventory: localbrowser.Inventory{}, selection: localbrowser.Selection{}, bundle: []byte("bundle"), waitTimeout: time.Minute,
	})

	select {
	case <-client.waitCalled:
		assert.Equal(t, profileImportStageApplying, job.Stage())
	case <-time.After(time.Second):
		t.Fatal("profile import did not reach server-side apply while the caller remained interactive")
	}

	client.mu.Lock()
	assert.Equal(t, []string{"inventory", "selection", "upload", "wait"}, client.calls)
	client.mu.Unlock()
	close(client.release)

	result, err := job.Wait(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "prof_1", result.status.Applied.Profiles[0].ProfileID)
	assert.Equal(t, profileImportStageReady, job.Stage())
}
