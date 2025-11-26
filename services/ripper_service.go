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

func newCollector() *colly.Collector {
	c := colly.NewCollector()
	c.SetRequestTimeout(60 * time.Second)
	c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0"
	return c
}

func RipImageBam(src string) (string, error) {
	logger.Debugf("Starting RipImageBam for %s", src)
	c := newCollector()
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
	c := newCollector()
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
	resp, err := http.Head(src)
	if err != nil {
		return "", fmt.Errorf("resolving Imx URL %s: %v", src, err)
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()
	imageURL := strings.ReplaceAll(finalURL, "/t/", "/i/")
	logger.Debugf("Transformed Imx.to URL: %s -> %s", src, imageURL)
	return imageURL, nil
}

func RipTurboImg(src string) (string, error) {
	logger.Debugf("Starting RipTurboImg for %s", src)
	c := newCollector()
	c.OnResponse(func(r *colly.Response) {
		logger.Debugf("RipTurboImg response for %s: Status %d", r.Request.URL.String(), r.StatusCode)
	})

	var imageURL string
	c.OnHTML("#uImageCont img", func(e *colly.HTMLElement) {
		imageURL = e.Attr("src")
		logger.Debugf("Extracted TurboImageHost URL: %s", imageURL)
	})
	if err := c.Visit(src); err != nil {
		return "", fmt.Errorf("visiting TurboImageHost %s: %v", src, err)
	}
	return imageURL, nil
}
