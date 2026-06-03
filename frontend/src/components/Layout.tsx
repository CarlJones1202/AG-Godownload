import { NavLink, Outlet } from 'react-router-dom';
import {
  LayoutDashboard,
  Globe,
  Images,
  Image,
  Film,
  Users,
  Settings,
  Loader2,
  CheckCircle2,
  AlertCircle,
  Clock,
  Wifi,
  WifiOff,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { useWebSocket } from '@/hooks/useWebSocket';

const navItems = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/sources', label: 'Sources', icon: Globe },
  { to: '/galleries', label: 'Galleries', icon: Images },
  { to: '/images', label: 'Images', icon: Image },
  { to: '/videos', label: 'Videos', icon: Film },
  { to: '/people', label: 'People', icon: Users },
  { to: '/admin', label: 'Admin', icon: Settings },
];

export function Layout() {
  const { status, connected } = useWebSocket();

  const activeCrawls = status?.crawler?.active_sources?.length ?? 0;
  const verificationActive = status?.verification?.is_running ?? false;
  const videosActive = status?.videos?.is_running ?? false;
  const totalActive = activeCrawls + (verificationActive ? 1 : 0) + (videosActive ? 1 : 0);

  return (
    <div className="flex h-screen">
      <aside className="w-56 shrink-0 border-r border-zinc-800 bg-zinc-900 flex flex-col">
        <div className="p-4 border-b border-zinc-800">
          <h1 className="text-lg font-semibold text-white tracking-tight">GoDownload</h1>
        </div>
        <nav className="flex-1 p-2 space-y-0.5">
          {navItems.map(({ to, label, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-2.5 px-3 py-2 rounded-md text-sm transition-colors',
                  isActive
                    ? 'bg-zinc-800 text-white'
                    : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50',
                )
              }
            >
              <Icon size={16} />
              {label}
            </NavLink>
          ))}
        </nav>

        <div className="border-t border-zinc-800 p-3 space-y-2">
          <div className="flex items-center gap-1.5 text-xs">
            {connected ? (
              <Wifi size={12} className="text-emerald-500" />
            ) : (
              <WifiOff size={12} className="text-zinc-500" />
            )}
            <span className={connected ? 'text-zinc-400' : 'text-zinc-600'}>
              {connected ? 'Live' : 'Disconnected'}
            </span>
          </div>

          {status && (
            <div className="grid grid-cols-2 gap-x-3 gap-y-1 text-xs">
              <div className="flex items-center gap-1 text-zinc-400">
                <Loader2 size={11} className={cn(
                  totalActive > 0 ? 'text-blue-400 animate-spin' : 'text-zinc-500',
                )} />
                <span>{totalActive} active</span>
              </div>
              <div className="flex items-center gap-1 text-zinc-400">
                <Clock size={11} className="text-zinc-500" />
                <span>{activeCrawls} crawls</span>
              </div>
              <div className="flex items-center gap-1 text-zinc-400">
                <CheckCircle2 size={11} className="text-emerald-500" />
                <span>{status?.verification?.processed ?? 0} verified</span>
              </div>
              <div className="flex items-center gap-1 text-zinc-400">
                <AlertCircle size={11} className={cn(
                  (status?.crawler?.active_sources?.length ?? 0) > 0 ? 'text-red-400' : 'text-zinc-500',
                )} />
                <span>{status?.verification?.missing_found ?? 0} missing</span>
              </div>
            </div>
          )}
        </div>
      </aside>

      <main className="flex-1 overflow-y-auto">
        <div className="max-w-7xl mx-auto p-6">
          <Outlet />
        </div>
      </main>
    </div>
  );
}
