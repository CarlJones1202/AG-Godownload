import { useCallback, useEffect, useRef, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Heart, Undo2, X } from 'lucide-react';
import { images } from '@/lib/api';
import { subscribeToFavoriteStateChanges } from '@/lib/favoriteFeedback';

interface FavoriteNotice {
  imageId: number;
  error?: string;
}

export function FavoriteUndoToast() {
  const queryClient = useQueryClient();
  const [notice, setNotice] = useState<FavoriteNotice | null>(null);
  const noticeRef = useRef<FavoriteNotice | null>(null);
  const timeoutRef = useRef<number | undefined>(undefined);

  const updateNotice = useCallback((next: FavoriteNotice | null) => {
    noticeRef.current = next;
    setNotice(next);
  }, []);

  useEffect(() => {
    const unsubscribe = subscribeToFavoriteStateChanges((imageId, isFavorite) => {
      if (isFavorite) {
        if (noticeRef.current?.imageId === imageId) {
          if (timeoutRef.current) window.clearTimeout(timeoutRef.current);
          timeoutRef.current = undefined;
          updateNotice(null);
        }
        return;
      }

      if (timeoutRef.current) window.clearTimeout(timeoutRef.current);
      updateNotice({ imageId });
      timeoutRef.current = window.setTimeout(() => updateNotice(null), 7000);
    });
    return () => {
      unsubscribe();
      if (timeoutRef.current) window.clearTimeout(timeoutRef.current);
    };
  }, [updateNotice]);

  const undoMutation = useMutation({
    mutationFn: (imageId: number) => images.toggleFavorite(imageId),
    onSuccess: (result, imageId) => {
      queryClient.invalidateQueries({ queryKey: ['favorites'] });
      queryClient.invalidateQueries({ queryKey: ['images'] });
      queryClient.invalidateQueries({ queryKey: ['videos'] });
      queryClient.invalidateQueries({ queryKey: ['similar'] });
      if (!result.is_favorite) {
        updateNotice({ imageId, error: 'The image is still not favorited. Try Undo again.' });
        return;
      }
      if (timeoutRef.current) window.clearTimeout(timeoutRef.current);
      timeoutRef.current = undefined;
      updateNotice(null);
    },
    onError: (error) => {
      updateNotice(noticeRef.current ? {
        ...noticeRef.current,
        error: error instanceof Error ? error.message : 'Could not restore favorite.',
      } : null);
    },
  });

  if (!notice) return null;

  return (
    <div
      role="status"
      className="fixed bottom-5 left-1/2 z-[60] flex -translate-x-1/2 items-center gap-3 rounded-lg border border-zinc-700 bg-zinc-900 px-4 py-3 text-sm text-zinc-100 shadow-xl"
    >
      <Heart size={16} className="shrink-0 fill-red-500 text-red-500" />
      <div className="min-w-0">
        <p>Removed from favorites</p>
        {notice.error && <p className="mt-1 text-xs text-red-400">{notice.error}</p>}
      </div>
      <button
        type="button"
        onClick={() => undoMutation.mutate(notice.imageId)}
        disabled={undoMutation.isPending}
        className="inline-flex shrink-0 items-center gap-1 rounded px-2 py-1 font-medium text-blue-400 hover:bg-zinc-800 hover:text-blue-300 disabled:opacity-50"
      >
        <Undo2 size={14} />
        Undo
      </button>
      <button
        type="button"
        onClick={() => {
          if (timeoutRef.current) window.clearTimeout(timeoutRef.current);
          timeoutRef.current = undefined;
          updateNotice(null);
        }}
        className="rounded p-1 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-300"
        aria-label="Dismiss"
      >
        <X size={14} />
      </button>
    </div>
  );
}
