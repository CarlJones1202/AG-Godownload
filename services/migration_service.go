package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"github.com/disintegration/imaging"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MigrateImagesToNewStructure migrates existing images from flat structure to source-based subdirectories
func MigrateImagesToNewStructure() error {
	logger.Info("Starting image migration to new directory structure...")

	// FIX: Load only what we need — NO Preload("Galleries") → avoids SQLite variable explosion
	var images []struct {
		ID          uint
		Filename    string
		DownloadURL string
	}
	if err := database.DB.
		Model(&models.Image{}).
		Select("id, filename, download_url").
		Find(&images).Error; err != nil {
		return fmt.Errorf("failed to load images: %w", err)
	}

	logger.Infof("Found %d images to migrate", len(images))

	migrated := 0
	redownloaded := 0
	skipped := 0
	errors := 0

	for _, image := range images {
		// Lazily determine source name — only when needed, and safely
		sourceName := "unknown"

		var sourceID *uint
		err := database.DB.
			Table("galleries").
			Select("galleries.source_id").
			Joins("JOIN image_galleries ON image_galleries.gallery_id = galleries.id").
			Where("image_galleries.image_id = ?", image.ID).
			Limit(1).
			Scan(&sourceID).Error

		if err == nil && sourceID != nil {
			var source models.Source
			if err := database.DB.Select("name").First(&source, *sourceID).Error; err == nil {
				sourceName = source.Name
			} else {
				sourceName = "uncategorized"
			}
		} else {
			sourceName = "uncategorized"
		}

		oldPath := filepath.Join(UploadsDir, image.Filename)
		oldThumbPath := filepath.Join(UploadsDir, "thumbnails", image.Filename)

		// Check if file exists in old location
		if _, err := os.Stat(oldPath); os.IsNotExist(err) {
			// Check if already in new location
			sourceDir := SanitizeDirectoryName(sourceName)
			newPath := filepath.Join(UploadsDir, sourceDir, image.Filename)

			if _, err := os.Stat(newPath); err == nil {
				skipped++
				continue
			}

			// File is truly missing → re-download
			logger.Warnf("Image file missing for %s, re-downloading from %s", image.Filename, image.DownloadURL)

			if image.DownloadURL == "" {
				logger.Errorf("Cannot re-download %s: no download URL", image.Filename)
				errors++
				continue
			}

			// Prefer using the download URL origin as the referer when re-downloading
			referer := ""
			if u, perr := url.Parse(image.DownloadURL); perr == nil {
				referer = u.Scheme + "://" + u.Host
			}

			result, err := DownloadImage(image.DownloadURL, sourceName, referer)
			if err != nil {
				// If imx and gallery-dl enabled, try fallback without logging the initial HTTP failure as final
				if strings.Contains(strings.ToLower(image.DownloadURL), "imx.to") && config.Global.GalleryDL.Enabled {
					logger.Debugf("HTTP re-download failed for %s; attempting gallery-dl fallback", image.Filename)
					gctx, gcancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GalleryDL.TimeoutSec)*time.Second)
					result, err = DownloadImageWithGalleryDL(gctx, image.DownloadURL, sourceName, time.Duration(config.Global.GalleryDL.TimeoutSec)*time.Second)
					gcancel()
					if err != nil {
						logger.Errorf("Failed to re-download %s after gallery-dl fallback: %v", image.Filename, err)
						errors++
						continue
					}
				} else {
					logger.Errorf("Failed to re-download %s: %v", image.Filename, err)
					errors++
					continue
				}
			}

			_, err = GenerateThumbnail(result.Path)
			if err != nil {
				logger.Warnf("Failed to generate thumbnail for %s: %v", image.Filename, err)
			}

			newFilename := filepath.Base(result.Path)
			if err := database.DB.Model(&models.Image{ID: image.ID}).Updates(map[string]interface{}{
				"filename":        newFilename,
				"dominant_colors": result.DominantColors,
			}).Error; err != nil {
				logger.Errorf("Failed to update filename in database: %v", err)
				errors++
				continue
			}

			redownloaded++
			logger.Debugf("Re-downloaded %s → %s", image.Filename, newFilename)
			continue
		}

		// File exists in old location → migrate it
		data, err := os.ReadFile(oldPath)
		if err != nil {
			logger.Errorf("Failed to read %s: %v", oldPath, err)
			errors++
			continue
		}

		hash := sha256.Sum256(data)
		hashStr := hex.EncodeToString(hash[:])
		ext := filepath.Ext(image.Filename)
		if ext == "" {
			ext = ".jpg"
		}
		newFilename := hashStr + ext

		sourceDir := SanitizeDirectoryName(sourceName)
		newDir := filepath.Join(UploadsDir, sourceDir)
		if err := os.MkdirAll(newDir, 0755); err != nil {
			logger.Errorf("Failed to create directory %s: %v", newDir, err)
			errors++
			continue
		}

		newPath := filepath.Join(newDir, newFilename)

		// Handle hash collision
		if _, err := os.Stat(newPath); err == nil {
			logger.Infof("Hash collision detected for %s, re-downloading to ensure separate copy", image.Filename)

			if image.DownloadURL != "" {
				// Prefer using the download URL origin as the referer when re-downloading
				referer := ""
				if u, perr := url.Parse(image.DownloadURL); perr == nil {
					referer = u.Scheme + "://" + u.Host
				}

				result, err := DownloadImage(image.DownloadURL, sourceName, referer)
				if err != nil {
					// Try gallery-dl fallback for imx when enabled; suppress initial HTTP failure as final
					if strings.Contains(strings.ToLower(image.DownloadURL), "imx.to") && config.Global.GalleryDL.Enabled {
						logger.Debugf("HTTP re-download failed for %s (hash collision path); attempting gallery-dl fallback", image.Filename)
						gctx, gcancel := context.WithTimeout(context.Background(), time.Duration(config.Global.GalleryDL.TimeoutSec)*time.Second)
						result, err = DownloadImageWithGalleryDL(gctx, image.DownloadURL, sourceName, time.Duration(config.Global.GalleryDL.TimeoutSec)*time.Second)
						gcancel()
						if err != nil {
							logger.Errorf("Failed to re-download %s after gallery-dl fallback: %v", image.Filename, err)
							// Fall back: just point to existing file
							database.DB.Model(&models.Image{ID: image.ID}).Update("filename", filepath.Join(sourceDir, newFilename))
							redownloaded++
							continue
						}
					} else {
						logger.Errorf("Failed to re-download %s: %v", image.Filename, err)
						// Fall back: just point to existing file
						database.DB.Model(&models.Image{ID: image.ID}).Update("filename", filepath.Join(sourceDir, newFilename))
						redownloaded++
						continue
					}
				} else {
					_, err = GenerateThumbnail(result.Path)
					if err != nil {
						logger.Warnf("Failed to generate thumbnail: %v", err)
					}
					freshFilename := filepath.Base(result.Path)
					database.DB.Model(&models.Image{ID: image.ID}).Updates(map[string]interface{}{
						"filename":        freshFilename,
						"dominant_colors": result.DominantColors,
					})
					redownloaded++
				}
			} else {
				database.DB.Model(&models.Image{ID: image.ID}).Update("filename", filepath.Join(sourceDir, newFilename))
			}

			os.Remove(oldPath)
			os.Remove(oldThumbPath)
			skipped++
			continue
		}

		// Actually move the file
		if err := os.Rename(oldPath, newPath); err != nil {
			logger.Errorf("Failed to move %s to %s: %v", oldPath, newPath, err)
			errors++
			continue
		}

		// Move thumbnail
		newThumbDir := filepath.Join(newDir, "thumbnails")
		os.MkdirAll(newThumbDir, 0755)
		newThumbPath := filepath.Join(newThumbDir, newFilename)
		if _, err := os.Stat(oldThumbPath); err == nil {
			os.Rename(oldThumbPath, newThumbPath)
		}

		// Update DB
		fullNewFilename := filepath.Join(sourceDir, newFilename)
		if err := database.DB.Model(&models.Image{ID: image.ID}).Update("filename", fullNewFilename).Error; err != nil {
			logger.Errorf("Failed to update filename in database: %v", err)
			errors++
			continue
		}

		migrated++
		logger.Debugf("Migrated %s → %s", image.Filename, fullNewFilename)
	}

	logger.Infof("Migration complete: %d migrated, %d re-downloaded, %d skipped, %d errors",
		migrated, redownloaded, skipped, errors)

	return nil
}

// MigrateMissingProviderThumbnails downloads missing provider thumbnails for existing galleries
func MigrateMissingProviderThumbnails() error {
	logger.Info("Starting migration for missing provider thumbnails...")

	var galleries []models.Gallery
	if err := database.DB.Where("provider != '' AND source_url != ''").
		Where("provider_thumbnail = '' OR provider_thumbnail_url = ''").
		Find(&galleries).Error; err != nil {
		return fmt.Errorf("failed to load galleries: %w", err)
	}

	logger.Infof("Found %d galleries with provider but missing thumbnail", len(galleries))

	downloaded := 0
	failed := 0

	for _, gallery := range galleries {
		// Re-scrape metadata to get thumbnail URL
		metadata, err := ScrapeGalleryMetadata(gallery.SourceURL, gallery.Provider, "")
		if err != nil {
			logger.Warnf("Failed to scrape metadata for gallery %d: %v", gallery.ID, err)
			failed++
			continue
		}

		if metadata.ThumbnailURL == "" {
			logger.Debugf("No thumbnail URL found for gallery %d", gallery.ID)
			continue
		}

		localPath, err := DownloadProviderThumbnail(metadata.ThumbnailURL)
		if err != nil {
			logger.Warnf("Failed to download thumbnail for gallery %d: %v", gallery.ID, err)
			failed++
			continue
		}

		gallery.ProviderThumbnail = localPath
		gallery.ProviderThumbnailURL = metadata.ThumbnailURL

		if err := database.DB.Save(&gallery).Error; err != nil {
			logger.Warnf("Failed to save gallery %d: %v", gallery.ID, err)
			failed++
			continue
		}

		downloaded++
	}

	// Also check for galleries where thumbnail file is missing
	var galleriesWithThumb []models.Gallery
	if err := database.DB.Where("provider_thumbnail != ''").Find(&galleriesWithThumb).Error; err != nil {
		return fmt.Errorf("failed to load galleries with thumbnails: %w", err)
	}

	missingFiles := 0
	for _, gallery := range galleriesWithThumb {
		if _, err := os.Stat(gallery.ProviderThumbnail); os.IsNotExist(err) {
			// File missing, try to re-download
			if gallery.ProviderThumbnailURL != "" {
				localPath, err := DownloadProviderThumbnail(gallery.ProviderThumbnailURL)
				if err != nil {
					logger.Warnf("Failed to re-download thumbnail for gallery %d: %v", gallery.ID, err)
					missingFiles++
					continue
				}
				gallery.ProviderThumbnail = localPath
				database.DB.Save(&gallery)
			}
		}
	}

	logger.Infof("Provider thumbnail migration complete: %d downloaded, %d failed, %d files missing",
		downloaded, failed, missingFiles)

	return nil
}

// ValidateProviderThumbnails ensures provider thumbnail files exist and are valid images.
// It attempts to re-download missing or invalid thumbnails using the stored ProviderThumbnailURL.
func ValidateProviderThumbnails() (int, int, error) {
	logger.Info("Validating provider thumbnails...")

	var galleries []models.Gallery
	if err := database.DB.Where("provider_thumbnail != ''").Find(&galleries).Error; err != nil {
		return 0, 0, fmt.Errorf("failed to load galleries with provider thumbnails: %w", err)
	}

	valid := 0
	fixed := 0

	for _, g := range galleries {
		path := g.ProviderThumbnail
		ok := false

		if path != "" {
			if _, err := os.Stat(path); err == nil {
				// Try opening to validate image
				if _, err := imaging.Open(path); err == nil {
					ok = true
				} else {
					logger.Debugf("Provider thumbnail invalid image for gallery %d: %v", g.ID, err)
					// remove invalid file
					_ = os.Remove(path)
				}
			} else {
				logger.Debugf("Provider thumbnail missing on disk for gallery %d: %s", g.ID, path)
			}
		}

		if !ok {
			// Attempt to re-download from stored URL if available
			if g.ProviderThumbnailURL != "" {
				logger.Infof("Re-downloading provider thumbnail for gallery %d from %s", g.ID, g.ProviderThumbnailURL)
				localPath, err := DownloadProviderThumbnail(g.ProviderThumbnailURL)
				if err == nil {
					// validate
					if _, err := imaging.Open(localPath); err == nil {
						g.ProviderThumbnail = localPath
						// update stored URL in case it changed
						g.ProviderThumbnailURL = g.ProviderThumbnailURL
						if err := database.DB.Save(&g).Error; err != nil {
							logger.Warnf("Failed to save re-downloaded thumbnail path for gallery %d: %v", g.ID, err)
						}
						fixed++
						continue
					}
					// remove invalid file
					_ = os.Remove(localPath)
				} else {
					logger.Warnf("Failed to re-download provider thumbnail for gallery %d: %v", g.ID, err)
				}

				// If the stored URL failed and the provider is MetArt, try to re-scrape metadata
				// and use the API-provided media path which points to an actual image.
				if strings.EqualFold(g.Provider, "MetArt") || strings.EqualFold(g.Provider, "Metart") {
					logger.Infof("Attempting MetArt API fallback for gallery %d", g.ID)
					if meta, serr := ScrapeGalleryMetadata(g.SourceURL, g.Provider, ""); serr == nil {
						if meta.ThumbnailURL != "" && meta.ThumbnailURL != g.ProviderThumbnailURL {
							logger.Infof("MetArt fallback thumbnail URL for gallery %d -> %s", g.ID, meta.ThumbnailURL)
							localPath2, derr := DownloadProviderThumbnail(meta.ThumbnailURL)
							if derr == nil {
								if _, err := imaging.Open(localPath2); err == nil {
									g.ProviderThumbnail = localPath2
									g.ProviderThumbnailURL = meta.ThumbnailURL
									if err := database.DB.Save(&g).Error; err != nil {
										logger.Warnf("Failed to save MetArt fallback thumbnail for gallery %d: %v", g.ID, err)
									}
									fixed++
									continue
								}
								_ = os.Remove(localPath2)
							} else {
								logger.Warnf("Failed to download MetArt fallback thumbnail for gallery %d: %v", g.ID, derr)
							}
						}
					} else {
						logger.Debugf("MetArt API scrape failed for gallery %d: %v", g.ID, serr)
					}
				}
			}

			// If we reach here, we couldn't validate or re-download — clear DB path
			g.ProviderThumbnail = ""
			if err := database.DB.Save(&g).Error; err != nil {
				logger.Warnf("Failed to clear invalid provider thumbnail for gallery %d: %v", g.ID, err)
			}
		} else {
			valid++
		}
	}

	logger.Infof("Provider thumbnail validation complete: %d valid, %d fixed/updated", valid, fixed)
	return valid, fixed, nil
}
