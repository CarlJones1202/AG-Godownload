# Content Similarity & "You Might Like"

Status: implemented (2026-11-20). This document describes the current design;
see the "Remaining work" section for the parts that were planned but not built.

## Goal

Recommend images by **content and feel** rather than by pixel similarity or other
users' behavior. Three signal families are combined:

1. A semantic or low-level embedding vector per image.
2. Structured tags and analytic descriptors (color, vibrancy, composition).
3. A shared preference profile learned from 1–5 ratings.

There is one shared profile (`user_id = 0`); records are scoped so per-user profiles
can be added later.

## Architecture

```
download → embed queue → embedding worker → PostgreSQL tables → in-memory index
                                      │                       │
                                      ├─ tags / low-level features
                                      └─ taste profile ← ratings
                                                          ↓
                       GET /similar/:id → blend + MMR → top-N recommendations
```

- `services/embedding_service.go` chooses the provider: a built-in low-level
  descriptor provider (51 dimensions) by default, or a local SigLIP 2 vision
  encoder (768 dimensions) when `EMBED_MODEL_PATH` is configured.
- `services/embedding_worker.go` processes a persisted `embed_queue`, computes the
  vector, upserts tags/features, and updates the in-memory index. Failures are
  retried and reflected in `/admin/embed/status`.
- `services/vector_index.go` holds normalized vectors and does bounded brute-force
  search; it rejects vectors from a different embedder version.
- `services/score_service.go` blends similarity signals and applies MMR-style
  diversity. `services/profile_service.go` maintains the shared taste profile.
- `main.go` builds the index and starts the worker at boot.

All embedding inference is local. With SigLIP 2, `init.ps1` creates a Python 3.13
virtual environment, installs CPU ONNX Runtime dependencies, and downloads the
int8 `siglip2-base-patch16-224` model (about 95 MB) with a SHA-256 check. Images are
not sent to a hosted service.

## Data model

GORM models in `models/embedding.go`, migrated by `database/db.go`:

| Table | Purpose |
|---|---|
| `image_embeddings` | One normalized vector per image, with `embedder`, `dimension`, and raw little-endian float32 bytes in a `BYTEA` column. Unique on `image_id`. |
| `image_ratings` | 1–5 ratings, `user_id` (currently always 0), timestamps. |
| `recommendation_events` | Set ID, image ID, mode, event, optional rating, timestamp. |
| `embed_queue` | Idempotent work queue with `pending`, `processing`, `failed`, and `deferred` states plus attempt counts. |

Low-level descriptors are stored as JSON in `images.image_content_features`; the
existing `images.dominant_colors` JSON array remains the source of truth for color
similarity. Tags reuse the existing `tags` / `image_tags` many-to-many relation.

## Tagging

Implemented: analytic low-level descriptors (vibrancy, brightness, contrast,
color grid, edge density, aspect, colorfulness) and known tags from existing
metadata.

Not implemented: the planned booru-style vision tagger (Tier A). `services/tagging_service.go`
leaves room for a model-backed tagger, but none is wired up today. Tag-derived
explanation strings therefore come from existing tags rather than a vision model.

## Scoring and ranking

For seed image `s` and candidate `c`:

```
Dpref  = cosine(profile_vec, vec(c))
Dsim   = cosine(vec(s), vec(c))
Dtags  = tag affinity overlap
Dcolor = existing palette similarity, normalized to [0,1]

score(c) = 0.5·Dsim + 0.25·Dtags + 0.15·Dcolor + 0.10·Dpref
```

The personal term is inactive until the profile has enough positive and negative
examples. Candidates are deduplicated and rescored against the full seed set, then
selected greedily with an MMR penalty for vector/tag overlap. Seed galleries are
excluded, and repeated galleries/sources are penalized rather than hard-capped.

Only images with `type='image'`, `file_exists=true`, a valid embedding, and no soft
delete are considered. Ratings of 1–2 are treated as explicit negatives and excluded
from the candidate pool; 4–5 are already-known likes. The Similar page exposes a
"Not interested" action that records a 1-star rating.

## API

The Go server registers these at root paths. The frontend prefixes calls with
`/api`; Vite strips that prefix when proxying to port 8080.

| Route | Purpose |
|---|---|
| `GET /similar/:id?limit=24` | Recommendations for one image. |
| `GET /similar?ids=<base64(json array)>` | Recommendations for a shared id list; first id is the seed. |
| `GET /similar?limit=24` | Recommendations from the current profile/favorites. |
| `GET /ratings/:imageId` | Read the current rating. |
| `PUT /ratings/:imageId` | Set a 1–5 rating. |
| `DELETE /ratings/:imageId` | Clear the rating. |
| `POST /profile/reset` | Reset the shared taste profile. |
| `POST /recommendations/events` | Record open/favorite/not-interested/rating events for a set. |
| `GET /admin/recommendations/metrics?days=30` | Totals and per-mode metrics for a 1–365 day window. |
| `POST /admin/embed/backfill` | Queue missing embeddings/features. |
| `GET /admin/embed/status` | Done/pending/failed counts and embedder version. |

Recommendations carry a set ID; the Similar page and Lightbox strip record events
against it. Impressions and engagement counts are unique image/set pairs, so open,
favorite, and not-interested rates are bounded by 1. Ratings count each action,
including repeat ratings.

## Frontend

- `/similar` renders a shared or favorites-based recommendation grid with reasons.
  `?ids=<base64>` links are deep-linkable.
- The Lightbox has a "You might like" strip with per-item ratings.
- `frontend/src/lib/api.ts` groups the client functions under `similar`, `ratings`,
  `profile`, and `recordRecommendationEvent`.
- Follow `designs/design-system.md` for UI changes.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `EMBED_MODEL_PATH` | empty | Local ONNX model path; empty selects the low-level provider. |
| `EMBED_MODEL_NAME` | `lowlevel-v1` / `siglip2-base-patch16-224-int8-v1` | Version tag stored with each vector. |
| `EMBED_DIM` | 51 / 768 | Expected vector dimension. |
| `EMBED_PYTHON` | `python` | Interpreter for the Python embedding worker. |
| `EMBED_TAG_THRESHOLD` | `0.3` | Tag score cutoff. |
| `EMBED_CONCURRENCY` | `2` | Embedding worker concurrency. |
| `REC_WEIGHTS` | `0.5,0.25,0.15,0.10` | Similarity, tags, color, preference weights; normalized if they do not sum to 1. |
| `REC_DIVERSITY_LAMBDA` | `0.6` | MMR diversity strength. |

Changing the embedding model requeues images under the new model version before
their vectors enter the index; mixed versions are never scored together.

## Remaining work

- Model-backed (booru/VLM) semantic tagger for Tier A tags.
- WebSocket broadcasts of embedding progress; currently optional and absent.
- ANN index (HNSW) if brute-force search becomes too slow.
- Per-user profiles.
- Representative-frame embeddings for videos.
- Near-duplicate (phash) prefilter.

## Risks

| Risk | Mitigation |
|---|---|
| SigLIP relevance or CPU latency is poor on some libraries | Low-level provider remains available by leaving `EMBED_MODEL_PATH` empty. |
| Cold start with no ratings | Content similarity works immediately; the profile term activates only with enough ratings. |
| Re-embed storms after a model change | Versioned embedder tag, persisted queue, bounded concurrency. |
| Recommendations too same-y | MMR diversity plus gallery/source penalties. |
| Database growth | 768 × 4 bytes per vector; index is in-memory and rebuilds from the database at boot. |
