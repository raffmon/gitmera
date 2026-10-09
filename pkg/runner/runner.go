
package runner

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

type RepoTask struct {
	Name string
	URI  string
	Path string

	Action string
}

type TaskResult struct {
	RepoName string
	Err      error
	Stderr   string
	Skipped  bool
}

type TaskActionFunc func(ctx context.Context, task RepoTask) (stderr string, skipped bool, err error)

type TaskEventType string

const (

	TaskStarted TaskEventType = "started"

	TaskFinished TaskEventType = "finished"

	TaskSkipped TaskEventType = "skipped"
)

type TaskEvent struct {
	RepoName string
	Type     TaskEventType
	Err      error
	Stderr   string
	Skipped  bool
}

func ExecuteTasks(
	ctx context.Context,
	tasks []RepoTask,
	concurrency int,
	failFast bool,
	taskTimeout time.Duration,
	action TaskActionFunc,
	eventChan chan<- TaskEvent,
) []TaskResult {
	results := make([]TaskResult, len(tasks))
	var resultsMu sync.Mutex

	var g errgroup.Group
	g.SetLimit(concurrency)

	groupCtx, cancelGroup := context.WithCancel(ctx)
	defer cancelGroup()

	for i, task := range tasks {
		g.Go(func() error {
			if groupCtx.Err() != nil {
				res := TaskResult{RepoName: task.Name, Err: groupCtx.Err(), Skipped: true}
				resultsMu.Lock()
				results[i] = res
				resultsMu.Unlock()
				if eventChan != nil {
					eventChan <- TaskEvent{
						RepoName: task.Name,
						Type:     TaskSkipped,
						Err:      groupCtx.Err(),
						Skipped:  true,
					}
				}
				return groupCtx.Err()
			}

			workerCtx, workerCancel := context.WithTimeout(groupCtx, taskTimeout)
			defer workerCancel()

			if eventChan != nil {
				eventChan <- TaskEvent{RepoName: task.Name, Type: TaskStarted}
			}

			stderr, skipped, err := action(workerCtx, task)

			res := TaskResult{
				RepoName: task.Name,
				Err:      err,
				Stderr:   stderr,
				Skipped:  skipped,
			}

			resultsMu.Lock()
			results[i] = res
			resultsMu.Unlock()

			if eventChan != nil {
				if skipped {
					eventChan <- TaskEvent{
						RepoName: task.Name,
						Type:     TaskSkipped,
						Err:      err,
						Stderr:   stderr,
						Skipped:  true,
					}
				} else {
					eventChan <- TaskEvent{
						RepoName: task.Name,
						Type:     TaskFinished,
						Err:      err,
						Stderr:   stderr,
					}
				}
			}

			if err != nil && failFast {
				cancelGroup()
				return err
			}

			return nil
		})
	}

	_ = g.Wait()

	resultsMu.Lock()
	for i, task := range tasks {
		if results[i].RepoName == "" {
			results[i] = TaskResult{
				RepoName: task.Name,
				Err:      groupCtx.Err(),
				Skipped:  true,
			}
			if eventChan != nil {
				eventChan <- TaskEvent{
					RepoName: task.Name,
					Type:     TaskSkipped,
					Err:      groupCtx.Err(),
					Skipped:  true,
				}
			}
		}
	}
	resultsMu.Unlock()

	return results
}
