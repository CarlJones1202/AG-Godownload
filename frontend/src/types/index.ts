export interface Source {
  id: number;
  created_at: string;
  updated_at: string;
  name: string;
  type: string;
  location: string;
  last_checked_at: string;
  status: string;
  priority: number;
  download_progress: number;
  downloaded_items: number;
  total_items: number;
}

export interface Gallery {
  id: number;
  created_at: string;
  updated_at: string;
  name: string;
  description?: string;
  source_id?: number;
  source?: Source;
  images?: Image[];
  people?: Person[];
  provider?: string;
  source_url?: string;
  provider_thumbnail?: string;     // Local path to downloaded thumbnail
  provider_thumbnail_url?: string; // Original URL for re-download
  cover_image_id?: number | null;
  rating?: number;
  release_date?: string;
  image_count?: number;
}

export interface PaginatedResult<T> {
  data: T[];
  meta: {
    current_page: number;
    total_pages: number;
    total_items: number;
    limit: number;
  };
}

export interface Image {
  id: number;
  created_at: string;
  updated_at: string;
  filename: string;
  title?: string;
  title_customized?: boolean;
  original_url?: string;
  gallery_id?: number;
  type: string;
  width?: number;
  height?: number;
  file_size?: number;
  file_hash?: string;
  dominant_colors?: string;
  is_favorite: boolean;
  is_video?: boolean;
  vr_mode: string;
  duration_seconds?: number;
  tags?: Tag[];
  thumbnail_path?: string;
}

export interface RatingProfile {
  n_likes: number;
  n_dislikes: number;
  ready: boolean;
}

export interface RatingResponse {
  rating: number;
  profile?: RatingProfile;
}

export interface SimilarImage {
  id: number;
  filename: string;
  similarity: number;
  reasons: string[];
  web_path: string;
  thumbnail_path: string;
  favorite: boolean;
  gallery_id?: number;
}

export interface SimilarImagesResponse {
  seed: {
    id: number;
    filename: string;
    tags: string[];
    web_path: string;
    thumbnail_path: string;
    embedded: boolean;
  };
  profile: RatingProfile;
  ids_b64?: string;
  data: SimilarImage[];
}

export interface EmbedStatus {
  total_images: number;
  embedded: number;
  pending: number;
  failed: number;
  deferred?: number;
  index_size: number;
  embedder: string;
  dimension: number;
}

export interface Person {
  id: number;
  created_at: string;
  updated_at: string;
  name: string;
  aliases?: string;
  nationality?: string;
  ethnicity?: string;
  hair_color?: string;
  eye_color?: string;
  height?: string;
  weight?: string;
  measurements?: string;
  tattoos?: string;
  piercings?: string;
  biography?: string;
  photos?: string;
  profile_image_id?: number | null;
  profile_image_path?: string;
  birth_date?: string;
  gallery_count?: number;
  galleries?: Gallery[];
  thumbnail_path?: string;
}

export interface PersonIdentifier {
  id: number;
  person_id: number;
  provider: string;
  external_id: string;
  created_at: string;
}

export interface GallerySearchResult {
  provider: string;
  title: string;
  url: string;
  thumbnail: string;
  release_date?: string;
  source_id?: string;
  id?: number;
}

export interface GalleryMetadata {
  provider: string;
  description: string;
  rating: number;
  release_date: string;
  source_url: string;
  thumbnail_url: string;
}

export interface ColorSearchResult {
  image: Image;
  distance: number;
}

export interface PaginationParams {
  limit?: number;
  offset?: number;
  [key: string]: unknown;
}

export interface Tag {
  id: number;
  name: string;
  count: number;
}

export interface PersonScanResult {
  id: number;
  person_id: number;
  source: string;
  provider?: string;
  alias?: string;
  status: string;
  results: any;
  created_at: string;
}

export interface ProviderAlias {
  id: number;
  person_id: number;
  provider: string;
  alias: string;
}

export interface DownloadStatus {
  crawler: {
    active_sources: ActiveSource[];
  };
  verification: {
    is_running: boolean;
    total_images: number;
    processed: number;
    missing_found: number;
    recovered: number;
    provider_status: Record<string, { active: number; max_allowed: number }>;
    active_downloads: ActiveSource[];
  };
  videos: {
    is_running: boolean;
    total_videos: number;
    processed: number;
    missing_found: number;
    recovered: number;
    active: number;
    max_allowed: number;
    active_downloads: ActiveSource[];
  };
}

export interface TagSuggestion {
  type: 'gallery' | 'video';
  id: number;
  name: string;
  matched_on: string;
  confidence: number;
}

export interface AutoTagResult {
  galleries_tagged: number;
  videos_tagged: number;
  suggestions: TagSuggestion[];
}

export interface PersonExclusion {
  id: number;
  created_at: string;
  person_id: number;
  gallery_id?: number;
  image_id?: number;
}

export interface PersonScanResponse {
  person_id: number;
  person_name: string;
  provider: string;
  found_count: number;
  existing_count: number;
  unsure_count: number;
  missing_count: number;
  missing_galleries: GallerySearchResult[];
  unsure_galleries: GallerySearchResult[];
}

export interface MissingGallery {
  person_id: number;
  person_name: string;
  provider: string;
  alias: string;
  gallery_url: string;
  gallery_name: string;
  thumbnail: string;
  found_count: number;
  missing_count: number;
  release_date: string;
}

export interface MissingGalleriesMeta {
  current_page: number;
  total_pages: number;
  total_items: number;
  limit: number;
  pending_scans: number;
}

export interface MissingGalleriesResponse {
  data: MissingGallery[];
  meta: MissingGalleriesMeta;
}

export interface NotWantedGallery {
  id: number;
  person_id: number;
  person_name: string;
  provider: string;
  source_url: string;
  title: string;
  reason?: string;
  created_at: string;
}

export interface IdentifierResult {
  external_id: string;
  name: string;
  disambiguation: string;
  preview_data: Record<string, unknown>;
}

export interface AttentionCounts {
  missing_galleries: number;
  missing_images: number;
  missing_videos: number;
  failed_sources: number;
  embed_pending: number;
  embed_failed: number;
  embed_deferred: number;
}

export interface DashboardStats {
  sources: number;
  galleries: number;
  images: number;
  videos: number;
  people: number;
  downloads: DownloadStatus;
  attention?: AttentionCounts;
}

export interface ActiveSource {
  id: number;
  name: string;
  location: string;
  source_name: string;
  download_progress: number;
  downloaded_items: number;
  total_items: number;
  is_favorite?: boolean;
  type?: string;
}
