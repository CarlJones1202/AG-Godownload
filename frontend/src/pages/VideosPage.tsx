import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { videos, images as imagesApi, people } from '@/lib/api';
import { formatDate, formatDuration, thumbnailUrl } from '@/lib/utils';
import {
  PageHeader,
  Card,
  Spinner,
  EmptyState,
  Badge,
  Button,
  Input,
  Pagination,
  ConfirmDialog,
} from '@/components/UI';
import { VideoPlayer } from '@/components/VideoPlayer';
import { Heart, Play, Trash2, UserPlus, X } from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';
import type { Image, Person } from '@/types';

export function VideosPage() {
  const queryClient = useQueryClient();
  const { page, offset, limit, prevPage, nextPage } = usePagination({ limit: 50 });
  const [confirmDeleteId, setConfirmDeleteId] = useState<number | null>(null);
  const [activeVideo, setActiveVideo] = useState<Image | null>(null);
  const [linkPersonImageId, setLinkPersonImageId] = useState<number | null>(null);
  const [personSearch, setPersonSearch] = useState('');

  const { data: videoList, isLoading } = useQuery({
    queryKey: ['videos', { offset, limit }],
    queryFn: () => videos.list({ limit, offset }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => imagesApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['videos'] });
      setConfirmDeleteId(null);
    },
  });

  const favMut = useMutation({
    mutationFn: (id: number) => imagesApi.toggleFavorite(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['videos'] });
    },
  });

  const vrModeMut = useMutation({
    mutationFn: ({ id, mode }: { id: number; mode: string }) => imagesApi.updateVrMode(id, mode),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['videos'] });
    },
  });

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

  const videosData = videoList?.data ?? [];
  const totalPages = videoList?.meta.total_pages ?? 1;

  return (
    <>
      <PageHeader title="Videos" description="Browse downloaded videos" />

      {isLoading ? (
        <Spinner />
      ) : videosData.length === 0 ? (
        <EmptyState message="No videos found." />
      ) : (
        <>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            {videosData.map((vid) => (
              <div
                key={vid.id}
                className="group cursor-pointer"
                onClick={() => setActiveVideo(vid)}
              >
              <Card
                className="overflow-hidden"
              >
                <div className="relative aspect-video bg-zinc-800 overflow-hidden">
                  <img
                    src={thumbnailUrl(vid.filename)}
                    alt={vid.filename}
                    className="w-full h-full object-cover"
                    loading="lazy"
                    onError={(e) => {
                      (e.target as HTMLImageElement).style.display = 'none';
                    }}
                  />
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
                  <p className="text-xs text-white truncate">{vid.filename}</p>
                  <div className="flex items-center gap-2 mt-1">
                    {vid.width != null && vid.height != null && (
                      <Badge variant="info">{vid.width}x{vid.height}</Badge>
                    )}
                    {vid.vr_mode !== 'none' && (
                      <Badge variant="warning">VR {vid.vr_mode}</Badge>
                    )}
                    <span className="text-xs text-zinc-500 ml-auto">{formatDate(vid.created_at)}</span>
                  </div>
                   <div className="flex items-center justify-between mt-2 opacity-0 group-hover:opacity-100 transition-opacity"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        title={vid.is_favorite ? 'Remove from favorites' : 'Add to favorites'}
                        onClick={() => favMut.mutate(vid.id)}
                      >
                        <Heart
                          size={14}
                          className={vid.is_favorite ? 'text-red-400 fill-red-400' : 'text-zinc-500 hover:text-red-400'}
                        />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        title="Link to person"
                        onClick={() => {
                          setLinkPersonImageId(vid.id);
                          setPersonSearch('');
                        }}
                      >
                        <UserPlus size={14} className="text-zinc-500 hover:text-blue-400" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        title="Delete video"
                        onClick={() => setConfirmDeleteId(vid.id)}
                      >
                        <Trash2 size={14} className="text-zinc-500 hover:text-red-400" />
                      </Button>
                    </div>
                    <select
                      value={vid.vr_mode || 'none'}
                      onChange={(e) => vrModeMut.mutate({ id: vid.id, mode: e.target.value })}
                      className="bg-zinc-800 border border-zinc-700 hover:border-zinc-600 rounded px-2 py-1 text-xs text-zinc-300 focus:outline-none transition-all cursor-pointer mr-1"
                    >
                      <option value="none">Flat</option>
                      <option value="180">VR 180</option>
                      <option value="360">VR 360</option>
                    </select>
                  </div>
                </div>
              </Card>
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

      {activeVideo && (
        <VideoPlayer video={activeVideo} onClose={() => setActiveVideo(null)} />
      )}

      {linkPersonImageId !== null && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={() => setLinkPersonImageId(null)}>
          <div
            className="w-full max-w-sm bg-zinc-900 border border-zinc-700 rounded-xl p-4 shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold text-white">Link Video to Person</h3>
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
        title="Delete Video"
        message="Delete this video? The file will be removed from disk. This cannot be undone."
        confirmLabel="Delete Video"
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
