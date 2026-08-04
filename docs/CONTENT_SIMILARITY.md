# Content Similarity & "You Might Like" — Design Plan

## 1. Goal

Give users a **content-based** recommendation engine for images ("you liked this, you might like this"). Instead of collaborative filtering ("people who liked this also liked…"), we inspect the image itself and recommend based on its *content and feel*.

Three signal families work together:

1. **Semantic embedding vector** for each image (the "feel" of the image).
2. **Structured tags** extracted from the image (objects, people, composition, color/vibrancy).
3. **A personalized preference profile** learned from the user's 1–5 ratings.

There are no users yet, so all preference data is a single shared profile — but everything is scoped so a per-user profile can drop in later.

## 2. Strategy Assessment (is this viable?)

**Yes.** This is a textbook content-based recommender (embedding + explicit features + learned taste vector) and it's a good fit for this app because:

- The library is self-contained (images on disk, no need to join third-party *user* behavior data).
- We already do per-image extraction at download time (thumbnails, dominant colors) — the embedding/tagging pipeline slots into the same spot.
- Every image is scored on its own merits, so the moment a new image lands it is immediately recommenable — no waiting for other users to interact with it.

### Key modifications to the proposal

| Original proposal | Recommended modification | Why |
|---|---|---|
| "Vector" for images (undefined) | A **semantic embedding** (CLIP-style) computed locally via ONNX, stored as a float32 blob. | The whole premise is "match the *feel*, not the pixels." Perceptual hashes (aHash/dHash/pHash) only capture pixel-level similarity. Only a learned embedding captures semantics. |
| Train one image tool to tag everything (objects, people, composition, colors) | **Split tagging into two tiers**: (A) discrete semantic tags from a vision model, (B) continuous low-level descriptors computed analytically in Go. | Composition and vibrancy are *continuous* measurements, not tags. A language-style tagger is terrible at "composition"; a few lines of image statistics are exact. Never force one tool to do both. |
| Personal modifier trained on 1–5 ratings | Rocchio-style preference vector in embedding space + tag-affinity weights, online-updated per rating. | With a single profile and sparse ratings, direct regression overweights noise. A weighted centroid of liked/disliked embeddings is robust, explainable, cheap, and easy to improve later. |
| Tags are only "input" data | Tags serve **two** purposes: input features for scoring **and** human-readable explanations ("because it matches *solo*, *outdoor*, *golden tones*"). | Tags give the UI a reason string. This is the feature users can actually see and trust. |
| — | Add **diversity/MMR** step + **embedder versioning** + **user-scoped tables from day one** + **async backfill worker** | Prevents 20 recommendations from the same gallery, makes model upgrades deterministic, and avoids a migration later. |

Bottom line: **the shape you proposed is right; the two big upgrades are (1) make the "vector" a real learned semantic embedding, and (2) separate continuous measurements (composition/color/vibrancy) from the discrete tagger output.**

## 3. Reference Architecture

```
┌────────────┐   download   ┌──────────────────────┐
│  Crawl/Rip │ ───────────► │  image on disk        │
└────────────┘              └──────────┬───────────┘
                                       │ existing: thumbnail + dominant colors
                                       ▼
                        ┌───────────────────────────┐
                        │  Embedding & Tag Worker   │  (new — async, queued)
                        │  · extract embedding      │  ONNX embedder → 512-dim vector
                        │  · extract semantic tags  │  vision tagger → tag list + scores
                        │  · compute low-level      │  vibrancy/saturation/brightness/
                        │    descriptors            │  contrast/grid/edge stats
                        └────────────┬──────────────┘
                                     ▼
                        ┌───────────────────────────┐
                        │  Index (in-memory)        │  id → normalized vector
                        │                           │  rebuilt from DB at boot
                        └────────────┬──────────────┘
                                     ▼
              user rates 1-5 ──► Profile (embedding centroid + tag affinities)
                                     ▼
        GET /images/:id/similar ──► score = blend(embedding sim, tag affinity,
                                               color sim) → MMR diversity → top-N
```

## 4. Data Model

New tables (registered in `database/db.go` `Migrate()`).

### 4.1 `image_embeddings`

One row per image that has been embedded.

```sql
CREATE TABLE image_embeddings (
  id              INTEGER PRIMARY KEY,
  image_id        INTEGER NOT NULL,
  embedder        TEXT    NOT NULL,          -- e.g. "clip-vit-b32-v1"
  dimension       INTEGER NOT NULL,
  vector          BLOB    NOT NULL,          -- raw float32 bytes, LE order
  created_at      DATETIME NOT NULL
);
CREATE UNIQUE INDEX idx_image_embeddings_image ON image_embeddings(image_id);
CREATE INDEX idx_image_embeddings_embedder ON image_embeddings(embedder);
```

- `embedder` is a **model version tag**. If the model changes (e.g. `v2`), old vectors stay but the index only uses the current version; a re-embed job upgrades stragglers. This makes upgrades deterministic.
- Float32 BLOB keeps SQLite simple (~2 KB/row at 512-dim).

### 4.2 `image_ratings`

```sql
CREATE TABLE image_ratings (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER NOT NULL DEFAULT 0,   -- 0 = shared/default profile until users exist
  image_id    INTEGER NOT NULL,
  rating      INTEGER NOT NULL CHECK (rating BETWEEN 1 AND 5),
  created_at  DATETIME NOT NULL,
  updated_at  DATETIME NOT NULL,
  UNIQUE (user_id, image_id)
);
```

### 4.3 (optional, phase 2) `taste_profiles`

Materialized profile state, only needed if we cache the computed vector instead of recomputing on demand.

```sql
CREATE TABLE taste_profiles (
  user_id      INTEGER PRIMARY KEY,        -- 0 = shared/default
  embedder     TEXT NOT NULL,
  pos_centroid BLOB NOT NULL,              -- mean of rated-high embeddings
  neg_centroid BLOB NOT NULL,              -- mean of rated-low embeddings
  tag_affinity TEXT NOT NULL,              -- JSON: {"solo": 2.1, "blonde": -0.6, ...}
  n_likes      INTEGER NOT NULL,
  n_dislikes   INTEGER NOT NULL,
  updated_at   DATETIME NOT NULL
);
```

If we don't cache, this is derived by a scan over `image_ratings` + `image_embeddings`; with cache, it's invalidated/rebuilt after N new ratings. Start uncached (simple), cache later if needed.

### 4.4 Tags (reuse existing)

Reuse the existing `tags` + `image_tags` tables. New categories to complement the existing `label/pose/mood/manual`:

- `object` — things in frame (furniture, water, plant, etc.)
- `people` — gender/count, pose, expression (e.g. `smile`, `standing`, `solo`, `1girl`)
- `composition` — reserved for the *discrete* bits the tagger gives us (e.g. `close-up`, `wide-shot`, `portrait`)
- `color` — only for dominant *named* colors (e.g. `red`, `black-and-white`), **not** the hex palette (that lives in `images.dominant_colors`)

`images.dominant_colors` (existing k-means hex palette) is the source of truth for color similarity; tags only provide human-meaningful names.

## 5. Embedding / Vector Layer

### 5.1 Model

- Embedding runtime: **ONNX Runtime** via a pure-Go binding (e.g. `github.com/yalue/onnxruntime_go`), loaded once at startup.
- Model: a CLIP-style image encoder (e.g. OpenCLIP `ViT-B/32` or `RN50`) exported to ONNX. Input 224×224, output **512-dim normalizeable vector**.
- Everything local. No cloud API — this matches the fully-local ethos of the pipeline (cookies, wireguard, gallery-dl are all self-hosted).
- **Spike before build**: confirm ONNX DLL availability on Windows, verify a `ViT-B/32` ONNX export runs and produces sane similarity neighborhoods on a handful of our images. If ONNX is a pain, fallback plan: a small Python sidecar worker (the AI-labeling pipeline was previously Python-backed) that exposes `POST /embed` — but Go/ONNX is the preferred path.

### 5.2 Why not pixel hashes?

aHash/dHash/pHash answer "is this a duplicate/crop", not "does this feel similar". Two photos of the same scene with different crops/resizing/lighting would be "identical" to a phash but the feature is explicitly meant to match *mood*. phash can still be kept as a cheap **near-duplicate filter** (optional, phase 2) so recommendations skip exact-rephotographed sets.

### 5.3 Index

- In-memory index: `map[uint][]float32` (normalized) rebuilt at boot with one query over `image_embeddings` joined to non-deleted images. Brute-force cosine is fine to mid-5-digit image counts.
- Abstract behind an interface:

```go
type VectorIndex interface {
    Upsert(imageID uint, vec []float32) error
    Remove(imageID uint) error
    Search(imageID uint, k int) []ScoredImage   // or by query vector
    Rebuild() error
}
```

- Swap in an ANN implementation (HNSW, e.g. `github.com/spotify/annoy`-style port or sqlite-vss) later without touching handlers. Rebuild is cheap: `n` BLOB reads.

### 5.4 Versioning

- `embedder` column prevents mixing vectors from different model versions.
- A background "re-embed" job upgrades rows whose `embedder != current` lazily (see §10). Old vectors are removed from the index once upgraded.

## 6. Tagging Layer (two tiers)

### 6.1 Tier A — discrete semantic tags (vision model)

Options, in order of preference:

1. **Booru-style tagger** (e.g. WD14/DeepDanbooru derivatives). Pros: trained on enormous adult+anime datasets, vocabulary includes exactly what we need — `solo`, `1girl`, `smile`, `blush`, `standing`, `close-up`, `outdoors`, plus object tags. ONNX exports exist. This content aligns perfectly with this library's genre.
2. An on-device VLM for richer structured JSON (`{objects: [...], people: {pose, expression}}`). More powerful, heavier, slower.

Recommendation: **evaluate the booru tagger first** — fastest path, vocab fits the domain. Take top-k tags (with score threshold, e.g. > 0.3) and store as `Tag` rows (`category` maps each tag to its semantic bucket via a small mapping table).

Each tag that gets a rating signal later also feeds the tag-affinity weights (§7).

### 6.2 Tier B — continuous low-level descriptors (pure Go, analytic)

Computed from the image with existing `disintegration/imaging`, cheap, deterministic:

| Descriptor | What it measures | Implementation |
|---|---|---|
| `vibrancy` | mean saturation of the HSV conversion (0–1) | histogram over sampled pixels |
| `brightness` | mean value/luma (0–1) | per-pixel luma |
| `contrast` | std-dev of luma | per-pixel luma |
| `saturation_spread` | saturation std-dev | per-pixel |
| `color_grid` | 3×3 grid of (mean hue, mean sat, mean luma) — catches layout/composition of tones | downsample then grid-sample |
| `edge_density` | fraction of edge pixels (cheap Sobel) | gives "busy vs clean" feel |
| `aspect` | width/height | composition proxy |
| `colorfulness` | Hasler-Süsstrunk metric | perceptually meaningful vibrancy score |

Store where? Two options:
- (a) JSON column `image_content_features` on `images` (like `dominant_colors` today), or
- (b) a `image_lowlevel_features` table (one row per image).

Recommendation: **(a) a JSON column on `images`** — matches the existing `dominant_colors` precedent, avoids a join, and the data is single-writer/single-reader.

### 6.3 Color similarity (existing)

Already implemented (`services/color_similarity.go`, `GET /search/color`). Reuse `FindSimilarColorInPalette` to compute a 0–100 similarity between the seed image's palette and each candidate. This becomes one weighted term of the score (§8), not a separate flow.

## 7. Personalization (the ratings → "modifier")

### 7.1 Rating semantics

| Rating | Meaning | Use in profile |
|---|---|---|
| 5 | Love it | strong positive |
| 4 | Like it | positive |
| 3 | Neutral | ignored (or weak positive) |
| 2 | Don't like | negative |
| 1 | Hate it | strong negative |

`IsFavorite` (existing) counts as a 5 where consistent.

### 7.2 Profile math (Rocchio-style)

Keep a preference vector in embedding space, plus per-tag affinities:

```
likes    = mean(kernel(image) for ratings ≥ 4 | x_fav)
dislikes = mean(kernel(image) for ratings ≤ 2)

profile_vec = γ * mean(all_embedded) + α * likes − β * dislikes   (α=1, β=0.6, γ=0.1 initially)
```

- Cosine similarity between `profile_vec` and any image vector gives an instant "how much would this user like it" score even for **never-rated** images.
- Tag affinity: for each tag `t`, `aff(t) = (count(liked with t) − 0.6·count(disliked with t)) / total_likes`, smoothed; clamp to a sane range. This gives interpretable explanations.
- Recomputed incrementally on each rating event (Rocchio updates are O(1) — maintain running means on the profile struct, or recompute from `image_ratings` when ratings ≤ a few thousand).

### 7.3 Simplicity first

Do **not** start with gradient-boosted regression over the 1–5 labels. Single profile + sparse noisy labels → overfitting. Rocchio + tag affinity is robust and transparent, and the same feature set (fixed-dim vector + tags) is the input for any fancier model later (logistic regression per tag, LightFM-style MF, etc.).

## 8. Scoring & Ranking

For a seed image `s` and candidate `c`, per-profile (or shared profile for the very first personal term):

```
Dpref  = cosine(profile_vec, vec(c))                         // personalized pull
Dsim   = cosine(vec(s), vec(c))                              // "you liked this one, here's like it"
Dtags  = Σ_t min(aff(t), cap) * [has_tag(c, t)] / |tags(s)∩tags(c)|…  // tag affinity overlap
Dcolor = normalize_Euclidean(s.palette, c.palette)           // existing color sim 0-100 /100

score(c) = w1·Dsim + w2·Dtags + w3·Dcolor + w4·Dpref
weights (start): w1=0.5, w2=0.25, w3=0.15, w4=0.10
```

Notes:
- `w4` (personalized pull) only kicks in once a profile has ≥ ~3 positive + ≥1 negative example; before that it's 0.
- **Diversity (MMR-style):** after scoring, greedily select top-N while subtracting `λ·max(sim(c, already_chosen))` (λ≈0.6) and **hard-exclude** candidates sharing the same gallery/source as the seed and the same gallery as each other beyond 1–2 per gallery. Prevents "20 images from the same photoshoot".
- Normalize each term to [0,1] so weights are comparable.
- Filter: only images with `type='image'`, `file_exists=true`, valid embedding, and `deleted_at IS NULL`.

## 9. API Design

All under the existing `/api` (proxy strips `/api` — note the current routes are registered without it; keep consistent with `main.go`).

### 9.1 Recommendations

```
GET /api/images/:id/similar?limit=24&group_by_gallery=1
```
Response:
```json
{
  "seed": { "id": 1, "tags": ["solo", "smile"], "features": {"vibrancy": 0.7} },
  "data": [ { "id": 42, "similarity": 0.83, "reasons": ["similar content", "tag: solo", "tag: golden tones"], "thumb": "/images/..." } ],
  "profile": { "n_ratings": 12, "status": "learning" }
}
```

Per TODO.md, the similarity page should also be loadable/shareable by a **base64-encoded list of ids**:

```
GET /api/images/similar?ids=<base64(json:[...])>
```
(seed derived from the first id; the id-list form powers the standalone page and deep-links.)

### 9.2 Ratings

```
PUT  /api/images/:id/rating        { "rating": 1..5 }   // upsert
GET  /api/images/:id/rating                          // my rating for this image
DELETE /api/images/:id/rating                        // clear
POST /api/profile/reset                              // wipe shared profile (single user TODO)
GET  /api/images/similar/for-me?seed=1              // pure personal pull, mostly for debug/tuning
```

Rating returns the newly computed profile influence (`delta` on affected tags) so the UI could optionally show "preferences updated".

### 9.3 Maintenance / admin

```
POST /api/admin/embed/backfill      // enqueue all images missing embed/features
GET  /api/admin/embed/status        // counts: done/pending/failed, embedder version
```

### 9.4 WebSocket

Existing hub can broadcast "embedding worker progress" alongside download progress (`services/websocket_service.go`) — optional polish.

## 10. Background Pipeline

Mirror the existing worker pattern (`services/worker_service.go`, `StartScanWorker`, etc.).

1. **At download time** (in `downloadImageWithCookies`, right after `ExtractDominantColors`): enqueue `(imageID, path)` onto a new `embedQueue`.
2. **`services/embedding_worker.go`**: bounded worker pool that for each item:
   - loads & preprocesses image (resize 224×224),
   - computes embedding via ONNX → writes `image_embeddings`,
   - runs tagger → upserts `Tag`+`image_tags`,
   - computes low-level descriptors → updates `images.image_content_features` JSON,
   - upserts into the in-memory index.
   - On failure: retry with backoff, log, mark `embed_failed` so `/admin/embed/status` shows it.
3. **Backfill**: `StartEmbeddingWorker()` also sweeps any `type='image', file_exists=true` row lacking an embedding (startup + daily, like `StartDailyScanScheduler`). Idempotent via unique `image_id`.
4. Videos: skip (or embed the thumbnail frame in phase 2).

Queue should be bounded & persisted (small `embed_queue` table with `status`), not just in-memory, so restarts don't reprocess thousands.

## 11. Frontend

Follow `designs/design-system.md` strictly (per TODO.md instructions).

1. **Lightbox "You might like" panel** — right/under the lightbox: horizontal thumb strip of recommendations; click → navigate; each thumb shows a 1–5 star control.
2. **Star ratings on any image tile** (Images page, Gallery detail, Lightbox).
3. **Dedicated Similar page** at `/similar` loadable via `?ids=<b64>` (deep-linkable) — grid of the recommended set with reasons ("similar content · tag: solo").
4. **Preferences/status chip** — shows "profile learning… / tuned (12 ratings)". Maybe a small "reset profile" action.

New API client functions in `frontend/src/lib/api.ts` following the existing grouped style (`images.similar`, `images.rate`, etc.). New types in `frontend/src/types/index.ts`.

Recommended order: Lightbox ratings + "You might like" strip (the visible payoff) first, then the standalone `/similar` page.

## 12. Config & Env

Add to `config/config.go` + `.env`:

```
EMBED_MODEL_PATH=./models/vit-b32.onnx     # ONNX model file
EMBED_MODEL_NAME=clip-vit-b32-v1            # version tag for DB
EMBED_DIM=512
EMBED_TAGGER_PATH=./models/wd-tagger.onnx   # tagger ONNX (optional tier A)
EMBED_TAG_THRESHOLD=0.3
EMBED_CONCURRENCY=2
EMBED_INDEX_MODE=bruteforce                 # | ann
REC_WEIGHTS=0.5,0.25,0.15,0.10              # sim, tags, color, pref
REC_DIVERSITY_LAMBDA=0.6
```

Disable the whole feature by leaving `EMBED_MODEL_PATH` empty (endpoints return 503 "embedding disabled").

## 13. Open Questions (decide in the spike)

1. **Embedding model + export**: settle on `ViT-B/32` vs `RN50` vs eval'd booru-tagger-as-features; confirm ONNX runtime works on this Windows box; measure ms/image on our hardware (CPU).
2. **Tagger choice**: pure booru tagger (fast, domain-fit) vs full VLM (structured, slower). Decide after embedding spike.
3. **Low-level features on `images` column vs table**: recommend column (matches `dominant_colors`), confirm no migration pain.
4. **Rating scale handling for 3** = ignore vs weak-positive (default: ignore).
5. **Videos**: skip entirely in v1 (recommended) or embed the representative frame.
6. **Gallery/source-based hard-exclusion** scope for diversity — per-gallery limit of 1–2 in results.

## 14. Implementation Phases

### Phase 0 — Spike (validate assumptions, no production code)
- [ ] Get ONNX Runtime working in Go on this machine; run a CLIP `ViT-B/32` export over ~50 of our real images.
- [ ] Eye-check neighborhoods: do top-5 similar make sense ("feel")? Compare vs phash results to confirm semantic wins.
- [ ] Decide tagger path; if booru tagger, sanity-check vocab on 10 sample images.
- [ ] Measure throughput (images/min) to size the worker pool.

Exit: model files available locally, ms/image numbers, decision on tagger.

### Phase 1 — Data + pipeline
- [ ] `database/db.go`: add `image_embeddings`, `image_ratings` (+ optional `embed_queue`) and migrate.
- [ ] Add `image_content_features` JSON column to `Image` model.
- [ ] `services/embedding_service.go`: ONNX loader, preprocess, embedding extraction, low-level descriptors.
- [ ] `services/tagging_service.go`: tagger (whichever chosen) → tag upsert into existing `Tag`/`image_tags` with category mapping.
- [ ] `services/embedding_worker.go` + queue table; hook into download flow; backfill sweep; startup index build.
- [ ] `services/vector_index.go` + `bruteforce.go` (**note:** rename/alias to avoid collision with any existing symbol), interface + brute-force impl.
- [ ] `/admin/embed/*` endpoints + status.

Exit: all historical images embedded/tagged/featured; index rebuilds at boot; counts visible via admin status.

### Phase 2 — Similarity + ratings (the product)
- [ ] `services/score_service.go`: per-term scoring, normalization, MMR diversity, gallery exclusions.
- [ ] `GET /images/:id/similar` + base64-ids variant.
- [ ] `image_ratings` CRUD endpoints + profile (Rocchio + tag affinity) recompute.
- [ ] `POST /profile/reset`.
- [ ] Frontend: ratings control on tiles/lightbox; "You might like" strip; `/similar` page via `?ids=`; status chip.

Exit: can rate images, immediately see tailored suggestions, share a `/similar?ids=` link.

### Phase 3 — Hardening & tuning
- [ ] Hyperparameter tuning (weights, λ, count thresholds) using simple A/B: recommendations views → ratings order correlation.
- [ ] Model upgrade path: `embedder v2`, lazy re-embed, index migration.
- [ ] Optional: HNSW index, phash near-duplicate filter, per-user profiles, video frame embeddings.
- [ ] Persisted queue recovery, failure dashboard, concurrency tuning.

## 15. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| ONNX unavailable/painful on Windows | Phase 0 spike 1st; fallback = Python sidecar worker (precedent exists). Feature stays 100% local. |
| Embedding doesn't capture "feel" for adult photos | Phase 0 eye-check on real library before building; if CLIP poor, prefer booru-tagger features as the vector source. |
| Cold start (no ratings) | Purely content-based similarity works immediately; profile term only activates after enough ratings. |
| Tagger bias / NSFW mislabeling | Domain-fit tagger; threshold top-k; tags are suggestions, not moderation. |
| Re-embed storms after model change | Versioned embedder column; lazy backfill; concurrency caps. |
| Recommendations too same-y | MMR diversity + gallery hard-limits (§8). |
| DB bloat (512-dim BLOBs) | ~2 KB/image → tens of MB at 10k images; acceptable for SQLite. Vector index is in-memory anyway. |