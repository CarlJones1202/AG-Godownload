import { NavLink, Outlet } from 'react-router-dom';
import {
  LayoutDashboard,
  Globe,
  Images,
  Image,
  Film,
  Users,
  Tag,
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
  { to: '/tags', label: 'Tags', icon: Tag },
];

export function Layout() {
  const { status, connected } = useWebSocket();

  const activeCrawls = status?.crawler?.active_sources?.length ?? 0;
  const verificationActive = status?.verification?.is_running ?? false;
  const videosActive = status?.videos?.is_running ?? false;
  const totalActive = activeCrawls + (verificationActive ? 1 : 0) + (videosActive ? 1 : 0);

  return (
    <div className="min-h-screen bg-zinc-950">
      <header className="sticky top-0 z-40 border-b border-zinc-800 bg-zinc-950/80 backdrop-blur-lg">
        <div className="flex items-center justify-between h-14 px-4 sm:px-6 lg:px-8">
          <div className="flex items-center gap-8">
            <NavLink to="/dashboard" className="text-base font-semibold text-white tracking-tight shrink-0">
              GoDownload
            </NavLink>
            <nav className="hidden md:flex items-center gap-1">
              {navItems.map(({ to, label, icon: Icon }) => (
                <NavLink
                  key={to}
                  to={to}
                  className={({ isActive }) =>
                    cn(
                      'flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded-md transition-colors',
                      isActive
                        ? 'text-white bg-zinc-800'
                        : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50',
                    )
                  }
                >
                  <Icon size={15} />
                  {label}
                </NavLink>
              ))}
            </nav>
          </div>

          <div className="flex items-center gap-3">
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

        {/* Mobile nav */}
        <nav className="md:hidden flex items-center gap-1 px-4 pb-2 overflow-x-auto scrollbar-none">
          {navItems.map(({ to, label, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-1.5 px-2.5 py-1.5 text-xs font-medium rounded-md transition-colors whitespace-nowrap',
                  isActive
                    ? 'text-white bg-zinc-800'
                    : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50',
                )
              }
            >
              <Icon size={13} />
              {label}
            </NavLink>
          ))}
        </nav>
      </header>

      <main className="mx-auto max-w-[1440px] px-4 sm:px-6 lg:px-8 py-6 lg:py-8">
        <Outlet />
      </main>
    </div>
  );
}
