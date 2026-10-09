package ui

import (
	"testing"

	"gitmera/pkg/runner"
)

func TestUpdate_SpinnerTickMsg_WhenQuitting(t *testing.T) {
	tasks := []runner.RepoTask{{Name: "api"}}
	ch := make(chan runner.TaskEvent, 1)
	m := NewTUIModel(tasks, "cloning", ch, nil)
	m.quitting = true

	_, cmd := m.Update(spinnerTickMsg{})
	if cmd != nil {
		t.Error("expected nil cmd when model is quitting")
	}
}

func TestUpdate_SpinnerTickMsg_WhenNotQuitting(t *testing.T) {
	tasks := []runner.RepoTask{{Name: "api"}}
	ch := make(chan runner.TaskEvent, 1)
	m := NewTUIModel(tasks, "cloning", ch, nil)

	updated, cmd := m.Update(spinnerTickMsg{})
	m2 := updated.(TUIModel)

	if cmd == nil {
		t.Error("expected non-nil cmd (tickSpinner) when not quitting")
	}
	if m2.spinnerFrame != 1 {
		t.Errorf("expected spinnerFrame=1 after one tick, got %d", m2.spinnerFrame)
	}
}

func TestUpdate_EventClosedMsg(t *testing.T) {
	tasks := []runner.RepoTask{{Name: "api"}}
	ch := make(chan runner.TaskEvent, 1)
	m := NewTUIModel(tasks, "cloning", ch, nil)

	updated, cmd := m.Update(eventClosedMsg{})
	m2 := updated.(TUIModel)

	if !m2.closed {
		t.Error("expected closed=true after eventClosedMsg")
	}
	if !m2.quitting {
		t.Error("expected quitting=true after eventClosedMsg")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (tea.Quit) after eventClosedMsg")
	}
}

func TestUpdate_TaskEvent_WhenAlreadyClosed(t *testing.T) {
	tasks := []runner.RepoTask{{Name: "api"}}
	ch := make(chan runner.TaskEvent, 1)
	m := NewTUIModel(tasks, "cloning", ch, nil)
	m.closed = true

	_, cmd := m.Update(runner.TaskEvent{RepoName: "api", Type: runner.TaskStarted})
	if cmd != nil {
		t.Error("expected nil cmd when model is already closed")
	}
}

func TestUpdate_UnhandledMessageType(t *testing.T) {
	tasks := []runner.RepoTask{{Name: "api"}}
	ch := make(chan runner.TaskEvent, 1)
	m := NewTUIModel(tasks, "cloning", ch, nil)

	type unknownMsg struct{ data string }
	updated, cmd := m.Update(unknownMsg{"irrelevant"})
	m2 := updated.(TUIModel)

	if cmd != nil {
		t.Error("expected nil cmd for unhandled message type")
	}
	if m2.quitting {
		t.Error("expected quitting=false for unhandled message type")
	}
}

func TestWaitForEvent_ReceivesEvent(t *testing.T) {
	ch := make(chan runner.TaskEvent, 1)
	expected := runner.TaskEvent{RepoName: "api", Type: runner.TaskStarted}
	ch <- expected

	cmd := waitForEvent(ch)
	msg := cmd()

	event, ok := msg.(runner.TaskEvent)
	if !ok {
		t.Fatalf("expected runner.TaskEvent, got %T", msg)
	}
	if event.RepoName != expected.RepoName || event.Type != expected.Type {
		t.Errorf("expected event %+v, got %+v", expected, event)
	}
}

func TestWaitForEvent_ClosedChannel(t *testing.T) {
	ch := make(chan runner.TaskEvent)
	close(ch)

	cmd := waitForEvent(ch)
	msg := cmd()

	if _, ok := msg.(eventClosedMsg); !ok {
		t.Fatalf("expected eventClosedMsg, got %T", msg)
	}
}

func TestTickSpinner_ReturnsNonNilCmd(t *testing.T) {
	cmd := tickSpinner()
	if cmd == nil {
		t.Error("expected non-nil cmd from tickSpinner")
	}
}

func TestTickSpinner_InnerCallbackExecuted(t *testing.T) {
	cmd := tickSpinner()
	msg := cmd()
	if _, ok := msg.(spinnerTickMsg); !ok {
		t.Errorf("expected spinnerTickMsg from tickSpinner inner callback, got %T", msg)
	}
}
