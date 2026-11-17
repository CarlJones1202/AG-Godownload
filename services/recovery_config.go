package services

import "gallery_api/config"

// recoveryConcurrency returns the configured download-worker pool size used by
// startup recovery. Falls back to the documented default (10, the previous
// hardcoded value) when config has not been loaded (e.g. in tests).
func recoveryConcurrency() int {
	if config.Global.RecoveryConcurrency < 1 {
		return 10
	}
	return config.Global.RecoveryConcurrency
}

// recoveryPostprocessWorkers returns the configured post-processing pool size
// (dominant colors, thumbnails, trickplay). Falls back to the documented
// default (4) when config has not been loaded (e.g. in tests).
func recoveryPostprocessWorkers() int {
	if config.Global.RecoveryPostprocessWorkers < 1 {
		return 4
	}
	return config.Global.RecoveryPostprocessWorkers
}
