package services

import (
	"container/heap"
	"fmt"
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
	pq       PriorityQueue
	mu       sync.Mutex
	cond     *sync.Cond
	done     chan struct{}
	stopOnce sync.Once
}

func NewTaskQueue() *TaskQueue {
	tq := &TaskQueue{done: make(chan struct{})}
	tq.cond = sync.NewCond(&tq.mu)
	heap.Init(&tq.pq)
	return tq
}

func (tq *TaskQueue) Push(task *CrawlerTask) bool {
	tq.mu.Lock()
	select {
	case <-tq.done:
		tq.mu.Unlock()
		return false
	default:
	}
	heap.Push(&tq.pq, task)
	tq.mu.Unlock()
	tq.cond.Signal()
	return true
}

// Pop blocks until a task is available or the queue is stopped.
// Returns nil on stop without starting any remaining queued tasks.
func (tq *TaskQueue) Pop() *CrawlerTask {
	tq.mu.Lock()
	for {
		select {
		case <-tq.done:
			tq.mu.Unlock()
			return nil
		default:
		}

		if tq.pq.Len() > 0 {
			task := heap.Pop(&tq.pq).(*CrawlerTask)
			tq.mu.Unlock()
			return task
		}
		tq.cond.Wait()
	}
}

// Stop wakes blocked Pop callers and prevents queued tasks from being started.
func (tq *TaskQueue) Stop() {
	tq.stopOnce.Do(func() {
		close(tq.done)
		tq.mu.Lock()
		tq.cond.Broadcast()
		tq.mu.Unlock()
	})
}

var CrawlerTaskQueue = NewTaskQueue()

// Video queue (separate queue for known video sources)
var VideoTaskQueue = NewTaskQueue()

var crawlerWorkerWG sync.WaitGroup

func AddToCrawlerQueue(sourceID uint) {
	if err := QueueCrawlerSource(sourceID); err != nil {
		logger.Errorf("Failed to queue source %d for crawling: %v", sourceID, err)
	}
}

func QueueCrawlerSource(sourceID uint) error {
	var source models.Source
	if err := database.DB.Select("id, priority").First(&source, sourceID).Error; err != nil {
		return fmt.Errorf("find source: %w", err)
	}

	if err := database.DB.Model(&source).Update("Status", "queued").Error; err != nil {
		return fmt.Errorf("mark source queued: %w", err)
	}

	if !CrawlerTaskQueue.Push(&CrawlerTask{
		SourceID: source.ID,
		Priority: source.Priority,
	}) {
		return fmt.Errorf("crawler queue stopped; source remains queued for the next startup")
	}
	logger.Infof("Added source %d to crawler queue (priority %d)", sourceID, source.Priority)
	return nil
}

func AddToVideoQueue(sourceID uint) {
	if err := QueueVideoSource(sourceID); err != nil {
		logger.Errorf("Failed to queue source %d for video crawling: %v", sourceID, err)
	}
}

func QueueVideoSource(sourceID uint) error {
	var source models.Source
	if err := database.DB.Select("id, priority").First(&source, sourceID).Error; err != nil {
		return fmt.Errorf("find source: %w", err)
	}

	if err := database.DB.Model(&source).Update("Status", "queued").Error; err != nil {
		return fmt.Errorf("mark source queued: %w", err)
	}

	if !VideoTaskQueue.Push(&CrawlerTask{
		SourceID: source.ID,
		Priority: source.Priority,
	}) {
		return fmt.Errorf("video queue stopped; source remains queued for the next startup")
	}
	logger.Infof("Added source %d to video queue (priority %d)", sourceID, source.Priority)
	return nil
}

// RequeuePendingCrawls restores queued and interrupted sources from the database.
// It must run once before either worker pool starts.
func RequeuePendingCrawls() error {
	var sources []models.Source
	if err := database.DB.Where("status IN ?", []string{"crawling", "queued"}).Find(&sources).Error; err != nil {
		return err
	}

	logger.Infof("Found %d pending crawls, re-queueing...", len(sources))
	for _, source := range sources {
		if IsVideoURL(source.Location) || IsVideoFile(source.Location) {
			AddToVideoQueue(source.ID)
		} else {
			AddToCrawlerQueue(source.ID)
		}
	}
	return nil
}

func StartCrawlerWorker() {
	numWorkers := config.Global.CrawlerWorkers
	if numWorkers < 1 {
		numWorkers = 1
	}
	for i := 0; i < numWorkers; i++ {
		crawlerWorkerWG.Add(1)
		go func(workerID int) {
			defer crawlerWorkerWG.Done()
			logger.Debugf("Crawler worker %d started", workerID)
			for {
				task := CrawlerTaskQueue.Pop()
				if task == nil {
					logger.Debugf("Crawler worker %d shutting down", workerID)
					return
				}
				logger.Debugf("Worker %d processing source %d (priority %d)", workerID, task.SourceID, task.Priority)
				AddActiveCrawlerSource(task.SourceID)
				// Run the crawl in a recovered closure so a panic in a ripper
				// cannot kill this worker goroutine (and silently shrink the pool).
				func() {
					defer func() {
						RemoveActiveCrawlerSource(task.SourceID)
						if r := recover(); r != nil {
							logger.Errorf("Crawler worker %d panic crawling source %d: %v", workerID, task.SourceID, r)
						}
					}()
					if err := CrawlSource(task.SourceID); err != nil {
						logger.Errorf("Worker %d error crawling source %d: %v", workerID, task.SourceID, err)
					}
				}()
			}
		}(i)
	}
}

func StartVideoWorker() {
	numWorkers := config.Global.CrawlerWorkers
	if numWorkers < 1 {
		numWorkers = 1
	}
	for i := 0; i < numWorkers; i++ {
		crawlerWorkerWG.Add(1)
		go func(workerID int) {
			defer crawlerWorkerWG.Done()
			logger.Debugf("Video worker %d started", workerID)
			for {
				task := VideoTaskQueue.Pop()
				if task == nil {
					logger.Debugf("Video worker %d shutting down", workerID)
					return
				}
				logger.Debugf("Video worker %d processing source %d (priority %d)", workerID, task.SourceID, task.Priority)
				AddActiveCrawlerSource(task.SourceID)
				// Run the crawl in a recovered closure so a panic in a ripper
				// cannot kill this worker goroutine.
				func() {
					defer func() {
						RemoveActiveCrawlerSource(task.SourceID)
						if r := recover(); r != nil {
							logger.Errorf("Video worker %d panic crawling source %d: %v", workerID, task.SourceID, r)
						}
					}()
					if err := CrawlSource(task.SourceID); err != nil {
						logger.Errorf("Video worker %d error crawling source %d: %v", workerID, task.SourceID, err)
					}
				}()
			}
		}(i)
	}
}

// StopTaskQueues stops workers from starting queued tasks. Pending sources remain
// marked queued in the database and are restored by RequeuePendingCrawls.
func StopTaskQueues() {
	CrawlerTaskQueue.Stop()
	VideoTaskQueue.Stop()
	logger.Info("Task queues stopped")
}

// WaitForCrawlerWorkers waits until each active source crawl has completed.
func WaitForCrawlerWorkers() {
	crawlerWorkerWG.Wait()
}

/*
// AI tagging queue and workers
var AITagQueue = make(chan uint, 100)

func AddToAITagQueue(imageID uint) {
    // If AI tagging is disabled in config, do not enqueue images.
    if config.Global.AITagWorkers < 1 {
        logger.Debugf("AI tagging disabled; not queueing image %d", imageID)
        return
    }

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
        logger.Info("AI tagging disabled via configuration (AITagWorkers < 1). StartAITagWorker exiting.")
        return
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
*/
