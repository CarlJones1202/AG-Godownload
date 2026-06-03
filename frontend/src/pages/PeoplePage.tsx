import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { people } from '@/lib/api';
import {
  PageHeader,
  Button,
  Card,
  Spinner,
  EmptyState,
  Input,
  Pagination,
  ConfirmDialog,
} from '@/components/UI';
import { Plus, Search, Trash2, User } from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';

function parsePhotos(photos?: string): string[] {
  if (!photos) return [];
  try {
    const parsed = JSON.parse(photos);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function ProfileTile({ name, photo }: { name: string; photo?: string }) {
  return photo ? (
    <img src={photo} alt={name} className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.03]" />
  ) : (
    <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-zinc-800 to-zinc-950">
      <User size={44} className="text-zinc-600" />
    </div>
  );
}

export function PeoplePage() {
  const [editMode, setEditMode] = useState(false);
  const queryClient = useQueryClient();
  const { offset, limit, prevPage, nextPage, resetPage } = usePagination({ limit: 24 });
  const [search, setSearch] = useState('');
  const [activeSearch, setActiveSearch] = useState('');
  const [showCreate, setShowCreate] = useState(false);
  const [newPerson, setNewPerson] = useState({ name: '', aliases: '', nationality: '' });
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [confirmDelete, setConfirmDelete] = useState(false);

  const { data: peopleData, isLoading } = useQuery({
    queryKey: ['people', { offset, limit, q: activeSearch || undefined }],
    queryFn: () => people.list({ limit, offset, q: activeSearch || undefined }),
  });
  const personList = peopleData?.data ?? [];
  const totalPages = peopleData?.meta.total_pages ?? 1;
  const currentPage = peopleData?.meta.current_page ?? 1;

  const createMut = useMutation({
    mutationFn: () =>
      people.create({
        name: newPerson.name,
        aliases: newPerson.aliases || undefined,
        nationality: newPerson.nationality || undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['people'] });
      setShowCreate(false);
      setNewPerson({ name: '', aliases: '', nationality: '' });
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => people.delete(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['people'] }),
  });

  const handleSearch = () => {
    setActiveSearch(search);
    resetPage();
  };

  const toggleSelect = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (!personList) return;
    if (selected.size === personList.length) {
      setSelected(new Set());
    } else {
      setSelected(new Set(personList.map((p) => p.id)));
    }
  };

  return (
    <>
      <PageHeader title="People" description="Profiles and performer metadata">
        <Button onClick={() => setShowCreate(!showCreate)}>
          <Plus size={14} /> Add Person
        </Button>
        <Button variant={editMode ? "primary" : "secondary"} onClick={() => setEditMode(!editMode)}>
          {editMode ? "Done" : "Edit"}
        </Button>
      </PageHeader>

      <div className="mb-6 rounded-[2rem] border border-white/8 bg-white/5 p-4">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex flex-1 gap-2">
            <Input
              placeholder="Search profiles, aliases, or details..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
            />
            <Button size="sm" onClick={handleSearch}><Search size={14} /> Search</Button>
            {activeSearch && <Button variant="secondary" size="sm" onClick={() => { setSearch(''); setActiveSearch(''); resetPage(); }}>Clear</Button>}
          </div>

          {selected.size > 0 && (
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm text-zinc-400">{selected.size} selected</span>
              <Button variant="danger" size="sm" onClick={() => setConfirmDelete(true)}>
                <Trash2 size={14} /> Delete
              </Button>
            </div>
          )}
        </div>
      </div>

      {showCreate && (
        <Card className="mb-6 rounded-[1.75rem] border-white/8 bg-white/5">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
            <Input label="Name" placeholder="Person name" value={newPerson.name} onChange={(e) => setNewPerson({ ...newPerson, name: e.target.value })} />
            <Input label="Aliases" placeholder="Comma-separated aliases" value={newPerson.aliases} onChange={(e) => setNewPerson({ ...newPerson, aliases: e.target.value })} />
            <Input label="Nationality" placeholder="e.g. American" value={newPerson.nationality} onChange={(e) => setNewPerson({ ...newPerson, nationality: e.target.value })} />
          </div>
          <div className="flex justify-end gap-2 mt-3">
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(false)}>Cancel</Button>
            <Button size="sm" onClick={() => createMut.mutate()} disabled={!newPerson.name || createMut.isPending}>Create</Button>
          </div>
        </Card>
      )}

      {isLoading ? (
        <Spinner />
      ) : personList.length === 0 ? (
        <EmptyState message="No people found." />
      ) : (
        <>
          {editMode && (
            <div className="flex items-center gap-2 mb-3">
              <input
                type="checkbox"
                checked={personList.length > 0 && selected.size === personList.length}
                onChange={toggleSelectAll}
                className="rounded border-zinc-600 bg-zinc-800 text-blue-500"
              />
              <span className="text-xs text-zinc-500">Select page</span>
            </div>
          )}

          <div className="grid gap-3 grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
            {personList.map((p) => {
              const photo = parsePhotos(p.photos)[0];
              if (editMode) {
                return (
                  <Card key={p.id} className="group overflow-hidden rounded-lg bg-white/5 person-card-flat">
                    <div className="flex items-center p-2">
                      <input
                        type="checkbox"
                        checked={selected.has(p.id)}
                        onChange={() => toggleSelect(p.id)}
                        className="rounded border-zinc-600 bg-zinc-800 text-blue-500 mr-2"
                      />
                      <h3 className="text-sm font-medium text-white line-clamp-1">{p.name}</h3>
                    </div>
                    <div className="relative aspect-[4/5] overflow-hidden">
                      <ProfileTile name={p.name} photo={photo} />
                    </div>
                    <div className="flex flex-col items-start p-2 pt-0">
                      {p.aliases && (
                        <span className="block text-[10px] text-zinc-400 line-clamp-1 mb-1">{p.aliases.split(',').slice(0,3).join(', ')}{p.aliases.split(',').length > 3 && '…'}</span>
                      )}
                      <span className="block text-[10px] text-zinc-500 mt-auto">
                        {p.gallery_count != null ?
                          (p.gallery_count === 1 ? '1 gallery' : `${p.gallery_count} galleries`) :
                          'No galleries'}
                      </span>
                    </div>
                  </Card>
                );
              } else {
                return (
                  <Link to={`/people/${p.id}`} key={p.id} className="group overflow-hidden rounded-lg bg-white/5 person-card-flat block">
                    <div className="relative aspect-[4/5] overflow-hidden">
                      <ProfileTile name={p.name} photo={photo} />
                    </div>
                    <div className="flex flex-col items-start p-2">
                      <h3 className="text-sm font-medium text-white line-clamp-1 mb-0.5">{p.name}</h3>
                      {p.aliases && (
                        <span className="block text-[10px] text-zinc-400 line-clamp-1 mb-1">{p.aliases.split(',').slice(0,3).join(', ')}{p.aliases.split(',').length > 3 && '…'}</span>
                      )}
                      <span className="block text-[10px] text-zinc-500 mt-auto">
                        {p.gallery_count != null ?
                          (p.gallery_count === 1 ? '1 gallery' : `${p.gallery_count} galleries`) :
                          'No galleries'}
                      </span>
                    </div>
                  </Link>
                );
              }
            })}
          </div>

          <Pagination
            page={currentPage}
            totalPages={totalPages}
            hasMore={currentPage < totalPages}
            onPrev={prevPage}
            onNext={nextPage}
          />
        </>
      )}

      <ConfirmDialog
        open={confirmDelete}
        title="Bulk Delete"
        message={`Are you sure you want to delete ${selected.size} people? This cannot be undone.`}
        onConfirm={() => {
          selected.forEach((id) => deleteMut.mutate(id));
          setSelected(new Set());
          setConfirmDelete(false);
        }}
        onCancel={() => setConfirmDelete(false)}
      />
    </>
  );
}
