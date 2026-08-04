import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { people } from '@/lib/api';
import type { PersonScanResponse, TagSuggestion } from '@/types';
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
  Search,
  X,
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
  Check,
  ExternalLink,
  Camera,
} from 'lucide-react';
import { CoverGrid } from '@/components/CoverGrid';

const PROVIDER_LIST = [
  "MetArt", "MetartX", "Playboy", "PlayboyPlus", "Vixen",
  "SexArt", "LifeErotic", "EternalDesire", "MPLStudios",
  "VivThomas", "WowGirls", "RylskyArt",
] as const;

function parsePhotos(photos?: string): string[] {
  if (!photos) return [];
  try { const p = JSON.parse(photos); return Array.isArray(p) ? p : []; } catch { return []; }
}

function BioCard({ icon: Icon, label, value, color = "blue" }: { icon: any; label: string; value?: string | null; color?: string }) {
  if (!value) return null;
  const colors: Record<string, string> = {
    blue: "text-blue-300 bg-blue-500/10", pink: "text-pink-300 bg-pink-500/10",
    amber: "text-amber-300 bg-amber-500/10", emerald: "text-emerald-300 bg-emerald-500/10",
    violet: "text-violet-300 bg-violet-500/10", zinc: "text-zinc-300 bg-zinc-500/10",
  };
  return (
    <div className="flex items-center gap-2.5 p-2.5 rounded-lg bg-zinc-800/30 border border-zinc-800">
      <div className={`p-1.5 rounded-md ${colors[color] || colors.zinc}`}><Icon size={14} /></div>
      <div>
        <p className="text-[10px] uppercase tracking-wide text-zinc-500 font-medium">{label}</p>
        <p className="text-sm text-zinc-100">{value}</p>
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
  const [autoTagOpen, setAutoTagOpen] = useState(false);
  const [autoTagResults, setAutoTagResults] = useState<TagSuggestion[]>([]);
  const [selectedTags, setSelectedTags] = useState<Set<string>>(new Set());
  const [autoTagMinConfidence, setAutoTagMinConfidence] = useState(0.6);
  const [galleryTab, setGalleryTab] = useState<'galleries' | 'missing'>('galleries');
  const [scanSource, setScanSource] = useState('MetArt');
  const [scanAlias, setScanAlias] = useState('');
  const [scanResults, setScanResults] = useState<PersonScanResponse | null>(null);
  const [aliasProvider, setAliasProvider] = useState('MetArt');
  const [aliasName, setAliasName] = useState('');
  const [stashDbOpen, setStashDbOpen] = useState(false);
  const [stashDbSearchQuery, setStashDbSearchQuery] = useState('');
  const [profilePickerOpen, setProfilePickerOpen] = useState(false);

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
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['person', personId] }); setEditing(false); },
  });

  const linkMut = useMutation({
    mutationFn: (galleryId: number) => people.linkGallery(personId, galleryId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['person', personId] }); setLinkGalleryId(''); },
  });

  const autoTagMut = useMutation({
    mutationFn: () => people.autoTag(personId, autoTagMinConfidence, false),
    onSuccess: (result) => { setAutoTagResults(result.suggestions); setSelectedTags(new Set(result.suggestions.map((s) => `${s.type}-${s.id}`))); },
  });

  const applyAutoTagMut = useMutation({
    mutationFn: () => {
      const suggestions = Array.from(selectedTags).map((key) => {
        const [type, idStr] = key.split('-');
        return { type, id: parseInt(idStr) };
      });
      return people.applyAutoTagSuggestions(personId, suggestions);
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['person', personId] }); setAutoTagOpen(false); setAutoTagResults([]); setSelectedTags(new Set()); },
  });

  const { data: aliases, refetch: refetchAliases } = useQuery({
    queryKey: ['person-aliases', personId],
    queryFn: () => people.getProviderAliases(personId),
    enabled: showTools,
  });

  const { data: scans, refetch: refetchScans } = useQuery({
    queryKey: ['person-scans', personId],
    queryFn: () => people.getScans(personId),
  });

  const addAliasMut = useMutation({
    mutationFn: (data: { provider: string; alias: string }) => people.createProviderAlias(personId, data),
    onSuccess: () => { refetchAliases(); setAliasName(''); },
  });

  const deleteAliasMut = useMutation({
    mutationFn: (aliasId: number) => people.deleteProviderAlias(personId, aliasId),
    onSuccess: () => refetchAliases(),
  });

  const searchMut = useMutation({
    mutationFn: (data: { source: string; alias?: string }) => people.scanPerson(personId, data.source, data.alias),
    onSuccess: (result) => setScanResults(result),
  });

  const linkFoundMut = useMutation({
    mutationFn: (data: { provider: string; source_url: string; name: string; thumbnail_url?: string }) => people.linkFoundGallery(personId, data),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['person', personId] }); refetchScans(); },
  });

  const linkUnsureMut = useMutation({
    mutationFn: (data: { gallery_id: number; provider: string; source_url: string }) => people.linkUnsureGallery(personId, data),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['person', personId] }); refetchScans(); },
  });

  const excludeScanResultMut = useMutation({
    mutationFn: (data: { provider: string; source_url?: string; title?: string; reason?: string }) => people.excludeScanResult(personId, data),
    onSuccess: () => refetchScans(),
  });

  const autoLinkGalleriesMut = useMutation({
    mutationFn: () => people.autoLinkGalleries(personId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['person', personId] }); refetchScans(); },
  });

  const refetchExclusions = () => queryClient.invalidateQueries({ queryKey: ['person-exclusions', personId] });

  const { data: personExclusions } = useQuery({
    queryKey: ['person-exclusions', personId],
    queryFn: () => people.getExclusions(personId),
    enabled: showTools,
  });

  const removeExclusionMut = useMutation({
    mutationFn: (exclusionId: number) => people.removeExclusion(personId, exclusionId),
    onSuccess: () => { refetchExclusions(); },
  });

  const stashDbSearchMut = useMutation({
    mutationFn: (name: string) => people.searchStashDB(name),
  });

  const linkStashDbMut = useMutation({
    mutationFn: (stashId: string) => people.linkStashDB(personId, stashId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['person', personId] }); setStashDbOpen(false); },
  });

  const { data: personImages, isLoading: loadingPersonImages } = useQuery({
    queryKey: ['person-images', personId],
    queryFn: () => people.profileImages(personId),
    enabled: profilePickerOpen,
  });

  const setProfileImageMut = useMutation({
    mutationFn: (imageId: number) => people.setProfileImage(personId, imageId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person', personId] });
      queryClient.invalidateQueries({ queryKey: ['people'] });
      setProfilePickerOpen(false);
    },
  });

  const clearProfileImageMut = useMutation({
    mutationFn: () => people.clearProfileImage(personId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person', personId] });
      queryClient.invalidateQueries({ queryKey: ['people'] });
    },
  });

  const startEditing = () => { if (!person) return; setEditForm({ name: person.name }); setEditing(true); };

  if (loadingPerson) return <Spinner />;
  if (!person) return <EmptyState message="Profile not found" />;

  const photos = parsePhotos(person.photos);
  const hasCustomProfile = !!person.profile_image_path;
  const coverPhoto = person.profile_image_path ?? (photos[photoIndex] ?? photos[0]);
  const galleryList = person.galleries ?? [];
  const linkedSourceUrls = new Set(galleryList.map((g: any) => g.source_url).filter(Boolean));
  const statChips = [
    { label: 'Galleries', value: String(stats?.galleries ?? galleryList.length) },
    { label: 'Photos', value: String(stats?.photos ?? 0) },
    { label: 'Videos', value: String(stats?.videos ?? 0) },
    { label: 'Linked IDs', value: String(identifiers?.length ?? 0) },
  ];

  return (
    <div className="pb-16">
      <Link to="/people" className="inline-flex items-center gap-1.5 text-sm text-zinc-400 hover:text-zinc-200 transition-colors mb-6">
        <ArrowLeft size={16} /> Back to People
      </Link>

      {/* Profile Section */}
      <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4 md:p-5 mb-6">
        <div className="grid grid-cols-[140px_1fr] md:grid-cols-[180px_1fr] gap-4">
          <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-2">
            <div className="relative aspect-[3/4] overflow-hidden rounded-md bg-zinc-800">
              {coverPhoto ? (
                <img src={coverPhoto} alt={person.name} className="h-full w-full object-cover" />
              ) : (
                <div className="h-full w-full flex items-center justify-center"><User size={36} className="text-zinc-600" /></div>
              )}
              {hasCustomProfile && (
                <div className="absolute top-1.5 left-1.5 text-[9px] font-medium text-amber-300 bg-black/60 backdrop-blur-sm px-1.5 py-0.5 rounded">
                  Custom
                </div>
              )}
              <button onClick={() => setProfilePickerOpen(true)} title="Set profile picture from a gallery image"
                className="absolute bottom-1.5 right-1.5 inline-flex h-7 w-7 items-center justify-center rounded bg-black/60 hover:bg-blue-500/80 text-zinc-200 hover:text-white transition-colors">
                <Camera size={14} />
              </button>
            </div>
            {!hasCustomProfile && photos.length > 1 && (
              <div className="mt-2 flex items-center justify-between">
                <button onClick={() => setPhotoIndex((i) => (i - 1 + photos.length) % photos.length)}
                  className="inline-flex h-6 w-6 items-center justify-center rounded border border-zinc-700 text-zinc-300 hover:text-white">
                  <ChevronLeft size={12} />
                </button>
                <span className="text-[10px] text-zinc-400">{photoIndex + 1}/{photos.length}</span>
                <button onClick={() => setPhotoIndex((i) => (i + 1) % photos.length)}
                  className="inline-flex h-6 w-6 items-center justify-center rounded border border-zinc-700 text-zinc-300 hover:text-white">
                  <ChevronRight size={12} />
                </button>
              </div>
            )}
          </div>

          <div>
            <div className="flex flex-wrap items-start justify-between gap-2 mb-3">
              <div>
                <h1 className="text-xl md:text-2xl font-bold text-white">{person.name}</h1>
                {person.aliases && (
                  <p className="text-sm text-zinc-400 mt-0.5">
                    {typeof person.aliases === 'string' ? person.aliases.split(',').map((a) => a.trim()).join(' • ') : person.aliases}
                  </p>
                )}
              </div>
              <div className="flex items-center gap-2">
                <Button variant="ghost" size="sm" onClick={() => setShowTools(!showTools)}>
                  <Settings2 size={14} /> {showTools ? 'Hide Tools' : 'Tools'}
                </Button>
                <Button size="sm" onClick={startEditing}><Edit size={14} /> Edit</Button>
              </div>
            </div>

            <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 mb-3">
              {statChips.map((chip) => (
                <div key={chip.label} className="rounded border border-zinc-800 bg-zinc-900/50 px-3 py-2">
                  <p className="text-[10px] uppercase tracking-wide text-zinc-500">{chip.label}</p>
                  <p className="text-sm text-zinc-100 font-medium">{chip.value}</p>
                </div>
              ))}
            </div>

            <div className="flex flex-wrap gap-1.5 mb-3">
              {person.nationality && <Badge variant="info">{person.nationality}</Badge>}
              {person.ethnicity && <Badge>{person.ethnicity}</Badge>}
              {identifiers?.map((id) => <Badge key={id.id} variant="success">{id.provider}</Badge>)}
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
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
              <div className="mt-3 rounded border border-zinc-800 bg-zinc-900/50 p-3">
                <p className="text-[10px] uppercase tracking-wide text-zinc-500 mb-1">Biography</p>
                <p className="text-sm text-zinc-300 whitespace-pre-line">{person.biography}</p>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Tools Section (separated from profile) */}
      {showTools && (
        <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4 md:p-5 mb-6">
          <h3 className="text-sm font-medium text-zinc-200 mb-4">Person Management Tools</h3>
          <div className="space-y-4">
            {/* Action buttons row */}
            <div className="flex flex-wrap gap-2">
              <Button variant="secondary" size="sm" onClick={() => { setAutoTagOpen(true); setAutoTagResults([]); setSelectedTags(new Set()); }}>
                <Sparkles size={14} className="text-blue-400" /> Auto-Tag Galleries
              </Button>
              <Button variant="secondary" size="sm" onClick={() => { setStashDbOpen(true); setStashDbSearchQuery(person.name); stashDbSearchMut.mutate(person.name); }}>
                <Database size={14} className="text-purple-400" /> Search StashDB
              </Button>
              <Button size="sm" variant="secondary" onClick={() => autoLinkGalleriesMut.mutate()} disabled={autoLinkGalleriesMut.isPending}>
                {autoLinkGalleriesMut.isPending ? 'Linking...' : 'Auto-Link Galleries'}
              </Button>
            </div>

            {/* Link gallery + Provider Aliases row */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="bg-zinc-900/40 p-3 rounded-lg border border-zinc-800">
                <h4 className="text-xs font-semibold text-zinc-400 mb-2">Link Gallery</h4>
                <div className="flex gap-2">
                  <Input placeholder="Gallery ID number" value={linkGalleryId} onChange={(e) => setLinkGalleryId(e.target.value)} className="h-8 text-xs flex-1" />
                  <Button size="sm" className="h-8" onClick={() => { const gid = parseInt(linkGalleryId, 10); if (gid) linkMut.mutate(gid); }} disabled={!linkGalleryId || linkMut.isPending}>
                    <Link2 size={14} /> Link
                  </Button>
                </div>
              </div>

              <div className="bg-zinc-900/40 p-3 rounded-lg border border-zinc-800">
                <h4 className="text-xs font-semibold text-zinc-400 mb-2">Provider Aliases</h4>
                <div className="flex gap-2 items-end">
                  <select value={aliasProvider} onChange={(e) => setAliasProvider(e.target.value)}
                    className="bg-zinc-900 border border-zinc-700 rounded px-2 py-1.5 text-xs text-zinc-200 focus:outline-none cursor-pointer h-8 flex-1">
                    {PROVIDER_LIST.map((p) => <option key={p} value={p}>{p}</option>)}
                  </select>
                  <Input placeholder="Alias/ID" value={aliasName} onChange={(e) => setAliasName(e.target.value)} className="h-8 text-xs flex-1" />
                  <Button size="sm" className="h-8" onClick={() => { if (aliasProvider && aliasName) addAliasMut.mutate({ provider: aliasProvider, alias: aliasName }); }}
                    disabled={!aliasProvider || !aliasName || addAliasMut.isPending}>Add</Button>
                </div>
                {aliases && aliases.length > 0 && (
                  <div className="mt-2 space-y-1 max-h-32 overflow-y-auto">
                    {aliases.map((al) => (
                      <div key={al.id} className="flex items-center justify-between bg-zinc-900 border border-zinc-800 px-2.5 py-1.5 rounded text-xs">
                        <div className="flex items-center gap-1.5">
                          <Badge variant="info">{al.provider}</Badge>
                          <span className="text-zinc-200">{al.alias}</span>
                        </div>
                        <button onClick={() => deleteAliasMut.mutate(al.id)} className="p-0.5 text-zinc-500 hover:text-red-400 rounded">
                          <X size={12} />
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>

            {/* Exclusions */}
            {personExclusions && personExclusions.length > 0 && (
              <div className="bg-zinc-900/40 p-3 rounded-lg border border-zinc-800">
                <h4 className="text-xs font-semibold text-zinc-400 mb-2">Exclusions ({personExclusions.length})</h4>
                <div className="space-y-1 max-h-32 overflow-y-auto">
                  {personExclusions.map((ex: any) => (
                    <div key={ex.id} className="flex items-center justify-between bg-zinc-900 border border-zinc-800 px-2.5 py-1.5 rounded text-xs">
                      <span className="text-zinc-300 truncate">{ex.provider} — {ex.title ?? ex.source_url}</span>
                      <button onClick={() => removeExclusionMut.mutate(ex.id)} className="p-0.5 text-zinc-400 hover:text-red-400 rounded" title="Remove">
                        <Trash2 size={12} />
                      </button>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Gallery Tabs */}
      <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4 md:p-5">
        <div className="flex items-center border-b border-zinc-800 -mx-4 md:-mx-5 px-4 md:px-5 mb-4">
          <button onClick={() => setGalleryTab('galleries')}
            className={`flex items-center gap-1.5 px-3 py-2.5 text-xs font-medium transition-colors border-b-2 -mb-[1px] ${galleryTab === 'galleries' ? 'text-white border-blue-500' : 'text-zinc-500 border-transparent hover:text-zinc-300'}`}>
            <Layers size={14} /> Galleries <span className="text-[10px] text-zinc-500 ml-1">({galleryList.length})</span>
          </button>
          <button onClick={() => setGalleryTab('missing')}
            className={`flex items-center gap-1.5 px-3 py-2.5 text-xs font-medium transition-colors border-b-2 -mb-[1px] ${galleryTab === 'missing' ? 'text-white border-blue-500' : 'text-zinc-500 border-transparent hover:text-zinc-300'}`}>
            <Search size={14} /> Missing Galleries
          </button>
        </div>

        {galleryTab === 'galleries' && (
          galleryList.length === 0 ? <EmptyState message="No galleries linked yet." /> :
          <CoverGrid items={galleryList.map((g: any) => ({
            id: g.id, title: g.name ?? null,
            thumbnailPath: g.provider_thumbnail ? g.provider_thumbnail.replace(/\\/g, '/').split('/').pop() : g.images?.[0]?.filename,
            provider: g.provider ?? null, createdAt: g.created_at,
          }))} />
        )}

        {galleryTab === 'missing' && (
          <div className="space-y-4">
            <div className="flex gap-2 items-end">
              <select value={scanSource} onChange={(e) => { setScanSource(e.target.value); setScanResults(null); }}
                className="bg-zinc-800 border border-zinc-700 rounded px-2.5 py-1.5 text-xs text-zinc-200 focus:outline-none cursor-pointer h-9">
                {PROVIDER_LIST.map((p) => <option key={p} value={p}>{p}</option>)}
              </select>
              <Input placeholder="Alias" value={scanAlias} onChange={(e) => setScanAlias(e.target.value)} className="h-9 text-xs flex-1" />
              <Button size="sm" className="h-9" onClick={() => searchMut.mutate({ source: scanSource, alias: scanAlias || undefined })} disabled={searchMut.isPending}>
                {searchMut.isPending ? 'Searching...' : 'Search'}
              </Button>
            </div>

            {searchMut.isPending && <p className="text-xs text-zinc-400 animate-pulse text-center py-4">Searching {scanSource}...</p>}

            {scanResults && (() => {
              const activeMissingFiltered = (scanResults.missing_galleries ?? []).filter((mg) => !linkedSourceUrls.has(mg.url));
              const activeUnsureFiltered = (scanResults.unsure_galleries ?? []).filter((ug) => !linkedSourceUrls.has(ug.url));
              return (
                <div className="space-y-3">
                  <div className="flex gap-3 text-[10px] text-zinc-400">
                    <span>Found: <span className="text-zinc-200 font-medium">{scanResults.found_count}</span></span>
                    <span>Existing: <span className="text-zinc-200 font-medium">{scanResults.existing_count}</span></span>
                    <span>Missing: <span className="text-zinc-200 font-medium">{activeMissingFiltered.length}</span></span>
                    <span>Unsure: <span className="text-zinc-200 font-medium">{activeUnsureFiltered.length}</span></span>
                  </div>

                  {activeMissingFiltered.length > 0 && (
                    <div>
                      <p className="text-[10px] uppercase font-bold text-zinc-400 mb-2">Missing galleries:</p>
                      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2">
                        {activeMissingFiltered.map((mg, idx) => (
                          <div key={idx} className="group relative flex flex-col rounded-lg overflow-hidden border border-zinc-800 bg-zinc-900/80 hover:border-zinc-700 transition-all">
                            <div className="aspect-[3/4] bg-zinc-800 overflow-hidden">
                              {mg.thumbnail ? <img src={mg.thumbnail} alt={mg.title} className="w-full h-full object-cover" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />
                              : <div className="w-full h-full flex items-center justify-center text-zinc-600 text-[10px]">No thumb</div>}
                            </div>
                            <div className="p-2 flex-1 flex flex-col">
                              <p className="text-[10px] text-zinc-200 font-medium line-clamp-2 mb-1">{mg.title}</p>
                              <span className="text-[8px] text-zinc-500 mt-auto">{mg.release_date ?? ''}</span>
                            </div>
                            <div className="absolute inset-0 bg-black/60 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                              <Button size="sm" className="h-7 px-2 text-[10px]" disabled={linkFoundMut.isPending}
                                onClick={() => linkFoundMut.mutate({ provider: scanResults.provider, source_url: mg.url, name: mg.title, thumbnail_url: mg.thumbnail })}>
                                Scrape
                              </Button>
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {activeUnsureFiltered.length > 0 && (
                    <div>
                      <p className="text-[10px] uppercase font-bold text-amber-400 mb-2">Unsure matches:</p>
                      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2">
                        {activeUnsureFiltered.map((ug, idx) => (
                          <div key={idx} className="group relative flex flex-col rounded-lg overflow-hidden border border-amber-500/20 bg-zinc-900/80 hover:border-amber-500/40 transition-all">
                            <div className="aspect-[3/4] bg-zinc-800 overflow-hidden">
                              {ug.thumbnail ? <img src={ug.thumbnail} alt={ug.title} className="w-full h-full object-cover" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />
                              : <div className="w-full h-full flex items-center justify-center text-zinc-600 text-[10px]">No thumb</div>}
                            </div>
                            <div className="p-2 flex-1 flex flex-col">
                              <p className="text-[10px] text-zinc-200 font-medium line-clamp-2 mb-1">{ug.title}</p>
                              <span className="text-[8px] text-zinc-500 mt-auto">{ug.release_date ?? ''}</span>
                            </div>
                            <div className="absolute inset-0 bg-black/60 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center gap-1.5">
                              <Button size="sm" className="h-7 px-2 text-[10px]" disabled={linkUnsureMut.isPending}
                                onClick={() => { if (ug.id) linkUnsureMut.mutate({ gallery_id: ug.id, provider: scanResults.provider, source_url: ug.url }); }}>Link</Button>
                              <Button size="sm" variant="secondary" className="h-7 px-2 text-[10px]"
                                onClick={() => excludeScanResultMut.mutate({ provider: scanResults.provider, source_url: ug.url, title: ug.title })}>Exclude</Button>
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {activeMissingFiltered.length === 0 && activeUnsureFiltered.length === 0 && (
                    <p className="text-xs text-zinc-500 italic">All galleries from this provider already in database.</p>
                  )}
                </div>
              );
            })()}

            {!scanResults && !searchMut.isPending && (
              <p className="text-xs text-zinc-500 italic text-center py-4">Select a provider and alias, then search to find missing galleries.</p>
            )}

            {/* Recent Scans — compact list */}
            {scans && scans.length > 0 && (
              <div className="border-t border-zinc-800 pt-4">
                <h4 className="text-xs font-semibold text-zinc-400 mb-3">Recent Scans ({scans.length})</h4>
                <div className="space-y-3">
                  {scans.slice().reverse().map((scan) => {
                    const res = scan.results;
                    const missingGalsFiltered = (res?.missing_galleries || []).filter((mg: any) => !linkedSourceUrls.has(mg.url));
                    return (
                      <div key={scan.id} className="bg-zinc-900/80 border border-zinc-800 p-3 rounded text-xs space-y-2">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-1.5">
                            <Badge variant="info">{scan.provider || (scan as any).source}</Badge>
                            <span className="text-zinc-400">Alias: {scan.alias}</span>
                          </div>
                          <div className="flex items-center gap-2">
                            <span className="text-zinc-500">{res ? `${res.found_count ?? 0} found, ${missingGalsFiltered.length} missing` : ''}</span>
                            <Badge variant={scan.status === 'completed' ? 'success' : 'warning'}>{scan.status}</Badge>
                          </div>
                        </div>
                        {missingGalsFiltered.length > 0 && (
                          <div className="border-t border-zinc-800 pt-2 space-y-1">
                            {missingGalsFiltered.map((mg: any, idx: number) => (
                              <div key={idx} className={`flex items-center gap-3 p-2 rounded-lg border transition-all ${mg.unsure ? 'border-amber-500/20 hover:border-amber-500/40' : 'border-zinc-800 hover:border-zinc-700'} bg-zinc-900/50`}>
                                <div className="w-10 h-14 rounded overflow-hidden bg-zinc-800 shrink-0">
                                  {mg.thumbnail ? <img src={mg.thumbnail} alt="" className="w-full h-full object-cover" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />
                                  : <div className="w-full h-full flex items-center justify-center text-zinc-600 text-[8px]">—</div>}
                                </div>
                                <div className="flex-1 min-w-0">
                                  <p className="text-xs text-zinc-200 truncate">{mg.title}</p>
                                  {mg.release_date && <p className="text-[10px] text-zinc-500">{mg.release_date}</p>}
                                </div>
                                <div className="flex items-center gap-1.5 shrink-0">
                                  {mg.unsure ? (
                                    <>
                                      <Button size="sm" className="h-7 px-2 text-[10px]" disabled={linkUnsureMut.isPending}
                                        onClick={() => { const gid = mg.gallery_id || mg.id; if (gid) linkUnsureMut.mutate({ gallery_id: gid, provider: scan.provider ?? '', source_url: mg.url ?? '' }); }}>Link</Button>
                                      <Button size="sm" variant="secondary" className="h-7 px-2 text-[10px]" disabled={excludeScanResultMut.isPending}
                                        onClick={() => excludeScanResultMut.mutate({ provider: scan.provider ?? '', source_url: mg.url ?? '', title: mg.title ?? '' })}>Exclude</Button>
                                    </>
                                  ) : (
                                    <>
                                      <Button size="sm" className="h-7 px-2 text-[10px]" disabled={linkFoundMut.isPending}
                                        onClick={() => linkFoundMut.mutate({ provider: scan.provider ?? '', source_url: mg.url, name: mg.title, thumbnail_url: mg.thumbnail })}>Scrape</Button>
                                      <a href={`https://vipergirls.to/search.php?do=process&query=${encodeURIComponent(`"${scan.alias || ''}" "${mg.title || ''}"`)}&titleonly=1&forumchoice%5B%5D=235&childforums=1`}
                                        target="_blank" rel="noopener noreferrer"
                                        className="h-7 px-2 text-[10px] inline-flex items-center justify-center rounded-md bg-blue-500/20 text-blue-400 hover:bg-blue-500/30 border border-blue-500/30 transition-all"
                                        title="Search vipergirls.to"><ExternalLink size={10} /></a>
                                    </>
                                  )}
                                </div>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Edit Modal */}
      {editing && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
          <div className="w-full max-w-sm bg-zinc-900 border border-zinc-700 rounded-lg p-6">
            <h2 className="text-base font-semibold text-white mb-4">Edit Name</h2>
            <Input label="Name" value={editForm.name} onChange={(e) => setEditForm({ ...editForm, name: e.target.value })} />
            <div className="flex justify-end gap-2 mt-4">
              <Button variant="secondary" size="sm" onClick={() => setEditing(false)}>Cancel</Button>
              <Button size="sm" onClick={() => updateMut.mutate()} disabled={updateMut.isPending}>
                <Save size={14} /> Save
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Auto-Tag Modal */}
      {autoTagOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
          <div className="w-full max-w-2xl max-h-[85vh] flex flex-col rounded-lg border border-zinc-800 bg-zinc-950 shadow-2xl">
            <div className="flex items-center justify-between px-6 py-4 border-b border-zinc-800">
              <div>
                <h2 className="text-base font-semibold text-white">Auto-Tag Galleries</h2>
                <p className="text-xs text-zinc-400 mt-0.5">Scan galleries for "{person.name}"</p>
              </div>
              <button onClick={() => setAutoTagOpen(false)} className="p-1.5 text-zinc-400 hover:text-white hover:bg-zinc-800 rounded transition-colors">
                <X size={18} />
              </button>
            </div>
            <div className="px-6 py-4 border-b border-zinc-800 bg-zinc-900/30">
              <div className="flex items-end gap-3">
                <div className="w-36">
                  <Input label="Min Confidence" type="number" step="0.05" min="0" max="1" value={autoTagMinConfidence}
                    onChange={(e) => setAutoTagMinConfidence(parseFloat(e.target.value) || 0.5)} />
                </div>
                <Button size="sm" onClick={() => autoTagMut.mutate()} disabled={autoTagMut.isPending}>
                  {autoTagMut.isPending ? 'Scanning...' : 'Run Scan'}
                </Button>
              </div>
            </div>
            <div className="flex-1 overflow-y-auto px-6 py-4 space-y-3">
              {autoTagMut.isPending && (
                <div className="py-12 flex flex-col items-center gap-3">
                  <Spinner />
                  <p className="text-xs text-zinc-500 animate-pulse">Scanning galleries and videos...</p>
                </div>
              )}
              {!autoTagMut.isPending && autoTagResults.length === 0 && (
                <p className="text-xs text-zinc-500 text-center py-8">Click "Run Scan" to find matching galleries and videos.</p>
              )}
              {autoTagResults.length > 0 && (
                <>
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-zinc-400">{autoTagResults.length} suggestions</span>
                    <div className="flex gap-2">
                      <Button variant="ghost" size="sm" onClick={() => setSelectedTags(new Set())}>Clear</Button>
                      <Button variant="ghost" size="sm" onClick={() => setSelectedTags(new Set(autoTagResults.map((s) => `${s.type}-${s.id}`)))}>All</Button>
                    </div>
                  </div>
                  <div className="space-y-1.5">
                    {autoTagResults.map((s) => {
                      const key = `${s.type}-${s.id}`;
                      const isSelected = selectedTags.has(key);
                      return (
                        <button key={key} onClick={() => { setSelectedTags((prev) => { const n = new Set(prev); if (n.has(key)) n.delete(key); else n.add(key); return n; }); }}
                          className={`w-full flex items-center gap-3 p-2.5 rounded-lg border transition-colors text-left text-xs ${
                            isSelected ? 'border-blue-500/50 bg-blue-500/10' : 'border-zinc-800 hover:border-zinc-700'
                          }`}>
                          <input type="checkbox" checked={isSelected} onChange={() => {}} className="rounded border-zinc-600 bg-zinc-800 text-blue-500" />
                          <Badge variant={s.type === 'gallery' ? 'info' : 'warning'}>{s.type}</Badge>
                          <span className="text-zinc-200 flex-1 truncate">{s.name}</span>
                          <span className="text-zinc-500">{(s.confidence * 100).toFixed(0)}%</span>
                        </button>
                      );
                    })}
                  </div>
                </>
              )}
            </div>
            {autoTagResults.length > 0 && (
              <div className="flex items-center justify-between px-6 py-4 border-t border-zinc-800 bg-zinc-900/30">
                <span className="text-xs text-zinc-400">{selectedTags.size} selected</span>
                <div className="flex gap-2">
                  <Button variant="secondary" size="sm" onClick={() => setAutoTagOpen(false)}>Cancel</Button>
                  <Button size="sm" onClick={() => applyAutoTagMut.mutate()} disabled={selectedTags.size === 0 || applyAutoTagMut.isPending}>
                    {applyAutoTagMut.isPending ? 'Applying...' : 'Apply Selected'}
                  </Button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Profile Picture Picker Modal */}
      {profilePickerOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
          <div className="w-full max-w-3xl max-h-[85vh] flex flex-col rounded-lg border border-zinc-800 bg-zinc-950 shadow-2xl">
            <div className="flex items-center justify-between px-6 py-4 border-b border-zinc-800">
              <div>
                <h2 className="text-base font-semibold text-white">Choose Profile Picture</h2>
                <p className="text-xs text-zinc-400 mt-0.5">Pick a gallery image of {person.name} as their profile picture</p>
              </div>
              <button onClick={() => setProfilePickerOpen(false)} className="p-1.5 text-zinc-400 hover:text-white hover:bg-zinc-800 rounded transition-colors">
                <X size={18} />
              </button>
            </div>
            <div className="flex-1 overflow-y-auto px-6 py-4">
              {loadingPersonImages ? (
                <div className="py-12 flex flex-col items-center gap-3"><Spinner /><p className="text-xs text-zinc-500 animate-pulse">Loading images...</p></div>
              ) : !personImages || personImages.data.length === 0 ? (
                <p className="text-xs text-zinc-500 text-center py-12">No images linked to this person's galleries yet.</p>
              ) : (
                <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 gap-2">
                  {personImages.data.map((img) => {
                    const isCurrent = img.id === person.profile_image_id;
                    const src = img.thumbnail_path && img.thumbnail_path.startsWith('/') ? img.thumbnail_path : undefined;
                    return (
                      <button key={img.id} disabled={setProfileImageMut.isPending}
                        onClick={() => setProfileImageMut.mutate(img.id)}
                        className={`relative aspect-[3/4] rounded-lg overflow-hidden border-2 transition-all bg-zinc-800 group ${
                          isCurrent ? 'border-amber-400' : 'border-transparent hover:border-zinc-600'
                        }`} title={img.filename}>
                        {src
                          ? <img src={src} alt={img.filename} className="w-full h-full object-cover" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />
                          : <div className="w-full h-full flex items-center justify-center text-zinc-600 text-[9px]">{img.filename}</div>}
                        <div className={`absolute inset-0 flex items-center justify-center text-[10px] font-medium transition-all ${
                          isCurrent ? 'bg-amber-500/20 text-amber-200' : 'bg-black/40 text-white opacity-0 group-hover:opacity-100'
                        }`}>
                          {isCurrent ? <><Check size={14} /> Current</> : 'Use as picture'}
                        </div>
                      </button>
                    );
                  })}
                </div>
              )}
            </div>
            <div className="flex items-center justify-between px-6 py-4 border-t border-zinc-800 bg-zinc-900/30">
              {person.profile_image_id ? (
                <Button variant="secondary" size="sm" disabled={clearProfileImageMut.isPending} onClick={() => clearProfileImageMut.mutate()}>
                  Remove Profile Picture
                </Button>
              ) : <span />}
              <Button size="sm" onClick={() => setProfilePickerOpen(false)}>Done</Button>
            </div>
          </div>
        </div>
      )}

      {/* StashDB Modal */}
      {stashDbOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
          <div className="w-full max-w-2xl max-h-[85vh] flex flex-col rounded-lg border border-zinc-800 bg-zinc-950 shadow-2xl">
            <div className="flex items-center justify-between px-6 py-4 border-b border-zinc-800">
              <div>
                <h2 className="text-base font-semibold text-white flex items-center gap-2">
                  <Database size={16} className="text-purple-400" /> Search StashDB
                </h2>
                <p className="text-xs text-zinc-400 mt-0.5">Find and link performer data</p>
              </div>
              <button onClick={() => setStashDbOpen(false)} className="p-1.5 text-zinc-400 hover:text-white hover:bg-zinc-800 rounded transition-colors">
                <X size={18} />
              </button>
            </div>
            <div className="px-6 py-4 border-b border-zinc-800 bg-zinc-900/30">
              <div className="flex gap-3">
                <div className="flex-1">
                  <Input label="Performer Name" value={stashDbSearchQuery} onChange={(e) => setStashDbSearchQuery(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && stashDbSearchMut.mutate(stashDbSearchQuery)} />
                </div>
                <Button size="sm" className="mt-5" onClick={() => stashDbSearchMut.mutate(stashDbSearchQuery)} disabled={stashDbSearchMut.isPending}>
                  {stashDbSearchMut.isPending ? 'Searching...' : 'Search'}
                </Button>
              </div>
            </div>
            <div className="flex-1 overflow-y-auto px-6 py-4 space-y-3">
              {stashDbSearchMut.isPending && <Spinner />}
              {!stashDbSearchMut.isPending && stashDbSearchMut.data && (stashDbSearchMut.data as any).data?.length > 0 && (
                <div className="grid grid-cols-1 gap-3">
                  {(stashDbSearchMut.data as any).data.map((perf: any) => (
                    <div key={perf.id} className="flex items-start gap-3 p-3 rounded-lg border border-zinc-800 bg-zinc-900/50 hover:border-zinc-700 transition-colors">
                      <div className="h-16 w-12 rounded overflow-hidden bg-zinc-800 shrink-0 border border-zinc-800">
                        {perf.images && perf.images.length > 0 ? <img src={perf.images[0].url} alt={perf.name} className="h-full w-full object-cover" />
                        : <div className="h-full w-full flex items-center justify-center text-zinc-700"><User size={16} /></div>}
                      </div>
                      <div className="min-w-0 flex-1">
                        <h4 className="text-sm font-medium text-white truncate">{perf.name}</h4>
                        {perf.disambiguation && <p className="text-[10px] text-purple-400 italic">{perf.disambiguation}</p>}
                        {perf.aliases?.length > 0 && <p className="text-[10px] text-zinc-500 truncate">Aliases: {perf.aliases.join(', ')}</p>}
                      </div>
                      <Button size="sm" disabled={linkStashDbMut.isPending} onClick={() => linkStashDbMut.mutate(perf.id)} className="shrink-0 self-center">
                        {linkStashDbMut.isPending ? '...' : 'Link'}
                      </Button>
                    </div>
                  ))}
                </div>
              )}
              {!stashDbSearchMut.isPending && stashDbSearchMut.data && (stashDbSearchMut.data as any).data?.length === 0 && (
                <p className="text-xs text-zinc-500 text-center py-4">No performers found.</p>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
