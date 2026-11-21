package services

import (
	"context"
	"fmt"
	"gallery_api/logger"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

func newCollector(targetURL string) *colly.Collector {
	c := colly.NewCollector()
	c.SetRequestTimeout(60 * time.Second)
	c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0"

	// Colly discards Set-Cookie headers unless a cookie jar is installed. Hosts
	// such as imagetwist and imx.to require session cookies obtained from a prior
	// page visit, so every collector gets a jar.
	jar, _ := cookiejar.New(nil)
	c.SetCookieJar(jar)

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
	} else {
		// Use shared optimized transport for regular requests
		c.WithTransport(GetGlobalTransport())
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
	release, err := acquireIMXRequest(context.Background())
	if err != nil {
		return "", fmt.Errorf("waiting to resolve IMX page %s: %w", src, err)
	}
	defer release()

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
	c.OnHTML("button[name='imgContinue']", func(e *colly.HTMLElement) {
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
			if strings.Contains(imgSrc, "/i/") || strings.Contains(imgSrc, "/u/i/") {
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
		c2.OnRequest(func(r *colly.Request) {
			r.Headers.Set("Referer", src)
			r.Headers.Set("Origin", "https://imx.to")
		})

		c2.OnHTML("img", func(e *colly.HTMLElement) {
			imgSrc := e.Attr("src")
			if strings.Contains(imgSrc, "icon") || strings.Contains(imgSrc, "logo") {
				return
			}

			if strings.Contains(imgSrc, "imx.to") || strings.Contains(imgSrc, "i.imx.to") {
				if strings.Contains(imgSrc, "/i/") || strings.Contains(imgSrc, "/u/i/") {
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
			if strings.HasPrefix(imageURL, "//") {
				imageURL = "https:" + imageURL
			} else {
				parsed, parseErr := url.Parse(src)
				if parseErr == nil {
					imageURL = parsed.ResolveReference(&url.URL{Path: imageURL}).String()
				}
			}
		}
		return imageURL, nil
	}
	return "", fmt.Errorf("could not extract an IMX image URL after visiting %s", src)
}

func RipTurboImg(src string) (string, error) {
	logger.Debugf("Starting RipTurboImg for %s", src)
	c := newCollector(src)
	// TurboImageHost returns 403 Forbidden for page visits without a Referer
	// header (hotlink/bot protection), regardless of User-Agent. A forum or
	// self-referer both pass.
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Referer", src)
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9")
	})
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

	setImageURL := func(e *colly.HTMLElement) {
		if imageURL != "" {
			return
		}
		candidate := preferredImagetwistImageURL(e.Request.AbsoluteURL(e.Attr("src")), src)
		if candidate != "" {
			imageURL = candidate
			logger.Debugf("Found Imagetwist image: %s", imageURL)
		}
	}

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
	c.OnHTML("img.pic", setImageURL)

	// Fallback image selector
	c.OnHTML("img.img-responsive", setImageURL)
	c.OnHTML("img[src]", setImageURL)

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

		c2.OnHTML("img.pic", setImageURL)
		c2.OnHTML("img.img-responsive", setImageURL)
		c2.OnHTML("img[src]", setImageURL)

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

func preferredImagetwistImageURL(candidateURL, pageURL string) string {
	candidate, err := url.Parse(candidateURL)
	if err != nil || candidate.Hostname() == "" {
		return ""
	}
	page, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(candidate.Hostname())
	pageHost := strings.ToLower(page.Hostname())
	if host != pageHost && !strings.HasSuffix(host, ".imagetwist.com") {
		return ""
	}

	candidatePath := strings.ToLower(candidate.Path)
	if strings.Contains(candidatePath, "premium_poster") {
		return ""
	}
	switch strings.ToLower(path.Ext(candidate.Path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
	default:
		return ""
	}

	imageID := imagetwistPageImageID(page)
	if imageID != "" && !strings.Contains(candidatePath, strings.ToLower(imageID)) {
		return ""
	}
	return candidate.String()
}

func imagetwistPageImageID(page *url.URL) string {
	segments := strings.Split(strings.Trim(page.Path, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		return ""
	}
	index := 0
	if strings.EqualFold(segments[0], "i") && len(segments) > 1 {
		index = 1
	}
	return strings.TrimSuffix(segments[index], path.Ext(segments[index]))
}

func RipMyMyPic(src string) (string, error) {
	logger.Debugf("RipMyMyPic returning direct URL: %s", src)
	// Typically mymypic.net links in JKF are either direct or need simple protocol fix
	if strings.HasPrefix(src, "//") {
		return "https:" + src, nil
	}
	return src, nil
}

// visitPageForCookies visits the given page URL with a Colly collector and returns
// the session cookies obtained during the visit (including those set via redirects).
func visitPageForCookies(pageURL string) ([]*http.Cookie, error) {
	c := newCollector(pageURL)
	// Colly does NOT initialize a cookie jar by default — without this, any
	// Set-Cookie headers from the server are silently discarded.
	jar, _ := cookiejar.New(nil)
	c.SetCookieJar(jar)
	if err := c.Visit(pageURL); err != nil {
		return nil, err
	}
	return c.Cookies(pageURL), nil
}
