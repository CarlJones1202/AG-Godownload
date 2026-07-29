package handlers

import (
	"encoding/json"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
)

func CreateSource(c *gin.Context) {
	var source models.Source
	if err := c.ShouldBindJSON(&source); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := createSingleSource(source.Name, source.Location, source.Type, source.Priority)
	if err != nil {
		c.JSON(err.Code, gin.H{"error": err.Message})
		return
	}

	c.JSON(http.StatusCreated, created)
}

type importError struct {
	Code    int
	Message string
}

var reFileExt = regexp.MustCompile(`\.(html?|php|asp|aspx|jsp|shtml|cfm|jpe?g|png|gif|bmp|webp|mp4|avi|mkv)$`)

// knownModelNames is populated lazily from the people table
var knownModelNames map[string]int
var knownModelNamesOnce sync.Once

// knownSurnameWords — second words of known 2-word model names (lowercase)
var knownSurnameWords = map[string]bool{
	"chey": true, "moss": true, "may": true, "ocean": true,
	"constance": true, "nekrasova": true, "isizzu": true,
	"k": true, "nass": true,
}

func loadKnownModelNames() {
	knownModelNames = make(map[string]int)
	var people []models.Person
	if err := database.DB.Find(&people).Error; err != nil {
		return
	}
	for _, p := range people {
		words := strings.Fields(strings.ToLower(strings.TrimSpace(p.Name)))
		if len(words) >= 1 && len(words) <= 2 {
			knownModelNames[strings.Join(words, " ")] = len(words)
		}
	}
}

func guessNameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) == 0 {
		return ""
	}

	raw := segments[len(segments)-1]
	raw, _ = url.QueryUnescape(raw)
	raw = reFileExt.ReplaceAllString(raw, "")
	raw = strings.SplitN(raw, "?", 2)[0]
	raw = strings.SplitN(raw, "#", 2)[0]

	title := strings.TrimSpace(regexp.MustCompile(`[-_+]+`).ReplaceAllString(raw, " "))
	if len(title) < 2 {
		return ""
	}

	words := strings.Fields(title)

	// Strip trailing .digits from any word before metadata stripping
	for i, w := range words {
		if dotIdx := strings.LastIndex(w, "."); dotIdx > 0 {
			suffix := w[dotIdx+1:]
			if isAllDigits(suffix) {
				words[i] = w[:dotIdx]
			}
		}
	}

	// 1. Strip leading numeric (thread ID)
	for len(words) > 0 && isAllDigits(words[0]) {
		words = words[1:]
	}
	if len(words) == 0 {
		return ""
	}

	// 2. Strip leading dates
	words = stripLeadingDates(words)
	if len(words) == 0 {
		return ""
	}

	// 3. Strip trailing metadata aggressively
	words = stripTrailingMeta(words)
	if len(words) == 0 {
		return ""
	}

	// Strip "highlight" and following words
	for i, w := range words {
		if strings.EqualFold(w, "highlight") {
			words = words[:i]
			break
		}
	}
	if len(words) == 0 {
		return ""
	}

	// 4. Strip leading site prefixes
	words = stripSitePrefix(words)
	if len(words) == 0 {
		return ""
	}

	// 5. Strip model name + optional connector, leaving the title
	words = stripModelAndConnector(words)
	if len(words) == 0 {
		return ""
	}

	// 5b. Strip trailing metadata again (some was hidden behind model words)
	words = stripTrailingMeta(words)
	if len(words) == 0 {
		return ""
	}

	// 6. Handle apostrophes
	for i, w := range words {
		if strings.Contains(w, "'") {
			parts := strings.SplitN(w, "'", 2)
			if len(parts[1]) == 1 || len(parts[1]) == 2 {
				words[i] = parts[0] + "'" + parts[1]
			}
		}
	}

	// 7. Strip trailing non-alphanumeric characters from each word (punctuation)
	for i, w := range words {
		words[i] = strings.TrimRight(w, "!?,;.:-")
	}

	// 8. Remove any words that are purely non-alphanumeric (like en dash)
	cleaned := make([]string, 0, len(words))
	for _, w := range words {
		hasLetterOrDigit := false
		for _, r := range w {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				hasLetterOrDigit = true
				break
			}
		}
		if hasLetterOrDigit {
			cleaned = append(cleaned, w)
		}
	}
	words = cleaned
	if len(words) == 0 {
		return ""
	}

	// 9. Title case
	return formatTitle(words)
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isMonth(s string) bool {
	lower := strings.ToLower(s)
	months := []string{"january", "february", "march", "april", "may", "june", "july", "august", "september", "october", "november", "december",
		"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}
	for _, m := range months {
		if lower == m {
			return true
		}
	}
	return false
}

func isConnectorWord(s string) bool {
	lower := strings.ToLower(s)
	connectors := []string{"in", "a", "an", "the"}
	for _, c := range connectors {
		if lower == c {
			return true
		}
	}
	return false
}

func isImageDesc(s string) bool {
	lower := strings.ToLower(s)
	desc := []string{"pictures", "pictures,", "photos", "photos,", "pix", "pics", "images", "files", "jpg", "jpeg", "set", "picture", "photo"}
	for _, d := range desc {
		if lower == d {
			return true
		}
	}
	return false
}

func sitePrefixes() [][]string {
	return [][]string{
		{"playboyplus", "com"},
		{"playboy", "com"},
		{"metartx", "com"},
		{"metart", "com"},
		{"sexart", "com"},
		{"vivthomas", "com"},
		{"wowgirls", "com"},
		{"rylskyart", "com"},
		{"eternaldesire", "com"},
		{"mplstudios", "com"},
		{"lifeerotic", "com"},
	}
}

func isMetaWord(s string) bool {
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "x") && isAllDigits(s[1:]) {
		return true
	}
	if matched, _ := regexp.MatchString(`^\d+px$`, lower); matched {
		return true
	}
	if matched, _ := regexp.MatchString(`^\d+[x×]\d+$`, lower); matched {
		return true
	}
	if matched, _ := regexp.MatchString(`^\d+[x×]\d+px$`, lower); matched {
		return true
	}
	if matched, _ := regexp.MatchString(`^\d+x$`, lower); matched {
		return true
	}
	if matched, _ := regexp.MatchString(`^x\d+px?$`, lower); matched {
		return true
	}
	if isMonth(s) {
		return true
	}
	return false
}

func isTrailingCount(s string) bool {
	if isAllDigits(s) {
		if n, err := strconv.Atoi(s); err == nil && n >= 100 {
			return true
		}
	}
	return false
}

func isExtraTag(s string) bool {
	lower := strings.ToLower(s)
	tags := []string{"pre-release", "prerelease", "pre", "hi-res", "hires", "hi", "res", "highlight", "release"}
	for _, t := range tags {
		if lower == t {
			return true
		}
	}
	return false
}

func stripLeadingDates(words []string) []string {
	// YYYY MM DD
	for len(words) >= 3 && isAllDigits(words[0]) && isAllDigits(words[1]) && isAllDigits(words[2]) {
		words = words[3:]
	}
	// YYYY Mon DD
	for len(words) >= 3 && isAllDigits(words[0]) && isMonth(words[1]) && isAllDigits(words[2]) {
		words = words[3:]
	}
	// YYYY Mon
	for len(words) >= 2 && isAllDigits(words[0]) && isMonth(words[1]) {
		words = words[2:]
	}
	// DD Mon YYYY
	for len(words) >= 3 && isAllDigits(words[0]) && isMonth(words[1]) && isAllDigits(words[2]) {
		words = words[3:]
	}
	// Mon DD, YYYY
	if len(words) >= 3 && isMonth(words[0]) && isAllDigits(words[1]) {
		words = words[3:]
	}
	// Single year token left over
	for len(words) > 0 && len(words[0]) == 4 && isAllDigits(words[0]) {
		words = words[1:]
	}
	// DD-MM-YY or similar numeric at start
	for len(words) >= 3 && isAllDigits(words[0]) && isAllDigits(words[1]) && isAllDigits(words[2]) {
		words = words[3:]
	}
	return words
}

func stripTrailingMeta(words []string) []string {
	for {
		n := len(words)
		if n == 0 {
			break
		}
		changed := false

		// Remove trailing single metadata token
		if isMetaWord(words[n-1]) {
			words = words[:n-1]
			changed = true
			continue
		}

		// Parenthesized single token at end
		last := words[n-1]
		if strings.HasPrefix(last, "(") && strings.HasSuffix(last, ")") {
			words = words[:n-1]
			changed = true
			continue
		}

		// Parenthesized 3-word group at end: ( word word word )
		if n >= 3 && strings.HasPrefix(words[n-3], "(") && strings.HasSuffix(words[n-1], ")") {
			words = words[:n-3]
			changed = true
			continue
		}

		// Parenthesized 2-word group at end: ( word word )
		if n >= 2 && strings.HasPrefix(words[n-2], "(") && strings.HasSuffix(words[n-1], ")") {
			words = words[:n-2]
			changed = true
			continue
		}

		// "number pictures/photos/pix" pair
		if n >= 2 && isAllDigits(words[n-2]) && isImageDesc(words[n-1]) {
			words = words[:n-2]
			changed = true
			continue
		}

		// "x 120"
		if n >= 2 && strings.EqualFold(words[n-2], "x") && isAllDigits(words[n-1]) {
			words = words[:n-2]
			changed = true
			continue
		}

		// Date at end: Mon DD YYYY or DD Mon YYYY
		if n >= 3 && isMonth(words[n-3]) && isAllDigits(words[n-2]) && isAllDigits(words[n-1]) {
			words = words[:n-3]
			changed = true
			continue
		}
		if n >= 3 && isAllDigits(words[n-3]) && isMonth(words[n-2]) && isAllDigits(words[n-1]) {
			words = words[:n-3]
			changed = true
			continue
		}

		// "Mon DD" or "DD Mon" at end
		if n >= 2 && isMonth(words[n-2]) && isAllDigits(words[n-1]) {
			words = words[:n-2]
			changed = true
			continue
		}
		if n >= 2 && isAllDigits(words[n-2]) && isMonth(words[n-1]) {
			words = words[:n-2]
			changed = true
			continue
		}

		// 2-digit year date at end: MM DD YY (first word must be 1-12)
		if n >= 3 && isAllDigits(words[n-3]) && isAllDigits(words[n-2]) && isAllDigits(words[n-1]) && len(words[n-1]) <= 2 {
			if first, err := strconv.Atoi(words[n-3]); err == nil && first >= 1 && first <= 12 {
				words = words[:n-3]
				changed = true
				continue
			}
		}

		// Trailing DD MM or MM DD pair (both plausibly 1-31, last ≤ 2 digits)
		if n >= 2 && isAllDigits(words[n-2]) && isAllDigits(words[n-1]) && len(words[n-1]) <= 2 {
			a, _ := strconv.Atoi(words[n-2])
			b, _ := strconv.Atoi(words[n-1])
			if (a >= 1 && a <= 31 && b >= 1 && b <= 31) && (a <= 12 || b <= 12) {
				words = words[:n-2]
				changed = true
				continue
			}
		}

		// YYYY MM (4-digit year + 1-2 digit month)
		if n >= 2 && len(words[n-2]) == 4 && isAllDigits(words[n-2]) && isAllDigits(words[n-1]) && len(words[n-1]) <= 2 {
			if yr, err := strconv.Atoi(words[n-2]); err == nil && yr >= 1900 && yr <= 2099 {
				words = words[:n-2]
				changed = true
				continue
			}
		}

		// All numeric date suffix: YYYY MM DD (plausible year first) or DD MM YYYY (plausible year last)
		if n >= 3 && isAllDigits(words[n-3]) && isAllDigits(words[n-2]) && isAllDigits(words[n-1]) {
			if yr, err := strconv.Atoi(words[n-3]); err == nil && yr >= 1900 && yr <= 2099 {
				words = words[:n-3]
				changed = true
				continue
			}
			if yr, err := strconv.Atoi(words[n-1]); err == nil && yr >= 1900 && yr <= 2099 {
				words = words[:n-3]
				changed = true
				continue
			}
		}

		// Single 4-digit year (plausible range)
		if n >= 1 && len(words[n-1]) == 4 && isAllDigits(words[n-1]) {
			if yr, err := strconv.Atoi(words[n-1]); err == nil && yr >= 1900 && yr <= 2099 {
				words = words[:n-1]
				changed = true
				continue
			}
		}

		// trailing count (all-digits >= 100) — only if it's not the last meaningful word
		if n >= 1 && isTrailingCount(words[n-1]) {
			if n-1 >= 2 {
				words = words[:n-1]
				changed = true
				continue
			}
		}

		// extra tags
		if n >= 1 && isExtraTag(words[n-1]) {
			words = words[:n-1]
			changed = true
			continue
		}

		// resolution at end: 3840x5760px or 3840x5760
		if n >= 1 {
			if matched, _ := regexp.MatchString(`^\d+[x×]\d+px?$`, words[n-1]); matched {
				words = words[:n-1]
				changed = true
				continue
			}
		}

		if !changed {
			break
		}
	}
	return words
}

func stripSitePrefix(words []string) []string {
	for _, prefix := range sitePrefixes() {
		if len(words) >= len(prefix) {
			match := true
			for i, p := range prefix {
				if !strings.EqualFold(words[i], p) {
					match = false
					break
				}
			}
			if match {
				words = words[len(prefix):]
				break
			}
		}
	}
	return words
}

func detectModelWords(words []string) int {
	// Try known model names first (case-insensitive)
	knownModelNamesOnce.Do(loadKnownModelNames)

	if len(words) >= 2 {
		two := strings.ToLower(words[0] + " " + words[1])
		if n, ok := knownModelNames[two]; ok {
			return n
		}
	}
	if len(words) >= 1 {
		one := strings.ToLower(words[0])
		if n, ok := knownModelNames[one]; ok {
			return n
		}
	}

	// No known model: surname heuristic
	if len(words) >= 2 && knownSurnameWords[strings.ToLower(words[1])] {
		return 2
	}

	// If the first word repeats later → model is 1 word
	for i := 1; i < len(words); i++ {
		if strings.EqualFold(words[i], words[0]) {
			return 1
		}
	}
	// If first 2 words repeat together later → model is 2 words
	if len(words) >= 4 {
		for i := 2; i < len(words)-1; i++ {
			if strings.EqualFold(words[i]+" "+words[i+1], words[0]+" "+words[1]) {
				return 2
			}
		}
	}

	// Default: check if stripping 2 gives a reasonable title
	if len(words) == 2 {
		return 1
	}
	if len(words) <= 2 {
		return len(words)
	}

	// Pick the stripping that gives at least 2 words, preferring 2-word model
	if len(words[1:]) >= 2 && len(words[2:]) >= 2 {
		return 2
	}
	if len(words[2:]) < 2 && len(words[1:]) >= 2 {
		return 1
	}
	return 2
}

func stripModelAndConnector(words []string) []string {
	if len(words) == 0 {
		return words
	}

	n := detectModelWords(words)
	if n >= len(words) {
		return nil
	}

	if n < len(words) && isConnectorWord(words[n]) && words[n] == strings.ToLower(words[n]) {
		n++
	}

	if n >= len(words) {
		return nil
	}
	return words[n:]
}

func formatTitle(words []string) string {
	lowerExceptions := map[string]bool{
		"a": true, "an": true, "the": true,
		"in": true, "of": true, "for": true, "and": true, "or": true,
		"to": true, "on": true, "at": true, "by": true, "with": true,
		"from": true, "into": true, "onto": true, "upon": true,
	}

	result := ""
	for i, w := range words {
		if i > 0 {
			result += " "
		}
		lower := strings.ToLower(w)
		if i > 0 && lowerExceptions[lower] {
			result += lower
		} else if len(w) > 0 {
			runes := []rune(w)
			runes[0] = unicode.ToUpper(runes[0])
			result += string(runes)
		}
	}
	return result
}

func GuessName(c *gin.Context) {
	location := c.Query("url")
	if location == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url query parameter is required"})
		return
	}
	name := guessNameFromURL(location)
	c.JSON(http.StatusOK, gin.H{"name": name, "location": location})
}

func createSingleSource(name, location, sourceType string, priority int) (*models.Source, *importError) {
	if name == "" {
		name = guessNameFromURL(location)
	}
	source := models.Source{
		Name:     name,
		Type:     sourceType,
		Location: location,
		Priority: priority,
		Status:   "idle",
	}

	// Default type if not set
	if source.Type == "" {
		source.Type = "url"
	}

	// Check for duplicate location
	var existing models.Source
	if err := database.DB.Where("location = ?", source.Location).First(&existing).Error; err == nil {
		return nil, &importError{http.StatusConflict, "A source with this URL already exists"}
	}

	if err := database.DB.Create(&source).Error; err != nil {
		return nil, &importError{http.StatusInternalServerError, "Failed to create source"}
	}

	// Only create a gallery for non-video sources (videos are stored standalone)
	isVideo := services.IsVideoURL(source.Location) || services.IsVideoFile(source.Location)
	if !isVideo {
		// Automatically create a gallery for this source
		gallery := models.Gallery{
			Name:     source.Name,
			SourceID: &source.ID,
		}
		if err := database.DB.Create(&gallery).Error; err != nil {
			return nil, &importError{http.StatusInternalServerError, "Failed to create default gallery for source"}
		}

		// Try to auto-link to people based on source name
		linkedPersonIDs := autoLinkPeopleToGallery(source.Name, gallery.ID)

		// Auto-link galleries by matching source URL against all people's names/aliases
		autoLinkGalleryToPeopleByURL(gallery.ID, source.Location)

		// Check if this gallery matches any missing galleries by name (across all providers)
		if len(linkedPersonIDs) > 0 {
			if _, err := services.CheckAndLinkMissingGalleriesByName(gallery.ID, source.Name, linkedPersonIDs); err != nil {
				logger.Warnf("Failed to check for missing gallery matches: %v", err)
			}
		}
	}

	// Queue for crawling (use video queue for known video sources)
	if isVideo {
		services.AddToVideoQueue(source.ID)
	} else {
		services.AddToCrawlerQueue(source.ID)
	}

	return &source, nil
}

func BulkImportSources(c *gin.Context) {
	var inputs []struct {
		URL  string  `json:"url"`
		Name *string `json:"name"`
	}
	if err := c.ShouldBindJSON(&inputs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	type resultEntry struct {
		URL      string `json:"url"`
		Name     string `json:"name"`
		Status   string `json:"status"`
		SourceID *uint  `json:"source_id,omitempty"`
		Error    string `json:"error,omitempty"`
	}

	results := make([]resultEntry, 0, len(inputs))
	created := 0
	duplicates := 0
	failed := 0

	for _, input := range inputs {
		entry := resultEntry{
			URL:  input.URL,
			Name: "",
		}

		name := ""
		if input.Name != nil {
			name = *input.Name
			entry.Name = name
		}

		createdSource, err := createSingleSource(name, input.URL, "url", 0)
		if err != nil {
			entry.Status = "failed"
			if err.Code == http.StatusConflict {
				entry.Status = "duplicate"
				duplicates++
			} else {
				failed++
			}
			entry.Error = err.Message

			// Look up the existing source for duplicate entries
			if err.Code == http.StatusConflict {
				var existing models.Source
				if err := database.DB.Where("location = ?", input.URL).First(&existing).Error; err == nil {
					entry.SourceID = &existing.ID
				}
			}
		} else {
			entry.Status = "created"
			entry.SourceID = &createdSource.ID
			created++
		}

		results = append(results, entry)
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"summary": gin.H{
			"total":      len(inputs),
			"created":    created,
			"duplicates": duplicates,
			"failed":     failed,
		},
	})
}

func CrawlSource(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid source ID"})
		return
	}

	// Trigger crawl in background - route to video queue if applicable
	var src models.Source
	if err := database.DB.Select("location").First(&src, id).Error; err == nil {
		if services.IsVideoURL(src.Location) || services.IsVideoFile(src.Location) {
			services.AddToVideoQueue(uint(id))
		} else {
			services.AddToCrawlerQueue(uint(id))
		}
	} else {
		services.AddToCrawlerQueue(uint(id))
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "Crawl started"})
}

func GetSources(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	query := database.DB.Model(&models.Source{})

	// Search filter
	search := c.Query("q")
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("name LIKE ? OR location LIKE ?", searchPattern, searchPattern)
	}

	var total int64
	query.Count(&total)

	var sources []models.Source
	if err := query.Limit(limit).Offset(offset).Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch sources"})
		return
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))

	c.JSON(http.StatusOK, gin.H{
		"data": sources,
		"meta": gin.H{
			"current_page": page,
			"total_pages":  totalPages,
			"total_items":  total,
			"limit":        limit,
		},
	})
}

// DeleteSource removes a source and optionally its gallery and images
func DeleteSource(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid source ID"})
		return
	}

	// Check cascade options
	deleteGallery := c.Query("delete_gallery") == "true"
	deleteImages := c.Query("delete_images") == "true"

	var source models.Source
	if err := database.DB.First(&source, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Source not found"})
		return
	}

	if deleteGallery || deleteImages {
		// Find associated gallery
		var gallery models.Gallery
		if err := database.DB.Preload("Images").Where("source_id = ?", id).First(&gallery).Error; err == nil {
			if deleteImages {
				// Delete all images
				sourceDir := services.SanitizeDirectoryName(source.Name)
				for _, image := range gallery.Images {
					// Filename might be just basename or relative path.
					// Construct path based on source name.
					imagePath := filepath.Join(services.UploadsDir, sourceDir, filepath.Base(image.Filename))

					// If file doesn't exist at constructed path, try utilizing the stored filename directly
					// in case it was stored as a relative path "source/file.jpg"
					if _, err := os.Stat(imagePath); os.IsNotExist(err) {
						directPath := filepath.Join(services.UploadsDir, image.Filename)
						if _, err := os.Stat(directPath); err == nil {
							imagePath = directPath
						}
					}

					services.DeleteFile(imagePath)

					// Handle thumbnail
					// Thumbnails are usually in uploads/source_name/thumbnails/filename
					thumbnailPath := filepath.Join(services.UploadsDir, sourceDir, "thumbnails", filepath.Base(image.Filename))
					services.DeleteFile(thumbnailPath)
				}
				database.DB.Where("gallery_id = ?", gallery.ID).Delete(&models.Image{})
			}
			if deleteGallery {
				database.DB.Delete(&gallery)
			}
		}
	}

	// Delete source
	if err := database.DB.Delete(&source).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete source"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Source deleted successfully"})
}

func UpdateSourcePriority(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid source ID"})
		return
	}

	var input struct {
		Priority int `json:"priority"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := database.DB.Model(&models.Source{}).Where("id = ?", id).Update("priority", input.Priority).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update priority"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Priority updated", "priority": input.Priority})
}

func GetDownloadStatus(c *gin.Context) {
	status := services.GetGlobalDownloadStatus()

	// Add currently crawling sources from DB for complete picture
	var activeSources []models.Source
	database.DB.Where("status = ?", "crawling").Find(&activeSources)

	// We could also refine the status struct to include source details
	// but for now, the UI can match by ID from the sources list.

	c.JSON(http.StatusOK, status)
}

// GetFailedSources returns sources that have status == "error" for dashboard retry
func GetFailedSources(c *gin.Context) {
	var sources []models.Source
	if err := database.DB.Where("status = ?", "error").Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query sources"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sources})
}

// RetrySource triggers a crawl for a single source ID
func RetrySource(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid source ID"})
		return
	}
	// Reset status to queued and add to crawler/video queue as appropriate
	var src models.Source
	if err := database.DB.First(&src, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Source not found"})
		return
	}
	database.DB.Model(&src).Updates(models.Source{Status: "queued", LastCheckedAt: time.Now()})
	if services.IsVideoURL(src.Location) || services.IsVideoFile(src.Location) {
		services.AddToVideoQueue(src.ID)
	} else {
		services.AddToCrawlerQueue(src.ID)
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "Retry scheduled"})
}

// RetryAllSources schedules retry for all errored sources
func RetryAllSources(c *gin.Context) {
	var sources []models.Source
	if err := database.DB.Where("status = ?", "error").Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query sources"})
		return
	}
	for _, s := range sources {
		database.DB.Model(&s).Updates(models.Source{Status: "queued", LastCheckedAt: time.Now()})
		if services.IsVideoURL(s.Location) || services.IsVideoFile(s.Location) {
			services.AddToVideoQueue(s.ID)
		} else {
			services.AddToCrawlerQueue(s.ID)
		}
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "Retries scheduled", "count": len(sources)})
}

// autoLinkPeopleToGallery attempts to link people to a gallery based on name matching
// Returns the list of person IDs that were linked
func autoLinkPeopleToGallery(sourceName string, galleryID uint) []uint {
	if sourceName == "" {
		return nil
	}

	// Search for people with matching names (case-insensitive partial match)
	searchPattern := "%" + sourceName + "%"
	var people []models.Person
	database.DB.Where("LOWER(name) LIKE ?", strings.ToLower(searchPattern)).Find(&people)

	if len(people) == 0 {
		return nil
	}

	// Get the gallery
	var gallery models.Gallery
	if err := database.DB.First(&gallery, galleryID).Error; err != nil {
		return nil
	}

	// Find matching people and link them
	var matchedGalleries []*models.Gallery
	matchedGalleries = append(matchedGalleries, &gallery)

	var linkedPersonIDs []uint

	for i := range people {
		person := &people[i]
		// Check if name matches (case-insensitive)
		personNameLower := strings.ToLower(person.Name)
		sourceNameLower := strings.ToLower(sourceName)

		if strings.Contains(personNameLower, sourceNameLower) || strings.Contains(sourceNameLower, personNameLower) {
			// Use GORM's Association to append to existing galleries
			database.DB.Model(person).Association("Galleries").Append(&gallery)
			linkedPersonIDs = append(linkedPersonIDs, person.ID)
			// Auto-resolve missing galleries matching this gallery
			if err := services.AutoResolveMissingGalleries(person.ID, gallery.ID); err != nil {
				logger.Warnf("Failed to auto-resolve missing galleries for person %d, gallery %d: %v", person.ID, gallery.ID, err)
			}
		}
	}

	return linkedPersonIDs
}

// autoLinkGalleryToPeopleByURL checks the source URL against all people's names and aliases
// and links matching galleries. This mirrors the logic of LinkPersonToGalleries but runs
// automatically when a gallery is created from a source.
func autoLinkGalleryToPeopleByURL(galleryID uint, sourceLocation string) {
	if sourceLocation == "" {
		return
	}

	locationLower := strings.ToLower(sourceLocation)

	// Load all people with aliases
	var people []models.Person
	database.DB.Find(&people)

	if len(people) == 0 {
		return
	}

	var gallery models.Gallery
	if err := database.DB.First(&gallery, galleryID).Error; err != nil {
		return
	}

	for i := range people {
		person := &people[i]

		// Build search terms from name + aliases
		var searchTerms []string
		baseTerms := []string{person.Name}
		if person.Aliases != "" {
			var aliases []string
			if err := json.Unmarshal([]byte(person.Aliases), &aliases); err == nil {
				baseTerms = append(baseTerms, aliases...)
			}
		}

		for _, term := range baseTerms {
			term = strings.ToLower(term)
			searchTerms = append(searchTerms, term)
			if strings.Contains(term, " ") {
				searchTerms = append(searchTerms, strings.ReplaceAll(term, " ", "-"))
				searchTerms = append(searchTerms, strings.ReplaceAll(term, " ", "%20"))
			}
		}

		// Check if any search term matches the source URL
		for _, term := range searchTerms {
			if strings.Contains(locationLower, term) {
				database.DB.Model(person).Association("Galleries").Append(&gallery)
				logger.Infof("Auto-linked gallery %d to person %s via URL match (term: %s)", galleryID, person.Name, term)
				// Auto-resolve missing galleries matching this gallery
				if err := services.AutoResolveMissingGalleries(person.ID, gallery.ID); err != nil {
					logger.Warnf("Failed to auto-resolve missing galleries for person %d, gallery %d: %v", person.ID, gallery.ID, err)
				}
				break
			}
		}
	}
}
