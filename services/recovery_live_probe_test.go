package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/models"
	"github.com/disintegration/imaging"
	"github.com/gocolly/colly/v2"
)

func TestProbeLiveImageTwistMarkup(t *testing.T) {
	response, err := http.Get("http://localhost:8080/images?sort=oldest&limit=2000")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Data []struct {
			ID          uint   `json:"id"`
			OriginalURL string `json:"original_url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	var pageURL string
	for _, row := range payload.Data {
		if row.ID == 544 {
			pageURL = row.OriginalURL
			break
		}
	}
	if pageURL == "" {
		t.Fatal("image 544 was not found")
	}

	type result struct {
		label                                                                                                                    string
		bodyBytes, rawMatches, rawNonPosters, matches, posters, nonPosters, preferred, textareas, textID, textCandidates, status int
	}
	var results []result
	for _, mode := range []string{"Firefox91", "Chrome131", "Chrome131BrowserHeaders"} {
		var current result
		current.label = mode
		c := newCollector(pageURL)
		if mode != "Firefox91" {
			c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36"
		}
		if mode == "Chrome131BrowserHeaders" {
			c.OnRequest(func(r *colly.Request) {
				r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
				r.Headers.Set("Accept-Language", "en-US,en;q=0.9")
				r.Headers.Set("Upgrade-Insecure-Requests", "1")
			})
		}
		c.OnResponse(func(r *colly.Response) {
			current.status = r.StatusCode
			current.bodyBytes = len(r.Body)
			body := strings.ToLower(string(r.Body))
			current.rawMatches = strings.Count(body, "dki1gog4mj30")
			for _, match := range regexp.MustCompile(`(?is)<img\b[^>]*dki1gog4mj30[^>]*>`).FindAllString(body, -1) {
				if !strings.Contains(match, "premium_poster") {
					current.rawNonPosters++
				}
			}
		})
		c.OnHTML("img[src]", func(e *colly.HTMLElement) {
			candidate := e.Request.AbsoluteURL(e.Attr("src"))
			if strings.Contains(strings.ToLower(candidate), "dki1gog4mj30") {
				current.matches++
				if strings.Contains(strings.ToLower(candidate), "premium_poster") {
					current.posters++
				} else {
					current.nonPosters++
				}
				if preferredImagetwistImageURL(candidate, pageURL) != "" {
					current.preferred++
				}
			}
		})
		c.OnHTML("textarea", func(e *colly.HTMLElement) {
			current.textareas++
			if strings.Contains(strings.ToLower(e.Text), "dki1gog4mj30") {
				current.textID++
				for _, candidate := range regexp.MustCompile(`(?i)https?://[^"'<>[:space:]]+`).FindAllString(e.Text, -1) {
					if preferredImagetwistImageURL(candidate, pageURL) != "" {
						current.textCandidates++
					}
				}
			}
		})
		if err := c.Visit(pageURL); err != nil {
			t.Fatal(err)
		}
		results = append(results, current)
	}
	hardcodedCandidate := preferredImagetwistImageURL(
		"https://i1.imagetwist.com/i/15619/dki1gog4mj30.jpg",
		pageURL,
	)
	t.Logf("results=%+v helper_accepts_known_shape=%t", results, hardcodedCandidate != "")
}

func TestProbeLiveImageTwist544(t *testing.T) {
	response, err := http.Get("http://localhost:8080/images?sort=oldest&limit=2000")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("image API returned status %d", response.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID          uint   `json:"id"`
			OriginalURL string `json:"original_url"`
			DownloadURL string `json:"download_url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	var image struct {
		OriginalURL string
		DownloadURL string
	}
	for _, row := range payload.Data {
		if row.ID == 544 {
			image.OriginalURL = row.OriginalURL
			image.DownloadURL = row.DownloadURL
			break
		}
	}
	if image.OriginalURL == "" || image.DownloadURL == "" {
		t.Fatal("image 544 was not found in the local API response")
	}

	resolvedURL, err := RipImagetwist(image.OriginalURL)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedURL == image.DownloadURL {
		t.Fatal("ImageTwist resolver returned the stored poster URL")
	}

	oldUploadsDir := UploadsDir
	UploadsDir = t.TempDir()
	defer func() { UploadsDir = oldUploadsDir }()

	result, err := DownloadImage(resolvedURL, "recovery-probe", image.OriginalURL)
	if err != nil {
		t.Fatal(err)
	}
	downloaded, err := imaging.Open(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := filepath.Abs(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(info)
	if err != nil {
		t.Fatal(err)
	}
	if downloaded.Bounds().Dx() <= 177 || stat.Size() <= 8183 {
		t.Fatalf("resolved image still looks like the poster: %dx%d, %d bytes", downloaded.Bounds().Dx(), downloaded.Bounds().Dy(), stat.Size())
	}
	fmt.Printf("ImageTwist 544 live probe: %dx%d, %d bytes\n", downloaded.Bounds().Dx(), downloaded.Bounds().Dy(), stat.Size())
}

func TestProbeRecoverLiveImageTwist544(t *testing.T) {
	config.Load()
	database.Connect(config.Global.DatabaseDSN())
	if err := RecoverImage(544); err != nil {
		t.Fatal(err)
	}

	var row struct {
		Filename    string
		DownloadURL string
		FileExists  bool
	}
	if err := database.DB.Model(&models.Image{}).
		Select("filename, download_url, file_exists").
		Where("id = ?", 544).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if !row.FileExists || strings.Contains(strings.ToLower(row.DownloadURL), "premium_poster") {
		t.Fatalf("recovery did not persist the full image URL and file state")
	}

	imagePath := filepath.Join(UploadsDir, row.Filename)
	info, err := os.Stat(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	downloaded, err := imaging.Open(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= 8183 {
		t.Fatalf("recovered file is still placeholder-sized: %d bytes", info.Size())
	}
	fmt.Printf("ImageTwist 544 recovery: %dx%d, %d bytes, database updated\n",
		downloaded.Bounds().Dx(), downloaded.Bounds().Dy(), info.Size())
}
