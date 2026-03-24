package services

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// extractImageProvider identifies the image hosting provider from a download URL
func extractImageProvider(url string) string {
	urlLower := strings.ToLower(url)

	switch {
	case strings.Contains(urlLower, "imgur.com") || strings.Contains(urlLower, "i.imgur.com"):
		return "imgur"
	case strings.Contains(urlLower, "imx.to") || strings.Contains(urlLower, "i.imx.to"):
		return "imx"
	case strings.Contains(urlLower, "vipr.im") || strings.Contains(urlLower, "i.vipr.im"):
		return "viprimg"
	case strings.Contains(urlLower, "turboimagehost") || strings.Contains(urlLower, "turboimg"):
		return "turboimg"
	case strings.Contains(urlLower, "imagebam"):
		return "imagebam"
	case strings.Contains(urlLower, "imgbox"):
		return "imgbox"
	case strings.Contains(urlLower, "pixhost"):
		return "pixhost"
	case strings.Contains(urlLower, "postimages.org"):
		return "postimages"
	case strings.Contains(urlLower, "imagetwist"):
		return "imagetwist"
	case strings.Contains(urlLower, "acidimg"):
		return "acidimg"
	case strings.Contains(urlLower, "mymypic.net") || strings.Contains(urlLower, "mymyatt.net"):
		return "mymypic"
	case strings.HasSuffix(urlLower, ".jpg") || strings.HasSuffix(urlLower, ".jpeg") ||
		strings.HasSuffix(urlLower, ".png") || strings.HasSuffix(urlLower, ".gif") ||
		strings.HasSuffix(urlLower, ".webp"):
		return "direct"
	default:
		return "unknown"
	}
}

// VerifyDownloadedImages checks all images in the database and re-downloads any missing files
// VerifyDownloadedImages checks all images and re-downloads + fixes DB if needed
func VerifyDownloadedImages() error {
	logger.Info("Verifying downloaded images...")

	// Step 1: Identify images belonging to "favorited galleries"
	// A "favorited gallery" is one that contains at least one favorited image.
	var favGalleryIDs []uint
	if err := database.DB.Table("image_galleries").
		Joins("JOIN images ON images.id = image_galleries.image_id").
		Where("images.is_favorite = ?", true).
		Distinct("image_galleries.gallery_id").
		Pluck("image_galleries.gallery_id", &favGalleryIDs).Error; err != nil {
		logger.Errorf("Failed to identify favorite galleries: %v", err)
		// Continue without this optimization if it fails
	}

	// Map of ImageID -> bool for fast lookup of "is in favorited gallery"
	isInFavGallery := make(map[uint]bool)
	if len(favGalleryIDs) > 0 {
		var imgIDs []uint
		// Batch this in chunks if we have massive amounts of favorites, but for now
		// strict "IN" query should be okay for typical extensive usage (<50k favs).
		if err := database.DB.Table("image_galleries").
			Where("gallery_id IN ?", favGalleryIDs).
			Distinct("image_id").
			Pluck("image_id", &imgIDs).Error; err == nil {
			for _, id := range imgIDs {
				isInFavGallery[id] = true
			}
		}
	}

	var images []struct {
		ID          uint
		Filename    string
		DownloadURL string
		IsFavorite  bool
		CreatedAt   time.Time
	}

	if err := database.DB.
		Model(&models.Image{}).
		Select("id, filename, download_url, is_favorite, created_at").
		// Remove SQL order as we will sort in Go
		Find(&images).Error; err != nil {
		return fmt.Errorf("failed to query images: %w", err)
	}

	// Sort images by priority:
	// 1. IsFavorite (True)
	// 2. Belongs to a Favorite Gallery (True)
	// 3. Newest (CreatedAt Descending)
	sort.Slice(images, func(i, j int) bool {
		// 1. IsFavorite
		if images[i].IsFavorite != images[j].IsFavorite {
			return images[i].IsFavorite // True comes first
		}

		// 2. In Favorite Gallery
		inFavGalI := isInFavGallery[images[i].ID]
		inFavGalJ := isInFavGallery[images[j].ID]
		if inFavGalI != inFavGalJ {
			return inFavGalI // True comes first
		}

		// 3. Newest to Oldest
		return images[i].CreatedAt.After(images[j].CreatedAt)
	})

	go func() {
		SetVerificationRunning(true, len(images))
		defer SetVerificationRunning(false, 0)

		missingCount := 0
		var recoveredCount int32 = 0

		type recoveryTask struct {
			ID            uint
			CurrentDBPath string // what DB currently thinks the path is
			DownloadURL   string
			SourceName    string
		}

		var tasks []recoveryTask
		var mu sync.Mutex

		// Channel for batched updates
		type updateResult struct {
			ID             uint
			RelPath        string
			DominantColors string
		}
		resultChan := make(chan updateResult, 100)
		var wgBatch sync.WaitGroup

		// Start batch processor
		wgBatch.Add(1)
		go func() {
			defer wgBatch.Done()
			var buffer []updateResult
			const batchSize = 30
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			flush := func() {
				if len(buffer) == 0 {
					return
				}
				tx := database.DB.Begin()
				for _, res := range buffer {
					if err := tx.Unscoped().Model(&models.Image{}).
						Where("id = ?", res.ID).
						Updates(map[string]interface{}{
							"filename":        res.RelPath,
							"dominant_colors": res.DominantColors,
						}).Error; err != nil {
						logger.Errorf("Failed to update image %d: %v", res.ID, err)
					} else {
						atomic.AddInt32(&recoveredCount, 1)
						IncVerificationRecovered()
					}
				}
				tx.Commit()
				buffer = make([]updateResult, 0, batchSize)
			}

			for {
				select {
				case res, ok := <-resultChan:
					if !ok {
						flush() // Flush remaining items
						return
					}
					buffer = append(buffer, res)
					if len(buffer) >= batchSize {
						flush()
					}
				case <-ticker.C:
					flush()
				}
			}
		}()

		// Phase 1: scan and collect missing files
		for _, img := range images {
			IncVerificationProcessed()
			expectedFullPath := filepath.Join(UploadsDir, img.Filename)

			if _, err := os.Stat(expectedFullPath); os.IsNotExist(err) {
				missingCount++
				IncVerificationMissing()

				// Optional: regenerate thumbnail if main file exists but thumb doesn't
				thumbPath := filepath.Join(UploadsDir, "thumbnails", filepath.Base(img.Filename))
				if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
					if _, err := os.Stat(expectedFullPath); err == nil {
						go GenerateThumbnail(expectedFullPath)
					}
				}

				if img.DownloadURL == "" {
					continue
				}

				// Resolve source name (same logic you already had)
				sourceName := "uncategorized"
				var sourceID *uint
				err := database.DB.
					Table("galleries").
					Select("galleries.source_id").
					Joins("JOIN image_galleries ON image_galleries.gallery_id = galleries.id").
					Where("image_galleries.image_id = ?", img.ID).
					Limit(1).
					Scan(&sourceID).Error
				if err == nil && sourceID != nil {
					var src models.Source
					// Use Find() instead of First() to avoid "record not found" errors
					if database.DB.Select("name").Find(&src, *sourceID).RowsAffected > 0 {
						sourceName = src.Name
					}
					// If source not found (deleted), sourceName remains "uncategorized"
				}

				mu.Lock()
				tasks = append(tasks, recoveryTask{
					ID:            img.ID,
					CurrentDBPath: img.Filename,
					DownloadURL:   img.DownloadURL,
					SourceName:    sourceName,
				})
				mu.Unlock()
			}
		}

		if missingCount > 0 {
			fmt.Printf("Found %d missing images\n", missingCount)
		}

		// Phase 2: concurrent recovery – THE IMPORTANT PART
		if len(tasks) > 0 {

			// Per-provider semaphore system
			const maxConcurrentPerProvider = 10
			providerSemaphores := make(map[string]chan struct{})
			var semMutex sync.Mutex

			// Helper to get or create semaphore for a provider
			getSemaphore := func(provider string) chan struct{} {
				semMutex.Lock()
				defer semMutex.Unlock()
				if _, exists := providerSemaphores[provider]; !exists {
					providerSemaphores[provider] = make(chan struct{}, maxConcurrentPerProvider)
				}
				return providerSemaphores[provider]
			}

			var wg sync.WaitGroup

			for _, task := range tasks {
				wg.Add(1)
				go func(t recoveryTask) {
					defer wg.Done()

					// Extract provider and get its semaphore
					provider := extractImageProvider(t.DownloadURL)
					sem := getSemaphore(provider)

					sem <- struct{}{}
					UpdateProviderStatus(provider, len(sem), cap(sem))
					AddActiveVerificationDownload(t.ID, filepath.Base(t.CurrentDBPath), t.DownloadURL, t.SourceName)
					defer func() {
						<-sem
						UpdateProviderStatus(provider, len(sem), cap(sem))
						RemoveActiveVerificationDownload(t.ID)
					}()

					start := time.Now()

					// This returns the FINAL path where it saved the file
					// (e.g. /uploads/MySource/abc123.jpg or whatever it decided)
					result, err := DownloadImage(t.DownloadURL, t.SourceName)
					if err != nil {
						logger.Warnf("[%s] Failed to download image ID %d: %v", provider, t.ID, err)
						return
					}

					// We now trust DownloadImage to handle hashing and storage consistently.
					// We just need to calculate the relative path for the DB.
					// result.Path is like "uploads\Source\hash.jpg"
					// We want "Source\hash.jpg"
					relPath, err := filepath.Rel(UploadsDir, result.Path)
					if err != nil {
						relPath = filepath.Join(t.SourceName, filepath.Base(result.Path)) // Fallback
					}

					// Send result to batch processor instead of updating directly
					resultChan <- updateResult{
						ID:             t.ID,
						RelPath:        relPath,
						DominantColors: result.DominantColors,
					}

					// Generate thumbnail
					GenerateThumbnail(result.Path)

					duration := time.Since(start)
					logger.Debugf("[%s] Recovered image ID %d in %.2fs", provider, t.ID, duration.Seconds())
					if duration > 5*time.Second {
						time.Sleep(1 * time.Second) // polite pause
					}
				}(task)
			}

			wg.Wait()
			close(resultChan) // Signal batch processor to finish
			wgBatch.Wait()    // Wait for batch processor to complete writes
		}

		recovered := atomic.LoadInt32(&recoveredCount)
		stillMissing := missingCount - int(recovered)
		if missingCount > 0 {
			fmt.Printf("Image verification complete — Downloaded: %d | Still missing: %d\n", recovered, stillMissing)
		}
	}()

	return nil
}

// VerifyPersonImages checks all people with identifiers and re-downloads missing photos
func VerifyPersonImages() error {
	logger.Info("Verifying person images...")

	// Find all people that have identifiers (new system) OR stash_id (deprecated)
	var people []models.Person

	// Query people with identifiers from the new system
	var peopleWithIdentifiers []models.Person
	if err := database.DB.
		Joins("JOIN person_identifiers ON person_identifiers.person_id = people.id").
		Distinct("people.*").
		Find(&peopleWithIdentifiers).Error; err != nil {
		return fmt.Errorf("failed to query people with identifiers: %w", err)
	}

	// Also get people with deprecated stash_id
	var peopleWithStashID []models.Person
	if err := database.DB.
		Where("stash_id != ? AND stash_id != ''", "").
		Find(&peopleWithStashID).Error; err != nil {
		return fmt.Errorf("failed to query people with stash_id: %w", err)
	}

	// Merge both lists (avoiding duplicates)
	seen := make(map[uint]bool)
	for _, p := range peopleWithIdentifiers {
		if !seen[p.ID] {
			seen[p.ID] = true
			people = append(people, p)
		}
	}
	for _, p := range peopleWithStashID {
		if !seen[p.ID] {
			seen[p.ID] = true
			people = append(people, p)
		}
	}

	if len(people) == 0 {
		logger.Info("Person image verification complete. No people with identifiers found.")
		return nil
	}

	go func() {
		var recoveredCount int32 = 0

		registry := GetIdentifierRegistry()

		for _, person := range people {
			if person.Photos == "" {
				continue
			}

			var photoPaths []string
			if err := json.Unmarshal([]byte(person.Photos), &photoPaths); err != nil {
				continue
			}

			needsRecovery := false
			for _, webPath := range photoPaths {
				relativePath := strings.TrimPrefix(webPath, "/")
				if strings.HasPrefix(relativePath, "person-images/") {
					parts := strings.SplitN(relativePath, "/", 2)
					if len(parts) == 2 {
						localPath := filepath.Join(UploadsDir, "person_images", filepath.FromSlash(parts[1]))
						if _, err := os.Stat(localPath); os.IsNotExist(err) {
							fmt.Printf("Missing person image: %s (Person ID: %d)\n", localPath, person.ID)
							needsRecovery = true
							break
						}
					}
				}
			}

			if !needsRecovery {
				continue
			}

			// Collect all identifiers for this person
			var identifiers []models.PersonIdentifier
			database.DB.Where("person_id = ?", person.ID).Find(&identifiers)

			// Also add deprecated stash_id as a source if present
			if person.StashID != "" {
				identifiers = append(identifiers, models.PersonIdentifier{
					Source:     "stashdb",
					ExternalID: person.StashID,
				})
			}

			if len(identifiers) == 0 {
				continue
			}

			fmt.Printf("Recovering images for person %s (ID: %d)...\n", person.Name, person.ID)

			var newPhotoURLs []string
			personIDUint := person.ID

			// Try each identifier source to get photos
			for _, ident := range identifiers {
				provider, err := registry.GetProvider(ident.Source)
				if err != nil {
					continue
				}

				personData, err := provider.GetDetails(ident.ExternalID)
				if err != nil {
					continue
				}

				for _, img := range personData.Photos {
					localPath, err := DownloadPersonImage(img, personIDUint)
					if err != nil {
						fmt.Printf("Failed to re-download image %s: %v\n", img, err)
						continue
					}
					newPhotoURLs = append(newPhotoURLs, localPath)
				}
			}

			if len(newPhotoURLs) > 0 {
				photosJSON, _ := json.Marshal(newPhotoURLs)

				if err := database.DB.Model(&models.Person{ID: person.ID}).Update("photos", string(photosJSON)).Error; err != nil {
					fmt.Printf("Failed to update person photos for ID %d: %v\n", person.ID, err)
				} else {
					atomic.AddInt32(&recoveredCount, 1)
					fmt.Printf("Successfully recovered images for person ID %d\n", person.ID)
				}
			}
		}

		if recoveredCount > 0 {
			logger.Info(fmt.Sprintf("Completed verification of person images. Recovered: %d people.", recoveredCount))
		} else {
			logger.Info("Person image verification complete. No missing images found.")
		}
	}()

	return nil
}

// RemoveDuplicateImages identifies and removes duplicate images based on DownloadURL
func RemoveDuplicateImages() error {
	logger.Info("Checking for duplicate images...")

	// Find download URLs that appear more than once
	var duplicates []struct {
		DownloadURL string
		Count       int
	}

	if err := database.DB.Model(&models.Image{}).
		Select("download_url, count(*) as count").
		Where("download_url != ?", "").
		Group("download_url").
		Having("count(*) > 1").
		Find(&duplicates).Error; err != nil {
		return fmt.Errorf("failed to query duplicate images: %w", err)
	}

	if len(duplicates) == 0 {
		logger.Info("No duplicate images found.")
		return nil
	}

	logger.Infof("Found %d duplicate download URLs. Processing removals...", len(duplicates))

	removedCount := 0
	filesRemovedCount := 0

	for _, dup := range duplicates {
		var images []models.Image
		if err := database.DB.Where("download_url = ?", dup.DownloadURL).Find(&images).Error; err != nil {
			logger.Errorf("Failed to fetch images for url %s: %v", dup.DownloadURL, err)
			continue
		}

		if len(images) < 2 {
			continue
		}

		// determine keeper
		// Priority:
		// 1. IsFavorite
		// 2. File Exists on Disk
		// 3. Oldest CreatedAt (Keep the original import)

		// Sort so the best candidate is at index 0
		sort.Slice(images, func(i, j int) bool {
			// 1. IsFavorite
			if images[i].IsFavorite != images[j].IsFavorite {
				return images[i].IsFavorite // True (favorite) comes first
			}

			// 2. File Exists
			pathI := filepath.Join(UploadsDir, images[i].Filename)
			pathJ := filepath.Join(UploadsDir, images[j].Filename)
			_, errI := os.Stat(pathI)
			_, errJ := os.Stat(pathJ)
			existsI := errI == nil
			existsJ := errJ == nil

			if existsI != existsJ {
				return existsI // True (exists) comes first
			}

			// 3. Oldest CreatedAt (Stable ID usually implies age too, but use CreatedAt)
			return images[i].CreatedAt.Before(images[j].CreatedAt)
		})

		keeper := images[0]
		keeperPath := filepath.Join(UploadsDir, keeper.Filename)

		// The rest are to be removed
		for i := 1; i < len(images); i++ {
			toRemove := images[i]
			pathToRemove := filepath.Join(UploadsDir, toRemove.Filename)

			// Delete from DB
			if err := database.DB.Unscoped().Delete(&toRemove).Error; err != nil {
				logger.Errorf("Failed to delete duplicate image ID %d: %v", toRemove.ID, err)
				continue
			}
			removedCount++

			// Delete file ONLY if it is different from keeper's path
			// (If they point to the exact same filename, we don't delete the file!)
			if pathToRemove != keeperPath && toRemove.Filename != keeper.Filename {
				if err := os.Remove(pathToRemove); err != nil {
					if !os.IsNotExist(err) {
						logger.Warnf("Failed to delete duplicate file %s: %v", pathToRemove, err)
					}
				} else {
					filesRemovedCount++
					// Try to remove thumbnail too
					thumbPath := filepath.Join(UploadsDir, "thumbnails", filepath.Base(pathToRemove))
					os.Remove(thumbPath)
				}
			}
		}
	}

	logger.Infof("Duplicate removal complete. Removed %d DB entries and %d files.", removedCount, filesRemovedCount)
	return nil
}

// Helper for cross-device moves
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}
