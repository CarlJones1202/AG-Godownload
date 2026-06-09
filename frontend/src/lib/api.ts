import type {
  AutoTagResult,
  DashboardStats,
  DownloadStatus,
  Gallery,
  GallerySearchResult,
  IdentifierResult,
  Image,
  PaginatedResult,
  PaginationParams,
  Person,
  PersonExclusion,
  PersonIdentifier,
  PersonScanResult,
  ProviderAlias,
  Source,
  Tag,
} from '@/types';

const BASE = '/api';

class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...init?.headers },
    ...init,
  });
  if (res.status === 204) return undefined as T;
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new ApiError(res.status, body.error ?? res.statusText);
  }
  return res.json() as Promise<T>;
}

function qs(params: Record<string, unknown>): string {
  const entries = Object.entries(params).filter(([, v]) => v !== undefined && v !== '' && v !== false);
  if (entries.length === 0) return '';
  return '?' + new URLSearchParams(entries.map(([k, v]) => [k, String(v)]));
}

export const sources = {
  list: (params: PaginationParams & { q?: string } = {}) =>
    request<PaginatedResult<Source>>(`/sources${qs(params)}`),
  create: (data: { name: string; location: string; type?: string; priority?: number }) =>
    request<Source>('/sources', { method: 'POST', body: JSON.stringify(data) }),
  bulkCreate: (data: { url: string; name?: string }[]) =>
    request<{ results: { url: string; name: string; status: string; source_id?: number; error?: string }[]; summary: { total: number; created: number; duplicates: number; failed: number } }>(
      '/sources/bulk-import', { method: 'POST', body: JSON.stringify(data) },
    ),
  crawl: (id: number) =>
    request<{ message: string }>(`/sources/${id}/crawl`, { method: 'POST' }),
  delete: (id: number, deleteGallery?: boolean, deleteImages?: boolean) => {
    const params = new URLSearchParams();
    if (deleteGallery) params.set('delete_gallery', 'true');
    if (deleteImages) params.set('delete_images', 'true');
    const query = params.toString() ? `?${params.toString()}` : '';
    return request<void>(`/sources/${id}${query}`, { method: 'DELETE' });
  },
  updatePriority: (id: number, priority: number) =>
    request<{ message: string; priority: number }>(`/sources/${id}/priority`, {
      method: 'PATCH', body: JSON.stringify({ priority }),
    }),
};

export interface GalleryListParams extends PaginationParams {
  q?: string;
  search?: string;
  sort?: string;
  seed?: string;
  [key: string]: unknown;
}

export const galleries = {
  list: (params: GalleryListParams = {}) =>
    request<PaginatedResult<Gallery>>(`/galleries${qs(params)}`),
  get: (id: number) => request<Gallery>(`/galleries/${id}`),
  create: (data: Partial<Gallery>) =>
    request<Gallery>('/galleries', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: number, data: Partial<Gallery>) =>
    request<Gallery>(`/galleries/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: number) => request<void>(`/galleries/${id}`, { method: 'DELETE' }),
  searchMetadata: (id: number) =>
    request<{ results: GallerySearchResult[]; count: number }>(`/galleries/${id}/search-metadata`),
  scrapeMetadata: (id: number, data: { provider: string; source_url: string; source_id?: string }) =>
    request<{ message: string; gallery: Gallery; metadata: any }>(`/galleries/${id}/scrape-metadata`, { method: 'POST', body: JSON.stringify(data) }),
  updateProvider: (id: number, data: { provider: string; source_url?: string }) =>
    request<Gallery>(`/galleries/${id}/update-provider`, { method: 'POST', body: JSON.stringify(data) }),
  addImage: (id: number, data: { url: string; filename?: string }) =>
    request<Image>(`/galleries/${id}/images`, { method: 'POST', body: JSON.stringify(data) }),
  people: (id: number) =>
    request<Person[]>(`/galleries/${id}/people`),
};

export interface ImageListParams extends PaginationParams {
  gallery_id?: number;
  type?: string;
  favorites?: boolean;
  sort?: string;
  seed?: string;
  exists?: boolean;
  sort_by?: string;
  is_favorite?: boolean;
  is_video?: boolean;
  on_disk?: boolean;
  random_seed?: number;
  filter_tags?: string;
  [key: string]: unknown;
}

export const images = {
  list: (params: ImageListParams = {}) =>
    request<PaginatedResult<Image>>(`/images${qs(params)}`),
  get: (id: number) => request<Image>(`/images/${id}`),
  delete: (id: number) => request<void>(`/images/${id}`, { method: 'DELETE' }),
  toggleFavorite: (id: number) =>
    request<{ id: number; is_favorite: boolean }>(`/images/${id}/favorite`, { method: 'POST' }),
  updateVrMode: (id: number, vr_mode: string) =>
    request<{ id: number; vr_mode: string }>(`/images/${id}/vr-mode`, { method: 'PATCH', body: JSON.stringify({ vr_mode }) }),
  searchByColor: (color: string, limit?: number, threshold?: number) =>
    request<PaginatedResult<Image>>(`/search/color${qs({ color, limit, threshold })}`),
};

export const videos = {
  list: (params: PaginationParams = {}) =>
    request<PaginatedResult<Image>>(`/images${qs({ ...params, type: 'video' })}`),
  delete: (id: number) => request<void>(`/images/${id}`, { method: 'DELETE' }),
};

export interface PeopleListParams extends PaginationParams {
  q?: string;
  search?: string;
  [key: string]: unknown;
}

export const people = {
  autoTag: (id: number, minConfidence?: number, autoApply?: boolean) =>
    request<AutoTagResult>(`/people/${id}/auto-tag${qs({ minConfidence, autoApply })}`, { method: 'POST' }),
  applyAutoTagSuggestions: (id: number, suggestions: { type: string; id: number }[]) =>
    request<{ galleries_tagged: number; videos_tagged: number }>(`/people/${id}/auto-tag/apply`, {
      method: 'POST', body: JSON.stringify({ suggestions }),
    }),
  getExclusions: (id: number) =>
    request<PersonExclusion[]>(`/people/${id}/exclusions`),
  excludeGallery: (personId: number, galleryId: number) =>
    request<{ message: string }>(`/people/${personId}/exclude-gallery/${galleryId}`, { method: 'POST' }),
  excludeVideo: (personId: number, imageId: number) =>
    request<{ message: string }>(`/people/${personId}/exclude-video/${imageId}`, { method: 'POST' }),
  removeExclusion: (personId: number, exclusionId: number) =>
    request<void>(`/people/${personId}/exclusions/${exclusionId}`, { method: 'DELETE' }),
  list: (params: PeopleListParams = {}) =>
    request<PaginatedResult<Person>>(`/people${qs(params)}`),
  get: (id: number) => request<Person>(`/people/${id}`),
  create: (data: { name: string; aliases?: string; nationality?: string }) =>
    request<Person>('/people', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: number, data: Partial<Person>) =>
    request<Person>(`/people/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: number) => request<void>(`/people/${id}`, { method: 'DELETE' }),
  linkGallery: (personId: number, galleryId: number) =>
    request<{ message: string }>(`/people/${personId}/galleries/${galleryId}`, { method: 'POST' }),
  unlinkGallery: (personId: number, galleryId: number) =>
    request<{ message: string }>(`/people/${personId}/galleries/${galleryId}`, { method: 'DELETE' }),
  identifiers: (id: number) =>
    request<PersonIdentifier[]>(`/people/${id}/identifiers`),
  linkIdentifier: (id: number, data: { provider: string; external_id: string }) =>
    request<PersonIdentifier>(`/people/${id}/identifiers`, { method: 'POST', body: JSON.stringify(data) }),
  unlinkIdentifier: (personId: number, identifierId: number) =>
    request<void>(`/people/${personId}/identifiers/${identifierId}`, { method: 'DELETE' }),
  getStats: (id: number) =>
    request<{ videos: number; photos: number; galleries: number; tags: { name: string; count: number }[] }>(
      `/people/${id}/stats`,
    ),
  scanPerson: (id: number, source: string, alias?: string) =>
    request<{ message: string }>(`/people/${id}/scan${qs({ source, alias })}`),
  getScans: (id: number) =>
    request<PersonScanResult[]>(`/people/${id}/scans`),
  linkFoundGallery: (id: number, data: { provider: string; source_url: string; name: string; thumbnail_url?: string }) =>
    request<Gallery>(`/people/${id}/link-found-gallery`, { method: 'POST', body: JSON.stringify(data) }),
  linkUnsureGallery: (id: number, data: { gallery_id: number; provider: string; source_url: string }) =>
    request<Gallery>(`/people/${id}/link-unsure-gallery`, { method: 'POST', body: JSON.stringify(data) }),
  excludeScanResult: (id: number, data: { provider: string; source_url?: string; title?: string; reason?: string }) =>
    request<{ message: string }>(`/people/${id}/exclude-scan-result`, { method: 'POST', body: JSON.stringify(data) }),
  getProviderAliases: (id: number) =>
    request<ProviderAlias[]>(`/people/${id}/provider-aliases`),
  createProviderAlias: (id: number, data: { provider: string; alias: string }) =>
    request<ProviderAlias>(`/people/${id}/provider-aliases`, { method: 'POST', body: JSON.stringify(data) }),
  deleteProviderAlias: (personId: number, aliasId: number) =>
    request<void>(`/people/${personId}/provider-aliases/${aliasId}`, { method: 'DELETE' }),
  linkImage: (personId: number, imageId: number) =>
    request<{ message: string }>(`/people/${personId}/images/${imageId}`, { method: 'POST' }),
  unlinkImage: (personId: number, imageId: number) =>
    request<void>(`/people/${personId}/images/${imageId}`, { method: 'DELETE' }),
  autoLinkGalleries: (personId: number) =>
    request<{ message: string; galleries_linked: number }>(`/people/${personId}/link-galleries`, { method: 'POST' }),
  searchStashDB: (name: string) =>
    request<{ data: any[] }>(`/stashdb/search${qs({ name })}`),
  linkStashDB: (id: number, stash_id: string) =>
    request<{ message: string; person: Person }>(`/people/${id}/stashdb/link`, {
      method: 'POST',
      body: JSON.stringify({ stash_id }),
    }),
};

export const identifiers = {
  listSources: () =>
    request<{ sources: string[] }>('/identifiers/sources'),
  search: (source: string, name: string) =>
    request<{ data: IdentifierResult[] }>(`/identifiers/${source}/search${qs({ name })}`),
};

export const maintenance = {
  cleanupDupes: (token: string) =>
    request<{ total_images: number; deleted: number; url_duplicates: number; filename_duplicates: number; note: string }>(
      '/cleanup-dupes', { method: 'POST', headers: { 'X-Maintenance-Token': token, 'Content-Type': 'application/json' } },
    ),
};

export const admin = {
  missingGalleries: (_params?: PaginationParams & { q?: string; provider?: string; person_id?: number }) =>
    request<{ data: any[]; meta: any }>('/admin/missing-galleries'),
};

export const tagsApi = {
  list: () => request<Tag[]>('/tags'),
  top: (limit?: number) => request<Tag[]>(`/tags/top${qs({ limit })}`),
  search: (q: string) => request<Tag[]>(`/tags/search${qs({ q })}`),
  create: (name: string) => request<Tag>('/tags', { method: 'POST', body: JSON.stringify({ name }) }),
  linkToImage: (imageId: number, tagId: number) =>
    request<void>(`/images/${imageId}/tags/${tagId}`, { method: 'POST' }),
  unlinkFromImage: (imageId: number, tagId: number) =>
    request<void>(`/images/${imageId}/tags/${tagId}`, { method: 'DELETE' }),
};

export const downloadStatus = {
  get: () => request<DownloadStatus>('/downloads/status'),
};

export const stats = {
  dashboard: () => request<DashboardStats>('/api/stats'),
};
