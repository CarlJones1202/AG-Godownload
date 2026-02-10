package services

import (
	"container/heap"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"sync"
)

type CrawlerTask struct {
	SourceID uint
	Priority int
}

type PriorityQueue []*CrawlerTask

func (pq PriorityQueue) Len() int           { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool { return pq[i].Priority > pq[j].Priority }
func (pq PriorityQueue) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i] }

func (pq *PriorityQueue) Push(x interface{}) {
	item := x.(*CrawlerTask)
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

type TaskQueue struct {
	pq   PriorityQueue
	mu   sync.Mutex
	cond *sync.Cond
}

func NewTaskQueue() *TaskQueue {
	tq := &TaskQueue{}
	tq.cond = sync.NewCond(&tq.mu)
	heap.Init(&tq.pq)
	return tq
}

func (tq *TaskQueue) Push(task *CrawlerTask) {
	tq.mu.Lock()
	heap.Push(&tq.pq, task)
	tq.mu.Unlock()
	tq.cond.Signal()
}

func (tq *TaskQueue) Pop() *CrawlerTask {
	tq.mu.Lock()
	for tq.pq.Len() == 0 {
		tq.cond.Wait()
	}
	task := heap.Pop(&tq.pq).(*CrawlerTask)
	tq.mu.Unlock()
	return task
}

var CrawlerTaskQueue = NewTaskQueue()

func AddToCrawlerQueue(sourceID uint) {
	var source models.Source
	if err := database.DB.Select("id, priority").First(&source, sourceID).Error; err != nil {
		logger.Errorf("Failed to find source %d for queueing: %v", sourceID, err)
		return
	}

	// Update status to queued
	database.DB.Model(&source).Update("Status", "queued")

	CrawlerTaskQueue.Push(&CrawlerTask{
		SourceID: source.ID,
		Priority: source.Priority,
	})
	logger.Infof("Added source %d to crawler queue (priority %d)", sourceID, source.Priority)
}

func StartCrawlerWorker() {
	go func() {
		logger.Debug("Checking for interrupted or queued crawls...")
		var sources []models.Source
		// Find sources that are 'crawling' (interrupted) or 'queued' on startup
		if err := database.DB.Where("status IN ?", []string{"crawling", "queued"}).Find(&sources).Error; err == nil {
			logger.Infof("Found %d pending crawls, re-queueing...", len(sources))
			for _, s := range sources {
				AddToCrawlerQueue(s.ID)
			}
		}
	}()

	numWorkers := config.Global.CrawlerWorkers
	if numWorkers < 1 {
		numWorkers = 1
	}
	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			logger.Debugf("Crawler worker %d started", workerID)
			for {
				task := CrawlerTaskQueue.Pop()
				logger.Debugf("Worker %d processing source %d (priority %d)", workerID, task.SourceID, task.Priority)
				AddActiveCrawlerSource(task.SourceID)
				if err := CrawlSource(task.SourceID); err != nil {
					logger.Errorf("Worker %d error crawling source %d: %v", workerID, task.SourceID, err)
				}
				RemoveActiveCrawlerSource(task.SourceID)
			}
		}(i)
	}
}

var AITagQueue = make(chan uint, 100)

func AddToAITagQueue(imageID uint) {
	select {
	case AITagQueue <- imageID:
		logger.Debugf("Added image %d to AI tag queue", imageID)
	default:
		logger.Warn("AI tag queue full, skipping image", imageID)
	}
}

func StartAITagWorker() {
	numWorkers := config.Global.AITagWorkers
	if numWorkers < 1 {
		numWorkers = 1
	}

	for i := 0; i < numWorkers; i++ {
		go func() {
			logger.Debug("AI Tag worker started")
			for imageID := range AITagQueue {
				logger.Debugf("AI Tag worker processing image %d", imageID)
				if err := LabelImage(imageID); err != nil {
					logger.Errorf("Error AI tagging image %d: %v", imageID, err)
				}
			}
		}()
	}

	// Startup: scan for untagged images
	go func() {
		if err := ScanUntaggedImages(); err != nil {
			logger.Errorf("Failed to scan untagged images: %v", err)
		}
	}()
}
