import { useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useSearchParams, Link } from 'react-router-dom';
import { similar, ratings, profile as profileApi, images } from '@/lib/api';
import { PageHeader, Spinner, EmptyState, Button, Badge } from '@/components/UI';
import { Star, Heart, Link as LinkIcon, Trash2, Sparkles, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { SimilarImage } from '@/types';

function decodeIds(raw: string): number[] | null {
  try {
    const decoded = atob(raw);
    const parsed = JSON.parse(decoded);
    if (Array.isArray(parsed) && parsed.every((n) => typeof n === 'number' && n > 0)) {
      return parsed as number[];
    }
  } catch {
    return null;
  }
  return null;
}

function SimilarTile({ item }: { item: SimilarImage }) {
  const queryClient = useQueryClient();
  const [favorite, setFavorite] = useState(item.favorite);

  const { data: rating } = useQuery({
    queryKey: ['ratings', item.id],
    queryFn: () => ratings.get(item.id),
  });

  const setRatingMut = useMutation({
    mutationFn: (r: number) => ratings.set(item.id, r),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['ratings', item.id] }),
  });

  const clearRatingMut = useMutation({
    mutationFn: () => ratings.clear(item.id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['ratings', item.id] }),
  });

  const favMut = useMutation({
    mutationFn: () => images.toggleFavorite(item.id),
    onSuccess: (res) => setFavorite(res.is_favorite),
  });

  const handleRate = (n: number) => {
    if (n === (rating?.rating ?? 0)) {
      clearRatingMut.mutate();
    } else {
      setRatingMut.mutate(n);
    }
  };

  const body = (
    <div className="relative overflow-hidden rounded-lg border border-zinc-800 bg-zinc-900/50 group hover:border-zinc-600 transition-colors">
      <img
        src={item.thumbnail_path || item.web_path || undefined}
        alt={item.filename}
        loading="lazy"
        className="aspect-[3/4] w-full object-cover"
      />
      <div className="absolute top-2 right-2 text-[10px] font-mono px-1.5 py-0.5 rounded bg-black/75 text-white/90">
        {Math.round(item.similarity * 100)}%
      </div>
      {favorite && (
        <div className="absolute top-2 left-2">
          <Heart size={12} className="fill-red-500 text-red-500" />
        </div>
      )}

      {/* Always-visible reasons */}
      <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 to-transparent p-2">
        {item.reasons.length > 0 && (
          <div className="flex flex-wrap gap-1 mb-1">
            {item.reasons.slice(0, 3).map((r) => (
              <span
                key={r}
                className="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-white/10 text-white/80 whitespace-nowrap"
              >
                <Sparkles size={9} className="text-blue-400" />
                {r}
              </span>
            ))}
          </div>
        )}
      </div>

      {/* Hover controls */}
      <div className="absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity bg-black/40 flex flex-col items-center justify-end pb-2">
        <div className="flex items-center gap-0.5">
          {[1, 2, 3, 4, 5].map((n) => (
            <button
              key={n}
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                handleRate(n);
              }}
              disabled={setRatingMut.isPending || clearRatingMut.isPending}
              className="p-0.5 transition-transform hover:scale-110 disabled:opacity-50"
              title={n <= (rating?.rating ?? 0) ? 'Click to clear rating' : `Rate ${n} ${n === 1 ? 'star' : 'stars'}`}
            >
              <Star
                size={14}
                className={n <= (rating?.rating ?? 0) ? 'fill-amber-400 text-amber-400' : 'text-white/40'}
              />
            </button>
          ))}
          <div className="w-px h-4 bg-white/20 mx-1.5" />
          <button
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              favMut.mutate();
            }}
            className="p-0.5 transition-transform hover:scale-110"
            title="Toggle favorite"
          >
            <Heart size={14} className={favorite ? 'fill-red-500 text-red-500' : 'text-white/40'} />
          </button>
        </div>
        {item.gallery_id && (
          <span className="mt-1.5 text-[10px] text-blue-400/90">Open collection</span>
        )}
      </div>
    </div>
  );

  if (item.gallery_id) {
    return (
      <Link to={`/galleries/${item.gallery_id}`} className="block" title={item.reasons.join(' · ')}>
        {body}
      </Link>
    );
  }
  return (
    <a href={item.web_path} target="_blank" rel="noreferrer" className="block" title={item.reasons.join(' · ')}>
      {body}
    </a>
  );
}

export function SimilarPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const [copied, setCopied] = useState(false);

  const idsRaw = searchParams.get('ids') ?? '';
  const ids = useMemo(() => decodeIds(idsRaw), [idsRaw]);
  const idsValid = ids !== null && ids.length > 0;

  const { data, isLoading, isError, refetch, isFetching } = useQuery({
    queryKey: ['similar', 'ids', idsRaw],
    queryFn: () => similar.byIds(ids!),
    enabled: idsValid && ids!.length > 0,
  });

  const resetProfileMut = useMutation({
    mutationFn: () => profileApi.reset(),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['similar'] }),
  });

  const handleCopyLink = async () => {
    try {
      await navigator.clipboard.writeText(window.location.href);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  };

  const clearIds = () => {
    setSearchParams({}, { replace: true });
  };

  const profile = data?.profile;
  const seed = data?.seed;

  return (
    <>
      <PageHeader
        title="Similar Images"
        description={
          profile
            ? profile.ready
              ? `Tuned to your taste — ${profile.n_likes} liked, ${profile.n_dislikes} disliked.`
              : 'Your profile is still learning. Rate a few images to personalize suggestions.'
            : 'Recommended set shared via a link.'
        }
      >
        {data?.ids_b64 && (
          <Button variant="secondary" size="sm" onClick={handleCopyLink}>
            {copied ? <RefreshCw size={14} /> : <LinkIcon size={14} />}
            {copied ? 'Copied!' : 'Copy Link'}
          </Button>
        )}
        {profile && (
          <Badge variant={profile.ready ? 'success' : 'info'} className="gap-1">
            {profile.ready ? 'Tuned' : 'Learning'}
            <span className="opacity-70">· {profile.n_likes} likes</span>
          </Badge>
        )}
        {profile && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => resetProfileMut.mutate()}
            disabled={resetProfileMut.isPending}
          >
            <Trash2 size={14} /> Reset Profile
          </Button>
        )}
      </PageHeader>

      {!idsValid ? (
        <EmptyState message="No image set provided. Open the “You might like” strip on any image and choose “Open all” to see it here." />
      ) : isLoading ? (
        <Spinner />
      ) : isError ? (
        <div className="space-y-4">
          <EmptyState message="Couldn't load this similar-image set. The seed image may not be embedded yet." />
          <div className="flex justify-center">
            <Button variant="ghost" size="sm" onClick={() => refetch()}>
              <RefreshCw size={14} /> Try again
            </Button>
          </div>
        </div>
      ) : !data || data.data.length === 0 ? (
        <EmptyState message="No similar images found — embeddings are still being built in the background." />
      ) : (
        <div className="space-y-8">
          {seed && seed.web_path && (
            <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4 flex items-center gap-4">
              <img
                src={seed.thumbnail_path || seed.web_path || undefined}
                alt={seed.filename}
                className="h-20 w-16 object-cover rounded-md border border-zinc-700"
              />
              <div className="min-w-0">
                <div className="flex items-center gap-2 text-xs text-zinc-500 mb-1">
                  <Sparkles size={12} className="text-blue-400" />
                  <span className="uppercase tracking-widest text-[10px]">Seed — because you liked this</span>
                  {seed.embedded ? (
                    <Badge variant="success">Embedded</Badge>
                  ) : (
                    <Badge variant="warning">Not embedded yet</Badge>
                  )}
                </div>
                <p className="text-sm text-zinc-300 truncate">{seed.filename}</p>
                {seed.tags && seed.tags.length > 0 && (
                  <div className="flex flex-wrap gap-1 mt-1.5">
                    {seed.tags.slice(0, 8).map((t) => (
                      <span key={t} className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-400">
                        {t}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            </div>
          )}

          <div className={cn('grid gap-3 grid-cols-[repeat(auto-fill,minmax(180px,1fr))]', isFetching && 'opacity-60 transition-opacity')}>
            {data.data.map((item) => (
              <SimilarTile key={item.id} item={item} />
            ))}
          </div>

          <div className="flex justify-end">
            <Button variant="ghost" size="sm" onClick={clearIds}>
              Clear set
            </Button>
          </div>
        </div>
      )}
    </>
  );
}
