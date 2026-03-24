package services

import (
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
	"io"
)

func CrawlSource(sourceID uint) error {
	var source models.Source
	if err := database.DB.First(&source, sourceID).Error; err != nil {
		return err
	}

	// Update status to crawling
	database.DB.Model(&source).Updates(models.Source{Status: "crawling", LastCheckedAt: time.Now()})

	// Check if source is a local file
	if IsVideoFile(source.Location) || IsLocalPath(source.Location) {
		logger.Infof("Processing local source: %s", source.Location)
		if err := ProcessLocalSource(source); err != nil {
			database.DB.Model(&source).Update("Status", "error")
			return err
		}
		return nil
	}

	// Check if source is a video URL
	if IsVideoURL(source.Location) {
		logger.Infof("Processing video URL: %s", source.Location)
		if err := ProcessVideoSource(source); err != nil {
			database.DB.Model(&source).Update("Status", "error")
			return err
		}
		database.DB.Model(&source).Update("Status", "idle")
		return nil
	}

	// For image gallery sources, find or create gallery
	var gallery models.Gallery
	if err := database.DB.Where("source_id = ?", source.ID).First(&gallery).Error; err != nil {
		// Create if not exists
		gallery = models.Gallery{
			Name:     source.Name,
			SourceID: &source.ID,
		}
		if err := database.DB.Create(&gallery).Error; err != nil {
			database.DB.Model(&source).Update("Status", "error")
			return err
		}
	}

	// Fetch the URL using WireGuard if needed
	client := GetHTTPClient(source.Location)
	// Build a request so we can set headers (some sites block default Go UA)
	req, err := http.NewRequest("GET", source.Location, nil)
	if err != nil {
		database.DB.Model(&source).Update("Status", "error")
		return err
	}
	// Set a common browser User-Agent and accept headers to avoid simple bot blocks
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/117.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", source.Location)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("DNT", "1")
	resp, err := client.Do(req)
	if err != nil {
		database.DB.Model(&source).Update("Status", "error")
		return err
	}
	defer resp.Body.Close()

	logger.Debugf("Fetched URL: %s -> status=%d, content-length=%d, final-url=%s", source.Location, resp.StatusCode, resp.ContentLength, resp.Request.URL.String())
	if resp.StatusCode != 200 {
		// Read a small snippet of the body for debugging
		snippet := ""
		limited := io.LimitReader(resp.Body, 1024)
		if b, rerr := io.ReadAll(limited); rerr == nil {
			snippet = string(b)
		}
		database.DB.Model(&source).Update("Status", "error")
		return fmt.Errorf("failed to fetch source: status %d; body snippet: %s", resp.StatusCode, snippet)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		database.DB.Model(&source).Update("Status", "error")
		return err
	}

	// Debug: log how many message posts and how many selector matches we get
	postsFound := doc.Find("article.message--post").Length()
	logger.Debugf("Document contains %d article.message--post elements", postsFound)

	// Logic from reference: find div[id^='post_message_'] and extract images using rippers
	// Use a semaphore to limit concurrent downloads
	sem := make(chan struct{}, config.Global.MaxConcurrentDownloads)
	var wg sync.WaitGroup
	var imagesMutex sync.Mutex
	var imagesToInsert []models.Image

	// Pre-load existing image URLs and filenames for this gallery to avoid duplicate checks in loop
	existingURLs := make(map[string]bool)
	existingFilenames := make(map[string]bool)
	var existingImages []models.Image
	database.DB.Model(&models.Image{}).Where("gallery_id = ?", gallery.ID).Select("original_url, filename").Find(&existingImages)
	for _, img := range existingImages {
		existingURLs[img.OriginalURL] = true
		if img.Filename != "" {
			existingFilenames[img.Filename] = true
		}
	}

	// Parse the URL to check for a fragment
	u, err := url.Parse(source.Location)
	if err != nil {
		database.DB.Model(&source).Update("Status", "error")
		return err
	}
	fragment := u.Fragment

	var selection *goquery.Selection
	if strings.HasPrefix(fragment, "post") {
		postID := strings.TrimPrefix(fragment, "post")
		logger.Debugf("Crawling specific post ID: %s", postID)
		selection = doc.Find(fmt.Sprintf("div[id='post_message_%s']", postID))
	} else if strings.Contains(source.Location, "jkforum.net") {
		logger.Debug("Crawling jkforum.net")
		// For newer JKF (Nuxt), we might need to look for article or content divs
		selection = doc.Find("article, div.article-content, div.post-content, div[id^='post_message_']")
		if selection.Length() == 0 {
			logger.Debug("No specific post container found, falling back to body")
			selection = doc.Find("body")
		}
	} else if strings.Contains(source.Location, "kitty-kats.net") {
		logger.Debug("Crawling kitty-kats.net")
		// XenForo forum structure: posts are in article.message--post
		// target the message body wrapper where BBCode content and image links live
		selection = doc.Find("article.message--post .message-userContent .bbWrapper")
		if selection.Length() == 0 {
			logger.Debug("No kitty-kats specific container found, falling back to first post")
			selection = doc.Find("div[id^='post_message_']").First()
		}
	} else {
		logger.Debug("Crawling first post only")
		selection = doc.Find("div[id^='post_message_']").First()
	}

	// Pre-count total items to download for progress tracking
	totalImageLinks := 0
	selection.Each(func(i int, s *goquery.Selection) {
		s.Find("a img").Each(func(j int, img *goquery.Selection) {
			if img.AttrOr("alt", "") == "View Post" {
				return
			}
			totalImageLinks++
		})
	})

	// Initialize progress tracking
	database.DB.Model(&source).Updates(map[string]interface{}{
		"total_items":       totalImageLinks,
		"downloaded_items":  0,
		"download_progress": 0,
	})
	UpdateCrawlerProgress(source.ID, 0, 0, totalImageLinks)
	logger.Infof("Found %d images to download for source %s", totalImageLinks, source.Name)

	// Use atomic counters for progress tracking
	var downloadedCount int32
	var processedCount int32

	// Batch progress updates to reduce DB contention
	type progressUpdate struct {
		sourceID   uint
		progress   int
		downloaded int
		totalItems int
		isComplete bool
	}

	progressChan := make(chan progressUpdate, 100)
	var wgProgress sync.WaitGroup
	wgProgress.Add(1)

	// Background goroutine to batch and flush progress updates
	go func() {
		defer wgProgress.Done()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		latestProgress := make(map[uint]progressUpdate)
		flush := func() {
			if len(latestProgress) == 0 {
				return
			}
			for _, update := range latestProgress {
				database.DB.Exec("UPDATE sources SET download_progress = ?, downloaded_items = ? WHERE id = ?",
					update.progress, update.downloaded, update.sourceID)
			}
			latestProgress = make(map[uint]progressUpdate)
		}

		for {
			select {
			case update := <-progressChan:
				latestProgress[update.sourceID] = update
				if update.isComplete {
					flush()
					return
				}
			case <-ticker.C:
				flush()
			}
		}
	}()

	selection.Each(func(i int, s *goquery.Selection) {
		// Find images inside this div - look for <a> tags containing <img>
		s.Find("a img").Each(func(j int, img *goquery.Selection) {
			// Skip "View Post" images
			if img.AttrOr("alt", "") == "View Post" {
				logger.Debugf("Skipping element %d: alt='View Post'", j)
				return
			}

			// Get the parent <a> tag's href
			a := img.Parent()
			src, exists := a.Attr("href")
			if !exists {
				logger.Debugf("Element %d: No href found", j)
				return
			}
			logger.Debugf("Element %d: Found link %s", j, src)

			// Launch download in goroutine with semaphore
			wg.Add(1)
			go func(src string, imgSrc string) {
				defer wg.Done()
				defer func() {
					// Update processing progress
					newProcessed := atomic.AddInt32(&processedCount, 1)
					if totalImageLinks > 0 {
						progress := int((float64(newProcessed) / float64(totalImageLinks)) * 100)
						downloaded := atomic.LoadInt32(&downloadedCount)
						UpdateCrawlerProgress(source.ID, progress, int(downloaded), totalImageLinks)

						// Send progress update to batched background handler
						isComplete := newProcessed == int32(totalImageLinks)
						select {
						case progressChan <- progressUpdate{
							sourceID:   source.ID,
							progress:   progress,
							downloaded: int(downloaded),
							isComplete: isComplete,
						}:
						default:
							// Channel full, skip update
						}
					}
				}()

				// Acquire semaphore
				sem <- struct{}{}
				defer func() { <-sem }()

				// Deduplicate by original URL if we've seen this image before in this gallery
				if existingURLs[src] {
					logger.Debugf("Duplicate image detected (original URL exists): %s", src)
					return
				}

				// ... (existing ripping logic remains the same) ...
				// Use rippers to extract actual image URL based on hosting site
				var imageURL string
				var err error
				switch {
				case strings.Contains(src, "imagebam"):
					logger.Debug("Ripping from ImageBam")
					imageURL, err = RipImageBam(src)
				case strings.Contains(src, "imgbox"):
					logger.Debug("Ripping from ImgBox")
					imageURL, err = RipImageBox(src)
				case strings.Contains(src, "imx.to"):
					logger.Debug("Ripping from Imx.to")
					imageURL, err = RipImx(src)
				case strings.Contains(src, "turboimagehost"):
					logger.Debug("Ripping from TurboImageHost")
					imageURL, err = RipTurboImg(src)
				case strings.Contains(src, "vipr.im"):
					logger.Debug("Ripping from Vipr.im")
					imageURL, err = RipViprIm(imgSrc)
				case strings.Contains(src, "pixhost"):
					logger.Debug("Ripping from PixHost")
					imageURL, err = RipPixHost(imgSrc)
				case strings.Contains(src, "postimages.org"):
					logger.Debug("Ripping from PostImages")
					imageURL, err = RipPostImages(src)
				case strings.Contains(src, "imagetwist"):
					logger.Debug("Ripping from Imagetwist")
					imageURL, err = RipImagetwist(src)
				case strings.Contains(src, "acidimg"):
					logger.Debug("Ripping from AcidImg")
					imageURL, err = RipAcidImg(imgSrc)
				case strings.Contains(src, "mymypic.net") || strings.Contains(imgSrc, "mymypic.net") ||
					strings.Contains(src, "mymyatt.net") || strings.Contains(imgSrc, "mymyatt.net"):
					logger.Debug("Ripping from MyMyPic/MyMyAtt")
					imageURL, err = RipMyMyPic(src)
					if imageURL == "" {
						imageURL, err = RipMyMyPic(imgSrc)
					}
				case strings.Contains(src, "pixxxels.cc") || strings.Contains(src, "freeimage.us"):
					logger.Debugf("Skipping unsupported host: %s", src)
					return
				default:
					// If it's a direct image link, use it
					lowerSrc := strings.ToLower(src)
					if strings.HasSuffix(lowerSrc, ".jpg") || strings.HasSuffix(lowerSrc, ".png") || strings.HasSuffix(lowerSrc, ".jpeg") || strings.HasSuffix(lowerSrc, ".gif") {
						imageURL = src
					} else {
						logger.Debugf("Unknown image source %s", src)
						return
					}
				}

				if err != nil {
					logger.Warnf("Error ripping %s: %v", src, err)
					return
				}

				if imageURL == "" {
					logger.Debugf("No image URL extracted from %s", src)
					return
				}

				// Basic deduplication check (by URL)
				if existingURLs[imageURL] {
					logger.Debugf("Image already exists: %s", imageURL)
					return
				}

				// Download the actual image with retry logic
				var result *DownloadImageResult
				maxRetries := 3
				for attempt := 1; attempt <= maxRetries; attempt++ {
					result, err = DownloadImage(imageURL, source.Name)
					if err == nil {
						break
					}
					logger.Debugf("Download attempt %d failed for %s: %v", attempt, imageURL, err)
					if attempt < maxRetries {
						time.Sleep(time.Duration(attempt*2) * time.Second)
					}
				}

				if err != nil {
					logger.Warnf("Failed to download %s after %d attempts: %v", imageURL, maxRetries, err)
					return
				}

				// Successfully downloaded
				atomic.AddInt32(&downloadedCount, 1)

				// Generate thumbnail
				_, err = GenerateThumbnail(result.Path)
				if err != nil {
					logger.Warnf("Failed to generate thumbnail: %v", err)
				}

				// Save to slice for batch insert
				relPath, err := filepath.Rel(UploadsDir, result.Path)
				if err != nil {
					relPath = filepath.Base(result.Path)
				}

				// Deduplicate by filename if this image already exists in the gallery
				if existingFilenames[relPath] {
					logger.Debugf("Duplicate image detected (filename exists): %s", relPath)
					return
				}

				image := models.Image{
					GalleryID:      gallery.ID,
					Filename:       relPath,
					OriginalURL:    src,
					DownloadURL:    imageURL,
					DominantColors: result.DominantColors,
					Galleries:      []*models.Gallery{&gallery},
				}
				imagesMutex.Lock()
				imagesToInsert = append(imagesToInsert, image)
				imagesMutex.Unlock()
				logger.Debugf("Successfully downloaded and saved image: %s", imageURL)
			}(src, img.AttrOr("src", ""))
		})
	})

	// Wait for all downloads to complete
	wg.Wait()

	// Wait for progress updater to finish
	wgProgress.Wait()

	// Batch insert all images
	if len(imagesToInsert) > 0 {
		logger.Infof("Batch inserting %d images...", len(imagesToInsert))

		// First, create the images without associations
		// We need to clear the Galleries field temporarily for batch insert
		imagesWithoutAssoc := make([]models.Image, len(imagesToInsert))
		for i := range imagesToInsert {
			imagesWithoutAssoc[i] = imagesToInsert[i]
			imagesWithoutAssoc[i].Galleries = nil
		}

		// Use CreateInBatches for safer large inserts
		if err := database.DB.CreateInBatches(&imagesWithoutAssoc, 100).Error; err != nil {
			logger.Errorf("Failed to batch insert images: %v", err)
		} else {
			logger.Infof("Successfully inserted %d images", len(imagesWithoutAssoc))

			// Now update the M2M associations using raw SQL for performance to avoid N+1 updates
			var valueStrings []string
			var valueArgs []interface{}

			for i := range imagesWithoutAssoc {
				for _, g := range imagesToInsert[i].Galleries {
					valueStrings = append(valueStrings, "(?, ?)")
					valueArgs = append(valueArgs, imagesWithoutAssoc[i].ID, g.ID)
				}
			}

			// Chunked insert to avoid SQLite variable limit
			// SQLite default limit is 999 variables pre-3.32.0, significantly higher in newer versions.
			// Each row has 2 params. 400 rows = 800 params, safe for old SQLite.
			const chunkSize = 400
			totalAssocs := len(valueStrings)
			if totalAssocs > 0 {
				for i := 0; i < totalAssocs; i += chunkSize {
					end := i + chunkSize
					if end > totalAssocs {
						end = totalAssocs
					}

					chunkStrings := valueStrings[i:end]
					// Each entry corresponds to 2 arguments (image_id, gallery_id)
					chunkArgs := valueArgs[i*2 : end*2]

					stmt := fmt.Sprintf("INSERT INTO image_galleries (image_id, gallery_id) VALUES %s",
						strings.Join(chunkStrings, ","))
					stmt += " ON CONFLICT DO NOTHING"

					if err := database.DB.Exec(stmt, chunkArgs...).Error; err != nil {
						logger.Warnf("Failed to batch associate images chunk: %v", err)
					}
				}
				logger.Infof("Successfully associated %d images with galleries", len(imagesWithoutAssoc))
			}
		}
	}

	database.DB.Model(&source).Updates(map[string]interface{}{
		"Status":            "idle",
		"download_progress": 0,
		"downloaded_items":  0,
		"total_items":       0,
	})

	// Auto-link gallery to people if it matches missing galleries from scans
	if gallery.SourceURL != "" {
		if linkedIDs, err := CheckAndLinkFoundGallery(gallery.SourceURL, gallery.Name, gallery.Provider); err != nil {
			logger.Warnf("Failed to check for missing gallery matches: %v", err)
		} else if len(linkedIDs) > 0 {
			logger.Infof("Gallery %s auto-linked to %d people", gallery.Name, len(linkedIDs))
		}
	}

	return nil
}

// ProcessVideoSource handles video URL sources
func ProcessVideoSource(source models.Source) error {
	logger.Infof("Processing video source: %s", source.Location)

	// Extract video URL based on the hosting site
	var videoURL, videoTitle string
	var err error
	var isLocalFile bool
	var localPath string

	if strings.Contains(source.Location, "tnaflix.com") {
		videoURL, videoTitle, err = RipTnaFlix(source.Location)
		if err != nil {
			return fmt.Errorf("failed to extract TnaFlix video: %w", err)
		}
	} else if strings.Contains(source.Location, "pornhub.com") {
		logger.Infof("Detected Pornhub URL, using yt-dlp...")
		localPath, videoTitle, err = RipYouTube(source.Location)
		if err != nil {
			return fmt.Errorf("failed to download Pornhub video: %w", err)
		}
		isLocalFile = true
	} else if strings.Contains(source.Location, "pmvhaven.com") {
		logger.Infof("Detected PMVHaven URL, invoking RipPMVHaven...")
		videoURL, videoTitle, err = RipPMVHaven(source.Location)
		if err != nil {
			return fmt.Errorf("failed to extract PMVHaven video: %w", err)
		}
	} else if strings.Contains(source.Location, "youtube.com") || strings.Contains(source.Location, "youtu.be") {
		logger.Infof("Detected YouTube URL, invoking RipYouTube...")
		localPath, videoTitle, err = RipYouTube(source.Location)
		if err != nil {
			return fmt.Errorf("failed to download YouTube video: %w", err)
		}
		isLocalFile = true
	} else {
		// For other video sites, we could add more rippers here
		return fmt.Errorf("unsupported video site: %s", source.Location)
	}

	logger.Infof("Extracted video title: %s", videoTitle)

	// Delete existing videos from this source before re-downloading
	var existingVideos []models.Image
	if err := database.DB.Where("source_id = ? AND type = ?", source.ID, "video").Find(&existingVideos).Error; err == nil {
		for _, existingVideo := range existingVideos {
			// Delete the file from disk
			fullPath := filepath.Join(UploadsDir, existingVideo.Filename)
			if err := os.Remove(fullPath); err != nil {
				logger.Warnf("Failed to delete old video file %s: %v", fullPath, err)
			}
			// Delete from database
			if err := database.DB.Delete(&existingVideo).Error; err != nil {
				logger.Warnf("Failed to delete old video record %d: %v", existingVideo.ID, err)
			} else {
				logger.Infof("Deleted old video: %s (ID: %d)", existingVideo.Title, existingVideo.ID)
			}
		}
	}

	var result *DownloadImageResult

	if isLocalFile {
		// For YouTube (and future local downloads), use ImportLocalVideo
		result, err = ImportLocalVideo(localPath, source.Name)
		if err != nil {
			return fmt.Errorf("failed to import video: %w", err)
		}
		videoURL = source.Location // Use original URL as download URL
	} else {
		// Download the video from URL
		result, err = DownloadVideo(videoURL, source.Name, source.Location, videoTitle)
		if err != nil {
			return fmt.Errorf("failed to download video: %w", err)
		}
	}

	logger.Infof("Successfully downloaded video to: %s", result.Path)

	// Calculate relative path for DB
	relPath, err := filepath.Rel(UploadsDir, result.Path)
	if err != nil {
		relPath = filepath.Base(result.Path)
	}

	// Create image record with Type="video" (no gallery association)
	image := models.Image{
		SourceID:       &source.ID,
		Filename:       relPath,
		OriginalURL:    source.Location,
		DownloadURL:    videoURL,
		Title:          result.Title,
		Duration:       result.Duration,
		Width:          result.Width,
		Height:         result.Height,
		SizeMB:         result.SizeMB,
		DominantColors: result.DominantColors,
		Type:           "video",
	}

	if err := database.DB.Create(&image).Error; err != nil {
		return fmt.Errorf("failed to create video record: %w", err)
	}

	logger.Infof("Created video record in database (ID: %d)", image.ID)
	return nil
}
