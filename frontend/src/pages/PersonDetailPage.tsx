import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { people } from '@/lib/api';
import type { TagSuggestion } from '@/types';
import {
  Badge,
  Button,
  Spinner,
  EmptyState,
  Input,
} from '@/components/UI';
import {
  ArrowLeft,
  Sparkles,
  Link2,
  Save,
  X,
  Check,
  User,
  Edit,
  ChevronLeft,
  ChevronRight,
  Layers,
  Settings2,
  Calendar,
  MapPin,
  Maximize2,
  Weight,
  Palette,
  Fingerprint,
  Database,
  Trash2,
} from 'lucide-react';
import { CoverGrid } from '@/components/CoverGrid';

function parsePhotos(photos?: string): string[] {
  if (!photos) return [];
  try {
    const parsed = JSON.parse(photos);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function BioCard({ icon: Icon, label, value, color = "blue" }: { icon: any, label: string; value?: string | null, color?: string }) {
  if (!value) return null;

  const colors: Record<string, string> = {
    blue: "text-blue-300 bg-blue-500/10 border-blue-500/20",
    pink: "text-pink-300 bg-pink-500/10 border-pink-500/20",
    amber: "text-amber-300 bg-amber-500/10 border-amber-500/20",
    emerald: "text-emerald-300 bg-emerald-500/10 border-emerald-500/20",
    violet: "text-violet-300 bg-violet-500/10 border-violet-500/20",
    zinc: "text-zinc-300 bg-zinc-500/10 border-zinc-500/20",
  };

  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-3">
      <div className="flex items-center gap-3">
      <div className={`p-2 rounded-md border ${colors[color] || colors.zinc}`}>
        <Icon size={18} />
      </div>
      <div>
        <p className="text-[11px] uppercase tracking-wide text-zinc-500 font-medium">{label}</p>
        <p className="text-sm text-zinc-100">{value}</p>
      </div>
      </div>
    </div>
  );
}

export function PersonDetailPage() {
  const { id } = useParams<{ id: string }>();
  const personId = Number(id);
  const queryClient = useQueryClient();

  const [editing, setEditing] = useState(false);
  const [showTools, setShowTools] = useState(false);
  const [editForm, setEditForm] = useState<any>({});
  const [photoIndex, setPhotoIndex] = useState(0);
  const [linkGalleryId, setLinkGalleryId] = useState('');

  // Auto-tag state
  const [autoTagOpen, setAutoTagOpen] = useState(false);
  const [autoTagResults, setAutoTagResults] = useState<TagSuggestion[]>([]);
  const [selectedTags, setSelectedTags] = useState<Set<string>>(new Set());
  const [autoTagMinConfidence, setAutoTagMinConfidence] = useState(0.6);

  const { data: person, isLoading: loadingPerson } = useQuery({
    queryKey: ['person', personId],
    queryFn: () => people.get(personId),
  });

  const { data: stats } = useQuery({
    queryKey: ['person-stats', personId],
    queryFn: () => people.getStats(personId),
  });

  const { data: identifiers } = useQuery({
    queryKey: ['person-identifiers', personId],
    queryFn: () => people.identifiers(personId),
  });

  const updateMut = useMutation({
    mutationFn: () => people.update(personId, { name: editForm.name }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person', personId] });
      setEditing(false);
    },
  });

  const linkMut = useMutation({
    mutationFn: (galleryId: number) => people.linkGallery(personId, galleryId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person', personId] });
      setLinkGalleryId('');
    },
  });

  const autoTagMut = useMutation({
    mutationFn: () => people.autoTag(personId, autoTagMinConfidence, false),
    onSuccess: (result) => {
      setAutoTagResults(result.suggestions);
      setSelectedTags(new Set(result.suggestions.map((s) => `${s.type}-${s.id}`)));
    },
  });

  const applyAutoTagMut = useMutation({
    mutationFn: () => {
      const suggestions = Array.from(selectedTags).map((key) => {
        const [type, idStr] = key.split('-');
        return { type, id: parseInt(idStr) };
      });
      return people.applyAutoTagSuggestions(personId, suggestions);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person', personId] });
      setAutoTagOpen(false);
      setAutoTagResults([]);
      setSelectedTags(new Set());
    },
  });

  const [newAliasProvider, setNewAliasProvider] = useState('');
  const [newAliasName, setNewAliasName] = useState('');
  const [scanSource, setScanSource] = useState('MetArt');
  const [scanAlias, setScanAlias] = useState('');

  const { data: aliases, refetch: refetchAliases } = useQuery({
    queryKey: ['person-aliases', personId],
    queryFn: () => people.getProviderAliases(personId),
    enabled: showTools,
  });

  const { data: scans, refetch: refetchScans } = useQuery({
    queryKey: ['person-scans', personId],
    queryFn: () => people.getScans(personId),
    enabled: showTools,
  });

  const addAliasMut = useMutation({
    mutationFn: (data: { provider: string; alias: string }) =>
      people.createProviderAlias(personId, data),
    onSuccess: () => {
      refetchAliases();
      setNewAliasProvider('');
      setNewAliasName('');
    },
  });

  const deleteAliasMut = useMutation({
    mutationFn: (aliasId: number) =>
      people.deleteProviderAlias(personId, aliasId),
    onSuccess: () => {
      refetchAliases();
    },
  });

  const scanMut = useMutation({
    mutationFn: (data: { source: string; alias?: string }) =>
      people.scanPerson(personId, data.source, data.alias),
    onSuccess: () => {
      refetchScans();
    },
  });

  const linkFoundMut = useMutation({
    mutationFn: (data: { provider: string; source_url: string; name: string; thumbnail_url?: string }) =>
      people.linkFoundGallery(personId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person', personId] });
      refetchScans();
    },
  });

  // Mutation: link an "unsure" gallery result to this person
  const linkUnsureMut = useMutation({
    mutationFn: (data: { gallery_id: number; provider: string; source_url: string }) =>
      people.linkUnsureGallery(personId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person', personId] });
      refetchScans();
    },
  });

  // Mutation: exclude a scan result (provider + source_url/title)
  const excludeScanResultMut = useMutation({
    mutationFn: (data: { provider: string; source_url?: string; title?: string; reason?: string }) =>
      people.excludeScanResult(personId, data),
    onSuccess: () => {
      refetchScans();
    },
  });

  // Mutation: run auto-link galleries operation
  const autoLinkGalleriesMut = useMutation({
    mutationFn: () => people.autoLinkGalleries(personId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person', personId] });
      refetchScans();
    },
  });

  // Person exclusions list (for showing and removing)
  const { data: personExclusions, refetch: refetchExclusions } = useQuery({
    queryKey: ['person-exclusions', personId],
    queryFn: () => people.getExclusions(personId),
    enabled: showTools,
  });

  const removeExclusionMut = useMutation({
    mutationFn: (exclusionId: number) => people.removeExclusion(personId, exclusionId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person-exclusions', personId] });
      refetchExclusions();
    },
  });

  const [stashDbOpen, setStashDbOpen] = useState(false);
  const [stashDbSearchQuery, setStashDbSearchQuery] = useState('');

  const stashDbSearchMut = useMutation({
    mutationFn: (name: string) => people.searchStashDB(name),
  });

  const linkStashDbMut = useMutation({
    mutationFn: (stashId: string) => people.linkStashDB(personId, stashId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person', personId] });
      setStashDbOpen(false);
    },
  });

  const startEditing = () => {
    if (!person) return;
    setEditForm({ name: person.name });
    setEditing(true);
  };

  if (loadingPerson) return <div className="py-40 flex justify-center"><Spinner size="lg" /></div>;
  if (!person) return <EmptyState message="Profile not found" />;

  const photos = parsePhotos(person.photos);
  const coverPhoto = photos[photoIndex] ?? photos[0];
  const galleryList = person.galleries ?? [];
  const statChips = [
    { label: 'Galleries', value: String(stats?.galleries ?? galleryList.length) },
    { label: 'Photos', value: String(stats?.photos ?? 0) },
    { label: 'Videos', value: String(stats?.videos ?? 0) },
    { label: 'Linked IDs', value: String(identifiers?.length ?? 0) },
  ];

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 pb-16">
      <div className="py-4">
        <Link to="/people" className="inline-flex items-center gap-2 text-sm text-zinc-400 hover:text-zinc-200 transition-colors">
          <ArrowLeft size={16} />
          Back to People
        </Link>
      </div>
          <section className="rounded-xl border border-zinc-800 bg-zinc-900 p-3 md:p-4 mb-4">
            <div className="grid items-start grid-cols-[180px_minmax(0,1fr)] md:grid-cols-[220px_minmax(0,1fr)] gap-4">
              <div className="rounded-lg border border-zinc-700 bg-zinc-800 p-2">
                <div className="relative aspect-[3/4] overflow-hidden rounded-md bg-zinc-800">
                  {coverPhoto ? (
                    <img src={coverPhoto} alt={person.name} className="h-full w-full object-cover" />
                  ) : (
                    <div className="h-full w-full flex items-center justify-center">
                      <User size={44} className="text-zinc-600" />
                    </div>
                  )}
                </div>
                {photos.length > 1 && (
                  <div className="mt-2 flex items-center justify-between">
                    <button
                      onClick={() => setPhotoIndex((i) => (i - 1 + photos.length) % photos.length)}
                      className="inline-flex h-7 w-7 items-center justify-center rounded border border-zinc-600 text-zinc-300 hover:text-white"
                    >
                      <ChevronLeft size={14} />
                    </button>
                    <span className="text-xs text-zinc-400">{photoIndex + 1}/{photos.length}</span>
                    <button
                      onClick={() => setPhotoIndex((i) => (i + 1) % photos.length)}
                      className="inline-flex h-7 w-7 items-center justify-center rounded border border-zinc-600 text-zinc-300 hover:text-white"
                    >
                      <ChevronRight size={14} />
                    </button>
                  </div>
                )}
              </div>

              <div className="rounded-lg border border-zinc-700 bg-zinc-950/50 p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <h1 className="text-2xl md:text-3xl font-semibold text-white">{person.name}</h1>
                    {person.aliases && (
                      <p className="text-sm text-zinc-400 mt-1">
                        {typeof person.aliases === 'string' ? person.aliases.split(',').map((a) => a.trim()).join(' • ') : person.aliases}
                      </p>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <Button variant="secondary" size="sm" onClick={() => setShowTools((v) => !v)}>
                      <Settings2 size={14} /> {showTools ? 'Hide Tools' : 'Tools'}
                    </Button>
                    <Button size="sm" onClick={startEditing}>
                      <Edit size={14} /> Edit
                    </Button>
                  </div>
                </div>

                <div className="mt-4 grid grid-cols-1 sm:grid-cols-4 gap-2">
                  {statChips.map((chip) => (
                    <div key={chip.label} className="rounded-md border border-zinc-800 bg-zinc-900 px-3 py-2">
                      <p className="text-[11px] uppercase tracking-wide text-zinc-500">{chip.label}</p>
                      <p className="text-base text-zinc-100">{chip.value}</p>
                    </div>
                  ))}
                </div>

                <div className="mt-3 flex flex-wrap gap-2">
                  {person.nationality && <Badge variant="info">{person.nationality}</Badge>}
                  {person.ethnicity && <Badge>{person.ethnicity}</Badge>}
                  {identifiers?.map((id) => (
                    <Badge key={id.id} variant="success">{id.provider}</Badge>
                  ))}
                </div>

                <div className="mt-4 grid grid-cols-1 lg:grid-cols-2 gap-3">
                  <div className="space-y-2">
                    <BioCard icon={Calendar} label="Birth Date" value={person.birth_date} color="blue" />
                    <BioCard icon={Maximize2} label="Height" value={person.height} color="amber" />
                    <BioCard icon={Weight} label="Weight" value={person.weight} color="emerald" />
                    <BioCard icon={Check} label="Measurements" value={person.measurements} color="blue" />
                  </div>
                  <div className="space-y-2">
                    <BioCard icon={MapPin} label="Nationality" value={person.nationality} color="pink" />
                    <BioCard icon={Fingerprint} label="Ethnicity" value={person.ethnicity} color="violet" />
                    <BioCard icon={Palette} label="Hair Color" value={person.hair_color} color="amber" />
                    <BioCard icon={Palette} label="Eye Color" value={person.eye_color} color="violet" />
                  </div>
                </div>

                {person.biography && (
                  <div className="mt-4 rounded-md border border-zinc-800 bg-zinc-900 p-3">
                    <p className="text-xs uppercase tracking-wide text-zinc-500 mb-1">Biography</p>
                    <p className="text-sm text-zinc-300 whitespace-pre-line">{person.biography}</p>
                  </div>
                )}

                {showTools && (
                  <div className="mt-6 rounded-xl border border-zinc-800 bg-zinc-900/50 p-6 space-y-6">
                    <div className="flex items-center justify-between border-b border-zinc-800 pb-4">
                      <h3 className="text-lg font-medium text-white flex items-center gap-2">
                        <Settings2 size={18} className="text-blue-400" />
                        Person Management Tools
                      </h3>
                    </div>

                    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                      {/* Linking & Aliases */}
                      <div className="space-y-6">
                        <div className="bg-zinc-950/40 p-4 rounded-lg border border-zinc-800/80 space-y-4">
                          <h4 className="text-sm font-semibold text-zinc-300">Linking & Auto-Tagging</h4>
                          <div className="flex gap-3">
                            <Button variant="secondary" className="flex-1" size="sm" onClick={() => {
                              setAutoTagOpen(true);
                              setAutoTagResults([]);
                              setSelectedTags(new Set());
                            }}>
                              <Sparkles size={14} className="mr-1.5 text-blue-400" /> Auto-Tag
                            </Button>
                            <Button variant="secondary" className="flex-1" size="sm" onClick={() => {
                              setStashDbOpen(true);
                              setStashDbSearchQuery(person.name);
                              stashDbSearchMut.mutate(person.name);
                            }}>
                              <Database size={14} className="mr-1.5 text-purple-400" /> StashDB
                            </Button>
                            <div className="flex-[2] flex gap-2">
                              <Input placeholder="Gallery ID" value={linkGalleryId} onChange={(e) => setLinkGalleryId(e.target.value)} className="h-9" />
                              <Button
                                size="sm"
                                onClick={() => {
                                  const gid = parseInt(linkGalleryId, 10);
                                  if (gid) linkMut.mutate(gid);
                                }}
                                disabled={!linkGalleryId || linkMut.isPending}
                              >
                                <Link2 size={14} /> Link
                              </Button>
                            </div>
                          </div>
                          <div className="mt-2 flex gap-2">
                            <Button size="sm" variant="secondary" onClick={() => autoLinkGalleriesMut.mutate()} disabled={autoLinkGalleriesMut.isPending}>
                              {autoLinkGalleriesMut.isPending ? 'Linking...' : 'Auto-Link Galleries'}
                            </Button>
                          </div>
                        </div>

                        <div className="bg-zinc-950/40 p-4 rounded-lg border border-zinc-800/80 space-y-4">
                          <h4 className="text-sm font-semibold text-zinc-300">Provider Aliases</h4>
                          {aliases && aliases.length > 0 ? (
                            <div className="space-y-2 max-h-40 overflow-y-auto pr-1">
                              {aliases.map((al) => (
                                <div key={al.id} className="flex items-center justify-between bg-zinc-900/80 border border-zinc-800 px-3 py-1.5 rounded text-sm">
                                  <div className="flex items-center gap-2">
                                    <Badge variant="info">{al.provider}</Badge>
                                    <span className="text-zinc-200 font-medium">{al.alias}</span>
                                  </div>
                                  <button
                                    onClick={() => deleteAliasMut.mutate(al.id)}
                                    className="p-1 text-zinc-500 hover:text-red-400 hover:bg-red-400/10 rounded transition-all"
                                    title="Remove alias"
                                  >
                                    <X size={14} />
                                  </button>
                                </div>
                              ))}
                            </div>
                          ) : (
                            <p className="text-xs text-zinc-500 italic">No provider aliases defined.</p>
                          )}

                          <div className="flex gap-2 items-end">
                            <div className="flex-1">
                              <label className="text-[10px] text-zinc-500 uppercase tracking-wider block mb-1">Provider</label>
                              <select
                                value={newAliasProvider}
                                onChange={(e) => setNewAliasProvider(e.target.value)}
                                className="bg-zinc-900 border border-zinc-700 rounded px-2.5 py-1.5 text-xs text-zinc-200 focus:outline-none transition-all cursor-pointer w-full h-9"
                              >
                                <option value="">Select...</option>
                                <option value="MetArt">MetArt</option>
                                <option value="MPLStudios">MPLStudios</option>
                                <option value="Playboy">Playboy</option>
                              </select>
                            </div>
                            <div className="flex-1">
                              <label className="text-[10px] text-zinc-500 uppercase tracking-wider block mb-1">Alias/ID</label>
                              <Input
                                placeholder="e.g. Alysa_A"
                                value={newAliasName}
                                onChange={(e) => setNewAliasName(e.target.value)}
                                className="h-9"
                              />
                            </div>
                            <Button
                              size="sm"
                              className="h-9"
                              onClick={() => {
                                if (newAliasProvider && newAliasName) {
                                  addAliasMut.mutate({ provider: newAliasProvider, alias: newAliasName });
                                }
                              }}
                              disabled={!newAliasProvider || !newAliasName || addAliasMut.isPending}
                            >
                              Add
                            </Button>
                          </div>
                        </div>
                      </div>

                      {/* Scanners & Scans */}
                      <div className="space-y-6">
                        <div className="bg-zinc-950/40 p-4 rounded-lg border border-zinc-800/80 space-y-4">
                          <h4 className="text-sm font-semibold text-zinc-300">Run Provider Scanner</h4>
                          <div className="flex gap-2 items-end">
                            <div className="flex-1">
                              <label className="text-[10px] text-zinc-500 uppercase tracking-wider block mb-1">Source Provider</label>
                              <select
                                value={scanSource}
                                onChange={(e) => setScanSource(e.target.value)}
                                className="bg-zinc-900 border border-zinc-700 rounded px-2.5 py-1.5 text-xs text-zinc-200 focus:outline-none transition-all cursor-pointer w-full h-9"
                              >
                                <option value="MetArt">MetArt</option>
                                <option value="MPLStudios">MPLStudios</option>
                                <option value="Playboy">Playboy</option>
                              </select>
                            </div>
                            <div className="flex-1">
                              <label className="text-[10px] text-zinc-500 uppercase tracking-wider block mb-1">Optional Override Alias</label>
                              <Input
                                placeholder="Use custom alias"
                                value={scanAlias}
                                onChange={(e) => setScanAlias(e.target.value)}
                                className="h-9"
                              />
                            </div>
                            <Button
                              size="sm"
                              className="h-9"
                              onClick={() => {
                                scanMut.mutate({ source: scanSource, alias: scanAlias || undefined });
                              }}
                              disabled={scanMut.isPending}
                            >
                              {scanMut.isPending ? 'Queued' : 'Trigger Scan'}
                            </Button>
                          </div>
                        </div>

                        <div className="bg-zinc-950/40 p-4 rounded-lg border border-zinc-800/80 space-y-4">
                          <h4 className="text-sm font-semibold text-zinc-300">Recent Scans & Results</h4>
                          {scans && scans.length > 0 ? (
                            <div className="space-y-3 max-h-80 overflow-y-auto pr-1">
                              {scans.map((scan) => {
                                const res = scan.results;
                                const missingGals = res?.missing_galleries || [];
                                return (
                                  <div key={scan.id} className="bg-zinc-900/80 border border-zinc-800 p-3 rounded text-xs space-y-2">
                                    <div className="flex items-center justify-between">
                                      <div className="flex items-center gap-1.5">
                                        <Badge variant="info">{scan.provider || (scan as any).source}</Badge>
                                        <span className="text-zinc-400">Alias: {scan.alias}</span>
                                      </div>
                                      <Badge variant={scan.status === 'completed' ? 'success' : 'warning'}>{scan.status}</Badge>
                                    </div>
                                    {res && (
                                      <div className="grid grid-cols-2 gap-2 text-[10px] text-zinc-400">
                                        <div>Found: <span className="text-zinc-200">{res.found_count ?? 0}</span></div>
                                        <div>Existing: <span className="text-zinc-200">{res.existing_count ?? 0}</span></div>
                                        <div>Unsure: <span className="text-zinc-200">{res.unsure_count ?? 0}</span></div>
                                        <div>Missing: <span className="text-zinc-200">{res.missing_count ?? 0}</span></div>
                                      </div>
                                    )}
                                    {missingGals.length > 0 && (
                                      <div className="border-t border-zinc-800 pt-2 space-y-1.5">
                                        <p className="text-[10px] uppercase font-bold text-zinc-400">Missing galleries from scan:</p>
                                        <div className="space-y-1.5 max-h-48 overflow-y-auto">
                                      {missingGals.map((mg: any, idx: number) => (
                                        <div key={idx} className="flex items-center justify-between bg-zinc-950 p-2 rounded border border-zinc-800/50">
                                          <div className="min-w-0 flex-1 pr-2">
                                            <p className="truncate text-zinc-300 font-medium" title={mg.title}>{mg.title}</p>
                                            {mg.release_date && <p className="text-[9px] text-zinc-500">{mg.release_date}</p>}
                                          </div>
                                          <Button
                                            size="sm"
                                            className="h-7 px-2 text-[10px] shrink-0"
                                            disabled={linkFoundMut.isPending}
                                            onClick={() => {
                                              linkFoundMut.mutate({
                                                provider: scan.provider ?? '',
                                                source_url: mg.url ?? '',
                                                name: mg.title ?? '',
                                                thumbnail_url: mg.thumbnail,
                                              });
                                            }}
                                          >
                                            Add
                                          </Button>
                                          {mg.unsure && (
                                            <div className="ml-2 flex items-center gap-2">
                                              <Button size="sm" className="h-7 px-2 text-[10px]" onClick={() => {
                                                // mg is an unsure gallery result with gallery_id
                                                if (mg.id || mg.gallery_id) {
                                                  const gid = mg.gallery_id || mg.id;
                                                  linkUnsureMut.mutate({ gallery_id: gid, provider: scan.provider ?? '', source_url: mg.url ?? '' });
                                                }
                                              }}>
                                                Link Unsure
                                              </Button>
                                              <Button size="sm" variant="secondary" className="h-7 px-2 text-[10px]" onClick={() => {
                                                excludeScanResultMut.mutate({ provider: scan.provider ?? '', source_url: mg.url ?? '', title: mg.title ?? '' });
                                              }}>
                                                Exclude
                                              </Button>
                                            </div>
                                          )}
                                        </div>
                                      ))}
                                        </div>
                                      </div>
                                    )}
                                  </div>
                                );
                              })}
                            </div>
                          ) : (
                            <p className="text-xs text-zinc-500 italic">No scans run yet.</p>
                          )}
                          {/* Exclusions list */}
                          {personExclusions && personExclusions.length > 0 && (
                            <div className="mt-3 border-t border-zinc-800 pt-3">
                              <p className="text-xs uppercase tracking-wide text-zinc-500 mb-2">Exclusions</p>
                              <div className="space-y-2">
                                {personExclusions.map((ex: any) => (
                                  <div key={ex.id} className="flex items-center justify-between bg-zinc-900/80 border border-zinc-800 px-3 py-2 rounded text-sm">
                                    <div className="text-zinc-200 text-sm">
                                      <div className="truncate">{ex.provider} — {ex.title ?? ex.source_url}</div>
                                      {ex.source_url && <div className="text-xs text-zinc-500">{ex.source_url}</div>}
                                    </div>
                                    <button
                                      onClick={() => removeExclusionMut.mutate(ex.id)}
                                      className="p-2 text-zinc-400 hover:text-red-400 rounded"
                                      title="Remove exclusion"
                                    >
                                      <Trash2 size={16} />
                                    </button>
                                  </div>
                                ))}
                              </div>
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </section>

          {/* Auto-Tag Modal */}
          {autoTagOpen && (
            <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 backdrop-blur-md p-4">
              <div className="w-full max-w-4xl max-h-[90vh] flex flex-col overflow-hidden rounded-[2.5rem] border border-white/10 bg-[#0b0b10] shadow-2xl">
                <div className="flex items-center justify-between border-b border-white/10 px-8 py-6">
                  <div>
                    <h2 className="text-xl font-bold text-white">Auto-Tag Galleries</h2>
                    <p className="text-sm text-zinc-400 mt-1">Scan galleries and videos matching &quot;{person.name}&quot;</p>
                  </div>
                  <button onClick={() => setAutoTagOpen(false)} className="rounded-full p-2 text-zinc-400 hover:bg-white/5 hover:text-white transition-colors">
                    <X size={20} />
                  </button>
                </div>

                <div className="px-8 py-6 bg-white/[0.02]">
                  <div className="flex items-end gap-4">
                    <div className="w-48">
                      <Input
                        label="Min Confidence"
                        type="number"
                        step="0.05"
                        min="0"
                        max="1"
                        value={autoTagMinConfidence}
                        onChange={(e) => setAutoTagMinConfidence(parseFloat(e.target.value) || 0.5)}
                      />
                    </div>
                    <Button size="sm" onClick={() => autoTagMut.mutate()} disabled={autoTagMut.isPending}>
                      {autoTagMut.isPending ? 'Scanning...' : 'Run Scan'}
                    </Button>
                  </div>
                </div>

                <div className="flex-1 overflow-y-auto px-8 py-6 space-y-4">
                  {autoTagMut.isPending && (
                    <div className="py-20 flex flex-col items-center justify-center gap-4">
                      <Spinner size="lg" />
                      <p className="text-zinc-500 animate-pulse">Scanning galleries and videos...</p>
                    </div>
                  )}

                  {!autoTagMut.isPending && autoTagResults.length === 0 && (
                    <div className="py-12 text-center">
                      <Sparkles size={48} className="text-zinc-700 mx-auto mb-3" />
                      <p className="text-zinc-500">Click &quot;Run Scan&quot; to find matching galleries and videos.</p>
                    </div>
                  )}

                  {autoTagResults.length > 0 && (
                    <>
                      <div className="flex items-center justify-between mb-2">
                        <p className="text-sm text-zinc-400">{autoTagResults.length} suggestions found</p>
                        <div className="flex gap-2">
                          <Button variant="secondary" size="sm" onClick={() => setSelectedTags(new Set())}>Clear All</Button>
                          <Button variant="secondary" size="sm" onClick={() => setSelectedTags(new Set(autoTagResults.map((s) => `${s.type}-${s.id}`)))}>Select All</Button>
                        </div>
                      </div>
                      <div className="space-y-2">
                        {autoTagResults.map((s) => {
                          const key = `${s.type}-${s.id}`;
                          const isSelected = selectedTags.has(key);
                          return (
                            <button
                              key={key}
                              onClick={() => {
                                setSelectedTags((prev) => {
                                  const next = new Set(prev);
                                  if (next.has(key)) next.delete(key);
                                  else next.add(key);
                                  return next;
                                });
                              }}
                              className={`w-full flex items-center gap-3 p-3 rounded-lg border transition-colors text-left ${
                                isSelected
                                  ? 'border-blue-500/50 bg-blue-500/10'
                                  : 'border-zinc-700 hover:border-zinc-500'
                              }`}
                            >
                              <input
                                type="checkbox"
                                checked={isSelected}
                                onChange={() => {}}
                                className="rounded border-zinc-600 bg-zinc-800 text-blue-500"
                              />
                              <Badge variant={s.type === 'gallery' ? 'info' : 'warning'}>{s.type}</Badge>
                              <span className="text-sm text-zinc-200 flex-1 truncate">{s.name}</span>
                              <span className="text-xs text-zinc-500">{(s.confidence * 100).toFixed(0)}%</span>
                              <span className="text-xs text-zinc-600">{s.matched_on}</span>
                            </button>
                          );
                        })}
                      </div>
                    </>
                  )}
                </div>

                {autoTagResults.length > 0 && (
                  <div className="flex items-center justify-between border-t border-white/10 px-8 py-6 bg-blue-500/5">
                    <p className="text-xs text-zinc-400">{selectedTags.size} selected</p>
                    <div className="flex gap-3">
                      <Button variant="secondary" onClick={() => setAutoTagOpen(false)}>Cancel</Button>
                      <Button onClick={() => applyAutoTagMut.mutate()} disabled={selectedTags.size === 0 || applyAutoTagMut.isPending}>
                        {applyAutoTagMut.isPending ? 'Applying...' : 'Apply Selected'}
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}

          <section className="rounded-xl border border-zinc-800 bg-zinc-900 p-4 md:p-5">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-semibold text-white flex items-center gap-2">
                <Layers size={16} />
                Galleries
              </h2>
              <Badge>{galleryList.length} total</Badge>
            </div>

            {galleryList.length === 0 ? (
              <EmptyState message="No galleries linked yet." />
            ) : (
              <CoverGrid
                items={galleryList.map((g: any) => ({
                  id: g.id,
                  title: g.name ?? null,
                  thumbnailPath: g.provider_thumbnail
                    ? g.provider_thumbnail.replace(/\\/g, '/').split('/').pop()
                    : g.images?.[0]?.filename,
                  provider: g.provider ?? null,
                  createdAt: g.created_at,
                }))}
              />
            )}
          </section>

      {/* Edit Modal */}
      {editing && (
        <div className="fixed inset-0 z-[110] flex items-center justify-center bg-black/80 backdrop-blur-xl p-4">
          <div className="w-full max-w-lg max-h-[90vh] flex flex-col overflow-hidden rounded-[3rem] border border-white/10 bg-[#0b0b10] shadow-2xl">
            <div className="flex items-center justify-between px-10 py-8">
              <h2 className="text-3xl font-black text-white tracking-tight">Edit Name</h2>
              <button onClick={() => setEditing(false)} className="p-2 rounded-full hover:bg-white/5 text-zinc-400 transition-colors">
                <X size={28} />
              </button>
            </div>

            <div className="px-10 pb-10">
              <Input label="Name" value={editForm.name} onChange={e => setEditForm({...editForm, name: e.target.value})} />
            </div>

            <div className="px-10 py-8 border-t border-white/5 flex justify-end gap-3 bg-white/[0.01]">
              <Button variant="secondary" onClick={() => setEditing(false)}>Discard</Button>
              <Button onClick={() => updateMut.mutate()} disabled={updateMut.isPending}>
                <Save size={18} className="mr-2" /> Save
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* StashDB Modal */}
      {stashDbOpen && (
        <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 backdrop-blur-md p-4">
          <div className="w-full max-w-4xl max-h-[90vh] flex flex-col overflow-hidden rounded-[2.5rem] border border-white/10 bg-[#0b0b10] shadow-2xl">
            <div className="flex items-center justify-between border-b border-white/10 px-8 py-6">
              <div>
                <h2 className="text-xl font-bold text-white flex items-center gap-2">
                  <Database size={20} className="text-purple-400" />
                  Search StashDB
                </h2>
                <p className="text-sm text-zinc-400 mt-1">Search and link performer data, photos, and social links</p>
              </div>
              <button onClick={() => setStashDbOpen(false)} className="rounded-full p-2 text-zinc-400 hover:bg-white/5 hover:text-white transition-colors">
                <X size={20} />
              </button>
            </div>

            <div className="px-8 py-6 bg-white/[0.02]">
              <div className="flex items-end gap-4">
                <div className="flex-1">
                  <Input
                    label="Performer Name"
                    value={stashDbSearchQuery}
                    onChange={(e) => setStashDbSearchQuery(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && stashDbSearchMut.mutate(stashDbSearchQuery)}
                  />
                </div>
                <Button size="sm" onClick={() => stashDbSearchMut.mutate(stashDbSearchQuery)} disabled={stashDbSearchMut.isPending}>
                  {stashDbSearchMut.isPending ? 'Searching...' : 'Search'}
                </Button>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto px-8 py-6 space-y-4">
              {stashDbSearchMut.isPending && (
                <div className="py-20 flex flex-col items-center justify-center gap-4">
                  <Spinner size="lg" />
                  <p className="text-zinc-500 animate-pulse">Searching StashDB API...</p>
                </div>
              )}

              {!stashDbSearchMut.isPending && !stashDbSearchMut.data && (
                <div className="py-12 text-center">
                  <Database size={48} className="text-zinc-700 mx-auto mb-3" />
                  <p className="text-zinc-500">Enter a name and search StashDB.</p>
                </div>
              )}

              {!stashDbSearchMut.isPending && stashDbSearchMut.data && (stashDbSearchMut.data as any).data?.length === 0 && (
                <div className="py-12 text-center">
                  <p className="text-zinc-500">No performers found matching &quot;{stashDbSearchQuery}&quot;.</p>
                </div>
              )}

              {!stashDbSearchMut.isPending && stashDbSearchMut.data && (stashDbSearchMut.data as any).data?.length > 0 && (
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4 animate-fade-in">
                  {(stashDbSearchMut.data as any).data.map((perf: any) => (
                    <div
                      key={perf.id}
                      className="flex items-start gap-4 p-4 rounded-xl border border-zinc-800 bg-zinc-950/40 hover:border-zinc-700 transition-colors"
                    >
                      <div className="h-20 w-16 rounded overflow-hidden bg-zinc-900 shrink-0 border border-zinc-800">
                        {perf.images && perf.images.length > 0 ? (
                          <img src={perf.images[0].url} alt={perf.name} className="h-full w-full object-cover" />
                        ) : (
                          <div className="h-full w-full flex items-center justify-center text-zinc-700">
                            <User size={24} />
                          </div>
                        )}
                      </div>
                      <div className="min-w-0 flex-1">
                        <h4 className="font-bold text-white text-sm truncate">{perf.name}</h4>
                        {perf.disambiguation && (
                          <p className="text-xs text-purple-400 italic truncate mb-1">{perf.disambiguation}</p>
                        )}
                        {perf.aliases && perf.aliases.length > 0 && (
                          <p className="text-[10px] text-zinc-500 truncate">
                            Aliases: {perf.aliases.join(', ')}
                          </p>
                        )}
                      </div>
                      <Button
                        size="sm"
                        disabled={linkStashDbMut.isPending}
                        onClick={() => linkStashDbMut.mutate(perf.id)}
                        className="shrink-0 self-center"
                      >
                        {linkStashDbMut.isPending ? 'Linking...' : 'Link'}
                      </Button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
