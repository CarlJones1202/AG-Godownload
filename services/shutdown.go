package services

import (
	"gallery_api/logger"
	"sync"
)

var (
	// shutdown is closed to signal all workers to stop. It is never sent to,
	// only closed exactly once via StopWorkers.
	shutdown = make(chan struct{})
	stopOnce sync.Once
)

// ShutdownChan returns the channel that workers should select on to detect
// a shutdown request. It is closed (never sent to) when StopWorkers runs.
func ShutdownChan() <-chan struct{} {
	return shutdown
}

// StopWorkers signals background workers to exit and waits for active source
// crawls to finish before the database is closed. Safe to call multiple times.
func StopWorkers() {
	stopOnce.Do(func() {
		StopTaskQueues()
		StopScanQueue()
		StopWebSocketHub()
		close(shutdown)
		WaitForCrawlerWorkers()
		StopEmbeddingWorkers()
		logger.Info("Workers signaled to stop")
	})
}
