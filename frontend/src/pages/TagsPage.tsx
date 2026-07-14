import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { tagsApi } from '@/lib/api';
import {
  PageHeader,
  Button,
  Spinner,
  EmptyState,
  Input,
} from '@/components/UI';
import { Plus, Tag, TrendingUp, Search } from 'lucide-react';

export function TagsPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const [newTagName, setNewTagName] = useState('');

  const { data: tags, isLoading } = useQuery({
    queryKey: ['tags', search ? 'search' : 'all'],
    queryFn: () => search ? tagsApi.search(search) : tagsApi.list(),
  });

  const { data: topTags } = useQuery({
    queryKey: ['tags', 'top'],
    queryFn: () => tagsApi.top(20),
  });

  const createMut = useMutation({
    mutationFn: (name: string) => tagsApi.create(name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tags'] });
      setNewTagName('');
    },
  });

  const displayTags = tags ?? [];

  return (
    <>
      <PageHeader title="Tags" description="Manage image tags">
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-500" />
            <Input
              placeholder="Search tags..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-44 h-9 pl-8"
            />
          </div>
          <div className="flex gap-2">
            <Input
              placeholder="New tag name"
              value={newTagName}
              onChange={(e) => setNewTagName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && newTagName.trim() && createMut.mutate(newTagName.trim())}
              className="w-36 h-9"
            />
            <Button
              size="sm"
              onClick={() => newTagName.trim() && createMut.mutate(newTagName.trim())}
              disabled={!newTagName.trim() || createMut.isPending}
            >
              <Plus size={14} /> Add
            </Button>
          </div>
        </div>
      </PageHeader>

      {topTags && topTags.length > 0 && !search && (
        <div className="mb-6">
          <div className="flex items-center gap-2 mb-3">
            <TrendingUp size={14} className="text-zinc-500" />
            <h3 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider">Top Tags</h3>
          </div>
          <div className="flex flex-wrap gap-2">
            {topTags.map((t) => (
              <button
                key={t.id}
                onClick={() => setSearch(t.name)}
                className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-zinc-800 hover:bg-zinc-700 border border-zinc-700 transition-all text-xs"
              >
                <Tag size={11} className="text-zinc-400" />
                <span className="text-zinc-200">{t.name}</span>
                <span className="text-[10px] text-zinc-500 ml-0.5">({t.count})</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {isLoading ? (
        <Spinner />
      ) : displayTags.length === 0 ? (
        <EmptyState message={search ? 'No tags match your search.' : 'No tags yet. Create one above.'} />
      ) : (
        <div className="flex flex-wrap gap-2">
          {displayTags.map((t) => (
            <div key={t.id} className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-zinc-800 bg-zinc-900/50">
              <Tag size={13} className="text-zinc-400" />
              <span className="text-sm text-zinc-200">{t.name}</span>
              {t.count > 0 && <span className="text-[10px] text-zinc-500 ml-0.5">({t.count})</span>}
            </div>
          ))}
        </div>
      )}
    </>
  );
}
