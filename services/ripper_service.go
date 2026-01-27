package services

import (
	"fmt"
	"gallery_api/logger"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

func newCollector(targetURL string) *colly.Collector {
	c := colly.NewCollector()
	c.SetRequestTimeout(60 * time.Second)
	c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0"

	// Use WireGuard for blocked domains
	if ShouldUseWireGuard(targetURL) {
		dialer, err := GetWireGuardDialer()
		if err == nil {
			logger.Debugf("Using WireGuard tunnel for Colly request to: %s", targetURL)
			c.WithTransport(&http.Transport{
				DialContext: dialer.DialContext,
			})
		} else {
			logger.Warnf("WireGuard not available, using direct connection: %v", err)
		}
	}

	return c
}

func RipImageBam(src string) (string, error) {
	logger.Debugf("Starting RipImageBam for %s", src)
	c := newCollector(src)
	cookieJar, _ := cookiejar.New(nil)
	cookies := []*http.Cookie{
		{Name: "nsfw_inter", Value: "1", Path: "/", Domain: "imagebam.com"},
		{Name: "expires", Value: time.Now().AddDate(0, 0, 1).String(), Path: "/", Domain: "imagebam.com"},
	}
	targetURL, _ := url.Parse("https://imagebam.com")
	cookieJar.SetCookies(targetURL, cookies)
	c.SetCookieJar(cookieJar)

	c.OnResponse(func(r *colly.Response) {
		logger.Debugf("RipImageBam response for %s: Status %d", r.Request.URL.String(), r.StatusCode)
	})

	var imageURL string
	c.OnHTML("img.main-image", func(e *colly.HTMLElement) {
		imageURL = e.Attr("src")
		logger.Debugf("Extracted ImageBam URL: %s", imageURL)
	})

	if err := c.Visit(src); err != nil {
		return "", fmt.Errorf("visiting ImageBam %s: %v", src, err)
	}
	return imageURL, nil
}

func RipImageBox(src string) (string, error) {
	logger.Debugf("Starting RipImageBox for %s", src)
	c := newCollector(src)
	c.OnResponse(func(r *colly.Response) {
		logger.Debugf("RipImageBox response for %s: Status %d", r.Request.URL.String(), r.StatusCode)
	})

	var imageURL string
	c.OnHTML("#img", func(e *colly.HTMLElement) {
		imageURL = e.Attr("src")
		logger.Debugf("Extracted ImgBox URL: %s", imageURL)
	})
	if err := c.Visit(src); err != nil {
		return "", fmt.Errorf("visiting ImgBox %s: %v", src, err)
	}
	return imageURL, nil
}

func RipPostImages(src string) (string, error) {
	logger.Debugf("RipPostImages returning direct URL: %s", src)
	return src, nil
}

func RipViprIm(src string) (string, error) {
	logger.Debugf("Starting RipViprIm for %s", src)
	imageURL := strings.ReplaceAll(src, "/th", "/i")
	logger.Debugf("Transformed Vipr.im URL: %s", imageURL)
	return imageURL, nil
}

func RipAcidImg(src string) (string, error) {
	logger.Debugf("Starting RipAcidImg for %s", src)
	imageURL := strings.ReplaceAll(src, "t.", "i.")
	imageURL = strings.ReplaceAll(imageURL, "/t", "/i")
	logger.Debugf("Transformed AcidImg URL: %s", imageURL)
	return imageURL, nil
}

func RipPixHost(src string) (string, error) {
	logger.Debugf("Starting RipPixHost for %s", src)
	imageURL := strings.ReplaceAll(src, "/thumbs", "/images")
	imageURL = strings.ReplaceAll(imageURL, "https://t", "https://img")
	logger.Debugf("Transformed PixHost URL: %s", imageURL)
	return imageURL, nil
}

func RipImx(src string) (string, error) {
	logger.Debugf("Starting RipImx for %s", src)

	// imx.to structure:
	// 1. Anchor href points to the image page (e.g., https://imx.to/i/abc123)
	// 2. That page MIGHT have a "Continue to your image..." button (POST form)
	// 3. The final page contains an <img> tag with the actual image URL

	c := newCollector(src)
	c.AllowURLRevisit = true

	var imageURL string
	var needsContinue bool
	var continueName string
	var continueValue string

	// 1. Check for "Continue" button
	c.OnHTML("input[name='imgContinue']", func(e *colly.HTMLElement) {
		needsContinue = true
		continueName = "imgContinue"
		continueValue = e.Attr("value")
		logger.Debug("Found imx.to continue button")
	})

	// 2. Look for the main image
	c.OnHTML("img", func(e *colly.HTMLElement) {
		imgSrc := e.Attr("src")

		// Skip small images, icons, logos
		if strings.Contains(imgSrc, "icon") || strings.Contains(imgSrc, "logo") ||
			strings.Contains(imgSrc, "avatar") || strings.Contains(imgSrc, "thumb") {
			return
		}

		// Look for images hosted on imx.to CDN
		if strings.Contains(imgSrc, "imx.to") || strings.Contains(imgSrc, "i.imx.to") {
			if strings.Contains(imgSrc, "/i/") {
				imageURL = imgSrc
				logger.Debugf("Found full-size image: %s", imgSrc)
			} else if imageURL == "" {
				imageURL = imgSrc
				logger.Debugf("Found image (main): %s", imgSrc)
			}
		}
	})

	// Visit the initial page
	if err := c.Visit(src); err != nil {
		return "", fmt.Errorf("visiting imx.to page %s: %v", src, err)
	}

	// If we found a continue button, we need to POST to the same URL
	if needsContinue && imageURL == "" {
		logger.Debug("Submitting continue form...")

		// Create a new collector for the POST request to avoid state issues
		c2 := newCollector(src)
		c2.AllowURLRevisit = true

		// Important: Cookies must be preserved from the first request
		c2.SetCookies(src, c.Cookies(src))

		c2.OnHTML("img", func(e *colly.HTMLElement) {
			imgSrc := e.Attr("src")
			if strings.Contains(imgSrc, "icon") || strings.Contains(imgSrc, "logo") {
				return
			}

			if strings.Contains(imgSrc, "imx.to") || strings.Contains(imgSrc, "i.imx.to") {
				if strings.Contains(imgSrc, "/i/") {
					imageURL = imgSrc
					logger.Debugf("Found full-size image after POST: %s", imgSrc)
				} else if imageURL == "" {
					imageURL = imgSrc
				}
			}
		})

		params := map[string]string{
			continueName: continueValue,
		}

		if err := c2.Post(src, params); err != nil {
			logger.Errorf("Failed to submit continue form: %v", err)
		}
	}

	if imageURL != "" {
		// Make sure it's an absolute URL
		if !strings.HasPrefix(imageURL, "http") {
			imageURL = "https:" + imageURL
		}
		return imageURL, nil
	}

	// Fallback: try HEAD request
	logger.Debug("No image found, trying HEAD request fallback")
	resp, err := http.Head(src)
	if err != nil {
		return "", fmt.Errorf("resolving Imx URL %s: %v", src, err)
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()
	imageURL = strings.ReplaceAll(finalURL, "/t/", "/i/")
	logger.Debugf("Transformed Imx.to URL: %s -> %s", src, imageURL)
	return imageURL, nil
}

func RipTurboImg(src string) (string, error) {
	logger.Debugf("Starting RipTurboImg for %s", src)
	c := newCollector(src)
	c.OnResponse(func(r *colly.Response) {
		logger.Debugf("RipTurboImg response for %s: Status %d", r.Request.URL.String(), r.StatusCode)
	})

	var imageURL string
	c.OnHTML("#img", func(e *colly.HTMLElement) {
		imageURL = e.Attr("src")
		logger.Debugf("Extracted TurboImageHost URL: %s", imageURL)
	})
	// Fallback for TurboImageHost
	if imageURL == "" {
		c.OnHTML("#uImageCont img", func(e *colly.HTMLElement) {
			imageURL = e.Attr("src")
			logger.Debugf("Extracted TurboImageHost URL (fallback): %s", imageURL)
		})
	}
	if err := c.Visit(src); err != nil {
		return "", fmt.Errorf("visiting TurboImageHost %s: %v", src, err)
	}
	return imageURL, nil
}

func RipImagetwist(src string) (string, error) {
	logger.Debugf("Starting RipImagetwist for %s", src)

	// imagetwist structure (similar to imx.to):
	// 1. Anchor href points to the image page
	// 2. That page MIGHT have a "Continue to your image..." button (POST form)
	// 3. The final page contains an <img> tag with the actual image URL

	c := newCollector(src)
	c.AllowURLRevisit = true

	var imageURL string
	var needsContinue bool
	var continueName string
	var continueValue string

	// 1. Check for "Continue" button - usually input type="submit" in a form
	c.OnHTML("form", func(e *colly.HTMLElement) {
		// Often checks if it's posting to the same page or has specific inputs
		if e.ChildAttr("input[type='submit']", "value") != "" {
			// Basic heuristic: if there's a submit button on an image host, it's likely an interstitial
			// But we need to be careful. Let's look for specific hidden inputs often used.
			// Imagetwist often uses names like 'ad_form_data' or just a generic continue.
			// Let's just grab the inputs we can find.
		}
	})

	// More specific check for the continue button often found on these sites
	c.OnHTML("input.btn-success", func(e *colly.HTMLElement) {
		// Verify if this looks like a continue button
		val := e.Attr("value")
		if strings.Contains(strings.ToLower(val), "continue") || strings.Contains(strings.ToLower(val), "image") {
			// This is likely it. We need the form fields.
			needsContinue = true
		}
	})

	// Also check for name="imgContinue" like imx.to, just in case
	c.OnHTML("input[name='imgContinue']", func(e *colly.HTMLElement) {
		needsContinue = true
		continueName = "imgContinue"
		continueValue = e.Attr("value")
		logger.Debug("Found Imagetwist continue button (imgContinue style)")
	})

	// Check for a generic form with a "Continue" submit button if specific classes fail
	c.OnHTML("form", func(e *colly.HTMLElement) {
		if needsContinue {
			return
		}
		submitVal := e.ChildAttr("input[type='submit']", "value")
		if strings.Contains(strings.ToLower(submitVal), "continue") {
			needsContinue = true
			logger.Debug("Found Imagetwist generic continue form")
		}
	})

	// 2. Look for the main image
	c.OnHTML("img.pic", func(e *colly.HTMLElement) {
		imageURL = e.Attr("src")
		logger.Debugf("Found Imagetwist image (img.pic): %s", imageURL)
	})

	// Fallback image selector
	c.OnHTML("img.img-responsive", func(e *colly.HTMLElement) {
		if imageURL == "" && !strings.Contains(e.Attr("src"), "logo") {
			imageURL = e.Attr("src")
			logger.Debugf("Found Imagetwist image (img.img-responsive): %s", imageURL)
		}
	})

	// Visit the initial page
	if err := c.Visit(src); err != nil {
		return "", fmt.Errorf("visiting Imagetwist page %s: %v", src, err)
	}

	// If we found a continue button/form and no image, we need to POST
	if needsContinue && imageURL == "" {
		logger.Debug("Submitting Imagetwist continue form...")

		c2 := newCollector(src)
		c2.AllowURLRevisit = true
		c2.SetCookies(src, c.Cookies(src))

		c2.OnHTML("img.pic", func(e *colly.HTMLElement) {
			imageURL = e.Attr("src")
			logger.Debugf("Found Imagetwist image after POST: %s", imageURL)
		})

		c2.OnHTML("img.img-responsive", func(e *colly.HTMLElement) {
			if imageURL == "" && !strings.Contains(e.Attr("src"), "logo") {
				imageURL = e.Attr("src")
				logger.Debugf("Found Imagetwist image after POST (fallback): %s", imageURL)
			}
		})

		// For Imagetwist and clones, usually we just need to hit the same URL with the same cookies
		// and mostly any POST data that was in the form.
		// Since extracting *all* form data generically is complex, we'll try a common pattern
		// or just a simple POST if the site allows it.
		// However, many of these just check for the cookie set by the first visit + a POST.

		// Let's try mimicking the 'ad_form_data' or generic post structure if we can't find specific inputs.
		// Actually, simpler approach: Most of these sites set a cookie on first visit,
		// then expect a POST (often empty or with specific token) to show image.
		// Let's try posting with the extracted specific param if found (imgContinue), or empty if generic.

		params := map[string]string{}
		if continueName != "" {
			params[continueName] = continueValue
		} else {
			// Try to add a generic one often used
			params["imgContinue"] = "Continue to image ..."
		}

		if err := c2.Post(src, params); err != nil {
			logger.Errorf("Failed to submit Imagetwist continue form: %v", err)
		}
	}

	if imageURL != "" {
		if !strings.HasPrefix(imageURL, "http") {
			imageURL = "https:" + imageURL // Most likely https, unlikely relative root without protocol but possible
		}
		return imageURL, nil
	}

	return "", fmt.Errorf("failed to extract Imagetwist image from %s", src)
}
func RipMyMyPic(src string) (string, error) {
	logger.Debugf("RipMyMyPic returning direct URL: %s", src)
	// Typically mymypic.net links in JKF are either direct or need simple protocol fix
	if strings.HasPrefix(src, "//") {
		return "https:" + src, nil
	}
	return src, nil
}
