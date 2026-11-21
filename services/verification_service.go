package services

import (
	"context"
	"encoding/json"
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"io"
	urlpkg "net/url"
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
// ImageRecoveryTask represents a missing image task to be recovered
type ImageRecoveryTask struct {
	ID            uint
	CurrentDBPath string
	DownloadURL   string
	OriginalURL   string
	SourceName    string
	IsFavorite    bool
	CreatedAt     time.Time
	GalleryIDs    []uint
}

// PrioritizeImageRecoveryTasks orders missing images according to the required priority:
// 1. Tier 1: Favorites first (is_favorite = true)
// 2. Tier 2: Missing images belonging to any gallery that contains a favorite
// 3. Tier 3: Remaining missing images, alternating between newest and oldest galleries (followed by orphans)
func PrioritizeImageRecoveryTasks(tasks []ImageRecoveryTask, favGalleryIDs map[uint]bool, galleryCreatedAt map[uint]time.Time) []ImageRecoveryTask {
	var tier1 []ImageRecoveryTask
	var tier2 []ImageRecoveryTask
	tier3Galleries := make(map[uint][]ImageRecoveryTask)
	var orphanTasks []ImageRecoveryTask

	for _, t := range tasks {
		if t.IsFavorite {
			tier1 = append(tier1, t)
			continue
		}

		hasFavGal := false
		for _, gid := range t.GalleryIDs {
			if favGalleryIDs[gid] {
				hasFavGal = true
				break
			}
		}

		if hasFavGal {
			tier2 = append(tier2, t)
		} else if len(t.GalleryIDs) > 0 {
			gid := t.GalleryIDs[0]
			tier3Galleries[gid] = append(tier3Galleries[gid], t)
		} else {
			orphanTasks = append(orphanTasks, t)
		}
	}

	// Sort non-favorite galleries by CreatedAt (oldest to newest)
	galIDs := make([]uint, 0, len(tier3Galleries))
	for gid := range tier3Galleries {
		galIDs = append(galIDs, gid)
	}

	sort.Slice(galIDs, func(i, j int) bool {
		timeI := galleryCreatedAt[galIDs[i]]
		timeJ := galleryCreatedAt[galIDs[j]]
		if !timeI.Equal(timeJ) {
			return timeI.Before(timeJ)
		}
		return galIDs[i] < galIDs[j]
	})

	// Alternate between newest and oldest galleries
	var tier3 []ImageRecoveryTask
	left := 0
	right := len(galIDs) - 1
	takeNewest := true

	for left <= right {
		if takeNewest {
			gid := galIDs[right]
			tier3 = append(tier3, tier3Galleries[gid]...)
			right--
			takeNewest = false
		} else {
			gid := galIDs[left]
			tier3 = append(tier3, tier3Galleries[gid]...)
			left++
			takeNewest = true
		}
	}
	tier3 = append(tier3, orphanTasks...)

	result := make([]ImageRecoveryTask, 0, len(tasks))
	result = append(result, tier1...)
	result = append(result, tier2...)
	result = append(result, tier3...)
	return result
}

// collectMissingImageTasks selects images whose files are not on disk, using
// the file_exists column that SyncFileExists computes immediately before
// recovery runs at startup (callers must run it first — no per-row stat is
// performed here). Videos are excluded: the video recovery pipeline owns them,
// and image recovery would otherwise download the entire video body only to
// fail image decoding and discard it. Returns the recovery tasks, the number
// of missing rows (with or without a download URL), and the first query error
// encountered while still returning everything collected up to that point.
func collectMissingImageTasks() ([]ImageRecoveryTask, int, error) {
	const chunkSize = 1000
	var lastID uint
	var tasks []ImageRecoveryTask
	missing := 0
	var firstErr error

	for {
		var chunk []struct {
			ID          uint
			Filename    string
			DownloadURL string
			OriginalURL string
			IsFavorite  bool
			CreatedAt   time.Time
		}

		if err := database.DB.
			Model(&models.Image{}).
			Select("id, filename, download_url, original_url, is_favorite, created_at").
			Where("(type IS NULL OR type <> ?) AND file_exists = ? AND id > ?", "video", false, lastID).
			Order("id ASC").
			Limit(chunkSize).
			Find(&chunk).Error; err != nil {
			if firstErr == nil {
				firstErr = err
			}
			logger.Errorf("failed to query missing image batch: %v", err)
			break
		}
		if len(chunk) == 0 {
			break
		}

		for _, img := range chunk {
			missing++
			IncVerificationProcessed()
			IncVerificationMissing()

			if img.DownloadURL == "" {
				continue
			}

			tasks = append(tasks, ImageRecoveryTask{
				ID:            img.ID,
				CurrentDBPath: img.Filename,
				DownloadURL:   img.DownloadURL,
				OriginalURL:   img.OriginalURL,
				IsFavorite:    img.IsFavorite,
				CreatedAt:     img.CreatedAt,
				SourceName:    "uncategorized",
			})
		}

		lastID = chunk[len(chunk)-1].ID
	}

	return tasks, missing, firstErr
}

// loadImageGalleryRelations batch-loads gallery IDs and source names for the
// given image IDs (chunked to bound IN-clause size). A failed batch is logged
// and skipped so remaining batches still resolve, matching the previous
// per-batch behavior; the first error is also returned for the caller to log.
func loadImageGalleryRelations(ids []uint) (map[uint][]uint, map[uint]string, error) {
	type imgGalRel struct {
		ImageID    uint
		GalleryID  uint
		SourceName *string
	}

	imgToGalleries := make(map[uint][]uint)
	imgToSource := make(map[uint]string)
	var firstErr error

	const queryBatchSize = 1000
	for i := 0; i < len(ids); i += queryBatchSize {
		end := i + queryBatchSize
		if end > len(ids) {
			end = len(ids)
		}

		var rels []imgGalRel
		err := database.DB.Table("image_galleries").
			Select("image_galleries.image_id, image_galleries.gallery_id, sources.name as source_name").
			Joins("JOIN galleries ON galleries.id = image_galleries.gallery_id").
			Joins("LEFT JOIN sources ON sources.id = galleries.source_id").
			Where("image_galleries.image_id IN ?", ids[i:end]).
			Scan(&rels).Error
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, r := range rels {
			imgToGalleries[r.ImageID] = append(imgToGalleries[r.ImageID], r.GalleryID)
			if r.SourceName != nil && *r.SourceName != "" && imgToSource[r.ImageID] == "" {
				imgToSource[r.ImageID] = *r.SourceName
			}
		}
	}

	return imgToGalleries, imgToSource, firstErr
}

// VerifyDownloadedImages checks all images in the database and re-downloads any missing files
// VerifyDownloadedImages checks all images and re-downloads + fixes DB if needed
func VerifyDownloadedImages() error {
	logger.Info("Verifying downloaded images...")

	// Count recoverable images first. Videos are owned by
	// VerifyDownloadedVideos: counting them here would inflate the progress
	// total, and image recovery would download video bodies only to discard
	// them when image decoding fails.
	var totalImages int64
	if err := database.DB.Model(&models.Image{}).
		Where("(type IS NULL OR type <> ?)", "video").
		Count(&totalImages).Error; err != nil {
		return fmt.Errorf("failed to count images: %w", err)
	}

	go func() {
		SetVerificationRunning(true, int(totalImages))
		defer SetVerificationRunning(false, 0)

		var recoveredCount int32 = 0

		type updateResult struct {
			ID             uint
			RelPath        string
			DominantColors string
		}
		resultChan := make(chan updateResult, 100)
		var wgBatch sync.WaitGroup

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
							"file_exists":     true,
						}).Error; err != nil {
						logger.Errorf("Failed to update image %d: %v", res.ID, err)
					} else {
						atomic.AddInt32(&recoveredCount, 1)
						IncVerificationRecovered()
						// Embedding is deferred until recovery completes (see
						// below) so the embedder does not compete with downloads
						// for CPU on a fresh install.
					}
				}
				tx.Commit()
				buffer = make([]updateResult, 0, batchSize)
			}

			for {
				select {
				case res, ok := <-resultChan:
					if !ok {
						flush()
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

		// Post-processing pool: dominant colors (k-means) and the Lanczos
		// thumbnail resize are CPU-bound and must not occupy download slots.
		type imagePostJob struct {
			result updateResult
			path   string
		}
		postChan := make(chan imagePostJob, 128)
		var wgPost sync.WaitGroup
		for w := 0; w < recoveryPostprocessWorkers(); w++ {
			wgPost.Add(1)
			go func() {
				defer wgPost.Done()
				for job := range postChan {
					if job.result.DominantColors == "" {
						// gallery-dl fallback results already carry colors.
						colors, err := ExtractDominantColors(job.path)
						if err != nil {
							logger.Warnf("Dominant color extraction failed for image %d: %v", job.result.ID, err)
							colors = "[]"
						}
						job.result.DominantColors = colors
					}
					if _, err := GenerateThumbnail(job.path); err != nil {
						logger.Warnf("Thumbnail generation failed for recovered image %d: %v", job.result.ID, err)
					}
					resultChan <- job.result
				}
			}()
		}

		// Phase 1: collect missing files from the file_exists column, which
		// SyncFileExists computed immediately before this runs at startup.
		// Rows are already false, so there is no per-row stat and no per-row
		// UPDATE. Videos are excluded (the video pipeline owns them).
		rawTasks, missingCount, collectErr := collectMissingImageTasks()
		if collectErr != nil {
			logger.Errorf("Missing-image scan finished with errors: %v", collectErr)
		} else {
			// Rows examined only via the column (files believed present) still
			// count as processed so the progress counter reaches the total.
			AddVerificationProcessed(int32(totalImages) - int32(missingCount))
		}
		if missingCount > 0 {
			logger.Infof("Found %d missing images", missingCount)
		}

		// Phase 2: prioritize and recover
		if len(rawTasks) > 0 {
			// 1. Batch load gallery associations and source names for missing images
			taskIDs := make([]uint, len(rawTasks))
			for i, t := range rawTasks {
				taskIDs[i] = t.ID
			}
			imgToGalleries, imgToSource, relErr := loadImageGalleryRelations(taskIDs)
			if relErr != nil {
				logger.Errorf("Failed to query gallery relations for missing images: %v", relErr)
			}
			referencedGalMap := make(map[uint]bool)
			for _, gids := range imgToGalleries {
				for _, gid := range gids {
					referencedGalMap[gid] = true
				}
			}

			// Apply resolved gallery IDs and source names to raw tasks
			for i := range rawTasks {
				if gids, ok := imgToGalleries[rawTasks[i].ID]; ok {
					rawTasks[i].GalleryIDs = gids
				}
				if src, ok := imgToSource[rawTasks[i].ID]; ok && src != "" {
					rawTasks[i].SourceName = src
				}
			}

			// 2. Query all gallery IDs that contain at least one favorite
			var favGalleryIDs []uint
			if err := database.DB.Table("image_galleries").
				Joins("JOIN images ON images.id = image_galleries.image_id").
				Where("images.is_favorite = ?", true).
				Distinct("image_galleries.gallery_id").
				Pluck("image_galleries.gallery_id", &favGalleryIDs).Error; err != nil {
				logger.Errorf("Failed to query favorite gallery IDs: %v", err)
			}

			favGalSet := make(map[uint]bool, len(favGalleryIDs))
			for _, gid := range favGalleryIDs {
				favGalSet[gid] = true
			}
			for _, t := range rawTasks {
				if t.IsFavorite {
					for _, gid := range t.GalleryIDs {
						favGalSet[gid] = true
					}
				}
			}

			// 3. Query creation dates for all referenced galleries
			const queryBatchSize = 1000
			var refGalIDs []uint
			for gid := range referencedGalMap {
				refGalIDs = append(refGalIDs, gid)
			}
			galleryCreatedAt := make(map[uint]time.Time)
			if len(refGalIDs) > 0 {
				for i := 0; i < len(refGalIDs); i += queryBatchSize {
					end := i + queryBatchSize
					if end > len(refGalIDs) {
						end = len(refGalIDs)
					}
					var gals []struct {
						ID        uint
						CreatedAt time.Time
					}
					if err := database.DB.Model(&models.Gallery{}).
						Select("id, created_at").
						Where("id IN ?", refGalIDs[i:end]).
						Find(&gals).Error; err == nil {
						for _, g := range gals {
							galleryCreatedAt[g.ID] = g.CreatedAt
						}
					}
				}
			}

			// 4. Prioritize tasks
			tasks := PrioritizeImageRecoveryTasks(rawTasks, favGalSet, galleryCreatedAt)
			logger.Infof("Prioritized %d missing images for recovery (Favorites -> Favorite Galleries -> Alternating Galleries)", len(tasks))

			// 5. Partition by provider and execute via worker pools
			providerTasks := make(map[string][]ImageRecoveryTask)
			for _, task := range tasks {
				provider := extractImageProvider(task.DownloadURL)
				providerTasks[provider] = append(providerTasks[provider], task)
			}

			maxConcurrentPerProvider := recoveryConcurrency()
			var wgProviders sync.WaitGroup

			for provider, pTasks := range providerTasks {
				wgProviders.Add(1)
				go func(prov string, taskList []ImageRecoveryTask) {
					defer wgProviders.Done()

					taskChan := make(chan ImageRecoveryTask, len(taskList))
					for _, t := range taskList {
						taskChan <- t
					}
					close(taskChan)

					numWorkers := maxConcurrentPerProvider
					if len(taskList) < numWorkers {
						numWorkers = len(taskList)
					}

					var wgWorkers sync.WaitGroup
					for w := 0; w < numWorkers; w++ {
						wgWorkers.Add(1)
						go func() {
							defer wgWorkers.Done()
							for t := range taskChan {
								func(task ImageRecoveryTask) {
									IncProviderActive(prov, maxConcurrentPerProvider)
									AddActiveVerificationDownload(task.ID, filepath.Base(task.CurrentDBPath), task.DownloadURL, task.SourceName, task.IsFavorite)
									defer func() {
										DecProviderActive(prov, maxConcurrentPerProvider)
										RemoveActiveVerificationDownload(task.ID)
									}()

									start := time.Now()

									referer := ""
									referer = imageDownloadReferer(prov, task.OriginalURL, task.DownloadURL)
									if referer == "" {
										if u, perr := urlpkg.Parse(task.DownloadURL); perr == nil {
											referer = u.Scheme + "://" + u.Host
										}
									}
									result, err := DownloadImageWithoutColors(task.DownloadURL, task.SourceName, referer)
									if err != nil {
										if prov == "imx" {
											logger.Debugf("HTTP download failed for image ID %d; attempting direct/IMX-page fallback", task.ID)
											gctx, gcancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GalleryDL.TimeoutSec)*time.Second)
											result, err = DownloadImageWithIMXFallbackWithoutPostProcessing(gctx, task.OriginalURL, task.DownloadURL, task.SourceName)
											gcancel()
											if err != nil {
												logger.Warnf("[%s] Failed to download image ID %d after IMX page fallback: %v", prov, task.ID, err)
												return
											}
										} else {
											logger.Warnf("[%s] Failed to download image ID %d: %v", prov, task.ID, err)
											return
										}
									}

									relPath, err := filepath.Rel(UploadsDir, result.Path)
									if err != nil {
										relPath = filepath.Join(task.SourceName, filepath.Base(result.Path))
									}

									postChan <- imagePostJob{
										result: updateResult{
											ID:             task.ID,
											RelPath:        relPath,
											DominantColors: result.DominantColors,
										},
										path: result.Path,
									}

									duration := time.Since(start)
									logger.Debugf("[%s] Recovered image ID %d in %.2fs", prov, task.ID, duration.Seconds())
									if duration > 5*time.Second {
										time.Sleep(1 * time.Second)
									}
								}(t)
							}
						}()
					}
					wgWorkers.Wait()
				}(provider, pTasks)
			}

			wgProviders.Wait()
			close(postChan) // Signal post-processing pool to finish
			wgPost.Wait()   // Colors/thumbnails done; resultChan fully fed
			close(resultChan)
			wgBatch.Wait() // Wait for batch processor to complete writes
			database.Checkpoint()

			// Embedding is deferred until recovery completes so the embedder
			// does not compete with downloads for CPU on a fresh install; the
			// periodic sweep would otherwise pick these up within 15 minutes.
			if n := EnqueueMissingEmbeddings(); n > 0 {
				logger.Infof("Enqueued %d recovered images for embedding", n)
			}
		} else {
			// No missing work: let the batch writer and post pool exit instead
			// of leaking them for the process lifetime.
			close(postChan)
			wgPost.Wait()
			close(resultChan)
			wgBatch.Wait()
		}

		recovered := atomic.LoadInt32(&recoveredCount)
		stillMissing := missingCount - int(recovered)
		if missingCount > 0 {
			logger.Infof("Image verification complete — Downloaded: %d | Still missing: %d", recovered, stillMissing)
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

		// Precompute on-disk existence once per image; stat'ing inside the sort
		// comparator would cost O(n log n) syscalls per duplicate group.
		type candidate struct {
			img    models.Image
			exists bool
		}
		candidates := make([]candidate, len(images))
		for i, img := range images {
			_, err := os.Stat(filepath.Join(UploadsDir, img.Filename))
			candidates[i] = candidate{img: img, exists: err == nil}
		}

		// Sort so the best candidate is at index 0
		sort.Slice(candidates, func(i, j int) bool {
			a, b := candidates[i].img, candidates[j].img

			// 1. IsFavorite
			if a.IsFavorite != b.IsFavorite {
				return a.IsFavorite // True (favorite) comes first
			}

			// 2. File Exists
			if candidates[i].exists != candidates[j].exists {
				return candidates[i].exists // True (exists) comes first
			}

			// 3. Oldest CreatedAt (Stable ID usually implies age too, but use CreatedAt)
			return a.CreatedAt.Before(b.CreatedAt)
		})

		keeper := candidates[0].img
		keeperPath := filepath.Join(UploadsDir, keeper.Filename)

		// The rest are to be removed
		for i := 1; i < len(candidates); i++ {
			toRemove := candidates[i].img
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
