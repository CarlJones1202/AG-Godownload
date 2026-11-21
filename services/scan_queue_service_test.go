package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestScanQueueWorkersProcessJobsConcurrently(t *testing.T) {
	queue := &ScanQueue{
		queue: []uint{1, 2},
		done:  make(chan struct{}),
	}
	queue.cond = sync.NewCond(&queue.mu)

	var activeMu sync.Mutex
	active := 0
	bothStarted := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	var stopOnce sync.Once
	workerDone := make(chan struct{}, 2)
	processed := make(chan struct{}, 2)
	process := func(uint) {
		activeMu.Lock()
		active++
		if active == 2 {
			close(bothStarted)
		}
		activeMu.Unlock()

		<-release

		activeMu.Lock()
		active--
		activeMu.Unlock()
		processed <- struct{}{}
	}
	stop := func() {
		stopOnce.Do(func() {
			queue.mu.Lock()
			close(queue.done)
			queue.cond.Broadcast()
			queue.mu.Unlock()
		})
	}
	for i := 0; i < 2; i++ {
		go func() {
			queue.runWorker(process)
			workerDone <- struct{}{}
		}()
	}
	defer func() {
		releaseOnce.Do(func() { close(release) })
		stop()
		<-workerDone
		<-workerDone
	}()

	select {
	case <-bothStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("two jobs were not processed concurrently")
	}

	releaseOnce.Do(func() { close(release) })
	for i := 0; i < 2; i++ {
		select {
		case <-processed:
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not finish its job")
		}
	}
	stop()
}

func TestRestorePendingScansRequeuesUnfinishedJobs(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:restore-pending-scans-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.AutoMigrate(&models.PersonScanQueue{}); err != nil {
		t.Fatal(err)
	}

	originalDB := database.DB
	database.DB = testDB
	defer func() { database.DB = originalDB }()

	startedAt := time.Now().Add(-time.Minute)
	scans := []models.PersonScanQueue{
		{PersonID: 1, Provider: "pending", Status: models.ScanStatusPending},
		{PersonID: 2, Provider: "interrupted", Status: models.ScanStatusProcessing, StartedAt: &startedAt},
		{PersonID: 3, Provider: "completed", Status: models.ScanStatusCompleted},
	}
	if err := testDB.Create(&scans).Error; err != nil {
		t.Fatal(err)
	}

	scanQueue.mu.Lock()
	originalQueue := scanQueue.queue
	scanQueue.queue = nil
	scanQueue.mu.Unlock()
	defer func() {
		scanQueue.mu.Lock()
		scanQueue.queue = originalQueue
		scanQueue.mu.Unlock()
	}()

	if err := RestorePendingScans(); err != nil {
		t.Fatal(err)
	}

	scanQueue.mu.Lock()
	gotIDs := append([]uint(nil), scanQueue.queue...)
	scanQueue.mu.Unlock()
	if len(gotIDs) != 2 || gotIDs[0] != scans[0].ID || gotIDs[1] != scans[1].ID {
		t.Fatalf("restored scan IDs = %v, want [%d %d]", gotIDs, scans[0].ID, scans[1].ID)
	}

	var interrupted models.PersonScanQueue
	if err := testDB.First(&interrupted, scans[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	if interrupted.Status != models.ScanStatusPending || interrupted.StartedAt != nil {
		t.Fatalf("interrupted scan after restore = status %q, started_at %v; want pending and unset",
			interrupted.Status, interrupted.StartedAt)
	}
}
