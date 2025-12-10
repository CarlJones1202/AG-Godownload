package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func CrawlSource(sourceID uint) error {
	var source models.Source
	if err := database.DB.First(&source, sourceID).Error; err != nil {
		return err
	}

	// Update status to crawling
	database.DB.Model(&source).Updates(models.Source{Status: "crawling", LastCheckedAt: time.Now()})

	// Find or create gallery for this source
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
		if err := ProcessVideoSource(source, gallery); err != nil {
			database.DB.Model(&source).Update("Status", "error")
			return err
		}
		database.DB.Model(&source).Update("Status", "idle")
		return nil
	}

	// Fetch the URL
	resp, err := http.Get(source.Location)
	if err != nil {
		database.DB.Model(&source).Update("Status", "error")
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		database.DB.Model(&source).Update("Status", "error")
		return fmt.Errorf("failed to fetch source: status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		database.DB.Model(&source).Update("Status", "error")
		return err
	}

	// Logic from reference: find div[id^='post_message_'] and extract images using rippers
	// Use a semaphore to limit concurrent downloads
	const maxConcurrent = 7 // Start with 7 concurrent downloads
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var imagesMutex sync.Mutex
	var imagesToInsert []models.Image

	// Pre-load existing image URLs for this gallery to avoid duplicate checks in loop
	existingURLs := make(map[string]bool)
	var existingImages []models.Image
	database.DB.Model(&models.Image{}).Where("gallery_id = ?", gallery.ID).Select("original_url").Find(&existingImages)
	for _, img := range existingImages {
		existingURLs[img.OriginalURL] = true
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
	} else {
		logger.Debug("Crawling first post only")
		selection = doc.Find("div[id^='post_message_']").First()
	}

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

				// Acquire semaphore
				sem <- struct{}{}
				defer func() { <-sem }()

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
					// Use the anchor href instead of img src
					// The 'src' variable in this goroutine already holds the anchor href.
					// The 'imgSrc' variable holds the img src.
					// The instruction is to use the anchor href for imx.to.
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
					// Use anchor href (src)
					imageURL, err = RipImagetwist(src)
				case strings.Contains(src, "acidimg"):
					logger.Debug("Ripping from AcidImg")
					imageURL, err = RipAcidImg(imgSrc)
				case strings.Contains(src, "postimages.org"):
					logger.Debug("Ripping from PostImages")
					imageURL, err = RipPostImages(src)
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
						// Exponential backoff
						time.Sleep(time.Duration(attempt*2) * time.Second)
					}
				}

				if err != nil {
					logger.Warnf("Failed to download %s after %d attempts: %v", imageURL, maxRetries, err)
					return
				}

				// Generate thumbnail
				_, err = GenerateThumbnail(result.Path)
				if err != nil {
					logger.Warnf("Failed to generate thumbnail: %v", err)
				}

				// Save to slice for batch insert
				// Calculate relative path for DB (e.g. "Source/file.jpg")
				relPath, err := filepath.Rel(UploadsDir, result.Path)
				if err != nil {
					// Fallback if Rel fails
					relPath = filepath.Base(result.Path)
				}

				image := models.Image{
					GalleryID:      gallery.ID,
					Filename:       relPath,
					OriginalURL:    src,      // The hosting page URL
					DownloadURL:    imageURL, // The final direct image URL
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

		if err := database.DB.Create(&imagesWithoutAssoc).Error; err != nil {
			logger.Errorf("Failed to batch insert images: %v", err)
		} else {
			logger.Infof("Successfully inserted %d images", len(imagesWithoutAssoc))

			// Now update the M2M associations
			for i := range imagesWithoutAssoc {
				if len(imagesToInsert[i].Galleries) > 0 {
					if err := database.DB.Model(&imagesWithoutAssoc[i]).Association("Galleries").Append(imagesToInsert[i].Galleries); err != nil {
						logger.Warnf("Failed to associate image %d with gallery: %v", imagesWithoutAssoc[i].ID, err)
					}
				}
			}
		}
	}

	database.DB.Model(&source).Update("Status", "idle")
	return nil
}

// ProcessVideoSource handles video URL sources
func ProcessVideoSource(source models.Source, gallery models.Gallery) error {
	logger.Infof("Processing video source: %s", source.Location)

	// Extract video URL based on the hosting site
	var videoURL string
	var err error

	if strings.Contains(source.Location, "tnaflix.com") {
		videoURL, err = RipTnaFlix(source.Location)
		if err != nil {
			return fmt.Errorf("failed to extract TnaFlix video: %w", err)
		}
	} else {
		// For other video sites, we could add more rippers here
		return fmt.Errorf("unsupported video site: %s", source.Location)
	}

	logger.Infof("Extracted video URL: %s", videoURL)

	// Check if video already exists in database
	var existingImage models.Image
	if err := database.DB.Where("download_url = ?", videoURL).First(&existingImage).Error; err == nil {
		logger.Infof("Video already exists in database: %s", videoURL)

		// Associate with this gallery if not already associated
		var count int64
		database.DB.Model(&models.Image{}).
			Joins("JOIN image_galleries ON image_galleries.image_id = images.id").
			Where("image_galleries.gallery_id = ? AND images.id = ?", gallery.ID, existingImage.ID).
			Count(&count)

		if count == 0 {
			if err := database.DB.Model(&existingImage).Association("Galleries").Append(&gallery); err != nil {
				logger.Warnf("Failed to associate existing video with gallery: %v", err)
			} else {
				logger.Infof("Associated existing video with gallery")
			}
		}
		return nil
	}

	// Download the video
	result, err := DownloadVideo(videoURL, source.Name, source.Location)
	if err != nil {
		return fmt.Errorf("failed to download video: %w", err)
	}

	logger.Infof("Successfully downloaded video to: %s", result.Path)

	// Calculate relative path for DB
	relPath, err := filepath.Rel(UploadsDir, result.Path)
	if err != nil {
		relPath = filepath.Base(result.Path)
	}

	// Create image record with Type="video"
	image := models.Image{
		GalleryID:      gallery.ID,
		SourceID:       &source.ID,
		Filename:       relPath,
		OriginalURL:    source.Location,
		DownloadURL:    videoURL,
		DominantColors: result.DominantColors,
		Type:           "video",
		Galleries:      []*models.Gallery{&gallery},
	}

	if err := database.DB.Create(&image).Error; err != nil {
		return fmt.Errorf("failed to create video record: %w", err)
	}

	logger.Infof("Created video record in database (ID: %d)", image.ID)
	return nil
}
