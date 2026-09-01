import { useEffect, useState } from 'react';
import { Link, Outlet, useLocation } from 'react-router-dom';
import {
  LayoutDashboard,
  AlertTriangle,
  Globe,
  Images,
  Image,
  Film,
  Users,
  Tag,
  Shuffle,
  Heart,
  Search,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useQuery } from '@tanstack/react-query';
import { stats } from '@/lib/api';
import { CommandPalette } from '@/components/CommandPalette';

interface NavItem {
  to: string;
  label: string;
  icon: typeof LayoutDashboard;
  badge?: number;
  match: (pathname: string, tab: string | null) => boolean;
}

interface NavGroup {
  label: string;
  items: NavItem[];
}

export function Layout() {
  const { status, connected } = useWebSocket();
  const location = useLocation();
  const [paletteOpen, setPaletteOpen] = useState(false);

  const { data: d } = useQuery({
    queryKey: ['dashboard-stats'],
    queryFn: () => stats.dashboard(),
    refetchInterval: 30000,
  });

  const a = d?.attention;
  const attentionTotal = a
    ? a.missing_galleries + a.missing_images + a.missing_videos + a.failed_sources + a.embed_failed
    : 0;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setPaletteOpen(true);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  const activeCrawls = status?.crawler?.active_sources?.length ?? 0;
  const verificationActive = status?.verification?.is_running ?? false;
  const videosActive = status?.videos?.is_running ?? false;
  const totalActive = activeCrawls + (verificationActive ? 1 : 0) + (videosActive ? 1 : 0);

  const tab = new URLSearchParams(location.search).get('tab');

  const groups: NavGroup[] = [
    {
      label: 'Home',
      items: [
        {
          to: '/dashboard',
          label: 'Dashboard',
          icon: LayoutDashboard,
          match: (p, t) => p === '/dashboard' && (t === null || t === 'overview'),
        },
        {
          to: '/dashboard?tab=attention',
          label: 'Attention',
          icon: AlertTriangle,
          badge: attentionTotal,
          match: (p, t) => p === '/dashboard' && t === 'attention',
        },
      ],
    },
    {
      label: 'Browse',
      items: [
        { to: '/galleries', label: 'Galleries', icon: Images, match: (p) => p.startsWith('/galleries') },
        { to: '/images', label: 'Images', icon: Image, match: (p) => p.startsWith('/images') },
        { to: '/videos', label: 'Videos', icon: Film, match: (p) => p.startsWith('/videos') },
        { to: '/tags', label: 'Tags', icon: Tag, match: (p) => p.startsWith('/tags') },
        { to: '/similar', label: 'Similar', icon: Shuffle, match: (p) => p.startsWith('/similar') },
      ],
    },
    {
      label: 'People',
      items: [
        { to: '/people', label: 'People', icon: Users, match: (p) => p.startsWith('/people') },
      ],
    },
    {
      label: 'Tools',
      items: [
        { to: '/sources', label: 'Sources', icon: Globe, match: (p) => p.startsWith('/sources') },
        { to: '/favorites', label: 'Favorites', icon: Heart, match: (p) => p.startsWith('/favorites') },
      ],
    },
  ];

  const renderItem = (item: NavItem, compact: boolean) => {
    const active = item.match(location.pathname, tab);
    return (
      <Link
        key={item.to}
        to={item.to}
        className={cn(
          'flex items-center gap-1.5 rounded-md font-medium transition-colors relative',
          compact ? 'px-2.5 py-1.5 text-xs whitespace-nowrap' : 'px-3 py-1.5 text-sm',
          active
            ? 'text-white bg-zinc-800'
            : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50',
        )}
      >
        <item.icon size={compact ? 13 : 15} />
        {item.label}
        {typeof item.badge === 'number' && item.badge > 0 && (
          <span className="inline-flex items-center justify-center min-w-[1.125rem] h-[1.125rem] px-1 rounded-full text-[10px] font-semibold bg-blue-600 text-white">
            {item.badge > 99 ? '99+' : item.badge}
          </span>
        )}
      </Link>
    );
  };

  return (
    <div className="min-h-screen bg-zinc-950">
      <header className="sticky top-0 z-40 border-b border-zinc-800 bg-zinc-950/80 backdrop-blur-lg">
        <div className="flex items-center justify-between h-14 px-4 sm:px-6 lg:px-8">
          <div className="flex items-center gap-4 min-w-0">
            <Link to="/dashboard" className="text-base font-semibold text-white tracking-tight shrink-0">
              GoDownload
            </Link>

            <nav className="hidden lg:flex items-center gap-1 flex-wrap">
              {groups.map((g, gi) => (
                <div key={g.label} className="flex items-center gap-1">
                  {gi > 0 && <div className="w-px h-5 bg-zinc-800 mx-2" />}
                  <span className="px-2 text-[10px] font-semibold uppercase tracking-wider text-zinc-600 select-none">
                    {g.label}
                  </span>
                  {g.items.map((item) => renderItem(item, false))}
                </div>
              ))}
            </nav>
          </div>

          <div className="flex items-center gap-3 shrink-0">
            <button
              onClick={() => setPaletteOpen(true)}
              className="flex items-center gap-2 rounded-md border border-zinc-800 bg-zinc-900/60 px-2.5 py-1.5 text-xs text-zinc-500 hover:text-zinc-300 hover:border-zinc-700 transition-colors hidden sm:flex"
              title="Search (Ctrl+K)"
            >
              <Search size={13} />
              <span className="hidden md:inline">Search…</span>
              <kbd className="hidden lg:inline px-1 py-0.5 rounded bg-zinc-800 text-zinc-500 text-[10px]">Ctrl K</kbd>
            </button>

            {status && (
              <div className="hidden sm:flex items-center gap-3 text-xs text-zinc-500">
                <span className={cn(
                  'flex items-center gap-1',
                  totalActive > 0 ? 'text-blue-400' : 'text-zinc-500',
                )}>
                  <span className={cn(
                    'w-1.5 h-1.5 rounded-full',
                    totalActive > 0 ? 'bg-blue-400 animate-pulse' : 'bg-zinc-600',
                  )} />
                  {totalActive > 0 ? `${totalActive} active` : 'Idle'}
                </span>
              </div>
            )}
            <span className={cn(
              'flex items-center gap-1.5 text-xs',
              connected ? 'text-zinc-500' : 'text-zinc-600',
            )}>
              <span className={cn(
                'w-1.5 h-1.5 rounded-full',
                connected ? 'bg-emerald-500' : 'bg-zinc-600',
              )} />
              <span className="hidden sm:inline">{connected ? 'Connected' : 'Offline'}</span>
            </span>
          </div>
        </div>

        {/* Mobile / compact nav */}
        <nav className="flex lg:hidden items-center gap-1 px-4 pb-2 overflow-x-auto scrollbar-none">
          {groups.map((g) => g.items.map((item) => renderItem(item, true)))}
        </nav>
      </header>

      <main className="mx-auto max-w-[1440px] px-4 sm:px-6 lg:px-8 py-6 lg:py-8">
        <Outlet />
      </main>

      {paletteOpen && <CommandPalette onClose={() => setPaletteOpen(false)} />}
    </div>
  );
}