import { useState, useMemo, useCallback } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { images, tagsApi, people } from '@/lib/api';
import { cn, parseColors, thumbnailUrl, imageUrl } from '@/lib/utils';
import {
  PageHeader,
  Card,
  Spinner,
  EmptyState,
  Input,
  Button,
  Pagination,
  ConfirmDialog,
  
} from '@/components/UI';
import { JustifiedGrid } from '@/components/JustifiedGrid';
import type { JustifiedItem } from '@/components/JustifiedGrid';
import { Lightbox } from '@/components/Lightbox';
import { Heart, Search, Palette, Trash2, Shuffle, HardDrive, Tag, UserPlus, X } from 'lucide-react';
import { Select } from '@/components/UI';
import { usePagination } from '@/hooks/usePagination';
import type { Person } from '@/types';

export function ImagesPage() {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const { offset, limit, prevPage, nextPage } = usePagination({ limit: 50 });

  const sortBy = (searchParams.get('sort') as any) || 'newest';
  const randomSeed = Number(searchParams.get('seed')) || 0;
  const onDiskOnly = searchParams.get('on_disk') === 'true';
  const favoritesOnly = searchParams.get('favorites') === 'true';

  const [colorSearch, setColorSearch] = useState('');
  const [activeColorSearch, setActiveColorSearch] = useState('');
  const [confirmDeleteId, setConfirmDeleteId] = useState<number | null>(null);
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);
  const [tagFilter, setTagFilter] = useState('');
  const [linkPersonImageId, setLinkPersonImageId] = useState<number | null>(null);
  const [personSearch, setPersonSearch] = useState('');

  const { data: personResults } = useQuery({
    queryKey: ['people', 'search', personSearch],
    queryFn: () => people.list({ q: personSearch, limit: 10 }),
    enabled: personSearch.length > 1,
  });

  const linkPersonMut = useMutation({
    mutationFn: (personId: number) => people.linkImage(personId, linkPersonImageId!),
    onSuccess: () => {
      setLinkPersonImageId(null);
      setPersonSearch('');
    },
  });

  const { data: allTags } = useQuery({
    queryKey: ['tags', 'all'],
    queryFn: () => tagsApi.list(),
  });

  const updateFilter = useCallback((key: string, value: any) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (value === null || value === false || value === '' || (key === 'sort' && value === 'newest') || (key === 'seed' && value === 0)) {
        next.delete(key);
      } else {
        next.set(key, String(value));
      }
      next.delete('page');
      return next;
    }, { replace: true });
  }, [setSearchParams]);

  const { data: imageList, isLoading } = useQuery({
    queryKey: ['images', { offset, limit, is_favorite: favoritesOnly || undefined, sort_by: sortBy, random_seed: sortBy === 'random' ? randomSeed : undefined, on_disk: onDiskOnly || undefined, filter_tags: tagFilter || undefined }],
    queryFn: () =>
      images.list({
        limit,
        offset,
        is_favorite: favoritesOnly || undefined,
        is_video: false,
        sort_by: sortBy,
        random_seed: sortBy === 'random' ? randomSeed : undefined,
        on_disk: onDiskOnly || undefined,
        filter_tags: tagFilter || undefined,
      }),
    enabled: !activeColorSearch,
  });

  const { data: colorResults, isLoading: isColorLoading } = useQuery({
    queryKey: ['images', 'color', activeColorSearch],
    queryFn: () => images.searchByColor(activeColorSearch, 50),
    enabled: !!activeColorSearch,
  });

  const favMut = useMutation({
    mutationFn: (id: number) => images.toggleFavorite(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images'] });
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => images.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images'] });
      setConfirmDeleteId(null);
    },
  });

  const handleColorSearch = () => {
    if (colorSearch.trim()) {
      setActiveColorSearch(colorSearch.trim());
    }
  };

  const clearColorSearch = () => {
    setColorSearch('');
    setActiveColorSearch('');
  };

  const displayImages = activeColorSearch
    ? (colorResults?.data ?? [])
    : (imageList?.data ?? []);

  const loading = activeColorSearch ? isColorLoading : isLoading;

  const gridItems: JustifiedItem[] = useMemo(() => {
    return displayImages.map((img) => {
      const colors = parseColors(img.dominant_colors);
      const tagCount = img.tags?.length ?? 0;
      return {
        id: img.id,
        src: imageUrl(img.filename),
        thumbSrc: thumbnailUrl(img.filename),
        width: img.width,
        height: img.height,
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
                    setConfirmDeleteId(img.id);
                  }}
                  className="p-1"
                  title="Delete image"
                >
                  <Trash2 size={16} className="text-white hover:text-red-400" />
                </button>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    setLinkPersonImageId(img.id);
                    setPersonSearch('');
                  }}
                  className="p-1"
                  title="Link to person"
                >
                  <UserPlus size={16} className="text-white hover:text-blue-400" />
                </button>
              </div>
              <div className="flex items-center gap-1">
                {tagCount > 0 && (
                  <span className="inline-flex items-center gap-0.5 text-[10px] text-zinc-300 bg-zinc-900/70 px-1 py-0.5 rounded">
                    <Tag size={10} />{tagCount}
                  </span>
                )}
                {img.width && img.height && (
                  <span className="text-[10px] text-white/70">
                    {img.width}x{img.height}
                  </span>
                )}
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
  }, [displayImages, favMut]);

  const lightboxImages = useMemo(() => {
    return displayImages.map((img) => ({
      src: imageUrl(img.filename),
      alt: img.filename,
    }));
  }, [displayImages]);

  return (
    <>
      <PageHeader title="Images" description="Browse and manage images">
        <Button
          variant={favoritesOnly ? 'primary' : 'secondary'}
          size="sm"
          onClick={() => updateFilter('favorites', !favoritesOnly)}
        >
          <Heart size={14} /> Favorites
        </Button>
      </PageHeader>

      <Card className="mb-4">
        <div className="flex flex-wrap items-end gap-4">
          <div className="w-48">
            <Select
              label="Sort By"
              value={sortBy}
              onChange={(e) => updateFilter('sort', e.target.value)}
              options={[
                { value: 'newest', label: 'Newest first' },
                { value: 'oldest', label: 'Oldest first' },
                { value: 'largest', label: 'Largest first' },
                { value: 'smallest', label: 'Smallest first' },
                { value: 'random', label: 'Random' },
              ]}
            />
          </div>

          {allTags && allTags.length > 0 && (
            <div className="w-48">
              <Select
                label="Filter by Tag"
                value={tagFilter}
                onChange={(e) => setTagFilter(e.target.value)}
                options={[
                  { value: '', label: 'All tags' },
                  ...allTags.map((t) => ({ value: String(t.id), label: `${t.name} (${t.count})` })),
                ]}
              />
            </div>
          )}

          {sortBy === 'random' && (
            <>
              <div className="w-32">
                <Input
                  label="Seed"
                  type="number"
                  value={randomSeed}
                  onChange={(e) => updateFilter('seed', e.target.value)}
                />
              </div>
              <Button
                variant="secondary"
                size="md"
                className="mb-0.5 h-10"
                onClick={() => updateFilter('seed', Math.floor(Math.random() * 1000000))}
              >
                <Shuffle size={14} className="mr-1" /> Shuffle
              </Button>
            </>
          )}

          <div className="flex items-center gap-2 h-10 mb-0.5">
            <input
              id="onDiskOnly"
              type="checkbox"
              checked={onDiskOnly}
              onChange={(e) => updateFilter('on_disk', e.target.checked)}
              className="w-4 h-4 rounded border-zinc-700 bg-zinc-800 text-blue-600 focus:ring-blue-500 focus:ring-offset-zinc-900"
            />
            <label htmlFor="onDiskOnly" className="text-sm font-medium text-zinc-300 cursor-pointer flex items-center gap-1.5">
              <HardDrive size={14} /> Only images on disk
            </label>
          </div>
        </div>
      </Card>

      <Card className="mb-4">
        <div className="flex items-center gap-2">
          <Palette size={16} className="text-zinc-400" />
          <span className="text-sm text-zinc-400">Color Search</span>
        </div>
        <div className="flex gap-2 mt-2">
          <div className="flex-1 flex gap-2">
            <Input
              placeholder="#ff0000 or ff0000"
              value={colorSearch}
              onChange={(e) => setColorSearch(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleColorSearch()}
            />
            {colorSearch && (
              <div
                className="w-10 h-10 rounded border border-zinc-700 shrink-0"
                style={{ backgroundColor: colorSearch.startsWith('#') ? colorSearch : `#${colorSearch}` }}
              />
            )}
          </div>
          <Button size="sm" onClick={handleColorSearch} disabled={!colorSearch.trim()}>
            <Search size={14} /> Search
          </Button>
          {activeColorSearch && (
            <Button variant="secondary" size="sm" onClick={clearColorSearch}>
              Clear
            </Button>
          )}
        </div>
      </Card>

      {loading ? (
        <Spinner />
      ) : displayImages.length === 0 ? (
        <EmptyState message="No images found." />
      ) : (
        <>
          <JustifiedGrid
            items={gridItems}
            rowHeight={220}
            gap={4}
            onItemClick={(index) => setLightboxIndex(index)}
          />

          {!activeColorSearch && imageList && (
            <Pagination
              page={imageList.meta.current_page}
              totalPages={imageList.meta.total_pages}
              onPrev={prevPage}
              onNext={nextPage}
              hasMore={imageList.meta.current_page < imageList.meta.total_pages}
            />
          )}
        </>
      )}

      {lightboxIndex !== null && (
        <Lightbox
          images={lightboxImages}
          index={lightboxIndex}
          onClose={() => setLightboxIndex(null)}
          onIndexChange={setLightboxIndex}
          imageData={displayImages}
          onToggleFavorite={(id) => favMut.mutate(id)}
        />
      )}

      {linkPersonImageId !== null && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={() => setLinkPersonImageId(null)}>
          <div
            className="w-full max-w-sm bg-zinc-900 border border-zinc-700 rounded-xl p-4 shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold text-white">Link Image to Person</h3>
              <button onClick={() => setLinkPersonImageId(null)} className="text-zinc-500 hover:text-white transition-colors">
                <X size={16} />
              </button>
            </div>
            <Input
              placeholder="Search person by name..."
              value={personSearch}
              onChange={(e) => setPersonSearch(e.target.value)}
              autoFocus
              className="mb-3"
            />
            {personResults && personResults.data.length > 0 && (
              <div className="space-y-1 max-h-48 overflow-y-auto">
                {personResults.data.map((p: Person) => (
                  <button
                    key={p.id}
                    onClick={() => linkPersonMut.mutate(p.id)}
                    disabled={linkPersonMut.isPending}
                    className="w-full flex items-center gap-2 px-3 py-2 rounded-lg bg-zinc-800 hover:bg-zinc-700 border border-zinc-700 hover:border-zinc-500 transition-all text-left text-sm"
                  >
                    <UserPlus size={14} className="text-zinc-400 shrink-0" />
                    <span className="text-zinc-200 flex-1 truncate">{p.name}</span>
                    {p.aliases && <span className="text-[10px] text-zinc-500 truncate max-w-[100px]">{p.aliases}</span>}
                  </button>
                ))}
              </div>
            )}
            {personSearch.length > 1 && personResults && personResults.data.length === 0 && (
              <p className="text-xs text-zinc-500 text-center py-2">No people found matching "{personSearch}"</p>
            )}
          </div>
        </div>
      )}

      <ConfirmDialog
        open={confirmDeleteId !== null}
        title="Delete Image"
        message="Delete this image? The file will be removed from disk. This cannot be undone."
        confirmLabel="Delete Image"
        onConfirm={() => {
          if (confirmDeleteId !== null) {
            deleteMut.mutate(confirmDeleteId);
          }
        }}
        onCancel={() => setConfirmDeleteId(null)}
      />
    </>
  );
}
