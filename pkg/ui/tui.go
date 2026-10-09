package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitmera/pkg/runner"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type repoProgress struct {
	name    string
	action  string
	status  string
	message string
	err     error
}

type spinnerTickMsg struct{}

func tickSpinner() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

type eventClosedMsg struct{}

func waitForEvent(ch <-chan runner.TaskEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return eventClosedMsg{}
		}
		return event
	}
}

type TUIModel struct {
	tasks        []repoProgress
	taskMap      map[string]int
	spinnerFrame int
	eventChan    <-chan runner.TaskEvent
	cancel       context.CancelFunc
	quitting     bool
	closed       bool
}

func NewTUIModel(tasks []runner.RepoTask, action string, eventChan <-chan runner.TaskEvent, cancel context.CancelFunc) TUIModel {
	progress := make([]repoProgress, len(tasks))
	taskMap := make(map[string]int, len(tasks))
	for i, t := range tasks {
		progress[i] = repoProgress{name: t.Name, action: action, status: "pending"}
		taskMap[t.Name] = i
	}
	return TUIModel{
		tasks:     progress,
		taskMap:   taskMap,
		eventChan: eventChan,
		cancel:    cancel,
	}
}

func (m TUIModel) Init() tea.Cmd {
	return tea.Batch(waitForEvent(m.eventChan), tickSpinner())
}

func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case runner.TaskEvent:
		if idx, found := m.taskMap[msg.RepoName]; found {
			switch msg.Type {
			case runner.TaskStarted:
				m.tasks[idx].status = "active"
			case runner.TaskFinished:
				if msg.Err != nil {
					m.tasks[idx].status = "failed"
					m.tasks[idx].err = msg.Err
				} else {
					m.tasks[idx].status = "success"
				}
			case runner.TaskSkipped:
				m.tasks[idx].status = "skipped"
				m.tasks[idx].err = msg.Err
				m.tasks[idx].message = msg.Stderr
			}
		}
		if m.closed {
			return m, nil
		}
		return m, waitForEvent(m.eventChan)

	case eventClosedMsg:
		m.closed = true
		m.quitting = true
		return m, tea.Quit

	case spinnerTickMsg:
		if m.quitting {
			return m, nil
		}
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		return m, tickSpinner()

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
			}
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

var (
	tuiPendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tuiActiveSpin   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	tuiSuccessMark  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	tuiFailedMark   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	tuiWarnMark     = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	tuiRepoNameBold = lipgloss.NewStyle().Bold(true)
)

func (m TUIModel) View() tea.View {
	var sb strings.Builder
	for _, task := range m.tasks {
		switch task.status {
		case "pending":
			sb.WriteString(tuiPendingStyle.Render("  "+task.name+": pending") + "\n")
		case "active":
			spinner := tuiActiveSpin.Render(spinnerFrames[m.spinnerFrame])
			name := tuiRepoNameBold.Render(task.name)
			fmt.Fprintf(&sb, "%s %s: %s...\n", spinner, name, task.action)
		case "success":
			chk := tuiSuccessMark.Render("✓")
			name := tuiRepoNameBold.Render(task.name)
			fmt.Fprintf(&sb, "%s %s: success\n", chk, name)
		case "failed":
			cross := tuiFailedMark.Render("✗")
			name := tuiRepoNameBold.Render(task.name)
			fmt.Fprintf(&sb, "%s %s: failed\n", cross, name)
		case "skipped":
			warn := tuiWarnMark.Render("⚠")
			name := tuiRepoNameBold.Render(task.name)
			if task.err != nil {
				fmt.Fprintf(&sb, "%s %s: cancelled\n", warn, name)
			} else {
				fmt.Fprintf(&sb, "%s %s: skipped\n", warn, name)
			}
		}
	}
	return tea.NewView(sb.String())
}

func RunSequentialFallback(eventChan <-chan runner.TaskEvent, logger *SafeLogger) {
	for event := range eventChan {
		repoStyle := lipgloss.NewStyle().Bold(true).Render(event.RepoName)
		switch event.Type {
		case runner.TaskStarted:
			logger.Print(fmt.Sprintf("[-] %s: started\n", repoStyle))
		case runner.TaskFinished:
			if event.Err != nil {
				cross := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("✗")
				logger.Print(fmt.Sprintf("[%s] %s: failed\n", cross, repoStyle))
			} else {
				chk := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓")
				logger.Print(fmt.Sprintf("[%s] %s: success\n", chk, repoStyle))
			}
		case runner.TaskSkipped:
			warn := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("⚠")
			if event.Err != nil {
				logger.Print(fmt.Sprintf("[%s] %s: cancelled\n", warn, repoStyle))
			} else {
				logger.Print(fmt.Sprintf("[%s] %s: skipped\n", warn, repoStyle))
			}
		}
	}
}

type ExecutionOptions struct {
	Interactive bool

	ActionLabel string

	Logger *SafeLogger
}

func OrchestrateExecution(
	ctx context.Context,
	tasks []runner.RepoTask,
	concurrency int,
	failFast bool,
	taskTimeout time.Duration,
	action runner.TaskActionFunc,
	opts ExecutionOptions,
) []runner.TaskResult {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	bufSize := len(tasks) * 2
	if bufSize < 1 {
		bufSize = 1
	}
	eventChan := make(chan runner.TaskEvent, bufSize)

	resultsChan := make(chan []runner.TaskResult, 1)
	go func() {
		results := runner.ExecuteTasks(runCtx, tasks, concurrency, failFast, taskTimeout, action, eventChan)
		close(eventChan)
		resultsChan <- results
	}()

	if opts.Interactive {
		model := NewTUIModel(tasks, opts.ActionLabel, eventChan, cancel)
		p := tea.NewProgram(model)
		_, _ = p.Run()
		DrainStdinNonblocking()
		FlushStdin()
	} else {
		RunSequentialFallback(eventChan, opts.Logger)
	}

	return <-resultsChan
}
