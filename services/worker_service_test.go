package services

import (
	"testing"
	"time"
)

func TestTaskQueueStopLeavesPendingTasksForRestart(t *testing.T) {
	queue := NewTaskQueue()
	task := &CrawlerTask{SourceID: 42, Priority: 1}
	if !queue.Push(task) {
		t.Fatal("Push returned false before shutdown")
	}

	queue.Stop()
	if got := queue.Pop(); got != nil {
		t.Fatalf("Pop after shutdown = %#v, want nil", got)
	}
	if queue.Push(&CrawlerTask{SourceID: 43}) {
		t.Fatal("Push accepted a task after shutdown")
	}
}

func TestTaskQueueStopWakesBlockedPop(t *testing.T) {
	queue := NewTaskQueue()
	popped := make(chan *CrawlerTask, 1)
	go func() {
		popped <- queue.Pop()
	}()

	queue.Stop()
	select {
	case task := <-popped:
		if task != nil {
			t.Fatalf("Pop after shutdown = %#v, want nil", task)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not wake blocked Pop")
	}
}
