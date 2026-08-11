import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { sources, admin, maintenance, stats, adminApi } from '@/lib/api';
import { Spinner, Badge, Button } from '@/components/UI';
import { Link } from 'react-router-dom';
import {
  Globe,
  Images,
  Image,
  Film,
  Users,
  Download,
  RefreshCw,
  Play,
  Loader2,
  Wrench,
  ListChecks,
  Settings2,
  ChevronRight,
} from 'lucide-react';
import { cn } from '@/lib/utils';

const quickLinks = [
  { to: '/galleries', label: 'Galleries', icon: Images, desc: 'Browse collections', color: 'text-blue-400' },
  { to: '/images', label: 'Images', icon: Image, desc: 'View all images', color: 'text-violet-400' },
  { to: '/videos', label: 'Videos', icon: Film, desc: 'Watch videos', color: 'text-rose-400' },
  { to: '/people', label: 'People', icon: Users, desc: 'Performer profiles', color: 'text-emerald-400' },
  { to: '/sources', label: 'Sources', icon: Globe, desc: 'Manage crawlers', color: 'text-amber-400' },
  { to: '/tags', label: 'Tags', icon: ListChecks, desc: 'Organize content', color: 'text-sky-400' },
];

export function DashboardPage() {
  const queryClient = useQueryClient();
  const [showAdmin, setShowAdmin] = useState(false);

  const { data: d } = useQuery({
    queryKey: ['dashboard-stats'],
    queryFn: () => stats.dashboard(),
    refetchInterval: 5000,
  });

  const { data: sourceList } = useQuery({
    queryKey: ['sources', 'all'],
    queryFn: () => sources.list({ limit: 200 }),
    enabled: showAdmin,
  });

  const { data: missingData, isLoading: loadingMissing } = useQuery({
    queryKey: ['admin', 'missing-galleries'],
    queryFn: () => admin.missingGalleries({ limit: 50 }),
    enabled: showAdmin,
  });

  const { data: failedImagesData } = useQuery({
    queryKey: ['admin', 'failed-images'],
    queryFn: () => adminApi.getFailedImages(),
    enabled: showAdmin,
  });

  const { data: failedSourcesData } = useQuery({
    queryKey: ['admin', 'failed-sources'],
    queryFn: () => adminApi.getFailedSources(),
    enabled: showAdmin,
  });

  const { data: embedStatus } = useQuery({
    queryKey: ['admin', 'embed-status'],
    queryFn: () => adminApi.getEmbedStatus(),
    enabled: showAdmin,
    refetchInterval: 5000,
  });

  const backfillMut = useMutation({
    mutationFn: () => adminApi.backfillEmbeddings(),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin', 'embed-status'] }),
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
      setCleanupStatus(`Deleted ${data.deleted} duplicates (${data.url_duplicates} URL, ${data.filename_duplicates} filename)`);
    },
    onError: (err: any) => {
      setCleanupStatus(`Error: ${err.message || 'Unknown'}`);
    },
  });

  const handleCleanupDupes = () => {
    const token = prompt('Enter maintenance token:');
    if (token !== null) {
      setCleanupStatus(null);
      cleanupMut.mutate(token);
    }
  };

  const [recheckStatus, setRecheckStatus] = useState<string | null>(null);
  const recheckAllMut = useMutation({
    mutationFn: () => admin.recheckAll(),
    onSuccess: (data) => {
      setRecheckStatus(`Recheck queued for ${data.queued} alias+provider combos across all people.`);
      queryClient.invalidateQueries({ queryKey: ['admin', 'missing-galleries'] });
    },
    onError: (err: any) => {
      setRecheckStatus(`Error: ${err.message || 'Failed to trigger recheck'}`);
    },
  });

  if (!d) return <Spinner />;

  const activeCrawls = d.downloads?.crawler?.active_sources ?? [];
  const verificationActive = d.downloads?.verification?.is_running ?? false;
  const videosActive = d.downloads?.videos?.is_running ?? false;
  const totalActive = activeCrawls.length + (verificationActive ? 1 : 0) + (videosActive ? 1 : 0);
  const sourceItems = sourceList?.data ?? [];
  const missingGalleries = missingData?.data ?? [];

  return (
    <div className="space-y-10">
      {/* Hero stats */}
      <section>
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-3xl font-bold text-white tracking-tight">Overview</h1>
            <p className="text-sm text-zinc-500 mt-1">Your media collection at a glance</p>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="sm" onClick={() => setShowAdmin(!showAdmin)}>
              <Settings2 size={14} />
              {showAdmin ? 'Hide Admin' : 'Admin'}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => queryClient.invalidateQueries()}>
              <RefreshCw size={14} /> Refresh
            </Button>
          </div>
        </div>

        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4">
            <div className="flex items-center gap-2 text-zinc-500 mb-1">
              <Globe size={14} />
              <span className="text-xs font-medium uppercase tracking-wider">Sources</span>
            </div>
            <p className="text-2xl font-bold text-white">{d.sources}</p>
          </div>
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4">
            <div className="flex items-center gap-2 text-zinc-500 mb-1">
              <Images size={14} />
              <span className="text-xs font-medium uppercase tracking-wider">Galleries</span>
            </div>
            <p className="text-2xl font-bold text-white">{d.galleries}</p>
          </div>
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4">
            <div className="flex items-center gap-2 text-zinc-500 mb-1">
              <Image size={14} />
              <span className="text-xs font-medium uppercase tracking-wider">Images</span>
            </div>
            <p className="text-2xl font-bold text-white">{d.images}</p>
          </div>
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4">
            <div className="flex items-center gap-2 text-zinc-500 mb-1">
              <Film size={14} />
              <span className="text-xs font-medium uppercase tracking-wider">Videos</span>
            </div>
            <p className="text-2xl font-bold text-white">{d.videos}</p>
          </div>
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4">
            <div className="flex items-center gap-2 text-zinc-500 mb-1">
              <Users size={14} />
              <span className="text-xs font-medium uppercase tracking-wider">People</span>
            </div>
            <p className="text-2xl font-bold text-white">{d.people}</p>
          </div>
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4">
            <div className="flex items-center gap-2 text-zinc-500 mb-1">
              <Download size={14} />
              <span className="text-xs font-medium uppercase tracking-wider">Active</span>
            </div>
            <div className="flex items-center gap-2">
              <p className="text-2xl font-bold text-white">{totalActive}</p>
              {totalActive > 0 && (
                <span className="w-2 h-2 rounded-full bg-blue-400 animate-pulse" />
              )}
            </div>
          </div>
        </div>
      </section>

      {/* Quick navigation */}
      <section>
        <h2 className="text-sm font-semibold text-zinc-400 uppercase tracking-wider mb-4">Quick Access</h2>
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
          {quickLinks.map((link) => (
            <Link
              key={link.to}
              to={link.to}
              className="group rounded-lg border border-zinc-800 bg-zinc-900/20 hover:bg-zinc-900/60 hover:border-zinc-700 p-4 transition-all"
            >
              <link.icon size={20} className={cn('mb-2', link.color)} />
              <p className="text-sm font-medium text-zinc-200 group-hover:text-white transition-colors">{link.label}</p>
              <p className="text-xs text-zinc-500 mt-0.5">{link.desc}</p>
            </Link>
          ))}
        </div>
      </section>

      {/* Activity bar */}
      <section className="rounded-lg border border-zinc-800 bg-zinc-900/20">
        <div className="p-4 border-b border-zinc-800">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold text-zinc-400 uppercase tracking-wider">Download Activity</h2>
          </div>
        </div>
        <div className="p-4">
          {totalActive > 0 ? (
            <div className="space-y-3">
              {activeCrawls.length > 0 && (
                <div>
                  <p className="text-xs font-medium text-zinc-500 mb-2">Active crawls</p>
                  <div className="space-y-2">
                    {activeCrawls.slice(0, 5).map((src) => (
                      <div key={src.id} className="flex items-center justify-between">
                        <span className="text-sm text-zinc-300 truncate">{src.name || src.location}</span>
                        <div className="flex items-center gap-2">
                          <div className="w-24 h-1.5 rounded-full bg-zinc-800 overflow-hidden">
                            <div className="h-full bg-blue-500 rounded-full transition-all" style={{ width: `${src.download_progress}%` }} />
                          </div>
                          <span className="text-xs text-zinc-500 w-8 text-right">{src.download_progress}%</span>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
              <div className="flex flex-wrap gap-3 text-xs text-zinc-500">
                {verificationActive && (
                  <span className="flex items-center gap-1.5">
                    <Loader2 size={12} className="animate-spin text-blue-400" />
                    Verification running
                  </span>
                )}
                {videosActive && (
                  <span className="flex items-center gap-1.5">
                    <Loader2 size={12} className="animate-spin text-blue-400" />
                    Video processing
                  </span>
                )}
              </div>
            </div>
          ) : (
            <p className="text-sm text-zinc-500">No active downloads</p>
          )}
        </div>
        <div className="px-4 pb-4 flex gap-4 text-xs text-zinc-500">
          <span>Missing: <span className="text-zinc-300 font-medium">{d.downloads?.verification?.missing_found ?? 0}</span></span>
          <span>Verified: <span className="text-zinc-300 font-medium">{d.downloads?.verification?.processed ?? 0}</span></span>
        </div>
      </section>

      {/* Admin section (collapsible) */}
      {showAdmin && (
        <section className="space-y-4">
          <h2 className="text-sm font-semibold text-zinc-400 uppercase tracking-wider">Administration</h2>

          {/* Sources */}
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/20">
            <div className="p-4 border-b border-zinc-800">
              <h3 className="text-sm font-medium text-zinc-200">Sources ({sourceItems.length})</h3>
            </div>
            <div className="p-4">
              {sourceItems.length === 0 ? (
                <p className="text-sm text-zinc-500">No sources configured.</p>
              ) : (
                <div className="space-y-2">
                  {sourceItems.map((src) => (
                    <div key={src.id} className="flex items-center justify-between py-2">
                      <div className="min-w-0 flex-1">
                        <span className="text-sm text-zinc-300 truncate block">{src.name}</span>
                        <span className="text-xs text-zinc-500">{src.location}</span>
                      </div>
                      <div className="flex items-center gap-2 shrink-0">
                        <Badge variant={src.status === 'crawling' ? 'warning' : 'default'}>{src.status}</Badge>
                        <Button variant="ghost" size="sm" onClick={() => crawlMut.mutate(src.id)} disabled={crawlMut.isPending}>
                          <Play size={12} />
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
              <Link to="/sources" className="inline-flex items-center gap-1 text-xs text-blue-400 hover:text-blue-300 mt-3">
                Manage all sources <ChevronRight size={12} />
              </Link>
            </div>
          </div>

          {/* Missing Galleries */}
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/20">
            <div className="p-4 border-b border-zinc-800 flex items-center justify-between">
              <h3 className="text-sm font-medium text-zinc-200">Missing Galleries ({missingGalleries.length})</h3>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  setRecheckStatus(null);
                  recheckAllMut.mutate();
                }}
                disabled={recheckAllMut.isPending}
                className="gap-1.5 text-xs"
              >
                <RefreshCw size={13} className={cn(recheckAllMut.isPending && 'animate-spin')} />
                {recheckAllMut.isPending ? 'Rechecking...' : 'Recheck All People'}
              </Button>
            </div>
            <div className="p-4">
              {recheckStatus && (
                <div
                  className={cn(
                    'mb-3 p-2.5 rounded text-xs border',
                    recheckStatus.startsWith('Error')
                      ? 'bg-red-950/50 text-red-400 border-red-800/80'
                      : 'bg-blue-950/50 text-blue-300 border-blue-800/80',
                  )}
                >
                  {recheckStatus}
                </div>
              )}
              {loadingMissing ? (
                <Spinner size="sm" />
              ) : missingGalleries.length === 0 ? (
                <p className="text-sm text-zinc-500">No missing galleries found.</p>
              ) : (
                <div className="space-y-2 max-h-64 overflow-y-auto">
                  {missingGalleries.map((mg: any, i: number) => (
                    <div key={i} className="flex items-center justify-between py-2">
                      <div className="min-w-0 flex-1">
                        <span className="text-sm text-zinc-300 truncate block">{mg.gallery_url || mg.gallery_name || `Missing #${i + 1}`}</span>
                        <span className="text-xs text-zinc-500">{mg.provider && `${mg.provider} · `}{mg.person_name && <Link to={`/people/${mg.person_id}`} className="hover:text-blue-400">{mg.person_name}</Link>}</span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* Failed Downloads */}
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/20">
            <div className="p-4 border-b border-zinc-800">
              <h3 className="text-sm font-medium text-zinc-200">Failed Downloads</h3>
            </div>
            <div className="p-4 space-y-4">
              <div>
                <div className="flex items-center justify-between mb-2">
                  <h4 className="text-xs font-medium text-zinc-400">Images (missing files)</h4>
                  <Button size="sm" variant="ghost" onClick={() => retryAllImagesMut.mutate()} disabled={retryAllImagesMut.isPending}>
                    Retry All
                  </Button>
                </div>
                <div className="space-y-1 max-h-40 overflow-y-auto">
                  {(failedImagesData?.data ?? []).length === 0 ? (
                    <p className="text-xs text-zinc-500">No missing images.</p>
                  ) : (
                    (failedImagesData?.data ?? []).map((img: any) => (
                      <div key={img.id} className="flex items-center justify-between py-1.5">
                        <span className="text-sm text-zinc-300 truncate">{img.filename || img.original_url || `#${img.id}`}</span>
                        <Button size="sm" variant="ghost" onClick={() => retryImageMut.mutate(img.id)} disabled={retryImageMut.isPending}>
                          Retry
                        </Button>
                      </div>
                    ))
                  )}
                </div>
              </div>
              <div>
                <div className="flex items-center justify-between mb-2">
                  <h4 className="text-xs font-medium text-zinc-400">Sources (errored)</h4>
                  <Button size="sm" variant="ghost" onClick={() => retryAllSourcesMut.mutate()} disabled={retryAllSourcesMut.isPending}>
                    Retry All
                  </Button>
                </div>
                <div className="space-y-1 max-h-40 overflow-y-auto">
                  {(failedSourcesData?.data ?? []).length === 0 ? (
                    <p className="text-xs text-zinc-500">No errored sources.</p>
                  ) : (
                    (failedSourcesData?.data ?? []).map((s: any) => (
                      <div key={s.id} className="flex items-center justify-between py-1.5">
                        <span className="text-sm text-zinc-300 truncate">{s.name || s.location}</span>
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
          </div>

          {/* Embedding Status */}
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/20">
            <div className="p-4 border-b border-zinc-800 flex items-center justify-between">
              <h3 className="text-sm font-medium text-zinc-200">Embedding Status</h3>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => backfillMut.mutate()}
                disabled={backfillMut.isPending}
                className="gap-1.5 text-xs"
              >
                <RefreshCw size={13} className={cn(backfillMut.isPending && 'animate-spin')} />
                {backfillMut.isPending ? 'Backfilling...' : 'Backfill Missing'}
              </Button>
            </div>
            <div className="p-4 space-y-3">
              {!embedStatus ? (
                <Spinner size="sm" />
              ) : (
                <>
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-zinc-400">Embedder</span>
                    <span className="font-mono text-zinc-300">
                      {embedStatus.embedder} (dim {embedStatus.dimension})
                    </span>
                  </div>
                  <div>
                    <div className="flex items-center justify-between text-xs mb-1.5">
                      <span className="text-zinc-400">Indexed</span>
                      <span className="font-mono text-zinc-300">
                        {embedStatus.embedded} / {embedStatus.total_images}
                      </span>
                    </div>
                    <div className="h-1.5 rounded-full bg-zinc-800 overflow-hidden">
                      <div
                        className="h-full rounded-full bg-blue-500 transition-all"
                        style={{
                          width: `${embedStatus.total_images ? Math.min(100, (embedStatus.embedded / embedStatus.total_images) * 100) : 0}%`,
                        }}
                      />
                    </div>
                  </div>
                  <div className="grid grid-cols-4 gap-2">
                    <div className="rounded border border-zinc-800 bg-zinc-900/40 p-2 text-center">
                      <div className="text-sm font-mono text-zinc-200">{embedStatus.index_size}</div>
                      <div className="text-[10px] uppercase tracking-wider text-zinc-500">Index Size</div>
                    </div>
                    <div className="rounded border border-zinc-800 bg-zinc-900/40 p-2 text-center">
                      <div className="text-sm font-mono text-amber-400">{embedStatus.pending}</div>
                      <div className="text-[10px] uppercase tracking-wider text-zinc-500">Pending</div>
                    </div>
                    <div className="rounded border border-zinc-800 bg-zinc-900/40 p-2 text-center">
                      <div className={cn('text-sm font-mono', embedStatus.failed > 0 ? 'text-red-400' : 'text-zinc-200')}>
                        {embedStatus.failed}
                      </div>
                      <div className="text-[10px] uppercase tracking-wider text-zinc-500">Failed</div>
                    </div>
                    <div className="rounded border border-zinc-800 bg-zinc-900/40 p-2 text-center" title="Waiting for their file to be re-downloaded">
                      <div className={cn('text-sm font-mono', (embedStatus.deferred ?? 0) > 0 ? 'text-sky-400' : 'text-zinc-200')}>
                        {embedStatus.deferred ?? 0}
                      </div>
                      <div className="text-[10px] uppercase tracking-wider text-zinc-500">Deferred</div>
                    </div>
                  </div>
                </>
              )}
            </div>
          </div>

          {/* Maintenance */}
          <div className="rounded-lg border border-zinc-800 bg-zinc-900/20">
            <div className="p-4 border-b border-zinc-800">
              <h3 className="text-sm font-medium text-zinc-200">Maintenance</h3>
            </div>
            <div className="p-4 space-y-3">
              <p className="text-xs text-zinc-500">Remove duplicate images. Requires maintenance token.</p>
              <Button size="sm" variant="danger" onClick={handleCleanupDupes} disabled={cleanupMut.isPending}>
                <Wrench size={14} /> {cleanupMut.isPending ? 'Running...' : 'Clean Up Duplicates'}
              </Button>
              <a
                href="/api/export/db"
                className="inline-flex items-center gap-1.5 rounded-md font-medium transition-colors px-2.5 py-1.5 text-xs bg-blue-600 text-white hover:bg-blue-500 disabled:opacity-40 disabled:pointer-events-none"
                download
              >
                <Download size={14} /> Export DB
              </a>
              {cleanupStatus && (
                <div className={cn(
                  'p-2.5 rounded text-xs border',
                  cleanupStatus.startsWith('Error')
                    ? 'bg-red-950/50 text-red-400 border-red-800/80'
                    : 'bg-emerald-950/50 text-emerald-400 border-emerald-800/80',
                )}>
                  {cleanupStatus}
                </div>
              )}
            </div>
          </div>
        </section>
      )}
    </div>
  );
}
