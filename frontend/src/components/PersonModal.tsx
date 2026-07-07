import { useState } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import { people } from '@/lib/api';
import type { PersonScanResponse } from '@/types';
import { Badge, Button, Input, Spinner } from '@/components/UI';
import { X, Search, Info, User, Calendar, MapPin, Maximize2, Weight, Palette, Fingerprint, Check } from 'lucide-react';

const PROVIDER_LIST = [
  "MetArt", "MetartX", "Playboy", "PlayboyPlus", "Vixen",
  "SexArt", "LifeErotic", "EternalDesire", "MPLStudios",
  "VivThomas", "WowGirls", "RylskyArt",
];

function parsePhotos(photos?: string): string[] {
  if (!photos) return [];
  try { return JSON.parse(photos); } catch { return []; }
}

export function PersonModal({ personId, onClose }: { personId: number; onClose: () => void }) {
  const [tab, setTab] = useState<'info' | 'missing'>('missing');
  const [scanSource, setScanSource] = useState('MetArt');
  const [scanAlias, setScanAlias] = useState('');
  const [scanResults, setScanResults] = useState<PersonScanResponse | null>(null);

  const { data: person, isLoading: loadingPerson } = useQuery({
    queryKey: ['person', personId],
    queryFn: () => people.get(personId),
  });

  const { data: identifiers } = useQuery({
    queryKey: ['person-identifiers', personId],
    queryFn: () => people.identifiers(personId),
  });

  const { data: aliases } = useQuery({
    queryKey: ['person-aliases', personId],
    queryFn: () => people.getProviderAliases(personId),
  });

  const { refetch: refetchScans } = useQuery({
    queryKey: ['person-scans', personId],
    queryFn: () => people.getScans(personId),
    enabled: false,
  });

  const searchMut = useMutation({
    mutationFn: (data: { source: string; alias?: string }) =>
      people.scanPerson(personId, data.source, data.alias),
    onSuccess: (result) => {
      setScanResults(result);
    },
  });

  const linkFoundMut = useMutation({
    mutationFn: (data: { provider: string; source_url: string; name: string; thumbnail_url?: string }) =>
      people.linkFoundGallery(personId, data),
    onSuccess: () => {
      refetchScans();
    },
  });

  const linkUnsureMut = useMutation({
    mutationFn: (data: { gallery_id: number; provider: string; source_url: string }) =>
      people.linkUnsureGallery(personId, data),
    onSuccess: () => {
      refetchScans();
    },
  });

  const photos = parsePhotos(person?.photos);
  const galleryList = person?.galleries ?? [];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70" onClick={onClose}>
      <div className="bg-zinc-900 border border-zinc-700 rounded-xl w-full max-w-3xl max-h-[85vh] mx-4 flex flex-col overflow-hidden" onClick={(e) => e.stopPropagation()}>
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-zinc-800">
          <div className="flex items-center gap-3">
            <div className="h-8 w-8 rounded-full overflow-hidden bg-zinc-800 flex-shrink-0">
              {photos[0] ? (
                <img src={photos[0]} alt="" className="h-full w-full object-cover" />
              ) : (
                <div className="h-full w-full flex items-center justify-center text-zinc-500">
                  <User size={16} />
                </div>
              )}
            </div>
            <div>
              <h2 className="text-sm font-semibold text-white">{person?.name ?? 'Loading...'}</h2>
              <p className="text-[10px] text-zinc-500">{galleryList.length} galleries</p>
            </div>
          </div>
          <button onClick={onClose} className="p-1.5 rounded-lg text-zinc-500 hover:text-white hover:bg-zinc-800 transition-all">
            <X size={18} />
          </button>
        </div>

        {/* Tabs */}
        <div className="flex border-b border-zinc-800">
          <button
            onClick={() => setTab('info')}
            className={`flex items-center gap-1.5 px-4 py-2.5 text-xs font-medium transition-colors ${tab === 'info' ? 'text-white border-b-2 border-blue-500' : 'text-zinc-500 hover:text-zinc-300'}`}
          >
            <Info size={14} /> Info
          </button>
          <button
            onClick={() => setTab('missing')}
            className={`flex items-center gap-1.5 px-4 py-2.5 text-xs font-medium transition-colors ${tab === 'missing' ? 'text-white border-b-2 border-blue-500' : 'text-zinc-500 hover:text-zinc-300'}`}
          >
            <Search size={14} /> Missing Galleries
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-4">
          {loadingPerson ? (
            <div className="flex items-center justify-center py-12"><Spinner /></div>
          ) : tab === 'info' ? (
            <div className="space-y-4">
              {/* Aliases */}
              {aliases && aliases.length > 0 && (
                <div>
                  <p className="text-[10px] uppercase font-bold text-zinc-500 mb-2">Provider Aliases</p>
                  <div className="flex flex-wrap gap-2">
                    {aliases.map((a) => (
                      <Badge key={a.id} variant="info" className="text-[10px]">{a.provider}: {a.alias}</Badge>
                    ))}
                  </div>
                </div>
              )}
              {/* Identifiers */}
              {identifiers && identifiers.length > 0 && (
                <div>
                  <p className="text-[10px] uppercase font-bold text-zinc-500 mb-2">Linked Identifiers</p>
                  <div className="flex flex-wrap gap-2">
                    {identifiers.map((id) => (
                      <Badge key={id.id} variant="success" className="text-[10px]">{id.provider}: {id.external_id}</Badge>
                    ))}
                  </div>
                </div>
              )}
              {/* Bio Stats */}
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
                {[
                  { icon: Calendar, label: 'Born', value: person?.birth_date },
                  { icon: Maximize2, label: 'Height', value: person?.height },
                  { icon: Weight, label: 'Weight', value: person?.weight },
                  { icon: Check, label: 'Measurements', value: person?.measurements },
                  { icon: MapPin, label: 'Nationality', value: person?.nationality },
                  { icon: Fingerprint, label: 'Ethnicity', value: person?.ethnicity },
                  { icon: Palette, label: 'Hair', value: person?.hair_color },
                  { icon: Palette, label: 'Eyes', value: person?.eye_color },
                ].filter(s => s.value).map((s, i) => (
                  <div key={i} className="rounded-lg bg-zinc-800/60 border border-zinc-800 p-2.5">
                    <div className="flex items-center gap-1.5 text-zinc-500 mb-0.5">
                      <s.icon size={12} />
                      <span className="text-[9px] uppercase tracking-wide">{s.label}</span>
                    </div>
                    <p className="text-xs text-zinc-200">{s.value}</p>
                  </div>
                ))}
              </div>
              {/* Biography */}
              {person?.biography && (
                <div>
                  <p className="text-[10px] uppercase font-bold text-zinc-500 mb-2">Biography</p>
                  <p className="text-xs text-zinc-300 leading-relaxed">{person.biography}</p>
                </div>
              )}
            </div>
          ) : (
            <div className="space-y-4">
              {/* Scanner Controls */}
              <div className="flex gap-2 items-end">
                <div className="flex-1">
                  <label className="text-[10px] text-zinc-500 uppercase tracking-wider block mb-1">Provider</label>
                  <select
                    value={scanSource}
                    onChange={(e) => { setScanSource(e.target.value); setScanResults(null); }}
                    className="bg-zinc-800 border border-zinc-700 rounded px-2.5 py-1.5 text-xs text-zinc-200 focus:outline-none transition-all cursor-pointer w-full h-9"
                  >
                    {PROVIDER_LIST.map((p) => (
                      <option key={p} value={p}>{p}</option>
                    ))}
                  </select>
                </div>
                <div className="flex-1">
                  <label className="text-[10px] text-zinc-500 uppercase tracking-wider block mb-1">Search Alias</label>
                  <Input
                    placeholder={person?.name ?? ''}
                    value={scanAlias}
                    onChange={(e) => setScanAlias(e.target.value)}
                    className="h-9"
                  />
                </div>
                <Button
                  size="sm"
                  className="h-9"
                  onClick={() => searchMut.mutate({ source: scanSource, alias: scanAlias || undefined })}
                  disabled={searchMut.isPending}
                >
                  {searchMut.isPending ? 'Searching...' : 'Search'}
                </Button>
              </div>

              {/* Loading */}
              {searchMut.isPending && (
                <div className="py-8 flex items-center justify-center">
                  <span className="text-xs text-zinc-400 animate-pulse">Searching {scanSource}...</span>
                </div>
              )}

              {/* Results */}
              {scanResults && (
                <div className="space-y-3">
                  <div className="grid grid-cols-4 gap-2 text-[10px] text-zinc-400">
                    <div>Found: <span className="text-zinc-200 font-medium">{scanResults.found_count}</span></div>
                    <div>Existing: <span className="text-zinc-200 font-medium">{scanResults.existing_count}</span></div>
                    <div>Unsure: <span className="text-zinc-200 font-medium">{scanResults.unsure_count}</span></div>
                    <div>Missing: <span className="text-zinc-200 font-medium">{scanResults.missing_count}</span></div>
                  </div>

                  {/* Missing Galleries Grid */}
                  {scanResults.missing_galleries?.length > 0 && (
                    <div>
                      <p className="text-[10px] uppercase font-bold text-zinc-400 mb-2">Missing galleries:</p>
                      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2">
                        {scanResults.missing_galleries.map((mg, idx) => (
                          <div key={idx} className="group relative flex flex-col rounded-lg overflow-hidden border border-zinc-800 bg-zinc-900/80 hover:border-zinc-700 transition-all">
                            <div className="aspect-[3/4] bg-zinc-800 overflow-hidden">
                              {mg.thumbnail ? (
                                <img src={mg.thumbnail} alt={mg.title} className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />
                              ) : (
                                <div className="w-full h-full flex items-center justify-center text-zinc-600 text-[10px]">No thumb</div>
                              )}
                            </div>
                            <div className="p-2 flex-1 flex flex-col">
                              <p className="text-[10px] text-zinc-200 font-medium leading-tight line-clamp-2 mb-1">{mg.title}</p>
                              <div className="mt-auto flex items-center justify-between">
                                <span className="text-[8px] text-zinc-500">{mg.release_date ?? ''}</span>
                              </div>
                            </div>
                            <div className="absolute inset-0 bg-black/60 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center gap-1.5">
                              <Button
                                size="sm"
                                className="h-7 px-2 text-[10px]"
                                disabled={linkFoundMut.isPending}
                                onClick={() => linkFoundMut.mutate({ provider: scanResults.provider, source_url: mg.url, name: mg.title, thumbnail_url: mg.thumbnail })}
                              >
                                Scrape
                              </Button>
                              <a
                                href={`https://www.vipergirls.to/search.php?query="${encodeURIComponent(mg.title)}"&titleonly=1&search_in=topics&forumchoice%5B%5D=235&childforums=1`}
                                target="_blank" rel="noopener noreferrer"
                                className="h-7 px-2 inline-flex items-center justify-center text-[10px] font-medium rounded-lg bg-blue-500/20 text-blue-400 hover:bg-blue-500/30 border border-blue-500/30 transition-all"
                              >
                                VG
                              </a>
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Unsure Galleries */}
                  {scanResults.unsure_galleries?.length > 0 && (
                    <div>
                      <p className="text-[10px] uppercase font-bold text-amber-400 mb-2">Unsure matches (already in DB, different provider):</p>
                      <div className="space-y-1.5 max-h-48 overflow-y-auto">
                        {scanResults.unsure_galleries.map((ug, idx) => (
                          <div key={idx} className="flex items-center justify-between bg-zinc-950 p-2 rounded border border-zinc-800/50">
                            <div className="min-w-0 flex-1 pr-2">
                              <p className="truncate text-zinc-300 font-medium text-xs" title={ug.title}>{ug.title}</p>
                              {ug.release_date && <p className="text-[9px] text-zinc-500">{ug.release_date}</p>}
                            </div>
                            <div className="flex gap-1.5 shrink-0">
                              <Button size="sm" className="h-7 px-2 text-[10px]" disabled={linkUnsureMut.isPending} onClick={() => {
                                if (ug.id) linkUnsureMut.mutate({ gallery_id: ug.id, provider: scanResults.provider, source_url: ug.url });
                              }}>
                                Link
                              </Button>
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {scanResults.missing_galleries?.length === 0 && scanResults.unsure_galleries?.length === 0 && (
                    <p className="text-xs text-zinc-500 italic py-4 text-center">All galleries accounted for.</p>
                  )}
                </div>
              )}

              {!scanResults && !searchMut.isPending && (
                <div className="py-8 text-center">
                  <p className="text-xs text-zinc-500 italic">Select a provider and search to find galleries by this person.</p>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}