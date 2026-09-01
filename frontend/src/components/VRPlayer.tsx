import { useEffect, useRef, useState } from 'react';
import { X, Loader2 } from 'lucide-react';
import type { Image } from '@/types';
import { videoUrl } from '@/lib/utils';

interface VRPlayerProps {
  video: Image;
  onClose: () => void;
}

declare global {
  interface Window {
    AFRAME?: unknown;
  }
}

export function VRPlayer({ video, onClose }: VRPlayerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [aframeReady, setAframeReady] = useState(() => Boolean(window.AFRAME));
  const [videoReady, setVideoReady] = useState(false);

  const is360 = video.vr_mode === '360';
  const src = videoUrl(video.filename);

  useEffect(() => {
    document.body.style.overflow = 'hidden';
    return () => { document.body.style.overflow = ''; };
  }, []);

  // Load A-Frame library via CDN if not already present.
  useEffect(() => {
    if (window.AFRAME) return;
    const script = document.createElement('script');
    script.src = 'https://aframe.io/releases/1.7.0/aframe.min.js';
    script.async = true;
    script.onload = () => setAframeReady(true);
    document.head.appendChild(script);
    return () => { document.head.removeChild(script); };
  }, []);

  // Build the A-Frame scene once A-Frame is ready.
  useEffect(() => {
    if (!aframeReady || !containerRef.current) return;
    const container = containerRef.current;

    // Safety fallback so the loading overlay always dismisses.
    const fallback = window.setTimeout(() => setVideoReady(true), 6000);

    const scene = document.createElement('a-scene');
    scene.setAttribute('embedded', '');
    scene.setAttribute('vr-mode-ui', 'enabled: false');

    const camera = document.createElement('a-camera');
    camera.setAttribute('position', '0 1.6 0');

    const videosphere = document.createElement('a-videosphere');
    videosphere.setAttribute('src', src);
    videosphere.setAttribute('loop', 'true');
    videosphere.setAttribute('autoplay', 'true');
    videosphere.setAttribute('material', 'side: back;');
    if (!is360) {
      // Render only the front 180-degree half-sphere.
      videosphere.setAttribute(
        'geometry',
        'radius: 500; segmentsWidth: 64; segmentsHeight: 64; thetaStart: -90; thetaLength: 180;'
      );
    }

    camera.appendChild(videosphere);
    scene.appendChild(camera);
    container.appendChild(scene);

    // Track the underlying <video> element to dismiss the loading state.
    const videoEl = container.querySelector('video');
    const handleReady = () => setVideoReady(true);
    videoEl?.addEventListener('play', handleReady);
    videoEl?.addEventListener('loadeddata', handleReady);
    videoEl?.addEventListener('error', handleReady);

    return () => {
      window.clearTimeout(fallback);
      try { videoEl?.pause(); } catch { /* ignore */ }
      videoEl?.removeEventListener('play', handleReady);
      videoEl?.removeEventListener('loadeddata', handleReady);
      videoEl?.removeEventListener('error', handleReady);
      container.innerHTML = '';
    };
  }, [aframeReady, src, is360]);

  return (
    <div className="fixed inset-0 z-50 bg-black select-none">
      {/* A-Frame scene fills the screen; look-controls lets the user drag to look around */}
      <div ref={containerRef} className="absolute inset-0" />
      <button
        onClick={onClose}
        className="absolute top-4 right-4 z-30 p-1.5 text-white/60 hover:text-white transition-colors bg-black/50 rounded-lg"
        aria-label="Close"
      >
        <X size={22} />
      </button>
      <div className="absolute bottom-4 left-1/2 -translate-x-1/2 z-30 flex items-center gap-2 text-white/80 bg-black/50 rounded-lg px-3 py-2 pointer-events-none">
        <span className="text-sm font-medium">{is360 ? 'VR 360' : 'VR 180'}</span>
        <span className="text-xs text-white/50">Drag to look around</span>
      </div>
      {(!aframeReady || !videoReady) && (
        <div className="absolute inset-0 z-20 flex flex-col items-center justify-center gap-3 text-white/70">
          <Loader2 className="animate-spin" size={32} />
          <span className="text-sm">{aframeReady ? 'Loading video...' : 'Loading VR player...'}</span>
        </div>
      )}
    </div>
  );
}
