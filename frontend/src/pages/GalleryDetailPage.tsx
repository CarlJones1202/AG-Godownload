import { useState, useMemo } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient, useQueries } from '@tanstack/react-query';
import { galleries, images as imagesApi, people } from '@/lib/api';
import { formatDate, parseColors, thumbnailUrl, imageUrl } from '@/lib/utils';
import {
  Spinner,
  EmptyState,
  Badge,
  Button,
  ConfirmDialog,
  Input,
} from '@/components/UI';
import { JustifiedGrid } from '@/components/JustifiedGrid';
import type { GallerySearchResult } from '@/types';
import type { JustifiedItem } from '@/components/JustifiedGrid';
import { Lightbox } from '@/components/Lightbox';

import {
  Heart,
  ArrowLeft,
  Trash2,
  Edit2,
  Save,
  X,
  Calendar,
  ExternalLink,
  FileText,
  Star,
  Settings2,
} from 'lucide-react';
import { cn } from '@/lib/utils';

function parsePhotos(photos?: string): string[] {
  if (!photos) return [];
  try { const p = JSON.parse(photos); return Array.isArray(p) ? p : []; } catch { return []; }
}

export function GalleryDetailPage() {
  const { id } = useParams<{ id: string }>();
  const galleryId = Number(id);
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const [confirmDeleteGallery, setConfirmDeleteGallery] = useState(false);
  const [confirmDeleteImageId, setConfirmDeleteImageId] = useState<number | null>(null);
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);
  const [isEditingTitle, setIsEditingTitle] = useState(false);
  const [editedTitle, setEditedTitle] = useState('');
  const [sortBy, setSortBy] = useState<'newest' | 'oldest' | 'largest' | 'smallest'>('newest');
  const [showTools, setShowTools] = useState(false);
  const [searchResults, setSearchResults] = useState<GallerySearchResult[] | null>(null);

  const [scrapeProvider, setScrapeProvider] = useState('');
  const [scrapeUrl, setScrapeUrl] = useState('');
  const [updateProvider, setUpdateProvider] = useState('');
  const [updateSourceUrl, setUpdateSourceUrl] = useState('');
  const [addImageUrl, setAddImageUrl] = useState('');

  const { data: gallery, isLoading: loadingGallery } = useQuery({
    queryKey: ['gallery', galleryId],
    queryFn: () => galleries.get(galleryId),
  });

  const { data: imageList, isLoading: loadingImages } = useQuery({
    queryKey: ['images', { gallery_id: galleryId, sort_by: sortBy }],
    queryFn: () => imagesApi.list({ gallery_id: galleryId, limit: 200, sort_by: sortBy }),
  });

  const { data: linkedPeople } = useQuery({
    queryKey: ['gallery-people', galleryId],
    queryFn: () => galleries.people(galleryId),
  });

  const { data: allPeople } = useQuery({
    queryKey: ['people-all'],
    queryFn: () => people.list({ limit: 5000 }),
    enabled: !!gallery?.name,
  });

  const candidatePeople = useMemo(() => {
    if (!gallery?.name || !allPeople?.data) return [];
    const galleryNameLower = gallery.name.toLowerCase();
    const linkedIds = new Set((linkedPeople ?? []).map((p: any) => p.id));
    return allPeople.data.filter((p) => {
      if (linkedIds.has(p.id)) return true;
      return galleryNameLower.includes(p.name.toLowerCase());
    });
  }, [gallery?.name, allPeople?.data, linkedPeople]);

  const candidateScansQueries = useQueries({
    queries: candidatePeople.map((person) => ({
      queryKey: ['person-scans', person.id],
      queryFn: () => people.getScans(person.id),
      enabled: !!person.id,
    })),
  });

  interface AutoSuggestion extends GallerySearchResult { personId: number; personName: string; }

  const autoSuggestions = useMemo(() => {
    if (!gallery?.name) return [] as AutoSuggestion[];
    const suggestions: AutoSuggestion[] = [];
    const seenUrls = new Set<string>();
    const galleryNameNorm = gallery.name.toLowerCase().replace(/[^a-z0-9]/g, ' ').replace(/\s+/g, ' ').trim();
    for (let i = 0; i < candidatePeople.length; i++) {
      const person = candidatePeople[i];
      const query = candidateScansQueries[i];
      if (!query?.data) continue;
      for (const scan of query.data) {
        const missingGals = scan.results?.missing_galleries || [];
        for (const mg of missingGals) {
          if (!mg.title) continue;
          const mgNorm = mg.title.toLowerCase().replace(/[^a-z0-9]/g, ' ').replace(/\s+/g, ' ').trim();
          const match = galleryNameNorm === mgNorm || galleryNameNorm.includes(mgNorm) || mgNorm.includes(galleryNameNorm);
          if (match && mg.url !== gallery.source_url && !seenUrls.has(mg.url)) {
            seenUrls.add(mg.url);
            suggestions.push({ ...mg, provider: scan.provider || mg.provider || '', personId: person.id, personName: person.name });
          }
        }
      }
    }
    return suggestions;
  }, [candidateScansQueries, candidatePeople, gallery?.name, gallery?.source_url]);

  const favMut = useMutation({
    mutationFn: (imgId: number) => imagesApi.toggleFavorite(imgId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['images', { gallery_id: galleryId }] }),
  });

  const deleteGalleryMut = useMutation({
    mutationFn: () => galleries.delete(galleryId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['galleries'] }); navigate('/galleries'); },
  });

  const updateTitleMut = useMutation({
    mutationFn: (name: string) => galleries.update(galleryId, { name }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['gallery', galleryId] }); setIsEditingTitle(false); },
  });

  const deleteImageMut = useMutation({
    mutationFn: (imgId: number) => imagesApi.delete(imgId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['images', { gallery_id: galleryId }] }); setConfirmDeleteImageId(null); },
  });

  const unlinkPersonMut = useMutation({
    mutationFn: (personId: number) => people.unlinkGallery(personId, galleryId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['gallery-people', galleryId] }),
  });

  const searchMetaMut = useMutation({
    mutationFn: () => galleries.searchMetadata(galleryId),
    onSuccess: (data) => setSearchResults(data.results),
  });

  const scrapeMetaMut = useMutation({
    mutationFn: (params: { provider: string; source_url: string }) => galleries.scrapeMetadata(galleryId, params),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['gallery', galleryId] }); searchMetaMut.mutate(); },
  });

  const scrapeAndLinkMut = useMutation({
    mutationFn: async (params: { provider: string; source_url: string; personId: number }) => {
      await galleries.scrapeMetadata(galleryId, { provider: params.provider, source_url: params.source_url });
      await people.linkGallery(params.personId, galleryId);
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['gallery', galleryId] }); queryClient.invalidateQueries({ queryKey: ['gallery-people', galleryId] }); queryClient.invalidateQueries({ queryKey: ['person-scans'] }); },
  });

  const updateProviderMut = useMutation({
    mutationFn: () => galleries.updateProvider(galleryId, { provider: updateProvider, source_url: updateSourceUrl || undefined }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['gallery', galleryId] }); setUpdateProvider(''); setUpdateSourceUrl(''); },
  });

  const setCoverMut = useMutation({
    mutationFn: (imageId: number) => galleries.update(galleryId, { cover_image_id: imageId }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['gallery', galleryId] }); queryClient.invalidateQueries({ queryKey: ['galleries'] }); queryClient.invalidateQueries({ queryKey: ['images', { gallery_id: galleryId }] }); },
  });

  const addImageMut = useMutation({
    mutationFn: () => galleries.addImage(galleryId, { url: addImageUrl }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['images', { gallery_id: galleryId }] }); setAddImageUrl(''); },
  });

  const gridItems: JustifiedItem[] = useMemo(() => {
    if (!imageList) return [];
    return imageList.data.map((img) => {
      const colors = parseColors(img.dominant_colors);
      return {
        id: img.id,
        src: imageUrl(img.filename),
        thumbSrc: thumbnailUrl(img.filename),
        width: img.width, height: img.height,
        persistentOverlay: img.is_favorite ? (
          <div className="absolute bottom-0 left-0 p-2 pointer-events-auto">
            <button onClick={(e) => { e.stopPropagation(); favMut.mutate(img.id); }} className="p-1">
              <Heart size={16} className="fill-red-500 text-red-500" />
            </button>
          </div>
        ) : undefined,
        overlay: (
          <div className="flex flex-col justify-end h-full bg-gradient-to-t from-black/60 to-transparent p-2">
            <div className="flex items-center justify-between w-full">
              <div className="flex items-center gap-1">
                <button onClick={(e) => { e.stopPropagation(); favMut.mutate(img.id); }} className="p-1">
                  <Heart size={16} className={cn(img.is_favorite ? 'fill-red-500 text-red-500' : 'text-white')} />
                </button>
                <button onClick={(e) => { e.stopPropagation(); setConfirmDeleteImageId(img.id); }} className="p-1" title="Delete image">
                  <Trash2 size={16} className="text-white hover:text-red-400" />
                </button>
                <button onClick={(e) => { e.stopPropagation(); if (img.id === gallery?.cover_image_id) return; setCoverMut.mutate(img.id); }}
                  className={cn('p-1', img.id === gallery?.cover_image_id ? 'opacity-100' : '')}
                  title={img.id === gallery?.cover_image_id ? 'Gallery cover' : 'Set as gallery cover'}
                  disabled={setCoverMut.isPending}>
                  <Star size={16} className={cn(img.id === gallery?.cover_image_id ? 'text-amber-400 fill-amber-400' : 'text-white')} />
                </button>
              </div>
              <div className="flex items-center gap-1">
                {img.is_video && <Badge variant="info">Video</Badge>}
                {colors.length > 0 && (
                  <div className="flex h-2 rounded overflow-hidden">
                    {colors.map((c, i) => <div key={i} className="w-2" style={{ backgroundColor: c }} />)}
                  </div>
                )}
              </div>
            </div>
          </div>
        ),
      };
    });
  }, [imageList, favMut, gallery]);

  const lightboxImages = useMemo(() => {
    if (!imageList) return [];
    return imageList.data.map((img) => ({ src: imageUrl(img.filename), alt: img.filename }));
  }, [imageList]);

  if (loadingGallery) return <Spinner />;
  if (!gallery) return <EmptyState message="Gallery not found." />;

  const coverImageSrc = (() => {
    if (gallery.provider_thumbnail) {
      const filename = gallery.provider_thumbnail.replace(/\\/g, '/').split('/').pop()!;
      return { type: 'provider' as const, src: thumbnailUrl(filename) };
    }
    if (gallery.cover_image_id && imageList?.data) {
      const img = imageList.data.find((i) => i.id === gallery.cover_image_id);
      if (img) {
        const thumb = img.thumbnail_path?.startsWith('/') ? img.thumbnail_path : thumbnailUrl(img.filename);
        return { type: 'image' as const, src: thumb };
      }
    }
    if (imageList?.data && imageList.data.length > 0) {
      const img = imageList.data[0];
      const thumb = img.thumbnail_path?.startsWith('/') ? img.thumbnail_path : thumbnailUrl(img.filename);
      return { type: 'image' as const, src: thumb };
    }
    return null;
  })();

  return (
    <>
      <div className="relative">
        {/* Immersive Background */}
        <div className="absolute inset-x-0 -top-6 -mx-6 sm:-mx-8 h-[800px] pointer-events-none select-none overflow-hidden">
          {coverImageSrc ? (
            <div className="h-full w-full" style={{ maskImage: 'linear-gradient(to bottom, black 0%, transparent 100%)', WebkitMaskImage: 'linear-gradient(to bottom, black 0%, transparent 100%)' }}>
              <img src={coverImageSrc.src} alt="" className="h-full w-full object-cover scale-150 blur-[120px] opacity-60" />
            </div>
          ) : (
            <div className="h-full w-full bg-zinc-900" />
          )}
        </div>

        <div className="relative z-10">
          <Link to="/galleries" className="inline-flex items-center gap-1.5 text-sm text-zinc-400 hover:text-zinc-200 transition-colors mb-8">
            <ArrowLeft size={16} />
            Back to galleries
          </Link>

          <div className="flex flex-col md:flex-row gap-6 md:items-end mb-10">
            <div className="relative shrink-0">
              <div className="relative w-36 h-48 md:w-44 md:h-60 rounded-lg overflow-hidden bg-zinc-800 ring-1 ring-white/10 shadow-lg">
                {coverImageSrc ? (
                  <img src={coverImageSrc.src} alt={gallery.name} className="w-full h-full object-cover" />
                ) : (
                  <div className="w-full h-full flex items-center justify-center text-zinc-600">
                    <FileText size={36} />
                  </div>
                )}
              </div>
            </div>

            <div className="flex-1 min-w-0">
              <div className="flex flex-wrap items-center gap-2 mb-2">
                {gallery.provider && (
                  <span className="text-[10px] uppercase tracking-wider font-semibold text-blue-400 bg-blue-500/10 px-2 py-0.5 rounded">{gallery.provider}</span>
                )}
                {imageList && (
                  <span className="text-xs text-zinc-400">{imageList.meta.total_items} images</span>
                )}
              </div>

              {isEditingTitle ? (
                <div className="flex items-center gap-2 mb-3 max-w-xl">
                  <Input value={editedTitle} onChange={(e) => setEditedTitle(e.target.value)} className="text-lg font-bold bg-white/5 border-white/10 h-10" autoFocus onKeyDown={(e) => e.key === 'Enter' && updateTitleMut.mutate(editedTitle)} />
                  <Button size="sm" onClick={() => updateTitleMut.mutate(editedTitle)} disabled={updateTitleMut.isPending}><Save size={16} /></Button>
                  <Button size="sm" variant="ghost" onClick={() => setIsEditingTitle(false)}><X size={16} /></Button>
                </div>
              ) : (
                <h1 className="text-2xl md:text-4xl font-bold text-white mb-2 tracking-tight break-words">
                  {gallery.name || `Gallery #${gallery.id}`}
                </h1>
              )}

              <div className="flex flex-wrap items-center gap-2 text-xs text-zinc-500">
                {!isEditingTitle && (
                  <button onClick={() => { setEditedTitle(gallery.name || ''); setIsEditingTitle(true); }} className="p-1.5 rounded bg-white/5 hover:bg-white/10 text-zinc-400 hover:text-white transition-all" title="Edit title">
                    <Edit2 size={14} />
                  </button>
                )}
                <span className="flex items-center gap-1"><Calendar size={12} /> Added {formatDate(gallery.created_at)}</span>
                {gallery.release_date && <span className="flex items-center gap-1"><Star size={12} className="text-amber-400" /> Released {gallery.release_date}</span>}
              </div>
            </div>

            <div className="flex items-center gap-2 shrink-0">
              <select value={sortBy} onChange={(e) => setSortBy(e.target.value as typeof sortBy)}
                className="bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-1.5 text-xs text-zinc-200 focus:outline-none cursor-pointer">
                <option value="newest">Newest</option>
                <option value="oldest">Oldest</option>
                <option value="largest">Largest</option>
                <option value="smallest">Smallest</option>
              </select>
              <Button variant="ghost" size="sm" onClick={() => setShowTools(!showTools)}>
                <Settings2 size={14} /> Tools
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setConfirmDeleteGallery(true)} className="text-zinc-500 hover:text-red-400">
                <Trash2 size={14} />
              </Button>
            </div>
          </div>
        </div>

        {/* Info Cards */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-8">
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4">
            <h3 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-2">Source</h3>
            {gallery.source_url ? (
              <a href={gallery.source_url} target="_blank" rel="noopener noreferrer" className="flex items-center gap-2 p-2 rounded-lg bg-zinc-800/50 hover:bg-blue-500/10 hover:border-blue-500/30 border border-zinc-700 transition-all group">
                <span className="flex-1 text-sm text-blue-400 truncate">{gallery.source_url}</span>
                <ExternalLink size={12} className="text-blue-500 opacity-0 group-hover:opacity-100 transition-opacity shrink-0" />
              </a>
            ) : (
              <p className="text-sm text-zinc-500 italic">No source URL.</p>
            )}
          </div>

          <div className="md:col-span-2 rounded-lg border border-zinc-800 bg-zinc-900/30 p-4">
            <div className="flex items-center justify-between mb-2">
              <h3 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider">Description</h3>
              {gallery.rating != null && gallery.rating > 0 && (
                <span className="flex items-center gap-1 text-xs text-amber-400"><Star size={12} className="fill-amber-400" />{gallery.rating.toFixed(1)}</span>
              )}
            </div>
            {gallery.description ? (
              <p className="text-sm text-zinc-300 leading-relaxed">{gallery.description}</p>
            ) : (
              <p className="text-sm text-zinc-500 italic">No description provided.</p>
            )}
          </div>

          <div className="md:col-span-3 rounded-lg border border-zinc-800 bg-zinc-900/30 p-4">
            <div className="flex items-center gap-2 mb-3">
              <h3 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider">Linked People</h3>
              <span className="text-[10px] text-zinc-500 bg-zinc-800 px-1.5 py-0.5 rounded">{linkedPeople?.length ?? 0}</span>
            </div>
            {!linkedPeople || linkedPeople.length === 0 ? (
              <p className="text-sm text-zinc-500 italic">No performers linked to this gallery.</p>
            ) : (
              <div className="flex flex-wrap gap-2">
                {linkedPeople.map((person) => {
                  const photo = parsePhotos(person.photos)[0];
                  return (
                    <div key={person.id} className="flex items-center gap-2 rounded-full bg-zinc-800/50 border border-zinc-700 pr-3 pl-1 py-1 hover:bg-zinc-700/50 transition-all cursor-pointer"
                      onClick={() => navigate(`/people/${person.id}`)}>
                      <div className="w-7 h-7 rounded-full overflow-hidden bg-zinc-700">
                        {photo ? <img src={photo} alt={person.name} className="w-full h-full object-cover" />
                        : <div className="w-full h-full flex items-center justify-center text-[9px] text-zinc-500 font-bold">{person.name.charAt(0)}</div>}
                      </div>
                      <span className="text-xs text-zinc-300">{person.name}</span>
                      <button onClick={(e) => { e.stopPropagation(); unlinkPersonMut.mutate(person.id); }}
                        className="p-0.5 rounded-full text-zinc-500 hover:text-red-400 opacity-0 group-hover:opacity-100 transition-all" title="Unlink">
                        <X size={10} />
                      </button>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>

        {/* Tools section */}
        {showTools && (
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4 mb-6 space-y-4">
            <h3 className="text-sm font-medium text-zinc-200">Gallery Management Tools</h3>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {/* Search & Missing */}
              <div className="md:col-span-2 bg-zinc-900/40 p-3 rounded-lg border border-zinc-800 space-y-3">
                <div className="flex items-center justify-between">
                  <h4 className="text-xs font-semibold text-zinc-400">Search & Missing Galleries</h4>
                  <Button size="sm" onClick={() => searchMetaMut.mutate()} disabled={searchMetaMut.isPending}>
                    {searchMetaMut.isPending ? 'Searching...' : 'Search'}
                  </Button>
                </div>
                {!searchMetaMut.isPending && !searchResults && autoSuggestions.length > 0 && (
                  <div className="space-y-2">
                    <p className="text-[10px] uppercase font-bold text-amber-400">Suggested scrapes:</p>
                    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-2">
                      {autoSuggestions.map((r, idx) => (
                        <div key={idx} className="relative flex flex-col rounded-lg overflow-hidden border border-zinc-800 bg-zinc-900/80 hover:border-zinc-700 transition-all group">
                          <div className="aspect-[2/3] bg-zinc-800 overflow-hidden">
                            {r.thumbnail ? <img src={r.thumbnail} alt={r.title} className="w-full h-full object-cover" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />
                            : <div className="w-full h-full flex items-center justify-center text-zinc-600 text-[9px]">No thumb</div>}
                          </div>
                          <div className="p-1.5 space-y-0.5">
                            <p className="text-[9px] text-zinc-200 font-medium leading-tight line-clamp-2">{r.title}</p>
                            <span className="text-[8px] text-zinc-500">{r.provider}</span>
                          </div>
                          <div className="absolute inset-0 bg-black/60 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center gap-1">
                            <Button size="sm" className="h-6 px-1.5 text-[9px]" disabled={scrapeMetaMut.isPending}
                              onClick={() => scrapeAndLinkMut.mutate({ provider: r.provider, source_url: r.url, personId: r.personId })}>
                              {scrapeMetaMut.isPending ? '...' : 'Scrape'}
                            </Button>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
                {searchResults && searchResults.length > 0 && (
                  <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-2">
                    {searchResults.map((r, idx) => {
                      const isLinked = !!r.id;
                      return (
                        <div key={idx} className={`relative flex flex-col rounded-lg overflow-hidden border transition-all ${isLinked ? 'border-emerald-500/30 bg-emerald-500/5' : 'border-zinc-800 bg-zinc-900/80 hover:border-zinc-700'} group`}>
                          <div className="aspect-[2/3] bg-zinc-800 overflow-hidden">
                            {r.thumbnail ? <img src={r.thumbnail} alt={r.title} className="w-full h-full object-cover" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />
                            : <div className="w-full h-full flex items-center justify-center text-zinc-600 text-[9px]">No thumb</div>}
                          </div>
                          <div className="p-1.5 space-y-0.5">
                            <p className="text-[9px] text-zinc-200 font-medium leading-tight line-clamp-2">{r.title}</p>
                            <span className={`text-[8px] ${isLinked ? 'text-emerald-400' : 'text-amber-400'}`}>{isLinked ? 'Linked' : 'Missing'}</span>
                          </div>
                          {!isLinked && (
                            <div className="absolute inset-0 bg-black/60 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                              <Button size="sm" className="h-6 px-1.5 text-[9px]" disabled={scrapeMetaMut.isPending}
                                onClick={() => scrapeMetaMut.mutate({ provider: r.provider, source_url: r.url })}>
                                {scrapeMetaMut.isPending ? '...' : 'Scrape'}
                              </Button>
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}
                {searchResults && searchResults.length === 0 && (
                  <p className="text-xs text-zinc-500 italic">No matching galleries found.</p>
                )}
              </div>

              <div className="bg-zinc-900/40 p-3 rounded-lg border border-zinc-800 space-y-2">
                <h4 className="text-xs font-semibold text-zinc-400">Scrape Metadata</h4>
                <div className="flex gap-2">
                  <Input placeholder="Provider" value={scrapeProvider} onChange={(e) => setScrapeProvider(e.target.value)} className="h-8 text-xs" />
                  <Input placeholder="Gallery URL" value={scrapeUrl} onChange={(e) => setScrapeUrl(e.target.value)} className="h-8 text-xs flex-[2]" />
                </div>
                <Button size="sm" onClick={() => scrapeMetaMut.mutate({ provider: scrapeProvider, source_url: scrapeUrl })} disabled={!scrapeProvider || !scrapeUrl || scrapeMetaMut.isPending}>
                  {scrapeMetaMut.isPending ? 'Scraping...' : 'Scrape'}
                </Button>
              </div>

              <div className="bg-zinc-900/40 p-3 rounded-lg border border-zinc-800 space-y-2">
                <h4 className="text-xs font-semibold text-zinc-400">Update Provider</h4>
                <div className="flex gap-2">
                  <Input placeholder="Provider name" value={updateProvider} onChange={(e) => setUpdateProvider(e.target.value)} className="h-8 text-xs" />
                  <Input placeholder="Source URL (optional)" value={updateSourceUrl} onChange={(e) => setUpdateSourceUrl(e.target.value)} className="h-8 text-xs flex-[2]" />
                </div>
                <Button size="sm" onClick={() => updateProviderMut.mutate()} disabled={!updateProvider || updateProviderMut.isPending}>
                  {updateProviderMut.isPending ? 'Updating...' : 'Update'}
                </Button>
              </div>

              <div className="bg-zinc-900/40 p-3 rounded-lg border border-zinc-800 space-y-2">
                <h4 className="text-xs font-semibold text-zinc-400">Add Image</h4>
                <Input placeholder="Image URL to download" value={addImageUrl} onChange={(e) => setAddImageUrl(e.target.value)} className="h-8 text-xs" />
                <Button size="sm" onClick={() => addImageMut.mutate()} disabled={!addImageUrl || addImageMut.isPending}>
                  {addImageMut.isPending ? 'Downloading...' : 'Download & Add'}
                </Button>
              </div>
            </div>
          </div>
        )}

        {/* Image Grid */}
        <section>
          {loadingImages ? (
            <Spinner />
          ) : !imageList || imageList.data.length === 0 ? (
            <EmptyState message="No images in this gallery." />
          ) : (
            <JustifiedGrid items={gridItems} rowHeight={230} gap={4} onItemClick={(index) => setLightboxIndex(index)} />
          )}
        </section>
      </div>

      {lightboxIndex !== null && (
        <Lightbox images={lightboxImages} index={lightboxIndex} onClose={() => setLightboxIndex(null)}
          onIndexChange={setLightboxIndex} imageData={imageList!.data} onToggleFavorite={(id) => favMut.mutate(id)} />
      )}

      <ConfirmDialog open={confirmDeleteGallery} title="Delete Gallery"
        message="Delete this gallery and all its images? Files will be removed from disk. This cannot be undone."
        confirmLabel="Delete Gallery" onConfirm={() => deleteGalleryMut.mutate()} onCancel={() => setConfirmDeleteGallery(false)} />

      <ConfirmDialog open={confirmDeleteImageId !== null} title="Delete Image"
        message="Delete this image? The file will be removed from disk. This cannot be undone."
        confirmLabel="Delete Image"
        onConfirm={() => { if (confirmDeleteImageId !== null) deleteImageMut.mutate(confirmDeleteImageId); }}
        onCancel={() => setConfirmDeleteImageId(null)} />


    </>
  );
}
