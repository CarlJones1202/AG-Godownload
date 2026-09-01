import { useEffect, useMemo, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { Images, Globe, Users, Search, Loader2 } from 'lucide-react';
import { galleries, people, sources } from '@/lib/api';
import { cn } from '@/lib/utils';

interface CommandPaletteProps {
  onClose: () => void;
}

type PaletteItem = {
  key: string;
  kind: 'Person' | 'Gallery' | 'Source';
  title: string;
  subtitle: string;
  to: string;
};

export function CommandPalette({ onClose }: CommandPaletteProps) {
  const navigate = useNavigate();
  const [query, setQuery] = useState('');
  const [debounced, setDebounced] = useState('');
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const t = setTimeout(() => {
      setDebounced(query.trim());
      setActive(0);
    }, 200);
    return () => clearTimeout(t);
  }, [query]);

  const q = debounced.length >= 2 ? debounced : '';

  const peopleQuery = useQuery({
    queryKey: ['palette-people', q],
    queryFn: () => people.list({ q, limit: 6 }),
    enabled: q !== '',
  });
  const galleriesQuery = useQuery({
    queryKey: ['palette-galleries', q],
    queryFn: () => galleries.list({ q, limit: 6 }),
    enabled: q !== '',
  });
  const sourcesQuery = useQuery({
    queryKey: ['palette-sources', q],
    queryFn: () => sources.list({ q, limit: 6 }),
    enabled: q !== '',
  });

  const items = useMemo<PaletteItem[]>(() => {
    const out: PaletteItem[] = [];
    for (const p of peopleQuery.data?.data ?? []) {
      out.push({
        key: `p-${p.id}`,
        kind: 'Person',
        title: p.name,
        subtitle: p.aliases || 'Person',
        to: `/people/${p.id}`,
      });
    }
    for (const g of galleriesQuery.data?.data ?? []) {
      out.push({
        key: `g-${g.id}`,
        kind: 'Gallery',
        title: g.name,
        subtitle: [g.provider, g.release_date].filter(Boolean).join(' · ') || 'Gallery',
        to: `/galleries/${g.id}`,
      });
    }
    for (const s of sourcesQuery.data?.data ?? []) {
      out.push({
        key: `s-${s.id}`,
        kind: 'Source',
        title: s.name,
        subtitle: s.location,
        to: '/sources',
      });
    }
    return out;
  }, [peopleQuery.data, galleriesQuery.data, sourcesQuery.data]);

  const busy = q !== '' && (peopleQuery.isFetching || galleriesQuery.isFetching || sourcesQuery.isFetching);

  const go = (item?: PaletteItem) => {
    if (!item) return;
    onClose();
    navigate(item.to);
  };

  const goRef = useRef(go);
  useEffect(() => {
    goRef.current = go;
  });

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      } else if (e.key === 'Enter' && items.length > 0) {
        e.preventDefault();
        goRef.current(items[Math.min(active, items.length - 1)]);
      } else if (e.key === 'ArrowDown') {
        e.preventDefault();
        setActive((a) => (items.length === 0 ? 0 : Math.min(items.length - 1, a + 1)));
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setActive((a) => Math.max(0, a - 1));
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [items, active, onClose]);

  useEffect(() => {
    listRef.current?.querySelector('[data-active="true"]')?.scrollIntoView({ block: 'nearest' });
  }, [active]);

  const iconFor = (kind: PaletteItem['kind']) => {
    if (kind === 'Person') return <Users size={16} />;
    if (kind === 'Source') return <Globe size={16} />;
    return <Images size={16} />;
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/70 p-4 pt-[12vh]"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="w-full max-w-xl rounded-lg border border-zinc-700 bg-zinc-900 shadow-xl overflow-hidden">
        <div className="flex items-center gap-2.5 px-4 border-b border-zinc-800">
          <Search size={16} className="text-zinc-500 shrink-0" />
          <input
            ref={inputRef}
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search people, galleries, sources..."
            className="flex-1 bg-transparent py-3 text-sm text-zinc-100 placeholder-zinc-500 focus:outline-none"
          />
          {busy && <Loader2 size={14} className="animate-spin text-zinc-500 shrink-0" />}
        </div>

        <div ref={listRef} className="max-h-80 overflow-y-auto py-1.5">
          {q === '' ? (
            <p className="px-4 py-6 text-center text-xs text-zinc-500">
              Type at least 2 characters to search
            </p>
          ) : items.length === 0 ? (
            <p className="px-4 py-6 text-center text-xs text-zinc-500">
              {busy ? 'Searching...' : 'No matches found'}
            </p>
          ) : (
            <div className="space-y-0.5">
              {items.map((item, i) => (
                <button
                  key={item.key}
                  data-active={i === active}
                  onMouseMove={() => setActive(i)}
                  onClick={() => go(item)}
                  className={cn(
                    'w-full flex items-center gap-3 px-4 py-2 text-left cursor-pointer',
                    i === active ? 'bg-zinc-800' : 'hover:bg-zinc-800/60',
                  )}
                >
                  <span className="text-zinc-500 shrink-0">{iconFor(item.kind)}</span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm text-zinc-100 truncate">{item.title}</span>
                    <span className="block text-xs text-zinc-500 truncate">{item.subtitle}</span>
                  </span>
                  <span
                    className={cn(
                      'text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium shrink-0',
                      item.kind === 'Person' && 'bg-emerald-900/40 text-emerald-400',
                      item.kind === 'Gallery' && 'bg-blue-900/40 text-blue-400',
                      item.kind === 'Source' && 'bg-amber-900/40 text-amber-400',
                    )}
                  >
                    {item.kind}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="flex items-center gap-3 px-4 py-2 border-t border-zinc-800 text-[10px] text-zinc-600">
          <span><kbd className="px-1 py-0.5 rounded bg-zinc-800 text-zinc-400">↑</kbd> <kbd className="px-1 py-0.5 rounded bg-zinc-800 text-zinc-400">↓</kbd> navigate</span>
          <span><kbd className="px-1 py-0.5 rounded bg-zinc-800 text-zinc-400">↵</kbd> open</span>
          <span><kbd className="px-1 py-0.5 rounded bg-zinc-800 text-zinc-400">esc</kbd> close</span>
        </div>
      </div>
    </div>
  );
}