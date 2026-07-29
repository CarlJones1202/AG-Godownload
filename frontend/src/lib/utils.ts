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

export function guessTitleFromUrl(url: string): string {
  try {
    const parsed = new URL(url);
    const path = parsed.pathname.replace(/\/$/, '');
    const segments = path.split('/').filter(Boolean);
    if (segments.length === 0) return '';

    let raw = decodeURIComponent(segments[segments.length - 1]);

    // Strip common file extensions
    raw = raw.replace(/\.(html?|php|asp|aspx|jsp|shtml|cfm|jpe?g|png|gif|bmp|webp|mp4|avi|mkv)$/i, '');

    // Strip query/hash
    raw = raw.split('?')[0].split('#')[0];

    // Replace separators with spaces
    let title = raw.replace(/[-_+]+/g, ' ').replace(/\s+/g, ' ').trim();

    // If too short or purely numeric, try the parent segment
    if (title.length < 3 || /^\d+$/.test(title)) {
      if (segments.length >= 2) {
        let parent = decodeURIComponent(segments[segments.length - 2]);
        title = parent.replace(/[-_+]+/g, ' ').replace(/\s+/g, ' ').trim();
      }
    }

    if (!title) return '';

    const words = title.split(' ').filter(Boolean);

    // 1. Strip leading numeric (thread ID)
    while (words.length > 0 && /^\d+$/.test(words[0])) words.shift();
    if (words.length === 0) return '';

    // 2. Strip leading date patterns: YYYY MM DD, YYYY Mon DD, etc.
    while (words.length >= 3 && /^\d{4}$/.test(words[0]) && /^\d{1,2}$/.test(words[1]) && /^\d{1,2}$/.test(words[2])) {
      words.splice(0, 3);
    }
    while (words.length >= 3 && /^\d{4}$/.test(words[0]) && /^[a-z]{3,9}$/i.test(words[1]) && /^\d{1,2}$/.test(words[2])) {
      words.splice(0, 3);
    }
    if (words.length >= 2 && /^\d{4}$/.test(words[0]) && /^[a-z]{3,9}$/i.test(words[1])) {
      words.splice(0, 2);
    }
    if (words.length >= 3 && /^\d{1,2}$/.test(words[0]) && /^[a-z]{3,9}$/i.test(words[1]) && /^\d{4}$/.test(words[2])) {
      words.splice(0, 3);
    }
    while (words.length > 0 && /^\d{4}$/.test(words[0])) words.shift();
    if (words.length === 0) return '';

    // 3. Strip trailing metadata: xNN, NNpx, NNNNxNNNN, dates, pictures etc.
    while (words.length > 0) {
      const last = words[words.length - 1].toLowerCase();
      // x120, x80, etc.
      if (/^x\d+$/.test(last)) { words.pop(); continue; }
      // 1920px, 3000px, etc.
      if (/^\d+px$/.test(last)) { words.pop(); continue; }
      // 3840x5760 or 3840x5760px
      if (/^\d+[x×]\d+px?$/.test(last)) { words.pop(); continue; }
      // NNpx? wait this is covered above
      // All-digit number >= 100 (image count)
      if (/^\d+$/.test(last) && parseInt(last) >= 100) { words.pop(); continue; }
      // Parenthesized single token
      if (last.startsWith('(') && last.endsWith(')')) { words.pop(); continue; }
      // Parenthesized 3-word group
      if (words.length >= 3 && words[words.length - 3].startsWith('(') && words[words.length - 1].endsWith(')')) {
        words.splice(-3); continue;
      }
      // Parenthesized 2-word group
      if (words.length >= 2 && words[words.length - 2].startsWith('(') && words[words.length - 1].endsWith(')')) {
        words.splice(-2); continue;
      }
      // number pictures/photos/pix pair
      if (words.length >= 2 && /^\d+$/.test(words[words.length - 2]) && /^(pictures|photos|pix|pics|images|files|jpg|jpeg)$/i.test(words[words.length - 1])) {
        words.splice(-2); continue;
      }
      // YYYY MM DD date suffix
      if (words.length >= 3 && /^\d{4}$/.test(words[words.length - 3]) && /^\d{1,2}$/.test(words[words.length - 2]) && /^\d{1,2}$/.test(words[words.length - 1])) {
        words.splice(-3); continue;
      }
      // Mon DD YYYY or DD Mon YYYY
      if (words.length >= 3 && /^[a-z]{3,9}$/i.test(words[words.length - 3]) && /^\d{1,2}$/.test(words[words.length - 2]) && /^\d{4}$/.test(words[words.length - 1])) {
        words.splice(-3); continue;
      }
      // Mon DD
      if (words.length >= 2 && /^[a-z]{3,9}$/i.test(words[words.length - 2]) && /^\d{1,2}$/.test(words[words.length - 1])) {
        words.splice(-2); continue;
      }
      // Single 4-digit year
      if (words.length >= 1 && /^\d{4}$/.test(last)) { words.pop(); continue; }
      // Extra tags
      if (['pre-release', 'prerelease', 'pre', 'hi-res', 'hires', 'hi', 'res', 'highlight', 'release'].indexOf(last) >= 0) { words.pop(); continue; }
      break;
    }
    if (words.length === 0) return '';

    // 4. Strip leading site prefixes (playboy com, metart com, etc.)
    const sitePrefixes = [
      ['playboyplus', 'com'], ['playboy', 'com'], ['metartx', 'com'], ['metart', 'com'],
      ['sexart', 'com'], ['vivthomas', 'com'], ['wowgirls', 'com'], ['rylskyart', 'com'],
      ['eternaldesire', 'com'], ['mplstudios', 'com'], ['lifeerotic', 'com'],
    ];
    for (const prefix of sitePrefixes) {
      if (words.length >= prefix.length && prefix.every((p, i) => p === words[i].toLowerCase())) {
        words.splice(0, prefix.length);
        break;
      }
    }
    if (words.length === 0) return '';

    // 5. Strip model name if followed by lowercase connector "in", "a", "an", "the"
    // Simple heuristic: if 2nd or 3rd word is a lowercase connector, strip up to and including it
    for (let i = 1; i < Math.min(words.length, 3); i++) {
      const w = words[i];
      if ((w === 'in' || w === 'a' || w === 'an' || w === 'the') && w === w.toLowerCase()) {
        words.splice(0, i + 1);
        break;
      }
    }
    if (words.length === 0) return '';

    // Strip trailing punctuation
    for (let i = 0; i < words.length; i++) {
      words[i] = words[i].replace(/[!?,;.:-]+$/, '');
    }

    // Remove purely non-alphanumeric words
    const cleaned = words.filter(w => /[a-zA-Z0-9]/.test(w));
    if (cleaned.length === 0) return '';

    // Title-case with lowercase exceptions for articles/prepositions
    const lowerExceptions = new Set(['a', 'an', 'the', 'in', 'of', 'for', 'and', 'or', 'to', 'on', 'at', 'by', 'with', 'from', 'into', 'onto', 'upon']);
    return cleaned.map((w, i) => {
      const lower = w.toLowerCase();
      if (i > 0 && lowerExceptions.has(lower)) return lower;
      return w.charAt(0).toUpperCase() + w.slice(1).toLowerCase();
    }).join(' ');
  } catch {
    return '';
  }
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
