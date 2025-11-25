package services

import (
	"os"
)

// DeleteFile removes a file from the filesystem
func DeleteFile(path string) error {
	return os.Remove(path)
}
