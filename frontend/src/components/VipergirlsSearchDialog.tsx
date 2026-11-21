import { useEffect, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, Download, ExternalLink, Loader2, X } from 'lucide-react';
import { Badge, Button } from '@/components/UI';
import { admin } from '@/lib/api';
import type { VipergirlsSearchResult } from '@/types';

const SITE_LABELS: Record<string, string> = {
  vipergirls: 'vipergirls.to',
  'kitty-kats': 'kitty-kats.net',
};

interface VipergirlsSearchDialogProps {
  open: boolean;
  query: string;
  personId: number;
  provider: string;
  galleryName: string;
  missingUrl?: string;
  title?: string;
  onClose: () => void;
}

export function VipergirlsSearchDialog({
  open,
  query,
  personId,
  provider,
  galleryName,
  missingUrl,
  title,
  onClose,
}: VipergirlsSearchDialogProps) {
  const queryClient = useQueryClient();
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);
  const [scrapedKeys, setScrapedKeys] = useState<Set<string>>(new Set());
  const scrapeScope = `${personId}|${query}`;

  useEffect(() => {
    if (!open) return;
    const previouslyFocused = document.activeElement as HTMLElement | null;
    closeButtonRef.current?.focus();
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onCloseRef.current();
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      previouslyFocused?.focus();
    };
  }, [open]);

  const { data, isLoading, isFetching, error, refetch } = useQuery({
    queryKey: ['vipergirls-search', query],
    queryFn: () => admin.searchVipergirls(query),
    enabled: open && query.trim().length > 0,
    staleTime: 60_000,
    retry: 1,
  });

  const scrapeMut = useMutation({
    mutationFn: (result: VipergirlsSearchResult) =>
      admin.scrapeVipergirls({
        person_id: personId,
        provider,
        gallery_name: galleryName,
        missing_url: missingUrl,
        title: title || result.title,
        thread_url: result.thread_url,
        thumbnail_url: result.thumbnail_url,
      }),
    onSuccess: (_response, result) => {
      setScrapedKeys((previous) => new Set(previous).add(`${scrapeScope}|${result.thread_url}`));
      queryClient.invalidateQueries({ queryKey: ['admin', 'missing-galleries'] });
      queryClient.invalidateQueries({ queryKey: ['person-scans', personId] });
      queryClient.invalidateQueries({ queryKey: ['person', personId] });
      queryClient.invalidateQueries({ queryKey: ['dashboard-stats'] });
      queryClient.invalidateQueries({ queryKey: ['sources'] });
      onClose();
    },
  });

  if (!open) return null;

  const results = data?.data ?? [];
  const sourceErrors = (data?.sources ?? []).filter((source) => Boolean(source.error));
  const pendingUrl = scrapeMut.isPending ? scrapeMut.variables?.thread_url : undefined;
  const errorMessage = error instanceof Error ? error.message : error ? 'Search failed' : null;
  const scrapeError = scrapeMut.error instanceof Error
    ? scrapeMut.error.message
    : scrapeMut.error ? 'Scrape failed' : null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
      onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="vipergirls-dialog-title"
        className="flex max-h-[85vh] w-full max-w-5xl flex-col overflow-hidden rounded-lg border border-zinc-700 bg-zinc-900 shadow-none"
      >
        <div className="flex items-start justify-between gap-4 border-b border-zinc-800 p-4">
          <div className="min-w-0">
            <h2 id="vipergirls-dialog-title" className="text-base font-semibold text-white">
              Forum thread results
            </h2>
            <p className="mt-1 truncate text-xs text-zinc-500" title={query}>
              Search: <span className="text-zinc-300">{query || '(empty query)'}</span>
              {data && <span className="ml-2">· {results.length} result{results.length === 1 ? '' : 's'}</span>}
            </p>
          </div>
          <button
            ref={closeButtonRef}
            type="button"
            onClick={onClose}
            aria-label="Close search results"
            className="rounded-md p-1.5 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
          >
            <X size={18} />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto p-4">
          {(errorMessage || scrapeError) && (
            <div className="mb-4 flex items-start gap-2 rounded border border-red-900/70 bg-red-950/40 p-3 text-xs text-red-300">
              <AlertTriangle size={14} className="mt-0.5 shrink-0" />
              <div className="min-w-0 flex-1">
                <p>{scrapeError || errorMessage}</p>
                {errorMessage && (
                  <button
                    type="button"
                    onClick={() => refetch()}
                    className="mt-1 font-medium text-red-200 underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
                  >
                    Retry search
                  </button>
                )}
              </div>
            </div>
          )}

          {sourceErrors.length > 0 && !errorMessage && (
            <div className="mb-4 rounded border border-amber-900/70 bg-amber-950/30 p-3 text-xs text-amber-300">
              <p className="font-medium">Some forums could not be searched:</p>
              <ul className="mt-1 list-inside list-disc space-y-0.5">
                {sourceErrors.map((source) => (
                  <li key={source.site}>
                    <span className="text-amber-200">{SITE_LABELS[source.site] ?? source.label}:</span>{' '}
                    {source.error}
                  </li>
                ))}
              </ul>
            </div>
          )}

          {!query.trim() && (
            <p className="py-10 text-center text-xs text-zinc-500">
              No search terms are available for this missing gallery.
            </p>
          )}

          {query.trim() && isLoading && (
            <div className="flex items-center justify-center gap-2 py-12 text-xs text-zinc-400">
              <Loader2 size={14} className="animate-spin" /> Searching forums...
            </div>
          )}

          {query.trim() && !isLoading && !error && results.length === 0 && (
            <div className="py-10 text-center text-xs text-zinc-500">
              <p>No results found.</p>
              {(data?.sources ?? []).length > 0 && (
                <div className="mt-2 flex flex-wrap items-center justify-center gap-x-4 gap-y-1">
                  {(data?.sources ?? []).map((source) => (
                    <a
                      key={source.site}
                      href={source.search_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1 text-blue-400 underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                    >
                      Open on {SITE_LABELS[source.site] ?? source.label} <ExternalLink size={10} />
                    </a>
                  ))}
                </div>
              )}
            </div>
          )}

          {results.length > 0 && (
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
              {results.map((result) => {
                const isScraping = pendingUrl === result.thread_url;
                const wasScraped = scrapedKeys.has(`${scrapeScope}|${result.thread_url}`);
                return (
                  <div
                    key={`${result.site}|${result.thread_url}`}
                    className="flex flex-col overflow-hidden rounded-lg border border-zinc-800 bg-zinc-900/60 hover:border-zinc-700"
                  >
                    <div className="aspect-[3/2] overflow-hidden bg-zinc-800">
                      {result.thumbnail_url ? (
                        <img
                          src={result.thumbnail_url}
                          alt=""
                          loading="lazy"
                          className="h-full w-full object-cover"
                          onError={(event) => { event.currentTarget.style.display = 'none'; }}
                        />
                      ) : (
                        <div className="flex h-full items-center justify-center text-[10px] text-zinc-600">
                          No thumbnail
                        </div>
                      )}
                    </div>
                    <div className="flex flex-1 flex-col p-2.5">
                      <p className="line-clamp-2 text-xs font-medium text-zinc-200" title={result.title}>
                        {result.title}
                      </p>
                      <div className="mt-2 flex flex-wrap gap-1">
                        <Badge variant="default" className="text-[10px]">
                          {SITE_LABELS[result.site] ?? result.site}
                        </Badge>
                        <Badge variant="info" className="max-w-full truncate text-[10px]">
                          {result.sub_forum || 'Unknown forum'}
                        </Badge>
                        {wasScraped && (
                          <Badge variant="success" className="text-[10px]">Linked</Badge>
                        )}
                      </div>
                      <div className="mt-3 flex items-center gap-1.5 border-t border-zinc-800 pt-2">
                        <a
                          href={result.thread_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex h-7 items-center justify-center gap-1 rounded-md border border-zinc-700 px-2 text-[10px] text-zinc-300 hover:bg-zinc-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                        >
                          View post <ExternalLink size={10} />
                        </a>
                        <Button
                          size="sm"
                          className="h-7 px-2 text-[10px]"
                          disabled={isScraping || wasScraped}
                          onClick={() => scrapeMut.mutate(result)}
                        >
                          {isScraping ? <Loader2 size={10} className="animate-spin" /> : <Download size={10} />}
                          {isScraping ? 'Scraping...' : 'Scrape'}
                        </Button>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}

          {isFetching && !isLoading && results.length > 0 && (
            <p className="mt-3 text-center text-[10px] text-zinc-500">Refreshing results...</p>
          )}
        </div>
      </div>
    </div>
  );
}
