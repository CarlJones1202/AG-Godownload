import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { galleries } from '@/lib/api';
import {
  PageHeader,
  Spinner,
  EmptyState,
  Input,
  Button,
  Card,
  Textarea,
  Pagination,
  ConfirmDialog,
} from '@/components/UI';
import { CoverGrid } from '@/components/CoverGrid';
import { Search, Grid3X3, Plus } from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';

export function GalleriesPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const { page, limit, prevPage, nextPage, resetPage } = usePagination({ limit: 50 });
  const [confirmDeleteId, setConfirmDeleteId] = useState<number | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [newGallery, setNewGallery] = useState({
    name: '',
    source_url: '',
    provider: '',
    description: '',
  });

  const { data: galleryList, isLoading } = useQuery({
    queryKey: ['galleries', { search, page, limit }],
    queryFn: () =>
      galleries.list({
        q: search || undefined,
        limit,
        page,
      }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => galleries.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['galleries'] });
      setConfirmDeleteId(null);
    },
  });

  const createMut = useMutation({
    mutationFn: () => galleries.create({
      name: newGallery.name,
      source_url: newGallery.source_url || undefined,
      provider: newGallery.provider || undefined,
      description: newGallery.description || undefined,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['galleries'] });
      setShowCreate(false);
      setNewGallery({ name: '', source_url: '', provider: '', description: '' });
    },
  });

  const handleDelete = (id: number) => {
    setConfirmDeleteId(id);
  };

  const coverItems = galleryList?.data.map((g) => ({
    id: g.id,
    title: g.name || null,
    thumbnailPath: g.provider_thumbnail
      ? g.provider_thumbnail.replace(/\\/g, '/').split('/').pop()
      : g.images?.[0]?.filename,
    provider: g.provider,
    createdAt: g.created_at,
    url: g.source_url,
  })) ?? [];

  return (
    <>
      <PageHeader title="Galleries" description="Your image gallery collection">
        <div className="flex items-center gap-2">
          <Button size="sm" variant={showCreate ? 'primary' : 'secondary'} onClick={() => setShowCreate(!showCreate)}>
            <Plus size={14} /> Create
          </Button>
          <Grid3X3 size={18} className="text-zinc-400" />
        </div>
      </PageHeader>

      {showCreate && (
        <Card className="mb-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <Input
              label="Name"
              placeholder="Gallery name"
              value={newGallery.name}
              onChange={(e) => setNewGallery({ ...newGallery, name: e.target.value })}
            />
            <Input
              label="Source URL"
              placeholder="https://example.com/gallery/123"
              value={newGallery.source_url}
              onChange={(e) => setNewGallery({ ...newGallery, source_url: e.target.value })}
            />
            <Input
              label="Provider"
              placeholder="e.g. MetArt"
              value={newGallery.provider}
              onChange={(e) => setNewGallery({ ...newGallery, provider: e.target.value })}
            />
          </div>
          <Textarea
            label="Description"
            placeholder="Optional description"
            value={newGallery.description}
            onChange={(e) => setNewGallery({ ...newGallery, description: e.target.value })}
            rows={2}
            className="mt-3"
          />
          <div className="flex justify-end gap-2 mt-3">
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(false)}>Cancel</Button>
            <Button size="sm" onClick={() => createMut.mutate()} disabled={!newGallery.name || createMut.isPending}>
              {createMut.isPending ? 'Creating...' : 'Create Gallery'}
            </Button>
          </div>
        </Card>
      )}

      {/* Search bar */}
      <div className="mb-6 relative">
        <Search
          size={16}
          className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500"
        />
        <Input
          placeholder="Search galleries..."
          value={search}
          onChange={(e) => {
            setSearch(e.target.value);
            resetPage();
          }}
          className="pl-9 bg-zinc-900 border-zinc-700"
        />
      </div>

      {isLoading ? (
        <Spinner />
      ) : !galleryList || galleryList.data.length === 0 ? (
        <EmptyState message="No galleries found." />
      ) : (
        <>
          <CoverGrid items={coverItems} onDelete={handleDelete} />

          <Pagination
            page={galleryList.meta.current_page}
            totalPages={galleryList.meta.total_pages}
            onPrev={prevPage}
            onNext={nextPage}
            hasMore={galleryList.meta.current_page < galleryList.meta.total_pages}
          />
        </>
      )}

      <ConfirmDialog
        open={confirmDeleteId !== null}
        title="Delete Gallery"
        message="Delete this gallery and all its images? Files will be removed from disk. This cannot be undone."
        confirmLabel="Delete Gallery"
        onConfirm={() => {
          if (confirmDeleteId !== null) {
            deleteMut.mutate(confirmDeleteId);
          }
        }}
        onCancel={() => setConfirmDeleteId(null)}
      />
    </>
  );
}
