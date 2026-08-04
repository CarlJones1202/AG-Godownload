import { useEffect, useRef, useCallback, useState } from 'react';
import { X, Play, Pause, Volume2, VolumeX, Maximize, Minimize, RotateCw } from 'lucide-react';
import type { Image } from '@/types';
import { videoUrl, trickplayVttUrl, formatDuration } from '@/lib/utils';

interface VideoPlayerProps {
  video: Image;
  onClose: () => void;
}

const SPEEDS = [0.25, 0.5, 1, 1.5, 2];

export function VideoPlayer({ video, onClose }: VideoPlayerProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const progressRef = useRef<HTMLDivElement>(null);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout>>();

  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(video.duration_seconds ?? 0);
  const [loop, setLoop] = useState(false);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const [speed, setSpeed] = useState(1);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [controlsVisible, setControlsVisible] = useState(true);
  const [showSpeedMenu, setShowSpeedMenu] = useState(false);
  const [scrubbing, setScrubbing] = useState(false);

  useEffect(() => {
    document.body.style.overflow = 'hidden';
    return () => { document.body.style.overflow = ''; };
  }, []);

  const showControls = useCallback(() => {
    setControlsVisible(true);
    if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
    if (isPlaying) {
      hideTimerRef.current = setTimeout(() => setControlsVisible(false), 3000);
    }
  }, [isPlaying]);

  const togglePlay = useCallback(() => {
    const el = videoRef.current;
    if (!el) return;
    if (el.paused) { el.play(); } else { el.pause(); }
  }, []);

  const seek = useCallback((delta: number) => {
    const el = videoRef.current;
    if (!el) return;
    el.currentTime = Math.max(0, Math.min(el.duration, el.currentTime + delta));
  }, []);

  const handleProgressClick = useCallback((e: React.MouseEvent) => {
    const el = videoRef.current;
    const bar = progressRef.current;
    if (!el || !bar || !duration) return;
    const rect = bar.getBoundingClientRect();
    const frac = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
    el.currentTime = frac * duration;
  }, [duration]);

  const handleProgressDrag = useCallback((e: React.MouseEvent) => {
    if (!scrubbing) return;
    handleProgressClick(e);
  }, [scrubbing, handleProgressClick]);

  const toggleMute = useCallback(() => {
    const el = videoRef.current;
    if (!el) return;
    el.muted = !el.muted;
    setMuted(el.muted);
  }, []);

  const handleVolumeChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const val = parseFloat(e.target.value);
    const el = videoRef.current;
    if (!el) return;
    el.volume = val;
    el.muted = val === 0;
    setVolume(val);
    setMuted(val === 0);
  }, []);

  const toggleFullscreen = useCallback(async () => {
    if (!document.fullscreenElement) {
      await document.documentElement.requestFullscreen();
      setIsFullscreen(true);
    } else {
      await document.exitFullscreen();
      setIsFullscreen(false);
    }
  }, []);

  const changeSpeed = useCallback((s: number) => {
    const el = videoRef.current;
    if (!el) return;
    el.playbackRate = s;
    setSpeed(s);
    setShowSpeedMenu(false);
  }, []);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      switch (e.key) {
        case 'Escape':
          if (document.fullscreenElement) {
            document.exitFullscreen();
          } else {
            onClose();
          }
          break;
        case ' ':
          e.preventDefault();
          togglePlay();
          break;
        case 'ArrowLeft':
          seek(-5);
          break;
        case 'ArrowRight':
          seek(5);
          break;
        case 'f':
          toggleFullscreen();
          break;
        case 'l':
          setLoop((v) => !v);
          break;
        case 'm':
          toggleMute();
          break;
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onClose, togglePlay, seek, toggleFullscreen, toggleMute]);

  useEffect(() => {
    const handler = () => setIsFullscreen(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', handler);
    return () => document.removeEventListener('fullscreenchange', handler);
  }, []);

  const handleBackgroundClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) togglePlay();
  };

  const pct = duration > 0 ? (currentTime / duration) * 100 : 0;
  const src = videoUrl(video.filename);
  const vttSrc = trickplayVttUrl(video.filename);

  const formatSpeed = (s: number) => `${s === 1 ? 'Normal' : s + 'x'}`;

  return (
    <div
      className="fixed inset-0 z-50 bg-black select-none"
      onMouseMove={showControls}
      onMouseDown={() => setShowSpeedMenu(false)}
    >
      {/* Video element (no native controls) */}
      <video
        ref={videoRef}
        src={src}
        autoPlay
        loop={loop}
        playsInline
        className="absolute inset-0 w-full h-full object-contain outline-none"
        onPlay={() => setIsPlaying(true)}
        onPause={() => setIsPlaying(false)}
        onTimeUpdate={() => {
          if (videoRef.current) setCurrentTime(videoRef.current.currentTime);
        }}
        onLoadedMetadata={() => {
          if (videoRef.current) setDuration(Math.floor(videoRef.current.duration));
        }}
        onClick={handleBackgroundClick}
      >
        <track kind="metadata" src={vttSrc} default />
      </video>

      {/* Click-to-close backdrop (not blocking video clicks) */}
      <div className="absolute inset-0" onClick={handleBackgroundClick} />

      {/* Center play button (when paused) */}
      {!isPlaying && (
        <button
          onClick={togglePlay}
          className="absolute inset-0 flex items-center justify-center z-10 group"
        >
          <div className="w-20 h-20 rounded-full bg-white/10 backdrop-blur-sm flex items-center justify-center group-hover:bg-white/20 transition-all group-hover:scale-110">
            <Play size={40} className="text-white fill-white ml-1" />
          </div>
        </button>
      )}

      {/* Top bar */}
      <div
        className={`absolute top-0 left-0 right-0 z-20 transition-opacity duration-300 ${
          controlsVisible ? 'opacity-100' : 'opacity-0 pointer-events-none'
        }`}
      >
        <div className="bg-gradient-to-b from-black/80 to-transparent px-5 pt-4 pb-10">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3 min-w-0 mr-4">
              <span className="text-sm text-white/90 font-medium truncate">{video.title || video.filename}</span>
              {video.width && video.height && (
                <span className="text-xs text-white/50 shrink-0">{video.width}x{video.height}</span>
              )}
              {duration > 0 && (
                <span className="text-xs text-white/40 shrink-0">{formatDuration(duration)}</span>
              )}
            </div>
            <button
              onClick={onClose}
              className="shrink-0 p-1.5 text-white/60 hover:text-white transition-colors"
              aria-label="Close"
            >
              <X size={22} />
            </button>
          </div>
        </div>
      </div>

      {/* Bottom controls */}
      <div
        className={`absolute bottom-0 left-0 right-0 z-20 transition-opacity duration-300 ${
          controlsVisible ? 'opacity-100' : 'opacity-0 pointer-events-none'
        }`}
      >
        <div className="bg-gradient-to-t from-black/90 via-black/60 to-transparent px-4 pt-10 pb-3">
          {/* Progress bar */}
          <div
            ref={progressRef}
            className="relative h-1.5 bg-white/20 rounded-full cursor-pointer group/progress mb-3 mx-1"
            onClick={handleProgressClick}
            onMouseMove={handleProgressDrag}
            onMouseDown={() => setScrubbing(true)}
            onMouseUp={() => setScrubbing(false)}
            onMouseLeave={() => setScrubbing(false)}
          >
            <div
              className="absolute inset-y-0 left-0 bg-blue-500 rounded-full transition-[width] duration-100"
              style={{ width: `${pct}%` }}
            />
            <div
              className="absolute top-1/2 -translate-y-1/2 w-3.5 h-3.5 bg-blue-400 rounded-full shadow-lg opacity-0 group-hover/progress:opacity-100 transition-opacity"
              style={{ left: `calc(${pct}% - 7px)` }}
            />
          </div>

          {/* Controls row */}
          <div className="flex items-center gap-3 text-white">
            {/* Play/Pause */}
            <button onClick={togglePlay} className="p-1 hover:text-blue-400 transition-colors" title={isPlaying ? 'Pause' : 'Play'}>
              {isPlaying ? <Pause size={20} fill="currentColor" /> : <Play size={20} fill="currentColor" />}
            </button>

            {/* Time */}
            <span className="text-xs text-white/70 font-mono tabular-nums shrink-0 min-w-[90px]">
              {formatDuration(Math.floor(currentTime))} / {formatDuration(duration)}
            </span>

            <div className="flex-1" />

            {/* Speed */}
            <div className="relative">
              <button
                onClick={() => setShowSpeedMenu((v) => !v)}
                className="flex items-center gap-1 px-2 py-1 text-xs text-white/60 hover:text-white transition-colors rounded hover:bg-white/10"
              >
                <RotateCw size={13} />
                {formatSpeed(speed)}
              </button>
              {showSpeedMenu && (
                <div className="absolute bottom-full right-0 mb-2 bg-zinc-900 border border-zinc-700 rounded-lg shadow-xl py-1 min-w-[100px]" onMouseDown={(e) => e.stopPropagation()}>
                  {SPEEDS.map((s) => (
                    <button
                      key={s}
                      onClick={() => changeSpeed(s)}
                      className={`w-full text-left px-3 py-1.5 text-xs transition-colors ${
                        speed === s ? 'text-blue-400 bg-blue-500/10' : 'text-white/70 hover:text-white hover:bg-white/5'
                      }`}
                    >
                      {formatSpeed(s)}
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Loop */}
            <button
              onClick={() => setLoop((v) => !v)}
              className={`p-1.5 transition-colors ${loop ? 'text-blue-400' : 'text-white/50 hover:text-white'}`}
              title={`Loop: ${loop ? 'On' : 'Off'}`}
            >
              <RotateCw size={16} className={loop ? '' : 'opacity-60'} />
            </button>

            {/* Volume */}
            <div className="flex items-center gap-1.5 group/vol">
              <button onClick={toggleMute} className="p-1 text-white/50 hover:text-white transition-colors">
                {muted || volume === 0 ? <VolumeX size={18} /> : <Volume2 size={18} />}
              </button>
              <div className="w-0 group-hover/vol:w-20 overflow-hidden transition-all duration-200">
                <input
                  type="range"
                  min={0}
                  max={1}
                  step={0.05}
                  value={muted ? 0 : volume}
                  onChange={handleVolumeChange}
                  className="w-20 h-1 appearance-none bg-white/20 rounded-full cursor-pointer
                    [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:h-3
                    [&::-webkit-slider-thumb]:bg-white [&::-webkit-slider-thumb]:rounded-full"
                />
              </div>
            </div>

            {/* Fullscreen */}
            <button onClick={toggleFullscreen} className="p-1 text-white/50 hover:text-white transition-colors" title={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}>
              {isFullscreen ? <Minimize size={18} /> : <Maximize size={18} />}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}