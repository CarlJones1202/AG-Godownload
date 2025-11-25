package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"net/http"
	"path/filepath"
	"strings"
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
	doc.Find("div[id^='post_message_']").Each(func(i int, s *goquery.Selection) {
		// Find images inside this div - look for <a> tags containing <img>
		s.Find("a img").Each(func(j int, img *goquery.Selection) {
			// Skip "View Post" images
			if img.AttrOr("alt", "") == "View Post" {
				fmt.Printf("Skipping element %d: alt='View Post'\n", j)
				return
			}

			// Get the parent <a> tag's href
			a := img.Parent()
			src, exists := a.Attr("href")
			if !exists {
				fmt.Printf("Element %d: No href found\n", j)
				return
			}
			fmt.Printf("Element %d: Found link %s\n", j, src)

			// Use rippers to extract actual image URL based on hosting site
			var imageURL string
			var err error
			switch {
			case strings.Contains(src, "imagebam"):
				fmt.Println("Ripping from ImageBam")
				imageURL, err = RipImageBam(src)
			case strings.Contains(src, "imgbox"):
				fmt.Println("Ripping from ImgBox")
				imageURL, err = RipImageBox(src)
			case strings.Contains(src, "imx.to"):
				fmt.Println("Ripping from Imx.to")
				imageURL, err = RipImx(img.AttrOr("src", ""))
			case strings.Contains(src, "turboimagehost"):
				fmt.Println("Ripping from TurboImageHost")
				imageURL, err = RipTurboImg(src)
			case strings.Contains(src, "vipr.im"):
				fmt.Println("Ripping from Vipr.im")
				imageURL, err = RipViprIm(img.AttrOr("src", ""))
			case strings.Contains(src, "pixhost"):
				fmt.Println("Ripping from PixHost")
				imageURL, err = RipPixHost(img.AttrOr("src", ""))
			case strings.Contains(src, "acidimg"):
				fmt.Println("Ripping from AcidImg")
				imageURL, err = RipAcidImg(img.AttrOr("src", ""))
			case strings.Contains(src, "postimages.org"):
				fmt.Println("Ripping from PostImages")
				imageURL, err = RipPostImages(src)
			case strings.Contains(src, "pixxxels.cc") || strings.Contains(src, "freeimage.us"):
				fmt.Printf("Skipping unsupported host: %s\n", src)
				return
			default:
				// If it's a direct image link, use it
				lowerSrc := strings.ToLower(src)
				if strings.HasSuffix(lowerSrc, ".jpg") || strings.HasSuffix(lowerSrc, ".png") || strings.HasSuffix(lowerSrc, ".jpeg") || strings.HasSuffix(lowerSrc, ".gif") {
					imageURL = src
				} else {
					fmt.Printf("Unknown image source %s\n", src)
					return
				}
			}

			if err != nil {
				fmt.Printf("Error ripping %s: %v\n", src, err)
				return
			}

			if imageURL == "" {
				fmt.Printf("No image URL extracted from %s\n", src)
				return
			}

			// Basic deduplication check (by URL)
			var count int64
			database.DB.Model(&models.Image{}).Where("original_url = ? AND gallery_id = ?", imageURL, gallery.ID).Count(&count)
			if count > 0 {
				fmt.Printf("Image already exists: %s\n", imageURL)
				return
			}

			// Download the actual image
			filename := filepath.Base(imageURL)
			// Sanitize filename
			filename = strings.Split(filename, "?")[0]

			destPath, err := DownloadImage(imageURL, filename)
			if err != nil {
				fmt.Printf("Failed to download %s: %v\n", imageURL, err)
				return
			}

			// Generate thumbnail
			_, err = GenerateThumbnail(destPath)
			if err != nil {
				fmt.Printf("Failed to generate thumbnail for %s: %v\n", filename, err)
			}

			// Save to DB
			image := models.Image{
				GalleryID:   gallery.ID,
				Filename:    filepath.Base(destPath),
				OriginalURL: src,      // The hosting page URL
				DownloadURL: imageURL, // The final direct image URL
			}
			database.DB.Create(&image)
			fmt.Printf("Successfully downloaded and saved image: %s\n", imageURL)
		})
	})

	database.DB.Model(&source).Update("Status", "idle")
	return nil
}
