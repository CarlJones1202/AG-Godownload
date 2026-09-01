import { useEffect, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useSearchParams, Link } from 'react-router-dom';
import { sources, admin, maintenance, stats, adminApi, profile, people } from '@/lib/api';
import type { Image as GalleryImage, NotWantedGallery, Source } from '@/types';
import { Spinner, Badge, Button, Input, Tabs } from '@/components/UI';
import {
  Globe,
  Images,
  Image,
  Film,
  Users,
  Download,
  Upload,
  RefreshCw,
  Play,
  Loader2,
  Wrench,
  Settings2,
  ChevronRight,
  ExternalLink,
  Search,
  AlertTriangle,
  Database,
  Trash2,
  EyeOff,
} from 'lucide-react';
import { cn } from '@/lib/utils';

const PROVIDER_OPTIONS = [
  "MetArt", "MetartX", "Playboy", "PlayboyPlus", "Vixen",
  "SexArt", "LifeErotic", "EternalDesire", "MPLStudios",
  "VivThomas", "WowGirls", "RylskyArt",
];

type Tab = 'overview' | 'attention' | 'downloads' | 'admin';
const TAB_ORDER: Tab[] = ['overview', 'attention', 'downloads', 'admin'];

const quickLinks = [
  { to: '/galleries', label: 'Galleries', icon: Images, desc: 'Browse collections', color: 'text-blue-400' },
  { to: '/images', label: 'Images', icon: Image, desc: 'View all images', color: 'text-violet-400' },
  { to: '/videos', label: 'Videos', icon: Film, desc: 'Watch videos', color: 'text-rose-400' },
  { to: '/people', label: 'People', icon: Users, desc: 'Performer profiles', color: 'text-emerald-400' },
  { to: '/sources', label: 'Sources', icon: Globe, desc: 'Manage crawlers', color: 'text-amber-400' },
  { to: '/tags', label: 'Tags', icon: Settings2, desc: 'Organize content', color: 'text-sky-400' },
];

interface SectionCardProps {
  title: string;
  count?: number;
  action?: React.ReactNode;
  children: React.ReactNode;
}

function SectionCard({ title, count, action, children }: SectionCardProps) {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-900/20">
      <div className="p-4 border-b border-zinc-800 flex items-center justify-between gap-2">
        <h3 className="text-sm font-medium text-zinc-200 flex items-center gap-2">
          {title}
          {typeof count === 'number' && count > 0 && (
            <Badge variant="danger">{count}</Badge>
          )}
        </h3>
        {action}
      </div>
      <div className="p-4">{children}</div>
    </div>
  );
}

export function DashboardPage() {
  const queryClient = useQueryClient();
  const [params, setParams] = useSearchParams();
  const rawTab = params.get('tab') as Tab | null;
  const tab: Tab = rawTab && TAB_ORDER.includes(rawTab) ? rawTab : 'overview';
  const setTab = (t: Tab) => setParams(t === 'overview' ? {} : { tab: t }, { replace: true });

  const { data: d } = useQuery({
    queryKey: ['dashboard-stats'],
    queryFn: () => stats.dashboard(),
    refetchInterval: 5000,
  });

  const { data: sourceList } = useQuery({
    queryKey: ['sources', 'all'],
    queryFn: () => sources.list({ limit: 200 }),
    enabled: tab === 'admin',
  });

  const [missingPage, setMissingPage] = useState(1);
  const [missingQ, setMissingQ] = useState('');
  const [missingProvider, setMissingProvider] = useState('');
  const [rechecking, setRechecking] = useState(false);

  const { data: missingData, isLoading: loadingMissing, dataUpdatedAt: missingUpdatedAt } = useQuery({
    queryKey: ['admin', 'missing-galleries', { page: missingPage, q: missingQ, provider: missingProvider }],
    queryFn: () => admin.missingGalleries({
      page: missingPage,
      limit: 25,
      q: missingQ || undefined,
      provider: missingProvider || undefined,
    }),
    enabled: tab === 'attention',
    refetchInterval: rechecking ? 4000 : false,
  });

  const { data: failedImagesData } = useQuery({
    queryKey: ['admin', 'failed-images'],
    queryFn: () => adminApi.getFailedImages(),
    enabled: tab === 'attention',
  });

  const { data: failedSourcesData } = useQuery({
    queryKey: ['admin', 'failed-sources'],
    queryFn: () => adminApi.getFailedSources(),
    enabled: tab === 'attention',
  });

  const { data: embedStatus } = useQuery({
    queryKey: ['admin', 'embed-status'],
    queryFn: () => adminApi.getEmbedStatus(),
    enabled: tab === 'attention',
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
    onError: (err) => {
      setCleanupStatus(`Error: ${err instanceof Error ? err.message : 'Unknown'}`);
    },
  });

  const handleCleanupDupes = () => {
    const token = prompt('Enter maintenance token:');
    if (token !== null) {
      setCleanupStatus(null);
      cleanupMut.mutate(token);
    }
  };

  const [purgeStatus, setPurgeStatus] = useState<string | null>(null);
  const purgeMut = useMutation({
    mutationFn: (token: string) => maintenance.purgePlaceholders(token),
    onSuccess: (data) => {
      setPurgeStatus(
        data.placeholders_found > 0
          ? `Removed ${data.deleted_files} placeholder files (${data.deleted_from_db} from DB), scanned ${data.scanned_files} files`
          : `Scanned ${data.scanned_files} files — no placeholders found`,
      );
    },
    onError: (err) => {
      setPurgeStatus(`Error: ${err instanceof Error ? err.message : 'Unknown'}`);
    },
  });

  const handlePurgePlaceholders = () => {
    const token = prompt('Enter maintenance token:');
    if (token !== null) {
      setPurgeStatus(null);
      purgeMut.mutate(token);
    }
  };

  const resetProfileMut = useMutation({
    mutationFn: () => profile.reset(),
    onSuccess: () => {
      setCleanupStatus('Taste profile reset.');
      queryClient.invalidateQueries();
    },
    onError: (err) => {
      setCleanupStatus(`Error: ${err instanceof Error ? err.message : 'Unknown'}`);
    },
  });

  const handleResetProfile = () => {
    if (confirm('Reset the taste profile and clear all ratings?')) {
      setCleanupStatus(null);
      resetProfileMut.mutate();
    }
  };

  const [recheckStatus, setRecheckStatus] = useState<string | null>(null);
  const [recheckTriggeredAt, setRecheckTriggeredAt] = useState(0);
  const [importStatus, setImportStatus] = useState<string | null>(null);
  const [importing, setImporting] = useState(false);

  const handleImportFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    if (!confirm(`Are you sure you want to restore "${file.name}"?\nThis will overwrite the current database!`)) {
      e.target.value = '';
      return;
    }
    setImporting(true);
    setImportStatus(null);
    const formData = new FormData();
    formData.append('file', file);
    try {
      const res = await fetch('/api/import/db', {
        method: 'POST',
        body: formData,
      });
      const json = await res.json();
      if (!res.ok) {
        throw new Error(json.error || json.detail || 'Restore failed');
      }
      setImportStatus(`Success: ${json.message || 'Database restored'} (${json.file}, ${(json.bytes / 1024 / 1024).toFixed(2)} MB)`);
      queryClient.invalidateQueries();
    } catch (err) {
      setImportStatus(`Error: ${err instanceof Error ? err.message : 'Import failed'}`);
    } finally {
      setImporting(false);
      e.target.value = '';
    }
  };

  const recheckAllMut = useMutation({
    mutationFn: () => admin.recheckAll(),
    onSuccess: (data) => {
      setRechecking(true);
      setRecheckTriggeredAt(Date.now());
      setRecheckStatus(`Recheck queued for ${data.queued} alias+provider combos. Scanning...`);
      setMissingPage(1);
      queryClient.invalidateQueries({ queryKey: ['admin', 'missing-galleries'] });
    },
    onError: (err) => {
      setRechecking(false);
      setRecheckStatus(`Error: ${err instanceof Error ? err.message : 'Failed to trigger recheck'}`);
    },
  });

  useEffect(() => {
    if (
      rechecking &&
      missingData &&
      missingData.meta.pending_scans === 0 &&
      missingUpdatedAt > recheckTriggeredAt
    ) {
      setRechecking(false);
      setRecheckStatus('Recheck complete.');
    }
  }, [rechecking, missingData, missingUpdatedAt, recheckTriggeredAt]);

  const { data: notWantedData } = useQuery({
    queryKey: ['admin', 'not-wanted'],
    queryFn: () => admin.notWantedGalleries(),
    enabled: tab === 'attention',
  });
  const notWanted = notWantedData?.data ?? [];

  const excludeMissingMut = useMutation({
    mutationFn: (mg: { person_id: number; provider: string; source_url: string; title: string }) =>
      people.excludeScanResult(mg.person_id, { provider: mg.provider, source_url: mg.source_url, title: mg.title }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'missing-galleries'] });
      queryClient.invalidateQueries({ queryKey: ['admin', 'not-wanted'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard-stats'] });
    },
  });

  const unmarkMut = useMutation({
    mutationFn: (id: number) => admin.removeNotWanted(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'not-wanted'] });
      queryClient.invalidateQueries({ queryKey: ['admin', 'missing-galleries'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard-stats'] });
    },
  });

  if (!d) return <Spinner />;

  const a = d.attention ?? {
    missing_galleries: 0,
    missing_images: 0,
    missing_videos: 0,
    failed_sources: 0,
    embed_pending: 0,
    embed_failed: 0,
    embed_deferred: 0,
  };
  const attentionTotal =
    a.missing_galleries + a.missing_images + a.missing_videos + a.failed_sources + a.embed_failed;

  const activeCrawls = d.downloads?.crawler?.active_sources ?? [];
  const verification = d.downloads?.verification;
  const videos = d.downloads?.videos;
  const verificationActive = verification?.is_running ?? false;
  const videosActive = videos?.is_running ?? false;
  const totalActive = activeCrawls.length + (verificationActive ? 1 : 0) + (videosActive ? 1 : 0);
  const sourceItems = sourceList?.data ?? [];
  const missingGalleries = missingData?.data ?? [];
  const missingTotal = missingData?.meta?.total_items ?? 0;
  const missingPendingScans = missingData?.meta?.pending_scans ?? 0;
  const missingTotalPages = missingData?.meta?.total_pages ?? 1;
  const recheckBusy = recheckAllMut.isPending || rechecking;

  const tabs = [
    { value: 'overview' as Tab, label: 'Overview' },
    { value: 'attention' as Tab, label: 'Attention', count: attentionTotal },
    { value: 'downloads' as Tab, label: 'Downloads' },
    { value: 'admin' as Tab, label: 'Admin' },
  ];

  const attentionStrip = [
    { label: 'Missing galleries', count: a.missing_galleries, icon: Images },
    { label: 'Missing images', count: a.missing_images, icon: Image },
    { label: 'Missing videos', count: a.missing_videos, icon: Film },
    { label: 'Failed sources', count: a.failed_sources, icon: Globe },
    { label: 'Embed failures', count: a.embed_failed, icon: AlertTriangle },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold text-white tracking-tight">Control Center</h1>
          <p className="text-sm text-zinc-500 mt-1">Overview, attention, and everything in between</p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Button variant="ghost" size="sm" onClick={() => queryClient.invalidateQueries()}>
            <RefreshCw size={14} /> Refresh
          </Button>
        </div>
      </div>

      <Tabs value={tab} onChange={(v) => setTab(v as Tab)} tabs={tabs} />

      {tab === 'overview' && (
        <div className="space-y-6">
          {/* Stat cards */}
          <section>
            <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
              {[
                { label: 'Sources', value: d.sources, icon: Globe, to: '/sources' },
                { label: 'Galleries', value: d.galleries, icon: Images, to: '/galleries' },
                { label: 'Images', value: d.images, icon: Image, to: '/images' },
                { label: 'Videos', value: d.videos, icon: Film, to: '/videos' },
                { label: 'People', value: d.people, icon: Users, to: '/people' },
                { label: 'Active', value: totalActive, icon: Download },
              ].map((s) => (
                <Link
                  key={s.label}
                  to={s.to ?? '/dashboard'}
                  className={cn(
                    'rounded-lg border border-zinc-800 bg-zinc-900/30 p-4 transition-colors',
                    s.to && 'hover:border-zinc-700 hover:bg-zinc-900/60',
                  )}
                >
                  <div className="flex items-center gap-2 text-zinc-500 mb-1">
                    <s.icon size={14} />
                    <span className="text-xs font-medium uppercase tracking-wider">{s.label}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <p className="text-2xl font-bold text-white">{s.value}</p>
                    {s.label === 'Active' && totalActive > 0 && (
                      <span className="w-2 h-2 rounded-full bg-blue-400 animate-pulse" />
                    )}
                  </div>
                </Link>
              ))}
            </div>
          </section>

          {/* Attention strip */}
          <section className="rounded-lg border border-zinc-800 bg-zinc-900/20">
            <div className="p-4 border-b border-zinc-800 flex items-center justify-between">
              <h2 className="text-sm font-semibold text-zinc-400 uppercase tracking-wider">Needs Attention</h2>
              <Button variant="ghost" size="sm" onClick={() => setTab('attention')}>
                View all <ChevronRight size={12} />
              </Button>
            </div>
            <div className="p-4">
              {attentionTotal === 0 ? (
                <div className="flex items-center gap-2 text-sm text-emerald-400">
                  <span className="w-1.5 h-1.5 rounded-full bg-emerald-500" />
                  All clear — nothing needs attention right now.
                </div>
              ) : (
                <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
                  {attentionStrip.map((s) => (
                    <button
                      key={s.label}
                      onClick={() => setTab('attention')}
                      className={cn(
                        'flex items-center gap-3 rounded-lg border p-3 text-left transition-colors',
                        s.count > 0
                          ? 'border-red-900/60 bg-red-950/20 hover:bg-red-950/40'
                          : 'border-zinc-800 bg-zinc-900/40 hover:bg-zinc-900/60',
                      )}
                    >
                      <s.icon size={16} className={s.count > 0 ? 'text-red-400' : 'text-zinc-600'} />
                      <span className="min-w-0">
                        <span className={cn('block text-lg font-bold leading-tight', s.count > 0 ? 'text-white' : 'text-zinc-400')}>
                          {s.count}
                        </span>
                        <span className="block text-xs text-zinc-500 truncate">{s.label}</span>
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          </section>

          {/* Quick links + activity */}
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            <section className="lg:col-span-2">
              <h2 className="text-sm font-semibold text-zinc-400 uppercase tracking-wider mb-4">Quick Access</h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
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

            <section className="rounded-lg border border-zinc-800 bg-zinc-900/20 h-fit">
              <div className="p-4 border-b border-zinc-800">
                <h2 className="text-sm font-semibold text-zinc-400 uppercase tracking-wider">Download Activity</h2>
              </div>
              <div className="p-4">
                {totalActive > 0 ? (
                  <div className="space-y-3">
                    {activeCrawls.slice(0, 3).map((src) => (
                      <div key={src.id}>
                        <div className="flex items-center justify-between mb-1">
                          <span className="text-xs text-zinc-300 truncate">{src.name || src.location}</span>
                          <span className="text-xs text-zinc-500 shrink-0 ml-2">{src.download_progress}%</span>
                        </div>
                        <div className="w-full h-1.5 rounded-full bg-zinc-800 overflow-hidden">
                          <div className="h-full bg-blue-500 rounded-full transition-all" style={{ width: `${src.download_progress}%` }} />
                        </div>
                      </div>
                    ))}
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
                <Button variant="ghost" size="sm" className="mt-3" onClick={() => setTab('downloads')}>
                  View all activity <ChevronRight size={12} />
                </Button>
              </div>
            </section>
          </div>
        </div>
      )}

      {tab === 'attention' && (
        <div className="space-y-6">
          <SectionCard
            title="Missing Galleries"
            count={missingTotal}
            action={
              <div className="flex items-center gap-2">
                {recheckStatus && tab === 'attention' && (
                  <span className={cn(
                    'text-xs px-2.5 py-1 rounded border',
                    recheckStatus.startsWith('Error')
                      ? 'bg-red-950/50 text-red-400 border-red-800/80'
                      : 'bg-blue-950/50 text-blue-300 border-blue-800/80',
                  )}>
                    {recheckStatus}
                  </span>
                )}
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setRecheckStatus(null);
                    recheckAllMut.mutate();
                  }}
                  disabled={recheckBusy}
                  className="gap-1.5 text-xs"
                >
                  <RefreshCw size={13} className={cn(recheckBusy && 'animate-spin')} />
                  {recheckBusy ? 'Rechecking...' : 'Recheck All People'}
                </Button>
              </div>
            }
          >
            {missingPendingScans > 0 && (
              <div className="mb-3 flex items-center gap-2 p-2.5 rounded text-xs bg-blue-950/40 text-blue-300 border border-blue-800/60">
                <Loader2 size={12} className="animate-spin" />
                {missingPendingScans} scan{missingPendingScans === 1 ? '' : 's'} in progress — list updates automatically
              </div>
            )}
            <div className="flex gap-2 mb-3">
              <div className="relative flex-1">
                <Search size={13} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-500" />
                <Input
                  placeholder="Search title or person..."
                  value={missingQ}
                  onChange={(e) => { setMissingQ(e.target.value); setMissingPage(1); }}
                  className="pl-8 h-8 text-xs"
                />
              </div>
              <select
                value={missingProvider}
                onChange={(e) => { setMissingProvider(e.target.value); setMissingPage(1); }}
                className="bg-zinc-800 border border-zinc-700 rounded px-2 py-1 text-xs text-zinc-200 focus:outline-none cursor-pointer h-8"
              >
                <option value="">All providers</option>
                {PROVIDER_OPTIONS.map((p) => <option key={p} value={p}>{p}</option>)}
              </select>
            </div>
            {loadingMissing ? (
              <Spinner size="sm" />
            ) : missingGalleries.length === 0 ? (
              <p className="text-sm text-zinc-500">No missing galleries found.</p>
            ) : (
              <>
                <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-5 gap-3">
                  {missingGalleries.map((mg, i) => (
                    <div
                      key={`${mg.person_id}-${mg.gallery_url}-${i}`}
                      className="group relative block aspect-[3/2] rounded-lg overflow-hidden bg-zinc-900 border border-zinc-800 hover:border-zinc-700 transition-all"
                    >
                      {mg.thumbnail ? (
                        <img
                          src={mg.thumbnail}
                          alt={mg.gallery_name}
                          className="w-full h-full object-cover"
                          onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }}
                        />
                      ) : (
                        <div className="w-full h-full bg-zinc-800 flex items-center justify-center">
                          <span className="text-zinc-600 text-sm">No preview</span>
                        </div>
                      )}

                      <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 via-black/60 to-transparent pt-16 pb-3 px-3">
                        <p className="text-sm font-medium text-white truncate mb-1">{mg.gallery_name}</p>
                        <div className="flex items-center justify-between gap-2">
                          {mg.provider ? (
                            <span className="text-[10px] uppercase tracking-wider text-zinc-300 bg-black/50 px-1.5 py-0.5 rounded truncate">
                              {mg.provider}
                            </span>
                          ) : (
                            <span />
                          )}
                          {mg.release_date && (
                            <span className="text-[10px] text-zinc-400 shrink-0">{mg.release_date}</span>
                          )}
                        </div>
                      </div>

                      <div className="absolute top-2 left-2">
                        <Link
                          to={`/people/${mg.person_id}`}
                          className="text-[10px] text-zinc-200 bg-black/60 backdrop-blur-sm px-1.5 py-0.5 rounded hover:bg-black/80 hover:text-white transition-colors truncate max-w-[90%]"
                          title={mg.person_name}
                        >
                          {mg.person_name}
                        </Link>
                      </div>

                      <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center gap-3">
                        <a
                          href={`https://vipergirls.to/search.php?do=process&query=${encodeURIComponent(`"${mg.alias || mg.person_name || ''}" "${mg.gallery_name || ''}"`)}&titleonly=1&forumchoice%5B%5D=235&childforums=1`}
                          target="_blank"
                          rel="noopener noreferrer"
                          title="Search vipergirls.to"
                          className="p-2.5 bg-white/10 hover:bg-blue-500/80 rounded-full transition-colors"
                        >
                          <ExternalLink size={18} className="text-white" />
                        </a>
                        <button
                          onClick={() => excludeMissingMut.mutate({ person_id: mg.person_id, provider: mg.provider, source_url: mg.gallery_url, title: mg.gallery_name })}
                          disabled={excludeMissingMut.isPending}
                          title="Mark as Not Wanted"
                          className="p-2.5 bg-white/10 hover:bg-red-500/80 rounded-full transition-colors"
                        >
                          <EyeOff size={18} className="text-white" />
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
                {missingTotalPages > 1 && (
                  <div className="flex items-center justify-between mt-3 pt-3 border-t border-zinc-800">
                    <span className="text-xs text-zinc-500">Page {missingData?.meta?.current_page ?? 1} of {missingTotalPages}</span>
                    <div className="flex gap-2">
                      <Button variant="ghost" size="sm" disabled={missingPage <= 1} onClick={() => setMissingPage((p) => Math.max(1, p - 1))}>
                        Previous
                      </Button>
                      <Button variant="ghost" size="sm" disabled={(missingData?.meta?.current_page ?? 1) >= missingTotalPages} onClick={() => setMissingPage((p) => p + 1)}>
                        Next
                      </Button>
                    </div>
                  </div>
                )}
              </>
            )}
          </SectionCard>

          <SectionCard title="Not Wanted" count={notWanted.length}>
            {notWanted.length === 0 ? (
              <p className="text-sm text-zinc-500">No galleries marked as not wanted.</p>
            ) : (
              <div className="space-y-1 max-h-96 overflow-y-auto">
                {notWanted.map((nw: NotWantedGallery) => (
                  <div key={nw.id} className="flex items-center justify-between py-1.5 gap-2">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm text-zinc-200 truncate">{nw.title || nw.source_url}</p>
                      <div className="flex items-center gap-2 text-xs text-zinc-500">
                        <Badge variant="info">{nw.provider}</Badge>
                        <Link to={`/people/${nw.person_id}`} className="hover:text-blue-400">{nw.person_name}</Link>
                        {nw.reason && <span className="text-zinc-600 truncate">{nw.reason}</span>}
                      </div>
                    </div>
                    <Button size="sm" variant="ghost" onClick={() => unmarkMut.mutate(nw.id)} disabled={unmarkMut.isPending}>
                      Unmark
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </SectionCard>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <SectionCard
              title="Failed Downloads — Images"
              count={a.missing_images}
              action={
                <Button size="sm" variant="ghost" onClick={() => retryAllImagesMut.mutate()} disabled={retryAllImagesMut.isPending}>
                  Retry All
                </Button>
              }
            >
              <div className="space-y-1 max-h-72 overflow-y-auto">
                {(failedImagesData?.data ?? []).length === 0 ? (
                  <p className="text-xs text-zinc-500">No missing images.</p>
                ) : (
                  (failedImagesData?.data ?? []).map((img: GalleryImage) => (
                    <div key={img.id} className="flex items-center justify-between py-1.5 gap-2">
                      <span className="text-sm text-zinc-300 truncate">{img.filename || img.original_url || `#${img.id}`}</span>
                      <Button size="sm" variant="ghost" onClick={() => retryImageMut.mutate(img.id)} disabled={retryImageMut.isPending}>
                        Retry
                      </Button>
                    </div>
                  ))
                )}
              </div>
            </SectionCard>

            <SectionCard
              title="Failed Downloads — Sources"
              count={a.failed_sources}
              action={
                <Button size="sm" variant="ghost" onClick={() => retryAllSourcesMut.mutate()} disabled={retryAllSourcesMut.isPending}>
                  Retry All
                </Button>
              }
            >
              <div className="space-y-1 max-h-72 overflow-y-auto">
                {(failedSourcesData?.data ?? []).length === 0 ? (
                  <p className="text-xs text-zinc-500">No errored sources.</p>
                ) : (
                  (failedSourcesData?.data ?? []).map((s: Source) => (
                    <div key={s.id} className="flex items-center justify-between py-1.5 gap-2">
                      <span className="text-sm text-zinc-300 truncate">{s.name || s.location}</span>
                      <div className="flex items-center gap-2 shrink-0">
                        <Badge variant="danger">{s.status}</Badge>
                        <Button size="sm" variant="ghost" onClick={() => retrySourceMut.mutate(s.id)} disabled={retrySourceMut.isPending}>
                          Retry
                        </Button>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </SectionCard>
          </div>

          <SectionCard
            title="Missing Videos"
            count={a.missing_videos}
            action={
              <Link to="/videos" className="inline-flex items-center gap-1.5 text-xs text-blue-400 hover:text-blue-300">
                Browse videos <ChevronRight size={12} />
              </Link>
            }
          >
            {a.missing_videos > 0 ? (
              <p className="text-sm text-zinc-400">
                {a.missing_videos} video file{a.missing_videos === 1 ? '' : 's'} not found on disk. These are
                re-downloaded automatically during video verification (typically at startup or when a source
                is re-crawled), and their titles are preserved.
              </p>
            ) : (
              <p className="text-sm text-zinc-500">All video files are present on disk.</p>
            )}
          </SectionCard>

          <SectionCard
            title="Embedding Status"
            action={
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
            }
          >
            {!embedStatus ? (
              <Spinner size="sm" />
            ) : (
              <div className="space-y-3">
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
              </div>
            )}
          </SectionCard>
        </div>
      )}

      {tab === 'downloads' && (
        <div className="space-y-6">
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            <section className="lg:col-span-2 rounded-lg border border-zinc-800 bg-zinc-900/20">
              <div className="p-4 border-b border-zinc-800">
                <h2 className="text-sm font-semibold text-zinc-400 uppercase tracking-wider">
                  Active Crawls {activeCrawls.length > 0 && `(${activeCrawls.length})`}
                </h2>
              </div>
              <div className="p-4">
                {activeCrawls.length === 0 ? (
                  <p className="text-sm text-zinc-500">No active crawls</p>
                ) : (
                  <div className="space-y-3">
                    {activeCrawls.map((src) => (
                      <div key={src.id}>
                        <div className="flex items-center justify-between mb-1">
                          <span className="text-sm text-zinc-300 truncate">{src.name || src.location}</span>
                          <span className="text-xs text-zinc-500 shrink-0 ml-2">{src.download_progress}%</span>
                        </div>
                        <div className="w-full h-1.5 rounded-full bg-zinc-800 overflow-hidden">
                          <div className="h-full bg-blue-500 rounded-full transition-all" style={{ width: `${src.download_progress}%` }} />
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </section>

            <section className="rounded-lg border border-zinc-800 bg-zinc-900/20">
              <div className="p-4 border-b border-zinc-800">
                <h2 className="text-sm font-semibold text-zinc-400 uppercase tracking-wider">Pipeline</h2>
              </div>
              <div className="p-4 space-y-4">
                <div className="flex items-center justify-between">
                  <span className="flex items-center gap-2 text-sm text-zinc-300">
                    <Loader2 size={13} className={cn(verificationActive ? 'animate-spin text-blue-400' : 'text-zinc-600')} />
                    Verification
                  </span>
                  <Badge variant={verificationActive ? 'warning' : 'default'}>
                    {verificationActive ? 'Running' : 'Idle'}
                  </Badge>
                </div>
                <div className="grid grid-cols-3 gap-2 text-center">
                  <div className="rounded border border-zinc-800 bg-zinc-900/40 p-2">
                    <div className="text-sm font-mono text-zinc-200">{verification?.processed ?? 0}</div>
                    <div className="text-[10px] uppercase tracking-wider text-zinc-500">Verified</div>
                  </div>
                  <div className="rounded border border-zinc-800 bg-zinc-900/40 p-2">
                    <div className="text-sm font-mono text-amber-400">{verification?.missing_found ?? 0}</div>
                    <div className="text-[10px] uppercase tracking-wider text-zinc-500">Missing</div>
                  </div>
                  <div className="rounded border border-zinc-800 bg-zinc-900/40 p-2">
                    <div className="text-sm font-mono text-emerald-400">{verification?.recovered ?? 0}</div>
                    <div className="text-[10px] uppercase tracking-wider text-zinc-500">Recovered</div>
                  </div>
                </div>

                <div className="flex items-center justify-between">
                  <span className="flex items-center gap-2 text-sm text-zinc-300">
                    <Loader2 size={13} className={cn(videosActive ? 'animate-spin text-blue-400' : 'text-zinc-600')} />
                    Video Processing
                  </span>
                  <Badge variant={videosActive ? 'warning' : 'default'}>
                    {videosActive ? 'Running' : 'Idle'}
                  </Badge>
                </div>
                <div className="grid grid-cols-3 gap-2 text-center">
                  <div className="rounded border border-zinc-800 bg-zinc-900/40 p-2">
                    <div className="text-sm font-mono text-zinc-200">{videos?.processed ?? 0}</div>
                    <div className="text-[10px] uppercase tracking-wider text-zinc-500">Processed</div>
                  </div>
                  <div className="rounded border border-zinc-800 bg-zinc-900/40 p-2">
                    <div className="text-sm font-mono text-amber-400">{videos?.missing_found ?? 0}</div>
                    <div className="text-[10px] uppercase tracking-wider text-zinc-500">Missing</div>
                  </div>
                  <div className="rounded border border-zinc-800 bg-zinc-900/40 p-2">
                    <div className="text-sm font-mono text-emerald-400">{videos?.recovered ?? 0}</div>
                    <div className="text-[10px] uppercase tracking-wider text-zinc-500">Recovered</div>
                  </div>
                </div>
              </div>
            </section>
          </div>
        </div>
      )}

      {tab === 'admin' && (
        <div className="space-y-6">
          <SectionCard
            title={`Sources (${sourceItems.length})`}
            action={
              <Link to="/sources" className="inline-flex items-center gap-1.5 text-xs text-blue-400 hover:text-blue-300">
                Manage all sources <ChevronRight size={12} />
              </Link>
            }
          >
            {sourceItems.length === 0 ? (
              <p className="text-sm text-zinc-500">No sources configured.</p>
            ) : (
              <div className="space-y-2">
                {sourceItems.map((src) => (
                  <div key={src.id} className="flex items-center justify-between py-2">
                    <div className="min-w-0 flex-1">
                      <span className="text-sm text-zinc-300 truncate block">{src.name}</span>
                      <span className="text-xs text-zinc-500 truncate block">{src.location}</span>
                    </div>
                    <div className="flex items-center gap-2 shrink-0">
                      <Badge variant={src.status === 'crawling' ? 'warning' : 'default'}>{src.status}</Badge>
                      <Button variant="ghost" size="sm" onClick={() => crawlMut.mutate(src.id)} disabled={crawlMut.isPending} title="Crawl now">
                        <Play size={12} />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </SectionCard>

          <SectionCard title="Maintenance">
            <div className="space-y-3">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                <Button size="sm" variant="danger" onClick={handleCleanupDupes} disabled={cleanupMut.isPending} className="justify-start">
                  <Wrench size={14} /> {cleanupMut.isPending ? 'Running...' : 'Clean Up Duplicates'}
                </Button>
                <Button size="sm" variant="danger" onClick={handlePurgePlaceholders} disabled={purgeMut.isPending} className="justify-start">
                  <Trash2 size={14} /> {purgeMut.isPending ? 'Scanning...' : 'Purge Placeholder Images'}
                </Button>
                <Button size="sm" variant="secondary" onClick={handleResetProfile} disabled={resetProfileMut.isPending} className="justify-start">
                  <RefreshCw size={14} /> Reset Taste Profile
                </Button>
                <a
                  href="/api/export/db"
                  className="inline-flex items-center gap-1.5 rounded-md font-medium transition-colors px-3 py-2 text-xs bg-blue-600 text-white hover:bg-blue-500 disabled:opacity-40"
                  download
                >
                  <Download size={14} /> Export DB
                </a>
                <label className={cn(
                  "inline-flex items-center justify-center gap-1.5 rounded-md font-medium transition-colors px-3 py-2 text-xs bg-emerald-600 text-white hover:bg-emerald-500 cursor-pointer select-none",
                  importing && "opacity-50 pointer-events-none",
                )}>
                  {importing ? <Loader2 size={14} className="animate-spin" /> : <Upload size={14} />}
                  {importing ? 'Importing DB...' : 'Import DB'}
                  <input
                    type="file"
                    accept=".sql"
                    className="hidden"
                    onChange={handleImportFile}
                    disabled={importing}
                  />
                </label>
              </div>
              {(importStatus || purgeStatus) && (
                <div className={cn(
                  'p-2.5 rounded text-xs border',
                  (importStatus?.startsWith('Error') || purgeStatus?.startsWith('Error')) || cleanupStatus?.startsWith('Error')
                    ? 'bg-red-950/50 text-red-400 border-red-800/80'
                    : 'bg-emerald-950/50 text-emerald-400 border-emerald-800/80',
                )}>
                  {purgeStatus ?? importStatus}
                </div>
              )}
              {cleanupStatus && !purgeStatus && (
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
          </SectionCard>

          <SectionCard title="Database">
            <div className="flex items-start gap-3 text-sm text-zinc-500">
              <Database size={16} className="text-zinc-600 shrink-0 mt-0.5" />
              <p>
                Use <span className="text-zinc-300">Export DB</span> to create a SQL backup and{' '}
                <span className="text-zinc-300">Import DB</span> to restore from one. Maintenance actions
                (dedupe, placeholder purge) are protected by the maintenance token.
              </p>
            </div>
          </SectionCard>
        </div>
      )}
    </div>
  );
}