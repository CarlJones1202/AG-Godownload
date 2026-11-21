package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	kittyKatsBaseURL              = "https://kitty-kats.net"
	kittyKatsSearchPagePath       = "/search/"
	kittyKatsSearchSubmitPath     = "/search/search"
	kittyKatsMaxResults           = 50
	kittyKatsMaxThumbnailFetches  = 16
	kittyKatsThumbnailConcurrency = 4
	kittyKatsThumbnailTimeout     = 8 * time.Second
	kittyKatsMaxBodySize          = 10 << 20 // 10 MiB
	kittyKatsUserAgent            = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

var kittyKatsThreadPathRE = regexp.MustCompile(`^/threads/[^/]+\.(\d+)/?$`)

// IsKittyKatsThreadURL accepts canonical https kitty-kats.net thread URLs.
func IsKittyKatsThreadURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host != "kitty-kats.net" && host != "www.kitty-kats.net" {
		return false
	}
	return kittyKatsThreadPathRE.MatchString(u.Path)
}

func kittyKatsThreadID(threadURL string) string {
	u, err := url.Parse(threadURL)
	if err != nil {
		return ""
	}
	matches := kittyKatsThreadPathRE.FindStringSubmatch(u.Path)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// KittyKatsSearchURL is a display link for the site's search form. The actual
// request is a POST to /search/search because XenForo requires a CSRF token.
func KittyKatsSearchURL(query string) string {
	params := url.Values{}
	params.Set("q", query)
	params.Set("o", "relevance")
	return kittyKatsBaseURL + kittyKatsSearchPagePath + "?" + params.Encode()
}

func newKittyKatsClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	base := GetHTTPClient(kittyKatsBaseURL + "/")
	return &http.Client{
		Transport: base.Transport,
		Timeout:   kittyKatsSearchTimeout,
		Jar:       jar,
	}
}

func kittyKatsHeaders(req *http.Request) {
	req.Header.Set("User-Agent", kittyKatsUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
}

func canonicalKittyKatsURL(base *url.URL, raw string) (string, error) {
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	u := base.ResolveReference(ref)
	u.RawQuery = ""
	u.Fragment = ""
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	return u.String(), nil
}

func normalizeForumText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// ParseKittyKatsSearch extracts thread cards from a XenForo search results page.
func ParseKittyKatsSearch(reader io.Reader, base *url.URL) ([]ForumSearchResult, error) {
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, fmt.Errorf("parse kitty-kats search page: %w", err)
	}

	results := make([]ForumSearchResult, 0)
	seen := make(map[string]bool)
	doc.Find("li.block-row").Each(func(_ int, row *goquery.Selection) {
		if len(results) >= kittyKatsMaxResults {
			return
		}

		titleLink := row.Find("h3.contentRow-title > a").First()
		if titleLink.Length() == 0 {
			return
		}
		title := normalizeForumText(titleLink.Text())
		href, ok := titleLink.Attr("href")
		if title == "" || !ok || strings.TrimSpace(href) == "" {
			return
		}

		threadURL, err := canonicalKittyKatsURL(base, href)
		if err != nil || !IsKittyKatsThreadURL(threadURL) || seen[threadURL] {
			return
		}
		subForum := normalizeForumText(row.Find(`.contentRow-minor a[href*="/forums/"]`).First().Text())
		if !isPhotoForum(subForum) {
			return
		}
		seen[threadURL] = true

		results = append(results, ForumSearchResult{
			Site:      ForumSiteKittyKats,
			ThreadID:  kittyKatsThreadID(threadURL),
			Title:     title,
			SubForum:  subForum,
			ThreadURL: threadURL,
		})
	})

	return results, nil
}

func fetchKittyKatsToken(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kittyKatsBaseURL+kittyKatsSearchPagePath, nil)
	if err != nil {
		return "", err
	}
	kittyKatsHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kitty-kats search form returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, kittyKatsMaxBodySize))
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(doc.Find(`input[name="_xfToken"]`).First().AttrOr("value", ""))
	if token == "" {
		return "", fmt.Errorf("kitty-kats search form did not provide a CSRF token")
	}
	return token, nil
}

func fetchKittyKatsThumbnail(ctx context.Context, client *http.Client, threadURL string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, threadURL, nil)
	if err != nil {
		return ""
	}
	kittyKatsHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, kittyKatsMaxBodySize))
	if err != nil {
		return ""
	}
	base, err := url.Parse(threadURL)
	if err != nil {
		return ""
	}
	image := doc.Find("article.message--post .message-userContent .bbWrapper a img").
		FilterFunction(func(_ int, image *goquery.Selection) bool {
			return strings.TrimSpace(image.AttrOr("alt", "")) != "View Post"
		}).First()
	if image.Length() == 0 {
		return ""
	}
	src := strings.TrimSpace(image.AttrOr("data-url", ""))
	if src == "" {
		src = strings.TrimSpace(image.AttrOr("src", ""))
	}
	if src == "" {
		return ""
	}
	absolute, err := base.Parse(src)
	if err != nil || !isAbsoluteHTTPURL(absolute.String()) {
		return ""
	}
	return absolute.String()
}

// fillKittyKatsThumbnails makes at most one bounded request per visible result.
// Results beyond the grid's first page are left without a thumbnail.
func fillKittyKatsThumbnails(ctx context.Context, client *http.Client, results []ForumSearchResult) {
	limit := len(results)
	if limit > kittyKatsMaxThumbnailFetches {
		limit = kittyKatsMaxThumbnailFetches
	}

	sem := make(chan struct{}, kittyKatsThumbnailConcurrency)
	var wg sync.WaitGroup
	for i := 0; i < limit; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(index int) {
			defer wg.Done()
			defer func() { <-sem }()
			thumbCtx, cancel := context.WithTimeout(ctx, kittyKatsThumbnailTimeout)
			defer cancel()
			thumbnail := fetchKittyKatsThumbnail(thumbCtx, client, results[index].ThreadURL)
			if thumbnail != "" {
				results[index].ThumbnailURL = thumbnail
			}
		}(i)
	}
	wg.Wait()
}

// SearchKittyKats runs a fresh XenForo search and returns photo-forum threads.
func SearchKittyKats(query string) ([]ForumSearchResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), kittyKatsSearchTimeout)
	defer cancel()
	return SearchKittyKatsWithContext(ctx, query)
}

// SearchKittyKatsWithContext is the context-aware form used by the combined search.
func SearchKittyKatsWithContext(ctx context.Context, query string) ([]ForumSearchResult, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("search query is required")
	}
	if len(q) > vipergirlsMaxQueryLen {
		return nil, fmt.Errorf("search query must be %d characters or fewer", vipergirlsMaxQueryLen)
	}

	client := newKittyKatsClient()

	token, err := fetchKittyKatsToken(ctx, client)
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("keywords", q)
	form.Set("order", "relevance")
	form.Set("search_type", "")
	form.Set("_xfToken", token)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kittyKatsBaseURL+kittyKatsSearchSubmitPath, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	kittyKatsHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kitty-kats search returned status %d", resp.StatusCode)
	}

	base, err := url.Parse(kittyKatsBaseURL + "/")
	if err != nil {
		return nil, err
	}
	results, err := ParseKittyKatsSearch(io.LimitReader(resp.Body, kittyKatsMaxBodySize), base)
	if err != nil {
		return nil, err
	}
	fillKittyKatsThumbnails(ctx, client, results)
	return results, nil
}
