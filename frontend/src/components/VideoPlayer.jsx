import { useState, useEffect, useRef, useCallback } from 'react'
import VRPlayer from './VRPlayer'
import './VideoPlayer.css'

function VideoPlayer({ video, onClose, onNext, onPrev }) {
    const videoRef = useRef(null)
    const containerRef = useRef(null)
    const canvasRef = useRef(null)

    // State
    const [isPlaying, setIsPlaying] = useState(true)
    const [currentTime, setCurrentTime] = useState(0)
    const [duration, setDuration] = useState(0)
    const [volume, setVolume] = useState(1)
    const [showPreview, setShowPreview] = useState(false)
    const [previewTime, setPreviewTime] = useState(0)
    const [previewPosition, setPreviewPosition] = useState(0)
    const [vttData, setVttData] = useState([])
    const [spriteImage, setSpriteImage] = useState(null)
    const [showControls, setShowControls] = useState(true)
    const [showMetadata, setShowMetadata] = useState(false) // Sidebar state
    const [isFullscreen, setIsFullscreen] = useState(false)
    const [vrMode, setVrMode] = useState(null) // null, '180', '360'

    // Autohide controls logic
    const controlsTimeoutRef = useRef(null)

    const resetControlsTimeout = useCallback(() => {
        setShowControls(true)
        if (controlsTimeoutRef.current) clearTimeout(controlsTimeoutRef.current)
        if (isPlaying && !showMetadata) {
            controlsTimeoutRef.current = setTimeout(() => setShowControls(false), 3000)
        }
    }, [isPlaying, showMetadata])

    const handleMouseMove = () => {
        resetControlsTimeout()
    }

    useEffect(() => {
        resetControlsTimeout()
        return () => clearTimeout(controlsTimeoutRef.current)
    }, [resetControlsTimeout])

    // Keyboard shortcuts
    useEffect(() => {
        const handleKeyDown = (e) => {
            if (e.key === 'Escape') onClose()
            if (e.key === 'ArrowRight') {
                if (e.ctrlKey) onNext()
                else if (videoRef.current) videoRef.current.currentTime += 5
            }
            if (e.key === 'ArrowLeft') {
                if (e.ctrlKey) onPrev()
                else if (videoRef.current) videoRef.current.currentTime -= 5
            }
            if (e.key === ' ') {
                e.preventDefault()
                togglePlay()
            }
            if (e.key === 'f') toggleFullscreen()
        }
        window.addEventListener('keydown', handleKeyDown)
        return () => window.removeEventListener('keydown', handleKeyDown)
    }, [onClose, onNext, onPrev]) // togglePlay dependence handled via ref/function

    // Load VTT & Sprite
    useEffect(() => {
        if (!video.trickplay_vtt || !video.trickplay_sprite) return
        const img = new Image()
        img.src = `/api${video.trickplay_sprite}`
        img.onload = () => setSpriteImage(img)

        fetch(`/api${video.trickplay_vtt}`)
            .then(res => res.text())
            .then(text => setVttData(parseVTT(text)))
            .catch(err => console.error('Failed to load VTT:', err))
    }, [video.trickplay_vtt, video.trickplay_sprite])

    // Progress persistence
    useEffect(() => {
        const savedProgress = localStorage.getItem(`video_progress_${video.id}`)
        if (savedProgress && videoRef.current) {
            const progress = JSON.parse(savedProgress)
            if (progress.currentTime > 5 && progress.currentTime < progress.duration - 30) {
                videoRef.current.currentTime = progress.currentTime
            }
        }
    }, [video.id])

    useEffect(() => {
        const interval = setInterval(() => {
            if (videoRef.current && !videoRef.current.paused) {
                localStorage.setItem(`video_progress_${video.id}`, JSON.stringify({
                    currentTime: videoRef.current.currentTime,
                    duration: videoRef.current.duration,
                    lastUpdated: Date.now()
                }))
            }
        }, 5000)
        return () => clearInterval(interval)
    }, [video.id])

    // Helper functions
    const togglePlay = () => {
        if (!videoRef.current) return
        if (videoRef.current.paused) {
            videoRef.current.play()
            setIsPlaying(true)
        } else {
            videoRef.current.pause()
            setIsPlaying(false)
            setShowControls(true)
        }
    }

    const toggleFullscreen = () => {
        if (!document.fullscreenElement) {
            containerRef.current.requestFullscreen().catch(err => console.error(err))
            setIsFullscreen(true)
        } else {
            document.exitFullscreen()
            setIsFullscreen(false)
        }
    }

    const toggleVrMode = () => {
        setVrMode(prev => {
            if (prev === null) return '180'
            if (prev === '180') return '360'
            return null
        })
    }

    // Timeline logic
    const handleTimelineHover = (e) => {
        if (!vttData.length || !spriteImage) return
        const rect = e.currentTarget.getBoundingClientRect()
        const x = e.clientX - rect.left
        const percent = Math.max(0, Math.min(1, x / rect.width))
        const time = percent * duration

        setPreviewTime(time)
        setPreviewPosition(x)
        setShowPreview(true)

        // Draw sprite
        const cue = vttData.find(c => time >= c.start && time < c.end)
        if (cue && canvasRef.current) {
            const ctx = canvasRef.current.getContext('2d')
            canvasRef.current.width = cue.w
            canvasRef.current.height = cue.h
            ctx.drawImage(spriteImage, cue.x, cue.y, cue.w, cue.h, 0, 0, cue.w, cue.h)
        }
    }

    const handleTimelineClick = (e) => {
        const rect = e.currentTarget.getBoundingClientRect()
        const percent = (e.clientX - rect.left) / rect.width
        if (videoRef.current) videoRef.current.currentTime = percent * duration
    }

    // Formatters
    const formatTime = (s) => {
        if (!s) return "0:00"
        const m = Math.floor(s / 60)
        const sec = Math.floor(s % 60)
        const h = Math.floor(s / 3600)
        if (h > 0) return `${h}:${m % 60}:${sec.toString().padStart(2, '0')}`
        return `${m}:${sec.toString().padStart(2, '0')}`
    }

    // VTT Parser (simplified from before)
    const parseVTT = (text) => {
        const cues = []
        const lines = text.split('\n')
        let i = 0
        while (i < lines.length) {
            if (lines[i].includes('-->')) {
                const times = lines[i].split('-->')
                const start = parseTime(times[0].trim())
                const end = parseTime(times[1].trim())
                const coords = lines[++i]?.match(/#xywh=(\d+),(\d+),(\d+),(\d+)/)
                if (coords) {
                    cues.push({ start, end, x: +coords[1], y: +coords[2], w: +coords[3], h: +coords[4] })
                }
            }
            i++
        }
        return cues
    }

    const parseTime = (t) => {
        const parts = t.split(':')
        return (+parts[0]) * 3600 + (+parts[1]) * 60 + parseFloat(parts[2])
    }

    return (
        <div className={`video-player-root ${showMetadata ? 'sidebar-open' : ''}`}>
            <div
                ref={containerRef}
                className="video-container"
                onMouseMove={handleMouseMove}
                onMouseLeave={() => isPlaying && setShowControls(false)}
            >
                <video
                    ref={videoRef}
                    src={video.web_path || `/api/images/${video.filename}`}
                    className="video-element"
                    autoPlay
                    onClick={togglePlay}
                    onTimeUpdate={(e) => setCurrentTime(e.target.currentTime)}
                    onLoadedMetadata={(e) => setDuration(e.target.duration)}
                    onPlay={() => setIsPlaying(true)}
                    onPause={() => setIsPlaying(false)}
                />

                {/* VR Player Overlay */}
                {vrMode && videoRef.current && (
                    <div className="vr-player-wrapper" onMouseMove={handleMouseMove}>
                        <VRPlayer videoElement={videoRef.current} mode={vrMode} />
                    </div>
                )}

                {/* Top Controls (Close) */}
                <div className={`video-controls-top ${showControls ? 'visible' : ''}`}>
                    <button className="icon-btn" onClick={onClose} title="Close">
                        <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <line x1="18" y1="6" x2="6" y2="18"></line>
                            <line x1="6" y1="6" x2="18" y2="18"></line>
                        </svg>
                    </button>
                </div>

                {/* Bottom Controls */}
                <div className={`video-controls-bottom ${showControls ? 'visible' : ''}`}>
                    {/* Timeline */}
                    <div
                        className="timeline-track"
                        onMouseMove={handleTimelineHover}
                        onMouseLeave={() => setShowPreview(false)}
                        onClick={handleTimelineClick}
                    >
                        {/* Background Bar */}
                        <div className="timeline-bar-bg" />

                        {/* Buffered/Progress Bar */}
                        <div
                            className="timeline-bar-fill"
                            style={{ width: `${(currentTime / duration) * 100}%` }}
                        />

                        {/* Hover Preview Box */}
                        {showPreview && (
                            <div className="preview-box" style={{ left: previewPosition }}>
                                <canvas ref={canvasRef} />
                                <span className="preview-time-text">{formatTime(previewTime)}</span>
                            </div>
                        )}
                    </div>

                    <div className="controls-row">
                        <div className="controls-left">
                            <button className="icon-btn" onClick={togglePlay}>
                                {isPlaying ? (
                                    <svg viewBox="0 0 24 24" width="32" height="32" fill="currentColor"><path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z" /></svg>
                                ) : (
                                    <svg viewBox="0 0 24 24" width="32" height="32" fill="currentColor"><path d="M8 5v14l11-7z" /></svg>
                                )}
                            </button>

                            <div className="volume-container">
                                <span className="icon-btn-small">
                                    <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor"><path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM14 3.23v2.06c2.89.86 5 3.54 5 6.71s-2.11 5.85-5 6.71v2.06c4.01-.91 7-4.49 7-8.77s-2.99-7.86-7-8.77z" /></svg>
                                </span>
                                <input
                                    type="range" min="0" max="1" step="0.05"
                                    value={volume}
                                    onChange={(e) => {
                                        setVolume(e.target.value)
                                        if (videoRef.current) videoRef.current.volume = e.target.value
                                    }}
                                    className="volume-slider"
                                />
                            </div>

                            <span className="time-text">
                                {formatTime(currentTime)} / {formatTime(duration)}
                            </span>
                        </div>

                        <div className="controls-right">
                            <button className="icon-btn" onClick={toggleVrMode} title="Toggle VR Mode (2D / 180 / 360)">
                                {vrMode === null && <span style={{ fontSize: '14px', fontWeight: 'bold' }}>2D</span>}
                                {vrMode === '180' && <span style={{ fontSize: '14px', fontWeight: 'bold', color: '#3b82f6' }}>180°</span>}
                                {vrMode === '360' && <span style={{ fontSize: '14px', fontWeight: 'bold', color: '#10b981' }}>360°</span>}
                            </button>
                            {!showMetadata && (
                                <button className="icon-btn" onClick={() => setShowMetadata(true)} title="Info">
                                    <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                        <circle cx="12" cy="12" r="10"></circle>
                                        <line x1="12" y1="16" x2="12" y2="12"></line>
                                        <line x1="12" y1="8" x2="12.01" y2="8"></line>
                                    </svg>
                                </button>
                            )}
                            <button className="icon-btn" onClick={toggleFullscreen} title="Fullscreen">
                                <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                    <path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2-2h3" />
                                </svg>
                            </button>
                        </div>
                    </div>
                </div>
            </div>

            {/* Sidebar Metadata (Docked) */}
            <div className={`video-sidebar ${showMetadata ? 'open' : ''}`}>
                <div className="sidebar-header">
                    <h3>Video Info</h3>
                    <button onClick={() => setShowMetadata(false)} className="close-sidebar-btn">
                        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <line x1="18" y1="6" x2="6" y2="18"></line>
                            <line x1="6" y1="6" x2="18" y2="18"></line>
                        </svg>
                    </button>
                </div>
                <div className="sidebar-content">
                    <div className="info-group">
                        <label>Title</label>
                        <div className="info-value title">{video.title || video.filename}</div>
                    </div>

                    <div className="info-group">
                        <label>Source</label>
                        <div className="info-value link">{video.source?.name || 'Unknown'}</div>
                    </div>

                    <div className="info-group">
                        <label>Quality</label>
                        <div className="info-value">
                            {video.height}p {video.width >= 3840 && '(4K)'}
                        </div>
                    </div>

                    <div className="people-group">
                        <label>Performers</label>
                        <div className="tags-list">
                            {video.people && video.people.length > 0 ? (
                                video.people.map(p => (
                                    <span key={p.id} className="person-tag">{p.name}</span>
                                ))
                            ) : (
                                <span className="empty-tag">No performers tagged</span>
                            )}
                        </div>
                    </div>
                </div>
            </div>
        </div>
    )
}

export default VideoPlayer
