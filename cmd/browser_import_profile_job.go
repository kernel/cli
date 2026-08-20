package cmd

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	localbrowser "github.com/kernel/cli/internal/browserimport"
	"github.com/pterm/pterm"
)

type profileImportStage int32

const (
	profileImportStagePreparing profileImportStage = iota
	profileImportStageUploading
	profileImportStageApplying
	profileImportStageReady
)

type browserProfileImportClient interface {
	SubmitInventory(context.Context, string, string, localbrowser.Inventory) (localbrowser.Status, error)
	SubmitSelection(context.Context, string, localbrowser.Selection) (localbrowser.Status, error)
	Upload(context.Context, string, string, []byte) (localbrowser.Status, error)
	Wait(context.Context, string, time.Duration) (localbrowser.Status, error)
	WaitForProfile(context.Context, string, time.Duration) (localbrowser.Status, error)
}

type profileImportRequest struct {
	importID         string
	helperToken      string
	dashboardHandoff bool
	inventory        localbrowser.Inventory
	selection        localbrowser.Selection
	bundle           []byte
	waitTimeout      time.Duration
}

type profileImportResult struct {
	status   localbrowser.Status
	duration time.Duration
}

type profileImportJob struct {
	cancel context.CancelFunc
	stage  atomic.Int32
	done   chan struct{}

	mu     sync.Mutex
	result profileImportResult
	err    error
}

func startProfileImport(ctx context.Context, client browserProfileImportClient, request profileImportRequest) *profileImportJob {
	jobCtx, cancel := context.WithCancel(ctx)
	job := &profileImportJob{cancel: cancel, done: make(chan struct{})}
	job.stage.Store(int32(profileImportStagePreparing))
	go func() {
		defer close(job.done)
		result, err := runProfileImport(jobCtx, client, request, &job.stage)
		job.mu.Lock()
		job.result = result
		job.err = err
		job.mu.Unlock()
	}()
	return job
}

func runProfileImport(ctx context.Context, client browserProfileImportClient, request profileImportRequest, stage *atomic.Int32) (profileImportResult, error) {
	startedAt := time.Now()
	status, err := client.SubmitInventory(ctx, request.importID, request.helperToken, request.inventory)
	if err != nil {
		return profileImportResult{}, browserImportProgressError(request.importID, status.Phase, time.Since(startedAt), err)
	}
	status, err = client.SubmitSelection(ctx, request.importID, request.selection)
	if err != nil {
		return profileImportResult{}, browserImportProgressError(request.importID, status.Phase, time.Since(startedAt), err)
	}
	stage.Store(int32(profileImportStageUploading))
	status, err = client.Upload(ctx, request.importID, request.helperToken, request.bundle)
	if err != nil {
		return profileImportResult{}, browserImportProgressError(request.importID, status.Phase, time.Since(startedAt), err)
	}
	stage.Store(int32(profileImportStageApplying))
	waitCtx, cancel := context.WithTimeout(ctx, request.waitTimeout)
	defer cancel()
	if request.dashboardHandoff {
		status, err = client.WaitForProfile(waitCtx, request.importID, 2*time.Second)
	} else {
		status, err = client.Wait(waitCtx, request.importID, 2*time.Second)
	}
	if err != nil {
		return profileImportResult{}, fmt.Errorf("browser import %s did not complete: %w; check it with: kernel profiles import-status %s", request.importID, err, request.importID)
	}
	stage.Store(int32(profileImportStageReady))
	return profileImportResult{status: status, duration: time.Since(startedAt)}, nil
}

func (j *profileImportJob) Stage() profileImportStage {
	return profileImportStage(j.stage.Load())
}

func (j *profileImportJob) Wait(ctx context.Context) (profileImportResult, error) {
	select {
	case <-ctx.Done():
		return profileImportResult{}, ctx.Err()
	case <-j.done:
		j.mu.Lock()
		defer j.mu.Unlock()
		return j.result, j.err
	}
}

func (j *profileImportJob) Cancel() {
	j.cancel()
}

func waitForProfileImport(ctx context.Context, job *profileImportJob, targetName string, humanOutput bool) (profileImportResult, error) {
	if !humanOutput {
		return job.Wait(ctx)
	}
	current := job.Stage()
	progress, _ := pterm.DefaultProgressbar.
		WithTotal(len(profileImportProgressStages)).
		WithCurrent(int(current)).
		WithTitle(fmt.Sprintf("%s: %q", profileImportProgressStages[current], targetName)).
		WithShowElapsedTime().
		Start()
	defer progress.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return profileImportResult{}, ctx.Err()
		case <-job.done:
			result, err := job.Wait(ctx)
			if err == nil {
				progress.Current = len(profileImportProgressStages)
				progress.UpdateTitle(fmt.Sprintf("%s: %q", profileImportProgressStages[profileImportStageReady], targetName))
			}
			return result, err
		case <-ticker.C:
			next := job.Stage()
			if next == current {
				continue
			}
			current = next
			progress.Current = int(current)
			progress.UpdateTitle(fmt.Sprintf("%s: %q", profileImportProgressStages[current], targetName))
		}
	}
}
