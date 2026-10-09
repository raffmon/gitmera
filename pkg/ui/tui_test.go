package ui_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gitmera/pkg/runner"
	"gitmera/pkg/ui"

	tea "charm.land/bubbletea/v2"
)

func newTestTasks() []runner.RepoTask {
	return []runner.RepoTask{
		{Name: "api"},
		{Name: "web"},
		{Name: "infra"},
	}
}

func TestTUIModel_PendingToActiveTransition(t *testing.T) {
	tasks := newTestTasks()
	eventChan := make(chan runner.TaskEvent, 1)
	model := ui.NewTUIModel(tasks, "cloning", eventChan, nil)

	if !strings.Contains(model.View().Content, "api: pending") {
		t.Fatalf("expected initial view to show 'api' as pending, got: %q", model.View().Content)
	}

	updated, _ := model.Update(runner.TaskEvent{RepoName: "api", Type: runner.TaskStarted})
	m := updated.(ui.TUIModel)

	view := m.View().Content
	if strings.Contains(view, "api: pending") {
		t.Errorf("expected 'api' to no longer be pending after TaskStarted, got: %q", view)
	}
	if !strings.Contains(view, "api") {
		t.Errorf("expected view to still mention 'api', got: %q", view)
	}
}

func TestTUIModel_SuccessTransition(t *testing.T) {
	tasks := newTestTasks()
	eventChan := make(chan runner.TaskEvent, 2)
	model := ui.NewTUIModel(tasks, "cloning", eventChan, nil)

	updated, _ := model.Update(runner.TaskEvent{RepoName: "api", Type: runner.TaskStarted})
	m := updated.(ui.TUIModel)

	updated, _ = m.Update(runner.TaskEvent{RepoName: "api", Type: runner.TaskFinished})
	m = updated.(ui.TUIModel)

	view := m.View().Content
	if !strings.Contains(view, "✓") {
		t.Errorf("expected a green checkmark (✓) for a successful task, got: %q", view)
	}
	if !strings.Contains(view, "api") {
		t.Errorf("expected view to mention 'api', got: %q", view)
	}
}

func TestTUIModel_FailedTransition(t *testing.T) {
	tasks := newTestTasks()
	eventChan := make(chan runner.TaskEvent, 2)
	model := ui.NewTUIModel(tasks, "pushing", eventChan, nil)

	updated, _ := model.Update(runner.TaskEvent{RepoName: "web", Type: runner.TaskStarted})
	m := updated.(ui.TUIModel)

	updated, _ = m.Update(runner.TaskEvent{
		RepoName: "web",
		Type:     runner.TaskFinished,
		Err:      errors.New("push rejected"),
	})
	m = updated.(ui.TUIModel)

	view := m.View().Content
	if !strings.Contains(view, "✗") {
		t.Errorf("expected a red cross (✗) for a failed task, got: %q", view)
	}
}

func TestTUIModel_SkippedTransition(t *testing.T) {
	tasks := newTestTasks()
	eventChan := make(chan runner.TaskEvent, 1)
	model := ui.NewTUIModel(tasks, "checking out main", eventChan, nil)

	updated, _ := model.Update(runner.TaskEvent{
		RepoName: "infra",
		Type:     runner.TaskSkipped,
		Skipped:  true,
	})
	m := updated.(ui.TUIModel)

	view := m.View().Content
	if !strings.Contains(view, "⚠") {
		t.Errorf("expected a yellow warning (⚠) for a skipped task, got: %q", view)
	}
}

func TestTUIModel_SkippedWithErrorRendersCancelled(t *testing.T) {
	tasks := newTestTasks()
	eventChan := make(chan runner.TaskEvent, 1)
	model := ui.NewTUIModel(tasks, "cloning", eventChan, nil)

	updated, _ := model.Update(runner.TaskEvent{
		RepoName: "infra",
		Type:     runner.TaskSkipped,
		Err:      context.Canceled,
		Skipped:  true,
	})
	m := updated.(ui.TUIModel)

	view := m.View().Content
	if !strings.Contains(view, "⚠") {
		t.Errorf("expected a yellow warning (⚠) for a cancelled task, got: %q", view)
	}
	if !strings.Contains(view, "cancelled") {
		t.Errorf("expected view to mention 'cancelled' for a fail-fast-cancelled task, got: %q", view)
	}
}

func TestTUIModel_UnknownRepoEventIsIgnored(t *testing.T) {
	tasks := newTestTasks()
	eventChan := make(chan runner.TaskEvent, 1)
	model := ui.NewTUIModel(tasks, "cloning", eventChan, nil)

	updated, _ := model.Update(runner.TaskEvent{RepoName: "does-not-exist", Type: runner.TaskStarted})
	m := updated.(ui.TUIModel)

	view := m.View().Content
	for _, name := range []string{"api", "web", "infra"} {
		if !strings.Contains(view, name) {
			t.Errorf("expected view to still list %q, got: %q", name, view)
		}
	}
}

func TestTUIModel_CtrlCCancelsAndQuits(t *testing.T) {
	tasks := newTestTasks()
	eventChan := make(chan runner.TaskEvent, 1)

	cancelled := false
	cancel := func() { cancelled = true }

	model := ui.NewTUIModel(tasks, "cloning", eventChan, cancel)

	ctrlC := tea.KeyPressMsg(tea.Key{Mod: tea.ModCtrl, Code: 'c'})
	if ctrlC.String() != "ctrl+c" {
		t.Fatalf("test setup error: expected synthesized key to stringify as 'ctrl+c', got %q", ctrlC.String())
	}

	_, cmd := model.Update(ctrlC)

	if !cancelled {
		t.Error("expected ctrl+c to invoke the cancel function")
	}
	if cmd == nil {
		t.Fatal("expected ctrl+c to return a non-nil tea.Cmd")
	}

	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected ctrl+c command to yield tea.QuitMsg, got %T", msg)
	}
}

func TestTUIModel_CtrlCWithNilCancelDoesNotPanic(t *testing.T) {
	tasks := newTestTasks()
	eventChan := make(chan runner.TaskEvent, 1)
	model := ui.NewTUIModel(tasks, "cloning", eventChan, nil)

	ctrlC := tea.KeyPressMsg(tea.Key{Mod: tea.ModCtrl, Code: 'c'})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("expected no panic with nil cancel, got: %v", r)
		}
	}()

	_, cmd := model.Update(ctrlC)
	if cmd == nil {
		t.Fatal("expected a non-nil tea.Cmd even with nil cancel")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestTUIModel_InitReturnsNonNilCommand(t *testing.T) {
	tasks := newTestTasks()
	eventChan := make(chan runner.TaskEvent, 1)
	model := ui.NewTUIModel(tasks, "cloning", eventChan, nil)

	cmd := model.Init()
	if cmd == nil {
		t.Fatal("expected Init() to return a non-nil batched command")
	}
}

func TestTUIModel_ViewListsAllRepositories(t *testing.T) {
	tasks := newTestTasks()
	eventChan := make(chan runner.TaskEvent, 1)
	model := ui.NewTUIModel(tasks, "cloning", eventChan, nil)

	view := model.View().Content
	for _, task := range tasks {
		if !strings.Contains(view, task.Name) {
			t.Errorf("expected initial view to list repository %q, got: %q", task.Name, view)
		}
	}
}

func TestRunSequentialFallback_PrintsLifecycleLines(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	eventChan := make(chan runner.TaskEvent, 4)
	eventChan <- runner.TaskEvent{RepoName: "api", Type: runner.TaskStarted}
	eventChan <- runner.TaskEvent{RepoName: "api", Type: runner.TaskFinished}
	eventChan <- runner.TaskEvent{RepoName: "web", Type: runner.TaskFinished, Err: errors.New("boom")}
	eventChan <- runner.TaskEvent{RepoName: "infra", Type: runner.TaskSkipped, Skipped: true}
	close(eventChan)

	ui.RunSequentialFallback(eventChan, logger)

	out := buf.String()
	for _, want := range []string{
		"api: started",
		"api: success",
		"web: failed",
		"infra: skipped",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected sequential fallback output to contain %q, got: %q", want, out)
		}
	}
}

func TestRunSequentialFallback_NoColorStripsANSI(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	eventChan := make(chan runner.TaskEvent, 3)
	eventChan <- runner.TaskEvent{RepoName: "api", Type: runner.TaskStarted}
	eventChan <- runner.TaskEvent{RepoName: "api", Type: runner.TaskFinished}
	eventChan <- runner.TaskEvent{RepoName: "web", Type: runner.TaskFinished, Err: errors.New("boom")}
	close(eventChan)

	ui.RunSequentialFallback(eventChan, logger)

	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("expected no ANSI escape sequences with forceNoColor, got: %q", out)
	}
}

func TestRunSequentialFallback_CancelledTasksRenderWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	eventChan := make(chan runner.TaskEvent, 1)
	eventChan <- runner.TaskEvent{
		RepoName: "infra",
		Type:     runner.TaskSkipped,
		Err:      context.Canceled,
		Skipped:  true,
	}
	close(eventChan)

	ui.RunSequentialFallback(eventChan, logger)

	out := buf.String()
	if !strings.Contains(out, "infra: cancelled") {
		t.Errorf("expected a 'cancelled' line for a fail-fast-cancelled task, got: %q", out)
	}
}

func TestRunSequentialFallback_FailFastCancellationNoDuplicateLines(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	tasks := []runner.RepoTask{
		{Name: "fail_instant"},
		{Name: "queued1"},
	}
	action := func(ctx context.Context, task runner.RepoTask) (string, bool, error) {
		if task.Name == "fail_instant" {
			return "", false, errors.New("instant error")
		}
		return "", false, nil
	}

	eventChan := make(chan runner.TaskEvent, len(tasks)*2)
	go func() {
		runner.ExecuteTasks(context.Background(), tasks, 1, true, 1*time.Second, action, eventChan)
		close(eventChan)
	}()

	ui.RunSequentialFallback(eventChan, logger)

	out := buf.String()
	count := strings.Count(out, "queued1: cancelled")
	if count != 1 {
		t.Errorf("expected exactly 1 'queued1: cancelled' line, got %d in output: %q", count, out)
	}
}

func TestOrchestrateExecution_NonInteractiveRunsFallback(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	tasks := newTestTasks()
	action := func(ctx context.Context, task runner.RepoTask) (string, bool, error) {
		if task.Name == "web" {
			return "stderr detail", false, errors.New("simulated failure")
		}
		return "", false, nil
	}

	results := ui.OrchestrateExecution(context.Background(), tasks, 3, false, time.Second, action, ui.ExecutionOptions{
		Interactive: false,
		ActionLabel: "cloning",
		Logger:      logger,
	})

	if len(results) != len(tasks) {
		t.Fatalf("expected %d results, got %d", len(tasks), len(results))
	}

	out := buf.String()
	if !strings.Contains(out, "api: success") {
		t.Errorf("expected 'api: success' in fallback output, got: %q", out)
	}
	if !strings.Contains(out, "web: failed") {
		t.Errorf("expected 'web: failed' in fallback output, got: %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("expected no ANSI escape sequences with forceNoColor logger, got: %q", out)
	}
}

func TestOrchestrateExecution_BufferSizeGuarantee(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	const n = 25
	tasks := make([]runner.RepoTask, 0, n)
	for i := 0; i < n; i++ {
		tasks = append(tasks, runner.RepoTask{Name: "repo" + string(rune('a'+i))})
	}

	action := func(ctx context.Context, task runner.RepoTask) (string, bool, error) {
		return "", false, nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		ui.OrchestrateExecution(context.Background(), tasks, n, false, time.Second, action, ui.ExecutionOptions{
			Interactive: false,
			ActionLabel: "cloning",
			Logger:      logger,
		})
	}()

	select {
	case <-done:

	case <-time.After(5 * time.Second):
		t.Fatal("OrchestrateExecution did not complete in time; possible event channel deadlock")
	}
}

func TestOrchestrateExecution_EmptyTasks(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	action := func(ctx context.Context, task runner.RepoTask) (string, bool, error) {
		return "", false, nil
	}

	done := make(chan struct{})
	var results []runner.TaskResult
	go func() {
		defer close(done)
		results = ui.OrchestrateExecution(context.Background(), nil, 1, false, time.Second, action, ui.ExecutionOptions{
			Interactive: false,
			ActionLabel: "cloning",
			Logger:      logger,
		})
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("OrchestrateExecution with empty tasks did not complete in time")
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results for empty task list, got %d", len(results))
	}
}

func TestOrchestrateExecution_Interactive_NonTTY(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ui.OrchestrateExecution(context.Background(), nil, 1, false, time.Second,
			func(ctx context.Context, task runner.RepoTask) (string, bool, error) {
				return "", false, nil
			},
			ui.ExecutionOptions{Interactive: true, ActionLabel: "cloning"})
	}()

	select {
	case <-done:

	case <-time.After(10 * time.Second):
		t.Fatal("OrchestrateExecution with Interactive=true hung in non-TTY test environment")
	}
}

func TestFlushStdin_DoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("FlushStdin panicked: %v", r)
		}
	}()
	ui.FlushStdin()
}

func TestDrainStdinNonblocking_DoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DrainStdinNonblocking panicked: %v", r)
		}
	}()
	ui.DrainStdinNonblocking()
}
