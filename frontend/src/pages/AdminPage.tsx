import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { admin, downloadStatus, sources } from '@/lib/api';
import {
  PageHeader,
  Button,
  Card,
  Badge,
  Spinner,
  EmptyState,
} from '@/components/UI';
import { RefreshCw, Play, Loader2, ListChecks, AlertCircle } from 'lucide-react';

export function AdminPage() {
  const queryClient = useQueryClient();

  const { data: status } = useQuery({
    queryKey: ['download-status'],
    queryFn: () => downloadStatus.get(),
    refetchInterval: 5000,
  });

  const { data: missingData, isLoading: loadingMissing } = useQuery({
    queryKey: ['admin', 'missing-galleries'],
    queryFn: () => admin.missingGalleries({ limit: 50 }),
  });

  const { data: sourceList } = useQuery({
    queryKey: ['sources'],
    queryFn: () => sources.list({ limit: 200 }),
  });

  const crawlMut = useMutation({
    mutationFn: (id: number) => sources.crawl(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['sources'] }),
  });

  const missingGalleries = missingData?.data ?? [];

  return (
    <>
      <PageHeader title="Admin" description="System administration">
        <Button variant="secondary" size="sm" onClick={() => queryClient.invalidateQueries({ queryKey: ['admin'] })}>
          <RefreshCw size={14} /> Refresh
        </Button>
      </PageHeader>

      {/* System Status */}
      <Card className="mb-4">
        <h3 className="text-sm font-semibold text-white mb-3">System Status</h3>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm">
          <div className="flex items-center gap-2">
            <ListChecks size={14} className="text-zinc-400" />
            <span className="text-zinc-400">Crawlers:</span>
            <Badge variant={status?.crawler?.active_sources?.length ?? 0 > 0 ? 'warning' : 'default'}>
              {status?.crawler?.active_sources?.length ?? 0} active
            </Badge>
          </div>
          <div className="flex items-center gap-2">
            <Loader2 size={14} className="text-zinc-400" />
            <span className="text-zinc-400">Verification:</span>
            <Badge variant={status?.verification?.is_running ? 'warning' : 'default'}>
              {status?.verification?.is_running ? 'Running' : 'Idle'}
            </Badge>
          </div>
          <div className="flex items-center gap-2">
            <AlertCircle size={14} className="text-zinc-400" />
            <span className="text-zinc-400">Missing:</span>
            <Badge variant="danger">{status?.verification?.missing_found ?? 0}</Badge>
          </div>
          <div className="flex items-center gap-2">
            <Play size={14} className="text-zinc-400" />
            <span className="text-zinc-400">Videos:</span>
            <Badge variant={status?.videos?.is_running ? 'warning' : 'default'}>
              {status?.videos?.is_running ? 'Running' : 'Idle'}
            </Badge>
          </div>
        </div>
      </Card>

      {/* Sources */}
      <Card className="mb-4">
        <h3 className="text-sm font-semibold text-white mb-3">Sources</h3>
        {!sourceList ? (
          <Spinner />
        ) : sourceList.data.length === 0 ? (
          <EmptyState message="No sources configured." />
        ) : (
          <div className="space-y-2 max-h-64 overflow-y-auto">
            {sourceList.data.map((src) => (
              <div key={src.id} className="flex items-center justify-between p-2 rounded bg-zinc-800/50">
                <div className="min-w-0 flex-1">
                  <span className="text-sm text-zinc-300 truncate block">{src.name}</span>
                  <span className="text-xs text-zinc-500">{src.location}</span>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant={src.status === 'crawling' ? 'warning' : 'default'}>{src.status}</Badge>
                  <Button variant="ghost" size="sm" onClick={() => crawlMut.mutate(src.id)} disabled={crawlMut.isPending}>
                    <Play size={12} />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      {/* Missing Galleries */}
      <Card className="mb-4">
        <h3 className="text-sm font-semibold text-white mb-3">Missing Galleries</h3>
        {loadingMissing ? (
          <Spinner />
        ) : missingGalleries.length === 0 ? (
          <EmptyState message="No missing galleries found." />
        ) : (
          <div className="space-y-2 max-h-96 overflow-y-auto">
            {missingGalleries.map((mg: any, i: number) => (
              <div key={i} className="flex items-center justify-between p-2 rounded bg-zinc-800/50">
                <div className="min-w-0 flex-1">
                  <span className="text-sm text-zinc-300 truncate block">{mg.url || mg.name || `Missing #${i + 1}`}</span>
                  <span className="text-xs text-zinc-500">
                    {mg.provider && `${mg.provider} · `}
                    {mg.person_name && `Person: ${mg.person_name}`}
                  </span>
                </div>
                {mg.person_id && (
                  <Badge variant="info">Person #{mg.person_id}</Badge>
                )}
              </div>
            ))}
          </div>
        )}
      </Card>
    </>
  );
}
