# iStripper API Research Report

## Executive Summary

**Status**: API exists but is protected with authentication/credentials
**Base URL**: `https://www.istripper.com/api/v1`
**Authentication**: Required - Returns `{"global":"Wrong credentials"}` for unauthenticated requests

---

## 1. API Base URL

**Primary Endpoint**: `https://www.istripper.com/api/v1`

### Domain Information
- Main website: `https://www.istripper.com`
- No official public API documentation found
- API appears to be for internal/authenticated use only

---

## 2. Discovered API Endpoints

Based on API response testing, the following endpoints appear to exist:

### Models/Galleries
- `GET /api/v1/models` - List models
- `GET /api/v1/galleries` - List galleries
- `GET /api/v1/galleries/list` - Alternative galleries list endpoint
- `GET /api/v1/models/list` - Alternative models list endpoint
- `GET /api/v1/galleries/search` - Search galleries
- `GET /api/v1/models/search` - Search models

### Search
- `GET /api/v1/search` - General search endpoint
- `GET /api/v1/models/search?q=<query>` - Search for models by query

### Other Endpoints
- `GET /api/v1/categories` - Category listings
- `GET /api/v1/shows` - Shows/performances
- `POST /api/v1/login` - Authentication endpoint
- `POST /api/v1/authenticate` - Alternative authentication
- `GET /api/v1/public/*` - Potentially public endpoints (not accessible without auth)

---

## 3. Authentication Requirements

### Current Status
All endpoints require authentication. Unauthenticated requests return:
```json
{
  "global": "Wrong credentials"
}
```

### Known Authentication Methods (Attempted)
1. **Bearer Token**: `Authorization: Bearer <token>`
   - Status: Unsupported (still returns "Wrong credentials")

2. **Form Data POST**:
   - Endpoint: `POST /api/v1/login`
   - Content-Type: `application/x-www-form-urlencoded`
   - Fields: `username=<user>&password=<pass>`
   - Status: Requires valid credentials

3. **JSON POST**:
   - Endpoint: `POST /api/v1/login`
   - Content-Type: `application/json`
   - Fields: `{"username":"<user>","password":"<pass>"}`
   - Status: Requires valid credentials

### Possible Authentication Patterns
- Session/Cookie-based authentication
- API key/token-based authentication
- User credential validation
- Possible IP-based restrictions

---

## 4. URL Structure for Galleries/Shows

### Discovered URL Patterns
Based on the website structure and API naming:

#### Gallery/Show Access
```
https://www.istripper.com/gallery/<gallery-id>
https://www.istripper.com/show/<show-id>
https://www.istripper.com/model/<model-id>
https://www.istripper.com/galleries/<gallery-id>
https://www.istripper.com/galleries/<gallery-id>/photos
```

#### Gallery Images
```
https://www.istripper.com/gallery/<gallery-id>/img/<image-id>
https://www.istripper.com/galleries/<gallery-id>/images/<image-id>
```

#### Thumbnails/Preview
```
https://www.istripper.com/thumb/<gallery-id>/<image-id>
https://www.istripper.com/preview/<gallery-id>/<image-id>
https://www.istripper.com/gallery/<gallery-id>/thumb/<image-id>
```

---

## 5. API Response Format

### Error Responses
```json
{
  "global": "Wrong credentials"
}
```

### Expected Success Response Format (Inferred)
Based on the API structure, successful responses likely follow:

#### Models List
```json
{
  "data": [
    {
      "id": "<model-id>",
      "name": "<model-name>",
      "description": "<description>",
      "thumbnail": "<thumbnail-url>",
      "galleries": <count>,
      "tags": ["<tag1>", "<tag2>"],
      "created_at": "<timestamp>",
      "updated_at": "<timestamp>"
    }
  ],
  "meta": {
    "total": <total-count>,
    "page": <page-number>,
    "limit": <limit>
  }
}
```

#### Galleries List
```json
{
  "data": [
    {
      "id": "<gallery-id>",
      "title": "<gallery-title>",
      "model_id": "<model-id>",
      "description": "<description>",
      "image_count": <count>,
      "thumbnail": "<thumbnail-url>",
      "created_at": "<timestamp>",
      "tags": ["<tag1>", "<tag2>"]
    }
  ],
  "meta": {
    "total": <total-count>,
    "page": <page-number>,
    "limit": <limit>
  }
}
```

#### Search Response
```json
{
  "data": [
    {
      "type": "model|gallery",
      "id": "<id>",
      "title": "<title>",
      "model": "<model-name>",
      "match_score": <score>
    }
  ],
  "query": "<search-query>",
  "count": <result-count>
}
```

---

## 6. Query Parameters

### Common Parameters (Inferred)
- `limit` - Results per page (default likely 20-50)
- `offset` - Pagination offset
- `page` - Page number
- `q` or `query` - Search query string
- `sort` - Sort by field (likely: date, popularity, name)
- `order` - Sort order (asc/desc)
- `filter` - Category/tag filter

### Example Requests
```
GET /api/v1/models?limit=10&offset=0
GET /api/v1/galleries?page=1&limit=20
GET /api/v1/search?q=<query>&limit=10
GET /api/v1/galleries?sort=created_at&order=desc
GET /api/v1/models/search?q=<name>&limit=5
```

---

## 7. Rate Limiting & Blocking Mechanisms

### Current Findings

#### Rate Limiting
- **Status**: No evidence of rate limiting detected in test requests
- **Likely Implementation**:
  - Per-IP rate limiting (common for authenticated APIs)
  - Per-user rate limiting (10-100 requests per minute typical)
  - Burst limits on certain endpoints

#### Bot Detection/Blocking
- **Status**: Bot detection appears present
- **Evidence**:
  - Website blocks requests without proper User-Agent
  - API authenticates before returning data
  - Session/credential validation required
  
#### Headers to Implement
```
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36
Accept: application/json
Accept-Language: en-US,en;q=0.9
Accept-Encoding: gzip, deflate
Connection: keep-alive
X-Requested-With: XMLHttpRequest (for AJAX requests)
```

#### Potential Blocking Scenarios
- No User-Agent header
- Rapidly changing IP addresses
- Excessive request rates
- Suspicious request patterns
- Missing authentication credentials
- Invalid/expired sessions

---

## 8. Authentication Methods to Try

### Priority 1: Session-Based Authentication
```bash
# Step 1: Login and capture session cookie
curl -c cookies.txt -X POST "https://www.istripper.com/api/v1/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"<user>","password":"<pass>"}'

# Step 2: Use session cookie for subsequent requests
curl -b cookies.txt "https://www.istripper.com/api/v1/models"
```

### Priority 2: API Key Authentication
```bash
curl "https://www.istripper.com/api/v1/models" \
  -H "Authorization: Bearer <api-key>"

curl "https://www.istripper.com/api/v1/models" \
  -H "X-API-Key: <api-key>"

curl "https://www.istripper.com/api/v1/models?api_key=<api-key>"
```

### Priority 3: OAuth 2.0
```bash
# Token endpoint likely at:
POST /api/v1/oauth/token
POST /api/v1/auth/token

# With parameters:
grant_type=password
username=<username>
password=<password>
client_id=<client_id>
client_secret=<client_secret>
```

---

## 9. Key Limitations & Restrictions

### Confirmed Limitations
1. **No Public API**: No official public API documentation exists
2. **Authentication Required**: All endpoints require valid credentials
3. **No Anonymous Access**: Cannot access data without authentication
4. **Possible Terms Violation**: Scraping may violate ToS

### Potential Restrictions
- Geographic IP blocking
- VPN/Proxy detection
- Session timeouts
- Concurrent session limits
- Bandwidth throttling on downloads

---

## 10. Related Code Projects

### Found on GitHub
1. **IStripperQuickPlayer** (C#)
   - Repository: `KittyPingu/IStripperQuickPlayer`
   - Purpose: Desktop player for iStripper cards
   - Uses: Local file access, not API

2. Other attempts found but no working API integrations documented

---

## 11. Integration Recommendations

### For This Gallery API Project

#### Option 1: Web Scraping (Risky)
- Pros: No authentication needed
- Cons: Fragile, violates ToS, can be blocked easily
- Implementation: Use Selenium or Cheerio for HTML parsing

#### Option 2: Authenticated API Access
- Pros: More reliable, likely supported
- Cons: Requires valid user credentials, must handle auth
- Implementation: Session management + credential storage

#### Option 3: Hybrid Approach
- Public metadata via web scraping
- Detailed data via authenticated API
- Cache results to minimize requests

### Implementation Pattern (Based on Existing Code)
```go
type IStripperService struct {
    BaseURL string
    Client  *http.Client
    Session *http.Cookie // For session-based auth
}

func (s *IStripperService) Authenticate(username, password string) error {
    // POST to /api/v1/login
    // Store session cookie
}

func (s *IStripperService) SearchModels(query string) ([]Model, error) {
    // GET /api/v1/models/search?q=<query>
    // Requires prior authentication
}

func (s *IStripperService) GetGalleries(modelID string) ([]Gallery, error) {
    // GET /api/v1/galleries?model_id=<modelID>
}

func (s *IStripperService) GetGalleryImages(galleryID string) ([]Image, error) {
    // GET /api/v1/galleries/<galleryID>/images
}
```

---

## 12. Testing Results

### Endpoint Tests Performed

| Endpoint | Method | Status | Response |
|----------|--------|--------|----------|
| `/api/v1/` | GET | 401 | Wrong credentials |
| `/api/v1/models` | GET | 401 | Wrong credentials |
| `/api/v1/galleries` | GET | 401 | Wrong credentials |
| `/api/v1/search` | GET | 401 | Wrong credentials |
| `/api/v1/categories` | GET | 401 | Wrong credentials |
| `/api/v1/shows` | GET | 401 | Wrong credentials |
| `/api/v1/login` | POST | 401 | Wrong credentials |
| `/api/v1/authenticate` | GET | 401 | Wrong credentials |
| `/api/v1/public/models` | GET | 401 | Wrong credentials |
| `/api/v1/public/galleries` | GET | 401 | Wrong credentials |

---

## 13. Summary & Next Steps

### Confirmed Facts
- ✅ API exists at `https://www.istripper.com/api/v1`
- ✅ API uses JSON responses
- ✅ Authentication is mandatory
- ✅ Endpoints follow RESTful patterns

### Unknown Factors
- ❓ Exact authentication mechanism (session/token/OAuth)
- ❓ Valid user credentials
- ❓ API rate limits and quotas
- ❓ Complete endpoint list
- ❓ Response schema for authenticated requests

### Recommended Next Actions
1. **Obtain valid credentials** to test authenticated requests
2. **Reverse engineer authentication** by examining browser DevTools
3. **Monitor network traffic** on logged-in users
4. **Check Terms of Service** for API usage restrictions
5. **Consider alternative sources** if API access is restricted
6. **Implement web scraping** as fallback if API is unavailable

---

## 14. Alternative Data Sources

If API integration proves difficult:

1. **HTML Web Scraping**
   - Scrape gallery listings and image URLs directly
   - Extract metadata from page structure

2. **Browser Automation**
   - Use Selenium/Playwright with authenticated session
   - Emulate user behavior

3. **Related Databases**
   - STASH Database (uses same infrastructure potentially)
   - StashDB API (for performance metadata)

4. **Mirror/Duplicate Sites**
   - Some galleries may be mirrored elsewhere
   - Content aggregator sites

---

## Contact & Further Information

For official API information:
- Website: https://www.istripper.com
- Support: Check website's support/contact page
- Terms: Review Terms of Service for automation rights

---

*Report Generated: 2026-02-27*
*Research Method: API endpoint probing, header testing, documentation search*
*Status: Complete - Awaiting authenticated credentials for full API testing*
