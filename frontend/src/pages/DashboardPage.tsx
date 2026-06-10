import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { sources, admin, maintenance, stats, /* adminApi included below */ } from '@/lib/api';
import { adminApi } from '@/lib/api';
import {
  PageHeader,
  StatCard,
  Card,
  Spinner,
  Badge,
  Button,
  EmptyState,
} from '@/components/UI';
import {
  Globe,
  Images,
  Image,
  Film,
  Users,
  ListChecks,
  Download,
  RefreshCw,
  Play,
  Loader2,
  AlertCircle,
  ChevronDown,
  ChevronRight,
  Wrench,
} from 'lucide-react';
import { Link } from 'react-router-dom';

type SectionId = 'system-status' | 'sources' | 'missing-galleries' | 'maintenance' | 'failed-downloads';

export function DashboardPage() {
  const queryClient = useQueryClient();
  const [expanded, setExpanded] = useState<Set<SectionId>>(new Set());

  const toggle = (id: SectionId) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const { data: dashboardStats } = useQuery({
    queryKey: ['dashboard-stats'],
    queryFn: () => stats.dashboard(),
    refetchInterval: 5000,
  });
  const { data: sourceList } = useQuery({
    queryKey: ['sources', 'all'],
    queryFn: () => sources.list({ limit: 200 }),
    enabled: expanded.has('sources'),
  });
  const { data: missingData, isLoading: loadingMissing } = useQuery({
    queryKey: ['admin', 'missing-galleries'],
    queryFn: () => admin.missingGalleries({ limit: 50 }),
    enabled: expanded.has('missing-galleries'),
  });
  const { data: failedImagesData } = useQuery({
    queryKey: ['admin', 'failed-images'],
    queryFn: () => adminApi.getFailedImages(),
    enabled: expanded.has('failed-downloads'),
  });
  const { data: failedSourcesData } = useQuery({
    queryKey: ['admin', 'failed-sources'],
    queryFn: () => adminApi.getFailedSources(),
    enabled: expanded.has('failed-downloads'),
  });

  const retryImageMut = useMutation({
    mutationFn: (id: number) => adminApi.retryImage(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin', 'failed-images'] }),
  });
  const retryAllImagesMut = useMutation({
    mutationFn: () => adminApi.retryAllImages(),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin', 'failed-images'] }),
  });
  const retrySourceMut = useMutation({
    mutationFn: (id: number) => adminApi.retrySource(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin', 'failed-sources'] }),
  });
  const retryAllSourcesMut = useMutation({
    mutationFn: () => adminApi.retryAllSources(),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin', 'failed-sources'] }),
  });

  const crawlMut = useMutation({
    mutationFn: (id: number) => sources.crawl(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['sources'] }),
  });

  const [cleanupStatus, setCleanupStatus] = useState<string | null>(null);
  const cleanupMut = useMutation({
    mutationFn: (token: string) => maintenance.cleanupDupes(token),
    onSuccess: (data) => {
      setCleanupStatus(`Deleted ${data.deleted} duplicates (${data.url_duplicates} URL-based, ${data.filename_duplicates} filename-based)`);
    },
    onError: (err: any) => {
      setCleanupStatus(`Error: ${err.message || 'Unknown error'}`);
    },
  });

  const handleCleanupDupes = () => {
    const token = prompt('Enter maintenance token:');
    if (token !== null) {
      setCleanupStatus(null);
      cleanupMut.mutate(token);
    }
  };

  if (!dashboardStats) return <Spinner />;

  const d = dashboardStats;
  const activeCrawls = d.downloads?.crawler?.active_sources ?? [];
  const verificationActive = d.downloads?.verification?.is_running ?? false;
  const videosActive = d.downloads?.videos?.is_running ?? false;
  const missingGalleries = missingData?.data ?? [];
  const sourceItems = sourceList?.data ?? [];

  const SectionToggle = ({ id, label, count }: { id: SectionId; label: string; count?: number }) => (
    <button
      onClick={() => toggle(id)}
      className="flex items-center gap-2 text-sm font-medium text-zinc-300 hover:text-white transition-colors"
    >
      {expanded.has(id) ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
      {label}
      {count !== undefined && <Badge>{count}</Badge>}
    </button>
  );

  return (
    <>
      <PageHeader title="Dashboard" description="System overview">
        <Button variant="secondary" size="sm" onClick={() => queryClient.invalidateQueries()}>
          <RefreshCw size={14} /> Refresh
        </Button>
      </PageHeader>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <StatCard label="Sources" value={d.sources} icon={<Globe size={20} />} />
        <StatCard label="Galleries" value={d.galleries} icon={<Images size={20} />} />
        <StatCard label="Images" value={d.images} icon={<Image size={20} />} />
        <StatCard label="Videos" value={d.videos} icon={<Film size={20} />} />
        <StatCard label="People" value={d.people} icon={<Users size={20} />} />
        <StatCard label="Active Tasks" value={activeCrawls.length + (verificationActive ? 1 : 0) + (videosActive ? 1 : 0)} icon={<ListChecks size={20} />} />
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
        <Card>
          <div className="flex items-center gap-2 mb-3">
            <Download size={16} className="text-zinc-400" />
            <h3 className="text-sm font-medium text-white">Download Activity</h3>
          </div>
          <div className="space-y-2 text-sm">
            {activeCrawls.length > 0 ? (
              activeCrawls.slice(0, 5).map((src) => (
                <div key={src.id} className="flex items-center justify-between">
                  <span className="text-zinc-400 truncate">{src.name || src.location}</span>
                  <Badge variant="info">{src.download_progress}%</Badge>
                </div>
              ))
            ) : (
              <p className="text-zinc-500 italic">No active downloads</p>
            )}
          </div>
        </Card>

        <Card>
          <div className="flex items-center gap-2 mb-3">
            <ListChecks size={16} className="text-zinc-400" />
            <h3 className="text-sm font-medium text-white">Queue Status</h3>
          </div>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-zinc-400">Active Crawls</span>
              <Badge variant="info">{activeCrawls.length}</Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-zinc-400">Verification Running</span>
              <Badge variant={verificationActive ? 'warning' : 'success'}>
                {verificationActive ? 'Yes' : 'No'}
              </Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-zinc-400">Video Verification</span>
              <Badge variant={videosActive ? 'warning' : 'success'}>
                {videosActive ? 'Yes' : 'No'}
              </Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-zinc-400">Missing Images</span>
              <Badge variant="danger">{d.downloads?.verification?.missing_found ?? 0}</Badge>
            </div>
          </div>
        </Card>
      </div>

      {/* Collapsible Admin Sections */}
      <div className="space-y-2 mb-6">
        {/* System Status */}
        <Card>
          <SectionToggle id="system-status" label="System Status" />
          {expanded.has('system-status') && (
            <div className="mt-3 grid grid-cols-2 md:grid-cols-4 gap-3 text-sm">
              <div className="flex items-center gap-2">
                <ListChecks size={14} className="text-zinc-400" />
                <span className="text-zinc-400">Crawlers:</span>
                <Badge variant={activeCrawls.length > 0 ? 'warning' : 'default'}>
                  {activeCrawls.length} active
                </Badge>
              </div>
              <div className="flex items-center gap-2">
                <Loader2 size={14} className="text-zinc-400" />
                <span className="text-zinc-400">Verification:</span>
                <Badge variant={verificationActive ? 'warning' : 'default'}>
                  {verificationActive ? 'Running' : 'Idle'}
                </Badge>
              </div>
              <div className="flex items-center gap-2">
                <AlertCircle size={14} className="text-zinc-400" />
                <span className="text-zinc-400">Missing:</span>
                <Badge variant="danger">{d.downloads?.verification?.missing_found ?? 0}</Badge>
              </div>
              <div className="flex items-center gap-2">
                <Play size={14} className="text-zinc-400" />
                <span className="text-zinc-400">Videos:</span>
                <Badge variant={videosActive ? 'warning' : 'default'}>
                  {videosActive ? 'Running' : 'Idle'}
                </Badge>
              </div>
            </div>
          )}
        </Card>

        {/* Sources */}
        <Card>
          <SectionToggle id="sources" label="Sources" count={sourceItems.length} />
          {expanded.has('sources') && (
            <div className="mt-3">
              {sourceItems.length === 0 ? (
                <EmptyState message="No sources configured." />
              ) : (
                <div className="space-y-2 max-h-64 overflow-y-auto">
                  {sourceItems.map((src) => (
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
              <div className="mt-3">
                <Link to="/sources" className="text-xs text-blue-400 hover:text-blue-300 transition-colors">
                  Manage all sources &rarr;
                </Link>
              </div>
            </div>
          )}
        </Card>

        {/* Missing Galleries */}
        <Card>
          <SectionToggle id="missing-galleries" label="Missing Galleries" count={missingGalleries.length} />
          {expanded.has('missing-galleries') && (
            <div className="mt-3">
              {loadingMissing ? (
                <Spinner />
              ) : missingGalleries.length === 0 ? (
                <EmptyState message="No missing galleries found." />
              ) : (
                <div className="space-y-2 max-h-96 overflow-y-auto">
                  {missingGalleries.map((mg: any, i: number) => (
                    <div key={i} className="flex items-center justify-between p-2 rounded bg-zinc-800/50">
                      <div className="min-w-0 flex-1">
                        <span className="text-sm text-zinc-300 truncate block">{mg.gallery_url || mg.gallery_name || `Missing #${i + 1}`}</span>
                        <span className="text-xs text-zinc-500">
                          {mg.provider && `${mg.provider} · `}
                          {mg.person_name && (
                            <Link to={`/people/${mg.person_id}`} className="hover:text-blue-400 transition-colors">
                              Person: {mg.person_name}
                            </Link>
                          )}
                        </span>
                      </div>
                      <div className="flex items-center gap-2 shrink-0">
                        {mg.person_id && (
                          <Link to={`/people/${mg.person_id}`}>
                            <Badge variant="info">Person #{mg.person_id}</Badge>
                          </Link>
                        )}
                        {mg.alias && <Badge variant="default">{mg.alias}</Badge>}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </Card>

        {/* Failed Downloads (images + sources) */}
        <Card>
          <SectionToggle id="failed-downloads" label="Failed Downloads" />
          {expanded.has('failed-downloads') && (
            <div className="mt-3 space-y-3">
              <div>
                <div className="flex items-center justify-between mb-2">
                  <h4 className="text-sm font-medium text-white">Images (missing files)</h4>
                  <div className="flex items-center gap-2">
                    <Button size="sm" variant="ghost" onClick={() => retryAllImagesMut.mutate()} disabled={retryAllImagesMut.isPending}>
                      Retry All
                    </Button>
                  </div>
                </div>
                <div className="space-y-2 max-h-48 overflow-y-auto">
                  {(failedImagesData?.data ?? []).length === 0 ? (
                    <p className="text-xs text-zinc-500">No missing images found.</p>
                  ) : (
                    (failedImagesData?.data ?? []).map((img: any) => (
                      <div key={img.id} className="flex items-center justify-between p-2 rounded bg-zinc-800/50">
                        <div className="min-w-0 flex-1">
                          <span className="text-sm text-zinc-300 truncate block">{img.filename || img.original_url || `#${img.id}`}</span>
                          <span className="text-xs text-zinc-500">ID: {img.id} · {img.type}</span>
                        </div>
                        <div className="flex items-center gap-2">
                          <Button size="sm" variant="ghost" onClick={() => retryImageMut.mutate(img.id)} disabled={retryImageMut.isPending}>
                            Retry
                          </Button>
                        </div>
                      </div>
                    ))
                  )}
                </div>
              </div>

              <div>
                <div className="flex items-center justify-between mb-2">
                  <h4 className="text-sm font-medium text-white">Sources (errored)</h4>
                  <div className="flex items-center gap-2">
                    <Button size="sm" variant="ghost" onClick={() => retryAllSourcesMut.mutate()} disabled={retryAllSourcesMut.isPending}>
                      Retry All
                    </Button>
                  </div>
                </div>
                <div className="space-y-2 max-h-48 overflow-y-auto">
                  {(failedSourcesData?.data ?? []).length === 0 ? (
                    <p className="text-xs text-zinc-500">No errored sources.</p>
                  ) : (
                    (failedSourcesData?.data ?? []).map((s: any) => (
                      <div key={s.id} className="flex items-center justify-between p-2 rounded bg-zinc-800/50">
                        <div className="min-w-0 flex-1">
                          <span className="text-sm text-zinc-300 truncate block">{s.name || s.location}</span>
                          <span className="text-xs text-zinc-500">{s.location}</span>
                        </div>
                        <div className="flex items-center gap-2">
                          <Badge variant="danger">{s.status}</Badge>
                          <Button size="sm" variant="ghost" onClick={() => retrySourceMut.mutate(s.id)} disabled={retrySourceMut.isPending}>
                            Retry
                          </Button>
                        </div>
                      </div>
                    ))
                  )}
                </div>
              </div>
            </div>
          )}
        </Card>

        {/* Maintenance */}
        <Card>
          <SectionToggle id="maintenance" label="Maintenance" />
          {expanded.has('maintenance') && (
            <div className="mt-3 space-y-3">
              <p className="text-xs text-zinc-500">Remove duplicate images across all galleries. Requires maintenance token.</p>
              <Button size="sm" variant="danger" onClick={handleCleanupDupes} disabled={cleanupMut.isPending}>
                <Wrench size={14} /> {cleanupMut.isPending ? 'Running...' : 'Clean Up Duplicates'}
              </Button>
              {cleanupStatus && (
                <div className={`p-2.5 rounded text-xs border ${
                  cleanupStatus.startsWith('Error')
                    ? 'bg-red-950/50 text-red-400 border-red-800/80'
                    : 'bg-emerald-950/50 text-emerald-400 border-emerald-800/80'
                }`}>
                  {cleanupStatus}
                </div>
              )}
            </div>
          )}
        </Card>
      </div>
    </>
  );
}
