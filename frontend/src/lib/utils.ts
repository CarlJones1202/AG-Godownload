export function cn(...classes: (string | false | null | undefined)[]): string {
  return classes.filter(Boolean).join(' ');
}

export function formatDate(dateStr: string | undefined): string {
  if (!dateStr) return '-';
  const d = new Date(dateStr);
  if (isNaN(d.getTime())) return '-';
  return d.toLocaleDateString('en-US', {
    year: 'numeric', month: 'short', day: 'numeric',
  });
}

export function formatDateTime(dateStr: string | undefined): string {
  if (!dateStr) return '-';
  const d = new Date(dateStr);
  if (isNaN(d.getTime())) return '-';
  return d.toLocaleString('en-US', {
    year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  });
}

export function parseColors(colorsJson: string | undefined): string[] {
  if (!colorsJson) return [];
  try {
    return JSON.parse(colorsJson) as string[];
  } catch {
    return [];
  }
}

export function thumbnailUrl(filename: string): string {
  // If the filename already looks like a full path with a source dir (e.g. "SourceName/hash.jpg"),
  // pass it through to the thumbnails handler which will inject /thumbnails/ in the right place.
  // Otherwise just pass the bare filename.
  return `/thumbnails/${filename}`;
}

export function imageUrl(filename: string): string {
  return `/images/${filename}`;
}

export function videoUrl(filename: string): string {
  return `/images/${filename}`;
}

export function trickplayVttUrl(filename: string): string {
  const dot = filename.lastIndexOf('.');
  const base = dot >= 0 ? filename.substring(0, dot) : filename;
  return `/images/${base}_sprites.vtt`;
}

export function formatDuration(seconds: number | undefined): string {
  if (!seconds || seconds <= 0) return '0:00';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  if (h > 0) {
    return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  }
  return `${m}:${String(s).padStart(2, '0')}`;
}
