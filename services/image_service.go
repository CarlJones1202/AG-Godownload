package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gallery_api/logger"
	"gallery_api/models"
	"io"
	"math"
	"net/http"
	urlpkg "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/disintegration/imaging"
	"gorm.io/gorm"
)

const (
	UploadsDir = "uploads"
)

func EnsureUploadsDir() error {
	if _, err := os.Stat(UploadsDir); os.IsNotExist(err) {
		return os.MkdirAll(UploadsDir, 0755)
	}
	return nil
}

// SanitizeDirectoryName removes invalid characters from directory names
func SanitizeDirectoryName(name string) string {
	// Replace invalid characters with underscores
	reg := regexp.MustCompile(`[<>:"/\\|?*]`)
	sanitized := reg.ReplaceAllString(name, "_")
	// Remove leading/trailing spaces and dots
	sanitized = strings.Trim(sanitized, " .")
	// Limit length
	if len(sanitized) > 100 {
		sanitized = sanitized[:100]
	}
	return sanitized
}

// DownloadImageResult contains the result of downloading an image
type DownloadImageResult struct {
	Path           string
	Title          string // extracted title for videos
	DominantColors string // JSON array of hex colors
	Duration       float64
	Width          int
	Height         int
	SizeMB         float64
}

// DownloadImage downloads an image and saves it with a content-based hash filename
// in a subdirectory named after the source. Returns the path and extracted colors.
func DownloadImage(url string, sourceName string) (*DownloadImageResult, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	// Add browser headers to avoid bot detection
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")

	// Use the shared client with retry logic
	resp, err := DoRequestWithRetry(context.Background(), req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download image: status code %d", resp.StatusCode)
	}

	// Read the entire response body into memory
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Calculate SHA-256 hash of the content
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// Determine extension from content type
	ext := ".jpg" // default
	contentType := resp.Header.Get("Content-Type")
	switch contentType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}

	// Create filename from hash
	filename := hashStr + ext

	// Sanitize source name for directory
	sourceDir := SanitizeDirectoryName(sourceName)
	if sourceDir == "" {
		sourceDir = "unknown"
	}

	// Create source subdirectory
	fullDir := filepath.Join(UploadsDir, sourceDir)
	if err := os.MkdirAll(fullDir, 0755); err != nil {
		return nil, err
	}

	// Full path for the image
	destPath := filepath.Join(fullDir, filename)

	// Check if file already exists (same content hash)
	fileExists := false
	if _, err := os.Stat(destPath); err == nil {
		fileExists = true
	}

	if !fileExists {
		// Write the file
		out, err := os.Create(destPath)
		if err != nil {
			return nil, err
		}
		defer out.Close()

		_, err = out.Write(data)
		if err != nil {
			return nil, err
		}
	}

	// Validate that the downloaded content is a valid image
	if _, err := imaging.Open(destPath); err != nil {
		// Remove the invalid file
		os.Remove(destPath)

		// Check for common "image not found" error patterns
		errLower := strings.ToLower(err.Error())
		if strings.Contains(errLower, "this model does not support image input") ||
			strings.Contains(errLower, "unknown format") ||
			strings.Contains(errLower, "invalid image") ||
			strings.Contains(errLower, "image:") {
			return nil, fmt.Errorf("image not found: %s returned a placeholder or error image instead of valid content", url)
		}
		// For other decode errors, still return the error but without assuming it's a placeholder
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Extract dominant colors
	colors, err := ExtractDominantColors(destPath)
	if err != nil {
		// Don't fail the whole operation if color extraction fails
		colors = "[]"
	}

	return &DownloadImageResult{
		Path:           destPath,
		DominantColors: colors,
	}, nil
}

func GenerateThumbnail(srcPath string) (string, error) {
	src, err := imaging.Open(srcPath)
	if err != nil {
		return "", err
	}

	// Resize to width 200 using Lanczos filter.
	dst := imaging.Resize(src, 200, 0, imaging.Lanczos)

	// Get the directory structure from source path
	// srcPath is like "uploads/source_name/hash.jpg"
	// We want "uploads/source_name/thumbnails/hash.jpg"
	dir := filepath.Dir(srcPath)
	thumbnailDir := filepath.Join(dir, "thumbnails")

	if err := os.MkdirAll(thumbnailDir, 0755); err != nil {
		return "", err
	}

	// Use same filename as original, but in thumbnails subdirectory
	filename := filepath.Base(srcPath)
	thumbPath := filepath.Join(thumbnailDir, filename)

	// Save the resulting image as JPEG.
	err = imaging.Save(dst, thumbPath, imaging.JPEGQuality(80))
	if err != nil {
		return "", err
	}

	return thumbPath, nil
}

// VideoMetadata stores extracted video information
type VideoMetadata struct {
	Duration float64
	Width    int
	Height   int
	SizeMB   float64
}

// GetVideoMetadata extracts duration and dimensions using ffprobe
func GetVideoMetadata(srcPath string) (*VideoMetadata, error) {
	// Check if file exists
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, err
	}
	sizeMB := float64(info.Size()) / (1024 * 1024)

	// Use ffprobe to get metadata
	// -v error: show only errors
	// -select_streams v:0: select first video stream
	// -show_entries: specify what to output
	// -of csv=p=0: output as csv without header
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration",
		"-of", "csv=p=0",
		srcPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &VideoMetadata{SizeMB: sizeMB}, fmt.Errorf("ffprobe failed: %w", err)
	}

	// Parse output "width,height,duration" (e.g., "1920,1080,120.5")
	parts := strings.Split(strings.TrimSpace(string(output)), ",")
	if len(parts) >= 2 { // Sometimes duration might be missing or in format container
		var meta VideoMetadata
		meta.SizeMB = sizeMB

		fmt.Sscanf(parts[0], "%d", &meta.Width)
		fmt.Sscanf(parts[1], "%d", &meta.Height)

		if len(parts) >= 3 {
			fmt.Sscanf(parts[2], "%f", &meta.Duration)
		}

		// If duration is 0, try container duration
		if meta.Duration == 0 {
			cmd = exec.Command("ffprobe",
				"-v", "error",
				"-show_entries", "format=duration",
				"-of", "default=noprint_wrappers=1:nokey=1",
				srcPath)
			if out, err := cmd.Output(); err == nil {
				fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &meta.Duration)
			}
		}

		return &meta, nil
	}

	return &VideoMetadata{SizeMB: sizeMB}, fmt.Errorf("failed to parse ffprobe output: %s", string(output))
}

// GenerateVideoThumbnail generates a thumbnail for a video file using ffmpeg
func GenerateVideoThumbnail(srcPath string) (string, error) {
	// Get the directory structure from source path
	dir := filepath.Dir(srcPath)
	thumbnailDir := filepath.Join(dir, "thumbnails")

	if err := os.MkdirAll(thumbnailDir, 0755); err != nil {
		return "", err
	}

	// Use same filename as original + .jpg
	filename := filepath.Base(srcPath) + ".jpg"
	thumbPath := filepath.Join(thumbnailDir, filename)

	// Check if thumbnail already exists
	if _, err := os.Stat(thumbPath); err == nil {
		return thumbPath, nil
	}

	// Download and generate thumbnail (ffmpeg logic would go here)
	// For now, return empty to indicate no thumbnail
	return "", nil
}

// DownloadProviderThumbnail downloads a thumbnail from a provider and saves it to the gallery_thumbnails directory
func DownloadProviderThumbnail(url string) (string, error) {
	if url == "" {
		return "", nil
	}
	logger.Debugf("Downloading provider thumbnail: %s", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	// Set conservative browser headers to reduce chance of blocking
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")

	// Use the page origin as referer when possible (some providers require it)
	if u, perr := urlpkg.Parse(url); perr == nil {
		origin := u.Scheme + "://" + u.Host
		req.Header.Set("Referer", origin)
	}

	resp, err := DoRequestWithRetry(context.Background(), req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// capture small snippet for debugging
		snippet := ""
		limited := io.LimitReader(resp.Body, 1024)
		if b, rerr := io.ReadAll(limited); rerr == nil {
			snippet = string(b)
		}
		return "", fmt.Errorf("failed to download thumbnail: status %d; snippet=%s", resp.StatusCode, snippet)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	ext := ".jpg"
	contentType := resp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "image/png") {
		ext = ".png"
	} else if strings.HasPrefix(contentType, "image/webp") {
		ext = ".webp"
	}

	filename := hashStr + ext
	thumbDir := filepath.Join(UploadsDir, "gallery_thumbnails")
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return "", err
	}

	thumbPath := filepath.Join(thumbDir, filename)

	if _, err := os.Stat(thumbPath); err != nil {
		if err := os.WriteFile(thumbPath, data, 0644); err != nil {
			return "", err
		}
	}

	return thumbPath, nil
}

// DownloadPersonImage downloads an image for a person and saves it to a specific directory
func DownloadPersonImage(url string, personID uint) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	// Add browser headers to avoid bot detection
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")

	// Use shared client with retry logic
	resp, err := DoRequestWithRetry(context.Background(), req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download image: status code %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Calculate hash for filename
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// Determine extension
	ext := ".jpg"
	contentType := resp.Header.Get("Content-Type")
	switch contentType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}

	filename := hashStr + ext

	// Create person-specific directory: uploads/person_images/{id}
	personDir := filepath.Join(UploadsDir, "person_images", fmt.Sprintf("%d", personID))
	if err := os.MkdirAll(personDir, 0755); err != nil {
		return "", err
	}

	destPath := filepath.Join(personDir, filename)

	// Write file if it doesn't exist
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return "", err
		}
	}

	// Return web-accessible path
	return fmt.Sprintf("/person-images/%d/%s", personID, filename), nil
}

// GenerateTrickplayData generates a sprite sheet and VTT file for video scrubbing previews
func GenerateTrickplayData(srcPath string) error {
	// Get the directory structure from source path
	dir := filepath.Dir(srcPath)
	trickplayDir := filepath.Join(dir, "trickplay")

	if err := os.MkdirAll(trickplayDir, 0755); err != nil {
		return err
	}

	// Base filename without extension
	baseFilename := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))

	// Temporary directory for individual thumbnails
	tempDir := filepath.Join(trickplayDir, "temp_"+baseFilename)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(tempDir) // Clean up temp files

	// Step 0: Get duration to calculate interval
	meta, err := GetVideoMetadata(srcPath)
	if err != nil {
		logger.Warnf("Failed to get metadata for trickplay generation: %v. Defaulting to 5s interval.", err)
		meta = &VideoMetadata{Duration: 0}
	}

	// Target ~100 images, but at least 5 seconds apart
	interval := 5.0
	if meta.Duration > 0 {
		calculated := meta.Duration / 100.0
		interval = math.Max(5.0, calculated)
	}

	// Step 1: Extract thumbnails based on calculated interval
	thumbPattern := filepath.Join(tempDir, "thumb_%04d.jpg")
	// fps = 1/interval
	fpsFilter := fmt.Sprintf("fps=1/%.2f,scale=160:90", interval)
	extractCmd := exec.Command("ffmpeg", "-i", srcPath, "-vf", fpsFilter, thumbPattern)

	if output, err := extractCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to extract thumbnails: %w, output: %s", err, string(output))
	}

	// Step 2: Count how many thumbnails were created
	thumbFiles, err := filepath.Glob(filepath.Join(tempDir, "thumb_*.jpg"))
	if err != nil || len(thumbFiles) == 0 {
		return fmt.Errorf("no thumbnails generated")
	}

	// Step 3: Calculate grid dimensions (try to make it roughly square)
	totalThumbs := len(thumbFiles)
	cols := int(math.Ceil(math.Sqrt(float64(totalThumbs))))
	rows := int(math.Ceil(float64(totalThumbs) / float64(cols)))

	// Step 4: Create sprite sheet using tile filter
	spriteFile := filepath.Join(trickplayDir, baseFilename+"_sprite.jpg")
	tileFilter := fmt.Sprintf("tile=%dx%d", cols, rows)

	spriteCmd := exec.Command("ffmpeg", "-y", "-i", thumbPattern, "-filter_complex", tileFilter, spriteFile)
	if output, err := spriteCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create sprite: %w, output: %s", err, string(output))
	}

	// Step 5: Generate VTT file
	vttFile := filepath.Join(trickplayDir, baseFilename+".vtt")
	vttContent := "WEBVTT\n\n"

	thumbWidth := 160
	thumbHeight := 90
	// interval is already defined above

	for i := 0; i < totalThumbs; i++ {
		startTime := float64(i) * interval
		endTime := startTime + interval

		row := i / cols
		col := i % cols
		x := col * thumbWidth
		y := row * thumbHeight

		vttContent += fmt.Sprintf("%s --> %s\n", formatVTTTime(startTime), formatVTTTime(endTime))
		vttContent += fmt.Sprintf("%s_sprite.jpg#xywh=%d,%d,%d,%d\n\n", baseFilename, x, y, thumbWidth, thumbHeight)
	}

	if err := os.WriteFile(vttFile, []byte(vttContent), 0644); err != nil {
		return fmt.Errorf("failed to write VTT file: %w", err)
	}

	return nil
}

// formatVTTTime formats seconds into VTT timestamp format (HH:MM:SS.mmm)
func formatVTTTime(seconds float64) string {
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60
	millis := int((seconds - float64(int(seconds))) * 1000)
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, secs, millis)
}

// ScanMissingMetadata finds videos with missing metadata and updates them
func ScanMissingMetadata(db *gorm.DB, force bool) error {
	var videos []models.Image

	query := db.Where("type = ?", "video")

	if !force {
		// Find videos with type='video' AND (missing metadata OR dirty titles)
		// We want to fix:
		// 1. Missing Duration (0)
		// 2. Missing Width (0)
		// 3. Titles with underscores or dots (likely raw filenames)
		query = query.Where("duration = ? OR width = ? OR title LIKE ? OR title LIKE ?", 0, 0, "%_%", "%.%")
	}

	if err := query.Find(&videos).Error; err != nil {
		return err
	}

	if len(videos) == 0 {
		return nil
	}

	logger.Infof("Found %d videos with missing metadata. Starting scan...", len(videos))

	for _, video := range videos {
		// Construct full path
		// We need to resolve the path similar to how ServeImage does or use what we know about storage
		// Usually UploadsDir + SourceName + Filename
		// But image.Filename might be relative or absolute-ish depending on legacy

		var sourceName string
		if video.SourceID != nil {
			var source models.Source
			if err := db.First(&source, *video.SourceID).Error; err == nil {
				sourceName = source.Name
			}
		} else {
			// Try gallery
			var gallery models.Gallery
			if err := db.Joins("JOIN image_galleries ON image_galleries.gallery_id = galleries.id").
				Where("image_galleries.image_id = ?", video.ID).First(&gallery).Error; err == nil {
				if gallery.SourceID != nil {
					var source models.Source
					if err := db.First(&source, *gallery.SourceID).Error; err == nil {
						sourceName = source.Name
					}
				}
			}
		}

		if sourceName == "" {
			sourceName = "uncategorized"
		}

		// Sanitize
		dirName := SanitizeDirectoryName(sourceName)
		fullPath := filepath.Join(UploadsDir, dirName, filepath.Base(video.Filename))

		// Verify file exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			// Try direct if video.Filename contains path
			if _, err := os.Stat(filepath.Join(UploadsDir, video.Filename)); err == nil {
				fullPath = filepath.Join(UploadsDir, video.Filename)
			} else if _, err := os.Stat(video.Filename); err == nil {
				// Handle absolute path stored in DB
				fullPath = video.Filename
			} else {
				// Fallback: Recursive search in uploads folder
				foundPath := ""
				baseName := filepath.Base(video.Filename)
				err := filepath.Walk(UploadsDir, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if !info.IsDir() && filepath.Base(path) == baseName {
						foundPath = path
						return io.EOF // Stop search
					}
					// Also check if filename matches without processing (legacy reasons)
					if !info.IsDir() && info.Name() == video.Filename {
						foundPath = path
						return io.EOF
					}
					return nil
				})

				if err != nil && err != io.EOF {
					logger.Warnf("Recursive search error for video %d: %v", video.ID, err)
				}

				if foundPath != "" {
					fullPath = foundPath
					logger.Infof("Found moved video file for %d at: %s", video.ID, fullPath)
				} else {
					logger.Warnf("Could not find file for video %d. Checked: %s and recursive search", video.ID, fullPath)
					continue
				}
			}
		}

		// Get Metadata
		meta, err := GetVideoMetadata(fullPath)
		if err != nil {
			logger.Warnf("Failed to extract metadata for video %d: %v", video.ID, err)
			continue
		}

		// Update DB
		video.Duration = meta.Duration
		video.Width = meta.Width
		video.Height = meta.Height
		video.SizeMB = meta.SizeMB

		// Backfill title if missing or if it looks like a filename (contains underscores)
		// We want to improve titles even if they "exist" but look like "my_video_file"
		shouldUpdateTitle := video.Title == "" || strings.Contains(video.Title, "_") || strings.Contains(video.Title, ".")

		if shouldUpdateTitle {
			// Use filename without extension and clean it
			baseName := filepath.Base(video.Filename)
			ext := filepath.Ext(baseName)
			rawName := strings.TrimSuffix(baseName, ext)

			// Replace separators
			cleanName := strings.ReplaceAll(rawName, "_", " ")
			cleanName = strings.ReplaceAll(cleanName, ".", " ")
			video.Title = strings.TrimSpace(cleanName)
		}

		// If title is empty or same as filename, try to prettify it or leave it
		// For now, let's just fix the technical metadata. User complained about "Unknown" size/quality.

		if err := db.Save(&video).Error; err != nil {
			logger.Errorf("Failed to save metadata for video %d: %v", video.ID, err)
		} else {
			logger.Infof("Updated metadata for video %d: %s (%dp)", video.ID, video.Filename, video.Height)
		}
	}

	logger.Infof("Metadata scan complete.")
	return nil
}
