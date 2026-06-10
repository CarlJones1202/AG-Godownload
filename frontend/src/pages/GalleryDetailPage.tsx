import { useState, useMemo } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
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
  Search,
  Download,
  Upload,
} from 'lucide-react';
import { cn } from '@/lib/utils';

function parsePhotos(photos?: string): string[] {
  if (!photos) return [];
  try {
    const parsed = JSON.parse(photos);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
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
  const [searchMetaQuery, setSearchMetaQuery] = useState('');
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

  const favMut = useMutation({
    mutationFn: (imgId: number) => imagesApi.toggleFavorite(imgId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images', { gallery_id: galleryId }] });
    },
  });

  const deleteGalleryMut = useMutation({
    mutationFn: () => galleries.delete(galleryId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['galleries'] });
      navigate('/galleries');
    },
  });

  const updateTitleMut = useMutation({
    mutationFn: (name: string) => galleries.update(galleryId, { name }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gallery', galleryId] });
      setIsEditingTitle(false);
    },
  });

  const startEditTitle = () => {
    setEditedTitle(gallery?.name || '');
    setIsEditingTitle(true);
  };

  const saveTitle = () => {
    updateTitleMut.mutate(editedTitle);
  };

  const cancelEditTitle = () => {
    setIsEditingTitle(false);
    setEditedTitle('');
  };

  const deleteImageMut = useMutation({
    mutationFn: (imgId: number) => imagesApi.delete(imgId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images', { gallery_id: galleryId }] });
      setConfirmDeleteImageId(null);
    },
  });

  const unlinkPersonMut = useMutation({
    mutationFn: (personId: number) => people.unlinkGallery(personId, galleryId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gallery-people', galleryId] });
    },
  });

  const searchMetaMut = useMutation({
    mutationFn: () => galleries.searchMetadata(galleryId),
    onSuccess: (data) => {
      setSearchMetaQuery(JSON.stringify(data.results, null, 2));
    },
  });

  const scrapeMetaMut = useMutation({
    mutationFn: () => galleries.scrapeMetadata(galleryId, { provider: scrapeProvider, source_url: scrapeUrl }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gallery', galleryId] });
      setScrapeProvider('');
      setScrapeUrl('');
    },
  });

  const updateProviderMut = useMutation({
    mutationFn: () => galleries.updateProvider(galleryId, { provider: updateProvider, source_url: updateSourceUrl || undefined }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gallery', galleryId] });
      setUpdateProvider('');
      setUpdateSourceUrl('');
    },
  });

  const setCoverMut = useMutation({
    mutationFn: (imageId: number) => galleries.update(galleryId, { cover_image_id: imageId }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gallery', galleryId] });
      queryClient.invalidateQueries({ queryKey: ['galleries'] });
      queryClient.invalidateQueries({ queryKey: ['images', { gallery_id: galleryId }] });
    },
  });

  const addImageMut = useMutation({
    mutationFn: () => galleries.addImage(galleryId, { url: addImageUrl }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images', { gallery_id: galleryId }] });
      setAddImageUrl('');
    },
  });

  // Build justified grid items from image list.
  const gridItems: JustifiedItem[] = useMemo(() => {
    if (!imageList) return [];
    return imageList.data.map((img) => {
      const colors = parseColors(img.dominant_colors);
      return {
        id: img.id,
        src: imageUrl(img.filename),
        thumbSrc: thumbnailUrl(img.filename),
        width: img.width,
        height: img.height,
        persistentOverlay: img.is_favorite ? (
          <div className="absolute bottom-0 left-0 p-2 pointer-events-auto">
            <button
              onClick={(e) => {
                e.stopPropagation();
                favMut.mutate(img.id);
              }}
              className="p-1"
            >
              <Heart size={16} className="fill-red-500 text-red-500" />
            </button>
          </div>
        ) : undefined,
        overlay: (
          <div className="flex flex-col justify-end h-full bg-gradient-to-t from-black/60 to-transparent p-2">
            <div className="flex items-center justify-between w-full">
              <div className="flex items-center gap-1">
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    favMut.mutate(img.id);
                  }}
                  className="p-1"
                >
                  <Heart
                    size={16}
                    className={cn(
                      img.is_favorite ? 'fill-red-500 text-red-500' : 'text-white',
                    )}
                  />
                </button>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    setConfirmDeleteImageId(img.id);
                  }}
                  className="p-1"
                  title="Delete image"
                >
                  <Trash2 size={16} className="text-white hover:text-red-400" />
                </button>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    if (img.id === gallery.cover_image_id) return;
                    setCoverMut.mutate(img.id);
                  }}
                  className={cn('p-1', img.id === gallery.cover_image_id ? 'opacity-100' : '')}
                  title={img.id === gallery.cover_image_id ? 'Gallery cover' : 'Set as gallery cover'}
                  disabled={setCoverMut.isLoading}
                >
                  <Star size={16} className={cn(img.id === gallery.cover_image_id ? 'text-amber-400 fill-amber-400' : 'text-white')} />
                </button>
              </div>
              <div className="flex items-center gap-1">
                {img.is_video && <Badge variant="info">Video</Badge>}
                {colors.length > 0 && (
                  <div className="flex h-2 rounded overflow-hidden">
                    {colors.map((c, i) => (
                      <div key={i} className="w-2" style={{ backgroundColor: c }} />
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>
        ),
      };
    });
  }, [imageList, favMut]);

  // Build lightbox images (full-size URLs).
  const lightboxImages = useMemo(() => {
    if (!imageList) return [];
    return imageList.data.map((img) => ({
      src: imageUrl(img.filename),
      alt: img.filename,
    }));
  }, [imageList]);

  if (loadingGallery) return <Spinner />;
  if (!gallery) return <EmptyState message="Gallery not found." />;

  // Determine gallery cover image source (preference: provider thumbnail -> manual cover image -> first image)
  const coverImageSrc = (() => {
    // Provider thumbnail (stored as local path to gallery_thumbnails)
    if (gallery.provider_thumbnail) {
      const filename = gallery.provider_thumbnail.replace(/\\/g, '/').split('/').pop()!;
      return { type: 'provider', src: thumbnailUrl(filename) };
    }

    // Manual cover image (cover_image_id) - try to find in imageList
    if (gallery.cover_image_id && imageList && imageList.data) {
      const img = imageList.data.find((i) => i.id === gallery.cover_image_id);
      if (img) {
        // Use thumbnail_path if present (absolute path), otherwise construct via thumbnailUrl
        const thumb = img.thumbnail_path && img.thumbnail_path.startsWith('/') ? img.thumbnail_path : thumbnailUrl(img.filename);
        return { type: 'image', src: thumb };
      }
    }

    // Fallback to first image in gallery
    if (imageList && imageList.data && imageList.data.length > 0) {
      const img = imageList.data[0];
      const thumb = img.thumbnail_path && img.thumbnail_path.startsWith('/') ? img.thumbnail_path : thumbnailUrl(img.filename);
      return { type: 'image', src: thumb };
    }

    return null;
  })();

  return (
    <>
    <div className="relative">
      {/* Immersive Background Layer with Masked Fade */}
      <div className="absolute inset-x-0 -top-6 -mx-6 h-[800px] pointer-events-none select-none overflow-hidden">
        {coverImageSrc ? (
          <div
            className="h-full w-full"
            style={{
              maskImage: 'linear-gradient(to bottom, black 0%, transparent 100%)',
              WebkitMaskImage: 'linear-gradient(to bottom, black 0%, transparent 100%)'
            }}
          >
            <img
              src={coverImageSrc.src}
              alt=""
              className="h-full w-full object-cover scale-150 blur-[120px] opacity-60"
            />
          </div>
        ) : (
          <div className="h-full w-full bg-zinc-900" />
        )}
      </div>

      <div className="relative z-10">
        <Link
          to="/galleries"
          className="group text-sm text-zinc-400 hover:text-white transition-colors inline-flex items-center gap-1.5 mb-8"
        >
          <div className="p-1.5 rounded-full bg-white/5 group-hover:bg-white/10 transition-colors">
            <ArrowLeft size={16} />
          </div>
          Back to galleries
        </Link>

        <div className="flex flex-col md:flex-row gap-8 items-start md:items-end mb-12">
          {/* Main Cover Image */}
          <div className="relative group shrink-0">
            <div className="absolute -inset-1 bg-gradient-to-r from-blue-500 to-purple-600 rounded-2xl blur opacity-25 group-hover:opacity-50 transition duration-1000 group-hover:duration-200" />
            <div className="relative w-48 h-64 md:w-56 md:h-80 rounded-xl overflow-hidden bg-zinc-800 shadow-2xl ring-1 ring-white/10">
              {coverImageSrc ? (
                <img
                  src={coverImageSrc.src}
                  alt={gallery.name}
                  className="w-full h-full object-cover"
                />
              ) : (
                <div className="w-full h-full flex items-center justify-center text-zinc-600">
                  <FileText size={48} />
                </div>
              )}
            </div>
          </div>

          <div className="flex-1 min-w-0 animate-fade-in-up">
            <div className="flex flex-wrap items-center gap-2 mb-3">
              {gallery.provider && (
                <Badge variant="info" className="px-2.5 py-1 text-[10px] uppercase tracking-wider font-bold">
                  {gallery.provider}
                </Badge>
              )}
              {imageList && (
                <Badge variant="default" className="bg-white/5 text-zinc-300 border border-white/10 px-2.5 py-1">
                  {imageList.meta.total_items} Images
                </Badge>
              )}
            </div>

            {isEditingTitle ? (
              <div className="flex items-center gap-2 mb-4 max-w-xl">
                <Input
                  value={editedTitle}
                  onChange={(e) => setEditedTitle(e.target.value)}
                  className="text-xl md:text-2xl font-bold bg-white/5 border-white/10 h-12"
                  autoFocus
                  onKeyDown={(e) => e.key === 'Enter' && saveTitle()}
                />
                <Button size="md" onClick={saveTitle} disabled={updateTitleMut.isPending} className="h-12 w-12 shrink-0">
                  <Save size={20} />
                </Button>
                <Button size="md" variant="ghost" onClick={cancelEditTitle} className="h-12 w-12 shrink-0">
                  <X size={20} />
                </Button>
              </div>
            ) : (
              <h1 className="text-3xl md:text-5xl font-bold text-white mb-4 tracking-tight drop-shadow-lg break-words">
                {gallery.name || `Gallery #${gallery.id}`}
              </h1>
            )}
 
            <div className="flex flex-wrap items-center gap-3">
              {!isEditingTitle && (
                <button
                  onClick={startEditTitle}
                  className="p-2 rounded-lg bg-white/5 hover:bg-white/10 text-zinc-400 hover:text-white transition-all"
                  title="Edit title"
                >
                  <Edit2 size={18} />
                </button>
              )}
              <div className="h-4 w-[1px] bg-white/10 mx-1" />
              <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-white/5 border border-white/5 text-xs text-zinc-400">
                <Calendar size={14} />
                <span>Added {formatDate(gallery.created_at)}</span>
              </div>
              {gallery.release_date && (
                <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-white/5 border border-white/5 text-xs text-zinc-400">
                  <Star size={14} className="text-amber-400" />
                  <span>Released {gallery.release_date}</span>
                </div>
              )}
            </div>
          </div>
 
          {/* Floating Action Bar */}
          <div className="flex items-center gap-2 glass p-1.5 rounded-xl shadow-xl ring-1 ring-white/10">
            <select
              value={sortBy}
              onChange={(e) => setSortBy(e.target.value as typeof sortBy)}
              className="bg-white/5 border border-transparent hover:border-white/10 rounded-lg px-3 py-1.5 text-xs text-zinc-200 focus:outline-none transition-all cursor-pointer"
            >
              <option value="newest">Newest first</option>
              <option value="oldest">Oldest first</option>
              <option value="largest">Largest first</option>
              <option value="smallest">Smallest first</option>
            </select>
            <div className="w-[1px] h-6 bg-white/10 mx-1" />
            <Button
              variant="danger"
              size="sm"
              onClick={() => setConfirmDeleteGallery(true)}
              className="bg-red-500/10 text-red-400 border-transparent hover:bg-red-500/20"
            >
              <Trash2 size={16} />
            </Button>
          </div>
        </div>
      </div>
 
    {/* Info Cards Grid */}
    <div className="grid grid-cols-1 md:grid-cols-3 gap-6 px-6 mb-12">
        {/* Source Card */}
        <div className="glass-card p-5 rounded-2xl flex flex-col gap-3">
          <div className="flex items-center gap-2 text-zinc-400 text-xs font-bold uppercase tracking-wider">
            <ExternalLink size={14} /> Source Information
          </div>
          {gallery.source_url ? (
            <div className="space-y-2">
              <a
                href={gallery.source_url}
                target="_blank"
                rel="noopener noreferrer"
                className="group flex items-center gap-2 p-2.5 rounded-xl bg-white/5 border border-white/5 hover:bg-blue-500/10 hover:border-blue-500/30 transition-all"
              >
                <div className="flex-1 min-w-0">
                  <p className="text-xs text-zinc-500 mb-0.5">Gallery URL</p>
                  <p className="text-sm text-blue-400 truncate">{gallery.source_url}</p>
                </div>
                <ExternalLink size={14} className="text-blue-500 opacity-0 group-hover:opacity-100 transition-opacity" />
              </a>
            </div>
          ) : (
            <p className="text-sm text-zinc-500 italic py-4">No source URL available.</p>
          )}
        </div>

        {/* Metadata Card */}
        <div className="md:col-span-2 glass-card p-5 rounded-2xl flex flex-col gap-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 text-zinc-400 text-xs font-bold uppercase tracking-wider">
              <FileText size={14} /> Description & Details
            </div>
            {gallery.rating != null && gallery.rating > 0 && (
              <div className="flex items-center gap-1 bg-amber-400/10 text-amber-400 px-2 py-0.5 rounded text-xs font-bold ring-1 ring-amber-400/20">
                <Star size={12} className="fill-amber-400" />
                {gallery.rating.toFixed(1)}
              </div>
            )}
          </div>
          {gallery.description ? (
            <div className="relative">
              <p className="text-sm text-zinc-300 leading-relaxed max-h-[120px] overflow-y-auto pr-2 custom-scrollbar">
                {gallery.description}
              </p>
            </div>
          ) : (
            <div className="flex-1 flex items-center justify-center border border-dashed border-white/5 rounded-xl py-6">
              <p className="text-sm text-zinc-500">No description provided for this gallery.</p>
            </div>
          )}
        </div>

        {/* People Card */}
        <div className="md:col-span-3 glass-card p-5 rounded-2xl">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2 text-zinc-400 text-xs font-bold uppercase tracking-wider">
              <Heart size={14} /> Linked People
            </div>
            <span className="text-[10px] bg-white/5 text-zinc-500 px-2 py-0.5 rounded-full border border-white/5">
              {linkedPeople?.length ?? 0} total
            </span>
          </div>
          {!linkedPeople || linkedPeople.length === 0 ? (
            <div className="py-4 border border-dashed border-white/5 rounded-xl text-center">
              <p className="text-sm text-zinc-500 italic">No performers linked to this gallery.</p>
            </div>
          ) : (
            <div className="flex flex-wrap gap-3">
              {linkedPeople.map((person) => {
                const photo = parsePhotos(person.photos)[0];
                return (
                  <Link
                    key={person.id}
                    to={`/people/${person.id}`}
                    className="group relative flex items-center gap-3 rounded-full bg-white/5 border border-white/5 pr-4 pl-1 py-1 hover:bg-white/10 hover:border-white/20 transition-all duration-300"
                  >
                    <div className="relative h-8 w-8 rounded-full overflow-hidden ring-1 ring-white/10 group-hover:ring-blue-500/50 transition-all">
                      {photo ? (
                        <img src={photo} alt={person.name} className="h-full w-full object-cover group-hover:scale-110 transition-transform duration-500" />
                      ) : (
                        <div className="h-full w-full bg-zinc-800 flex items-center justify-center text-[10px] text-zinc-500 font-bold uppercase">
                          {person.name.charAt(0)}
                        </div>
                      )}
                    </div>
                    <span className="text-sm font-medium text-zinc-300 group-hover:text-white transition-colors">
                      {person.name}
                    </span>
                    <button
                      onClick={(e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        unlinkPersonMut.mutate(person.id);
                      }}
                      disabled={unlinkPersonMut.isPending}
                      className="ml-1 p-1 rounded-full text-zinc-500 hover:text-red-400 hover:bg-red-400/10 opacity-0 group-hover:opacity-100 transition-all"
                      title={`Unlink ${person.name}`}
                    >
                      <X size={12} />
                    </button>
                  </Link>
                );
              })}
            </div>
          )}
        </div>
      </div>

      {/* Gallery Management Tools */}
      <div className="flex items-center justify-end mb-4 px-6">
        <Button variant="secondary" size="sm" onClick={() => setShowTools((v) => !v)}>
          <Settings2 size={14} /> {showTools ? 'Hide Tools' : 'Tools'}
        </Button>
      </div>

      {showTools && (
        <div className="mx-6 mb-8 rounded-xl border border-zinc-800 bg-zinc-900/50 p-6 space-y-6">
          <h3 className="text-lg font-medium text-white flex items-center gap-2 border-b border-zinc-800 pb-4">
            <Settings2 size={18} className="text-blue-400" />
            Gallery Management Tools
          </h3>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {/* Search Metadata */}
            <div className="bg-zinc-950/40 p-4 rounded-lg border border-zinc-800/80 space-y-3">
              <h4 className="text-sm font-semibold text-zinc-300 flex items-center gap-2">
                <Search size={14} /> Search Metadata
              </h4>
              <p className="text-xs text-zinc-500">Search provider for matching galleries based on name and linked people.</p>
              <Button size="sm" onClick={() => searchMetaMut.mutate()} disabled={searchMetaMut.isPending}>
                {searchMetaMut.isPending ? 'Searching...' : 'Search Providers'}
              </Button>
              {searchMetaQuery && (
                <pre className="text-[10px] text-zinc-400 max-h-40 overflow-y-auto bg-zinc-950 p-2 rounded border border-zinc-800">
                  {searchMetaQuery}
                </pre>
              )}
            </div>

            {/* Scrape Metadata */}
            <div className="bg-zinc-950/40 p-4 rounded-lg border border-zinc-800/80 space-y-3">
              <h4 className="text-sm font-semibold text-zinc-300 flex items-center gap-2">
                <Download size={14} /> Scrape Metadata
              </h4>
              <div className="flex gap-2">
                <div className="flex-1">
                  <Input placeholder="Provider (e.g. MetArt)" value={scrapeProvider} onChange={(e) => setScrapeProvider(e.target.value)} className="h-9 text-xs" />
                </div>
                <div className="flex-[2]">
                  <Input placeholder="Gallery URL" value={scrapeUrl} onChange={(e) => setScrapeUrl(e.target.value)} className="h-9 text-xs" />
                </div>
              </div>
              <Button size="sm" onClick={() => scrapeMetaMut.mutate()} disabled={!scrapeProvider || !scrapeUrl || scrapeMetaMut.isPending}>
                {scrapeMetaMut.isPending ? 'Scraping...' : 'Scrape'}
              </Button>
            </div>

            {/* Update Provider */}
            <div className="bg-zinc-950/40 p-4 rounded-lg border border-zinc-800/80 space-y-3">
              <h4 className="text-sm font-semibold text-zinc-300 flex items-center gap-2">
                <Upload size={14} /> Update Provider
              </h4>
              <div className="flex gap-2">
                <div className="flex-1">
                  <Input placeholder="Provider name" value={updateProvider} onChange={(e) => setUpdateProvider(e.target.value)} className="h-9 text-xs" />
                </div>
                <div className="flex-[2]">
                  <Input placeholder="Source URL (optional)" value={updateSourceUrl} onChange={(e) => setUpdateSourceUrl(e.target.value)} className="h-9 text-xs" />
                </div>
              </div>
              <Button size="sm" onClick={() => updateProviderMut.mutate()} disabled={!updateProvider || updateProviderMut.isPending}>
                {updateProviderMut.isPending ? 'Updating...' : 'Update'}
              </Button>
            </div>

            {/* Add Image by URL */}
            <div className="bg-zinc-950/40 p-4 rounded-lg border border-zinc-800/80 space-y-3">
              <h4 className="text-sm font-semibold text-zinc-300 flex items-center gap-2">
                <Download size={14} /> Add Image from URL
              </h4>
              <Input placeholder="Image URL to download" value={addImageUrl} onChange={(e) => setAddImageUrl(e.target.value)} className="h-9 text-xs" />
              <Button size="sm" onClick={() => addImageMut.mutate()} disabled={!addImageUrl || addImageMut.isPending}>
                {addImageMut.isPending ? 'Downloading...' : 'Download & Add'}
              </Button>
            </div>
          </div>
        </div>
      )}

      <section className="p-1">
        {loadingImages ? (
          <Spinner />
        ) : !imageList || imageList.data.length === 0 ? (
          <EmptyState message="No images in this gallery." />
        ) : (
          <JustifiedGrid
            items={gridItems}
            rowHeight={230}
            gap={4}
            onItemClick={(index) => setLightboxIndex(index)}
          />
        )}
      </section>

      {/* Lightbox */}
      {lightboxIndex !== null && (
        <Lightbox
          images={lightboxImages}
          index={lightboxIndex}
          onClose={() => setLightboxIndex(null)}
          onIndexChange={setLightboxIndex}
          imageData={imageList!.data}
          onToggleFavorite={(id) => favMut.mutate(id)}
        />
      )}

      {/* Confirm dialogs */}
      <ConfirmDialog
        open={confirmDeleteGallery}
        title="Delete Gallery"
        message="Delete this gallery and all its images? Files will be removed from disk. This cannot be undone."
        confirmLabel="Delete Gallery"
        onConfirm={() => deleteGalleryMut.mutate()}
        onCancel={() => setConfirmDeleteGallery(false)}
      />

      <ConfirmDialog
        open={confirmDeleteImageId !== null}
        title="Delete Image"
        message="Delete this image? The file will be removed from disk. This cannot be undone."
        confirmLabel="Delete Image"
        onConfirm={() => {
          if (confirmDeleteImageId !== null) {
            deleteImageMut.mutate(confirmDeleteImageId);
          }
        }}
        onCancel={() => setConfirmDeleteImageId(null)}
      />
      </div>
    </>
  );
}
