package services

import (
	"testing"
	"time"
)

func TestStartMissingFileRecoveryCoalescesLaunches(t *testing.T) {
	missingFileRecoveryMu.Lock()
	missingFileRecoveryRunning = false
	missingFileRecoveryMu.Unlock()

	started := make(chan struct{})
	release := make(chan struct{})
	if !startMissingFileRecovery(func() {
		close(started)
		<-release
	}) {
		t.Fatal("first recovery launch was rejected")
	}
	<-started
	if startMissingFileRecovery(func() {}) {
		t.Fatal("second recovery launch was not coalesced")
	}
	close(release)

	deadline := time.Now().Add(time.Second)
	for {
		missingFileRecoveryMu.Lock()
		running := missingFileRecoveryRunning
		missingFileRecoveryMu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recovery guard did not reset")
		}
		time.Sleep(time.Millisecond)
	}
}
