import { useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { images as imagesApi } from '@/lib/api';
import { formatDate, formatDuration, thumbnailUrl, imageUrl } from '@/lib/utils';
import {
  PageHeader,
  Spinner,
  EmptyState,
  Badge,
  Button,
  Pagination,
  ConfirmDialog,
  Tabs,
  VideoThumb,
} from '@/components/UI';
import { JustifiedGrid } from '@/components/JustifiedGrid';
import type { JustifiedItem } from '@/components/JustifiedGrid';
import { Lightbox } from '@/components/Lightbox';
import { VideoPlayer } from '@/components/VideoPlayer';
import { Heart, Play, Trash2 } from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';
import type { Image } from '@/types';

type Section = 'images' | 'videos';

export function FavoritesPage() {
  const queryClient = useQueryClient();
  const [params, setParams] = useSearchParams();
  const rawSection = params.get('section') as Section | null;
  const section: Section = rawSection === 'videos' ? 'videos' : 'images';
  const setSection = (s: Section) => setParams(s === 'images' ? {} : { section: s }, { replace: true });

  const { page, offset, limit, prevPage, nextPage } = usePagination({ limit: 48 });
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<number | null>(null);
  const [activeVideo, setActiveVideo] = useState<Image | null>(null);

  const { data: favList, isLoading } = useQuery({
    queryKey: ['favorites', section, { offset, limit }],
    queryFn: () => imagesApi.list({ limit, offset, is_favorite: true, is_video: section === 'videos' }),
  });

  const favMut = useMutation({
    mutationFn: (id: number) => imagesApi.toggleFavorite(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['favorites'] });
      queryClient.invalidateQueries({ queryKey: ['images'] });
      queryClient.invalidateQueries({ queryKey: ['videos'] });
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => imagesApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['favorites'] });
      setConfirmDeleteId(null);
    },
  });

  const items = useMemo(() => favList?.data ?? [], [favList]);
  const totalPages = favList?.meta.total_pages ?? 1;

  const gridItems: JustifiedItem[] = useMemo(() => {
    return items.map((img) => ({
      id: img.id,
      src: imageUrl(img.filename),
      thumbSrc: thumbnailUrl(img.filename),
      width: img.width,
      height: img.height,
      overlay: (
        <div className="flex justify-end h-full bg-gradient-to-t from-black/60 to-transparent p-2">
          <button
            onClick={(e) => { e.stopPropagation(); favMut.mutate(img.id); }}
            className="p-1"
            title="Remove from favorites"
          >
            <Heart size={16} className="fill-red-500 text-red-500" />
          </button>
        </div>
      ),
    }));
  }, [items, favMut]);

  const lightboxImages = useMemo(() => {
    return items.map((img) => ({
      src: imageUrl(img.filename),
      alt: img.filename,
    }));
  }, [items]);

  return (
    <>
      <PageHeader title="Favorites" description="Your starred images and videos" />

      <Tabs
        value={section}
        onChange={(v) => setSection(v as Section)}
        tabs={[
          { value: 'images', label: 'Images' },
          { value: 'videos', label: 'Videos' },
        ]}
      />

      <div className="mt-6">
        {isLoading ? (
          <Spinner />
        ) : items.length === 0 ? (
          <EmptyState message={section === 'images' ? 'No favorite images yet.' : 'No favorite videos yet.'} />
        ) : section === 'images' ? (
          <>
            <JustifiedGrid items={gridItems} rowHeight={220} gap={4} onItemClick={(index) => setLightboxIndex(index)} />
            <Pagination
              page={page}
              totalPages={totalPages}
              hasMore={page < totalPages}
              onPrev={prevPage}
              onNext={nextPage}
            />
          </>
        ) : (
          <>
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
              {items.map((vid) => (
                <div
                  key={vid.id}
                  className="group cursor-pointer rounded-lg border border-zinc-800 bg-zinc-900/30 hover:border-zinc-700 overflow-hidden transition-all"
                  onClick={() => setActiveVideo(vid)}
                >
                  <div className="relative aspect-video bg-zinc-800 overflow-hidden">
                    <VideoThumb filename={vid.filename} alt={vid.filename} />
                    <div className="absolute inset-0 flex items-center justify-center bg-black/20 opacity-0 group-hover:opacity-100 transition-opacity">
                      <div className="w-12 h-12 rounded-full bg-black/60 flex items-center justify-center">
                        <Play size={24} className="text-white ml-0.5" />
                      </div>
                    </div>
                    {vid.duration_seconds != null && vid.duration_seconds > 0 && (
                      <div className="absolute bottom-2 right-2 px-1.5 py-0.5 rounded bg-black/70 text-white text-xs font-mono">
                        {formatDuration(vid.duration_seconds)}
                      </div>
                    )}
                  </div>
                  <div className="p-2">
                    <p className="text-xs text-white truncate" title={vid.title || vid.filename}>{vid.title || vid.filename}</p>
                    <div className="flex items-center gap-2 mt-1">
                      {vid.width != null && vid.height != null && (
                        <Badge variant="info">{vid.width}x{vid.height}</Badge>
                      )}
                      {vid.vr_mode !== 'none' && <Badge variant="warning">VR {vid.vr_mode}</Badge>}
                      <span className="text-xs text-zinc-500 ml-auto">{formatDate(vid.created_at)}</span>
                    </div>
                    <div
                      className="flex items-center justify-end mt-2 opacity-0 group-hover:opacity-100 transition-opacity"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <div className="flex items-center gap-1">
                        <Button variant="ghost" size="sm" title="Remove from favorites" onClick={() => favMut.mutate(vid.id)}>
                          <Heart size={14} className="text-red-400 fill-red-400" />
                        </Button>
                        <Button variant="ghost" size="sm" title="Delete video" onClick={() => setConfirmDeleteId(vid.id)}>
                          <Trash2 size={14} className="text-zinc-500 hover:text-red-400" />
                        </Button>
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
            <Pagination
              page={page}
              totalPages={totalPages}
              hasMore={page < totalPages}
              onPrev={prevPage}
              onNext={nextPage}
            />
          </>
        )}
      </div>

      {lightboxIndex !== null && (
        <Lightbox
          images={lightboxImages}
          index={lightboxIndex}
          onClose={() => setLightboxIndex(null)}
          onIndexChange={setLightboxIndex}
          imageData={items}
          onToggleFavorite={(id) => favMut.mutate(id)}
        />
      )}

      {activeVideo && <VideoPlayer video={activeVideo} onClose={() => setActiveVideo(null)} />}

      <ConfirmDialog
        open={confirmDeleteId !== null}
        title="Delete Video"
        message="Delete this video? The file will be removed from disk. This cannot be undone."
        confirmLabel="Delete Video"
        onConfirm={() => { if (confirmDeleteId !== null) deleteMut.mutate(confirmDeleteId); }}
        onCancel={() => setConfirmDeleteId(null)}
      />
    </>
  );
}