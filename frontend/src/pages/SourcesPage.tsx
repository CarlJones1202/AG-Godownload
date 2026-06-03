import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { sources } from '@/lib/api';
import { formatDateTime } from '@/lib/utils';
import {
  PageHeader,
  Button,
  Card,
  Badge,
  Spinner,
  EmptyState,
  Input,
  Textarea,
  ConfirmDialog,
} from '@/components/UI';
import { Plus, Play, Trash2, Upload } from 'lucide-react';

export function SourcesPage() {
  const queryClient = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [showBulkImport, setShowBulkImport] = useState(false);
  const [bulkJson, setBulkJson] = useState('');
  const [bulkStatus, setBulkStatus] = useState<{
    type: 'success' | 'error' | null;
    message: string;
  }>({ type: null, message: '' });
  const [deleteId, setDeleteId] = useState<number | null>(null);
  const [newSource, setNewSource] = useState({ location: '', name: '', priority: 0 });

  const { data: sourceData, isLoading } = useQuery({
    queryKey: ['sources'],
    queryFn: () => sources.list({ limit: 200 }),
  });
  const sourceList = sourceData?.data ?? [];

  const createMut = useMutation({
    mutationFn: () => sources.create({
      name: newSource.name || newSource.location,
      location: newSource.location,
      priority: newSource.priority,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sources'] });
      setShowCreate(false);
      setNewSource({ location: '', name: '', priority: 0 });
    },
  });

  const bulkCreateMut = useMutation({
    mutationFn: (data: { url: string; name?: string }[]) =>
      sources.bulkCreate(data),
    onSuccess: (response) => {
      queryClient.invalidateQueries({ queryKey: ['sources'] });
      const s = response.summary;
      if (s.created > 0) {
        setBulkStatus({
          type: 'success',
          message: `Created ${s.created} source(s). ${s.duplicates} duplicate(s) skipped, ${s.failed} failed.`,
        });
        if (s.failed === 0 && s.duplicates === 0) {
          setBulkJson('');
          setTimeout(() => {
            setShowBulkImport(false);
            setBulkStatus({ type: null, message: '' });
          }, 1500);
        }
      } else {
        setBulkStatus({ type: 'error', message: `No sources created. ${s.duplicates} duplicates, ${s.failed} failed.` });
      }
    },
    onError: (err: any) => {
      setBulkStatus({ type: 'error', message: err.message || 'Failed to import sources' });
    },
  });

  const handleBulkImportSubmit = () => {
    setBulkStatus({ type: null, message: '' });
    try {
      const parsed = JSON.parse(bulkJson);
      if (!Array.isArray(parsed)) throw new Error('JSON must be an array');
      const formatted = parsed.map((item: any, idx: number) => {
        if (!item.url) throw new Error(`Item ${idx} is missing "url"`);
        return { url: String(item.url), name: item.name ? String(item.name) : undefined };
      });
      bulkCreateMut.mutate(formatted);
    } catch (e: any) {
      setBulkStatus({ type: 'error', message: `Error: ${e.message}` });
    }
  };

  const crawlMut = useMutation({
    mutationFn: (id: number) => sources.crawl(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['sources'] }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => sources.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sources'] });
      setDeleteId(null);
    },
  });

  if (isLoading) return <Spinner />;

  return (
    <>
      <PageHeader title="Sources" description="Manage crawlable content sources">
        <div className="flex gap-2">
          <Button
            variant="secondary"
            onClick={() => {
              setShowBulkImport(!showBulkImport);
              setShowCreate(false);
              setBulkStatus({ type: null, message: '' });
            }}
          >
            <Upload size={14} /> Bulk Import
          </Button>
          <Button
            onClick={() => {
              setShowCreate(!showCreate);
              setShowBulkImport(false);
            }}
          >
            <Plus size={14} /> Add Source
          </Button>
        </div>
      </PageHeader>

      {showCreate && (
        <Card className="mb-4">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <Input
              label="URL"
              placeholder="https://example.com/thread/123"
              value={newSource.location}
              onChange={(e) => setNewSource({ ...newSource, location: e.target.value })}
            />
            <Input
              label="Name"
              placeholder="Source name (optional)"
              value={newSource.name}
              onChange={(e) => setNewSource({ ...newSource, name: e.target.value })}
            />
            <Input
              label="Priority"
              type="number"
              value={newSource.priority}
              onChange={(e) => setNewSource({ ...newSource, priority: parseInt(e.target.value) || 0 })}
            />
          </div>
          <div className="flex justify-end gap-2 mt-3">
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(false)}>Cancel</Button>
            <Button size="sm" onClick={() => createMut.mutate()} disabled={!newSource.location || createMut.isPending}>Create</Button>
          </div>
        </Card>
      )}

      {showBulkImport && (
        <Card className="mb-4">
          <h3 className="text-sm font-semibold text-white mb-1">Bulk Import Sources</h3>
          <p className="text-xs text-zinc-400 mb-3">
            Paste a JSON array of sources. Each entry needs a <code>url</code>, with optional <code>name</code>.
          </p>
          <Textarea
            label="JSON Sources List"
            placeholder='[\n  { "url": "https://...", "name": "My Source" }\n]'
            value={bulkJson}
            onChange={(e) => setBulkJson(e.target.value)}
            rows={8}
            className="font-mono text-xs"
          />
          {bulkStatus.message && (
            <div className={`mt-3 p-2.5 rounded text-xs border ${
              bulkStatus.type === 'success'
                ? 'bg-emerald-950/50 text-emerald-400 border-emerald-800/80'
                : 'bg-red-950/50 text-red-400 border-red-800/80'
            }`}>
              {bulkStatus.message}
            </div>
          )}
          <div className="flex justify-end gap-2 mt-3">
            <Button variant="secondary" size="sm" onClick={() => { setShowBulkImport(false); setBulkStatus({ type: null, message: '' }); }}>Cancel</Button>
            <Button size="sm" onClick={handleBulkImportSubmit} disabled={!bulkJson.trim() || bulkCreateMut.isPending}>
              {bulkCreateMut.isPending ? 'Importing...' : 'Import Sources'}
            </Button>
          </div>
        </Card>
      )}

      {sourceList.length === 0 ? (
        <EmptyState message="No sources yet. Add one to get started." />
      ) : (
        <div className="space-y-2">
          {sourceList.map((src) => (
            <Card key={src.id} className="flex items-center justify-between">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-white truncate">{src.name}</span>
                  <Badge variant={src.status === 'idle' ? 'default' : src.status === 'crawling' ? 'warning' : src.status === 'error' ? 'danger' : 'info'}>
                    {src.status || 'idle'}
                  </Badge>
                  {src.priority > 0 && <Badge variant="info">P{src.priority}</Badge>}
                </div>
                <p className="text-xs text-zinc-500 truncate mt-0.5">{src.location}</p>
                <p className="text-xs text-zinc-600 mt-0.5">
                  {src.last_checked_at && src.last_checked_at !== '0001-01-01T00:00:00Z'
                    ? `Last checked: ${formatDateTime(src.last_checked_at)}`
                    : 'Never checked'}
                </p>
                {src.status === 'crawling' && (
                  <div className="mt-2 w-full bg-zinc-800 rounded-full h-1.5">
                    <div className="bg-blue-500 h-1.5 rounded-full transition-all" style={{ width: `${src.download_progress}%` }} />
                  </div>
                )}
              </div>
              <div className="flex items-center gap-1 ml-4">
                <Button variant="ghost" size="sm" title="Crawl" onClick={() => crawlMut.mutate(src.id)} disabled={crawlMut.isPending}>
                  <Play size={14} />
                </Button>
                <Button variant="ghost" size="sm" title="Delete" onClick={() => setDeleteId(src.id)}>
                  <Trash2 size={14} />
                </Button>
              </div>
            </Card>
          ))}
        </div>
      )}

      <ConfirmDialog
        open={deleteId !== null}
        title="Delete Source"
        message="Are you sure? This will remove the source. Galleries and images will not be deleted."
        onConfirm={() => deleteId && deleteMut.mutate(deleteId)}
        onCancel={() => setDeleteId(null)}
      />
    </>
  );
}
