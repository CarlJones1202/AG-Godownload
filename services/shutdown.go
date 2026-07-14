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

// StopWorkers signals all background workers to exit. Safe to call multiple
// times — only the first call takes effect. Stops task queues first so
// workers finish their current item and then exit, then the HTTP server
// shutdown can proceed, and finally the DB is closed.
func StopWorkers() {
	stopOnce.Do(func() {
		StopTaskQueues()
		StopScanQueue()
		StopWebSocketHub()
		close(shutdown)
		logger.Info("Workers signaled to stop")
	})
}
