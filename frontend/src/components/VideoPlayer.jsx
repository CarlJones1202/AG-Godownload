import { useState, useEffect, useRef } from 'react'
import './VideoPlayer.css'
import './PersonTagging.css'

function VideoPlayer({ video, onClose, onNext, onPrev }) {
    const videoRef = useRef(null)
    const canvasRef = useRef(null)
    const [isPlaying, setIsPlaying] = useState(true)
    const [currentTime, setCurrentTime] = useState(0)
    const [duration, setDuration] = useState(0)
    const [volume, setVolume] = useState(1)
    const [showPreview, setShowPreview] = useState(false)
    const [previewTime, setPreviewTime] = useState(0)
    const [previewPosition, setPreviewPosition] = useState({ x: 0, y: 0 })
    const [vttData, setVttData] = useState([])
    const [spriteImage, setSpriteImage] = useState(null)
    const [showPersonModal, setShowPersonModal] = useState(false)
    const [people, setPeople] = useState([])
    const [taggedPeople, setTaggedPeople] = useState(video.people || [])
    const [searchQuery, setSearchQuery] = useState('')

    // Load VTT file and sprite image
    useEffect(() => {
        if (!video.trickplay_vtt || !video.trickplay_sprite) return

        // Load sprite image
        const img = new Image()
        img.src = `/api${video.trickplay_sprite}`
        img.onload = () => setSpriteImage(img)

        // Load and parse VTT file
        fetch(`/api${video.trickplay_vtt}`)
            .then(res => res.text())
            .then(text => {
                const cues = parseVTT(text)
                setVttData(cues)
            })
            .catch(err => console.error('Failed to load VTT:', err))
    }, [video.trickplay_vtt, video.trickplay_sprite])

    // Update tagged people when video changes
    useEffect(() => {
        setTaggedPeople(video.people || [])
    }, [video.id, video.people])

    // Fetch people list for tagging
    const fetchPeople = async () => {
        try {
            const response = await fetch('/api/people?limit=1000')
            const data = await response.json()
            setPeople(data.data || [])
        } catch (error) {
            console.error('Failed to fetch people:', error)
        }
    }

    const handleTagPerson = async (personId) => {
        try {
            const response = await fetch(`/api/people/${personId}/images/${video.id}`, {
                method: 'POST'
            })
            if (response.ok) {
                // Refresh tagged people
                const person = people.find(p => p.id === personId)
                if (person && !taggedPeople.find(p => p.id === personId)) {
                    setTaggedPeople([...taggedPeople, person])
                }
                setShowPersonModal(false)
                setSearchQuery('')
            } else {
                alert('Failed to tag person')
            }
        } catch (error) {
            console.error('Error tagging person:', error)
            alert('Error tagging person')
        }
    }

    const handleUntagPerson = async (personId) => {
        try {
            const response = await fetch(`/api/people/${personId}/images/${video.id}`, {
                method: 'DELETE'
            })
            if (response.ok) {
                setTaggedPeople(taggedPeople.filter(p => p.id !== personId))
            } else {
                alert('Failed to untag person')
            }
        } catch (error) {
            console.error('Error untagging person:', error)
            alert('Error untagging person')
        }
    }

    // Load saved progress when video loads
    useEffect(() => {
        if (!videoRef.current || !video.id) return

        const savedProgress = localStorage.getItem(`video_progress_${video.id}`)
        if (savedProgress) {
            const progress = JSON.parse(savedProgress)
            // Only resume if more than 5 seconds in and not within last 30 seconds
            if (progress.currentTime > 5 && progress.currentTime < progress.duration - 30) {
                videoRef.current.currentTime = progress.currentTime
            }
        }
    }, [video.id])

    // Save progress periodically
    useEffect(() => {
        if (!videoRef.current || !video.id) return

        const saveProgress = () => {
            if (videoRef.current && duration > 0) {
                const progress = {
                    currentTime: videoRef.current.currentTime,
                    duration: duration,
                    lastUpdated: Date.now()
                }
                localStorage.setItem(`video_progress_${video.id}`, JSON.stringify(progress))
            }
        }

        // Save every 5 seconds while playing
        const interval = setInterval(() => {
            if (isPlaying) {
                saveProgress()
            }
        }, 5000)

        // Save when video is paused or closed
        const handleBeforeUnload = () => saveProgress()
        window.addEventListener('beforeunload', handleBeforeUnload)

        return () => {
            clearInterval(interval)
            window.removeEventListener('beforeunload', handleBeforeUnload)
            saveProgress() // Save one last time on cleanup
        }
    }, [video.id, duration, isPlaying])

    // Parse VTT file
    const parseVTT = (vttText) => {
        const lines = vttText.split('\n')
        const cues = []
        let i = 0

        while (i < lines.length) {
            const line = lines[i].trim()

            // Look for timestamp line (HH:MM:SS.mmm --> HH:MM:SS.mmm)
            if (line.includes('-->')) {
                const [startStr, endStr] = line.split('-->').map(s => s.trim())
                const start = parseVTTTime(startStr)
                const end = parseVTTTime(endStr)

                // Next line should be the sprite coordinates
                i++
                if (i < lines.length) {
                    const coordLine = lines[i].trim()
                    // Format: sprite.jpg#xywh=x,y,w,h
                    const match = coordLine.match(/#xywh=(\d+),(\d+),(\d+),(\d+)/)
                    if (match) {
                        cues.push({
                            start,
                            end,
                            x: parseInt(match[1]),
                            y: parseInt(match[2]),
                            w: parseInt(match[3]),
                            h: parseInt(match[4])
                        })
                    }
                }
            }
            i++
        }
        return cues
    }

    // Parse VTT timestamp to seconds
    const parseVTTTime = (timeStr) => {
        const parts = timeStr.split(':')
        const hours = parseInt(parts[0])
        const minutes = parseInt(parts[1])
        const seconds = parseFloat(parts[2])
        return hours * 3600 + minutes * 60 + seconds
    }

    // Find the appropriate sprite coordinates for a given time
    const getSpriteCoords = (time) => {
        for (const cue of vttData) {
            if (time >= cue.start && time < cue.end) {
                return cue
            }
        }
        return null
    }

    // Handle timeline hover
    const handleTimelineHover = (e) => {
        if (!videoRef.current || !vttData.length || !spriteImage) return

        const rect = e.currentTarget.getBoundingClientRect()
        const x = e.clientX - rect.left
        const percent = x / rect.width
        const time = percent * duration

        setPreviewTime(time)
        setPreviewPosition({ x: e.clientX, y: rect.top })
        setShowPreview(true)

        // Draw preview on canvas
        const coords = getSpriteCoords(time)
        if (coords && canvasRef.current) {
            const canvas = canvasRef.current
            const ctx = canvas.getContext('2d')
            canvas.width = coords.w
            canvas.height = coords.h
            ctx.drawImage(spriteImage, coords.x, coords.y, coords.w, coords.h, 0, 0, coords.w, coords.h)
        }
    }

    const handleTimelineLeave = () => {
        setShowPreview(false)
    }

    const handleTimelineClick = (e) => {
        if (!videoRef.current) return
        const rect = e.currentTarget.getBoundingClientRect()
        const x = e.clientX - rect.left
        const percent = x / rect.width
        videoRef.current.currentTime = percent * duration
    }

    const togglePlay = () => {
        if (!videoRef.current) return
        if (isPlaying) {
            videoRef.current.pause()
        } else {
            videoRef.current.play()
        }
        setIsPlaying(!isPlaying)
    }

    const handleVolumeChange = (e) => {
        const newVolume = parseFloat(e.target.value)
        setVolume(newVolume)
        if (videoRef.current) {
            videoRef.current.volume = newVolume
        }
    }

    const formatTime = (seconds) => {
        const h = Math.floor(seconds / 3600)
        const m = Math.floor((seconds % 3600) / 60)
        const s = Math.floor(seconds % 60)
        if (h > 0) {
            return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
        }
        return `${m}:${s.toString().padStart(2, '0')}`
    }

    return (
        <div className="video-player-overlay" onClick={onClose}>
            <div className="video-player-container" onClick={(e) => e.stopPropagation()}>
                <button className="video-close" onClick={onClose}>✕</button>

                <video
                    ref={videoRef}
                    src={video.web_path || `/api/images/${video.filename}`}
                    autoPlay
                    onTimeUpdate={(e) => setCurrentTime(e.target.currentTime)}
                    onLoadedMetadata={(e) => setDuration(e.target.duration)}
                    onPlay={() => setIsPlaying(true)}
                    onPause={() => setIsPlaying(false)}
                />

                {/* Tagged People Display */}
                {taggedPeople.length > 0 && (
                    <div className="video-tagged-people">
                        {taggedPeople.map(person => (
                            <div key={person.id} className="tagged-person-chip">
                                <span>{person.name}</span>
                                <button onClick={() => handleUntagPerson(person.id)} title="Remove tag">✕</button>
                            </div>
                        ))}
                    </div>
                )}

                <div className="video-controls">
                    <button onClick={togglePlay} className="play-btn">
                        {isPlaying ? '⏸' : '▶'}
                    </button>

                    <div className="time-display">
                        {formatTime(currentTime)} / {formatTime(duration)}
                    </div>

                    <div
                        className="timeline"
                        onMouseMove={handleTimelineHover}
                        onMouseLeave={handleTimelineLeave}
                        onClick={handleTimelineClick}
                    >
                        <div className="timeline-progress" style={{ width: `${(currentTime / duration) * 100}%` }} />

                        {/* Show resume point indicator */}
                        {(() => {
                            const savedProgress = localStorage.getItem(`video_progress_${video.id}`)
                            if (savedProgress && currentTime < 5) {
                                const progress = JSON.parse(savedProgress)
                                if (progress.currentTime > 5 && progress.currentTime < progress.duration - 30) {
                                    return (
                                        <div
                                            className="resume-indicator"
                                            style={{ left: `${(progress.currentTime / duration) * 100}%` }}
                                            title={`Resume at ${formatTime(progress.currentTime)}`}
                                        />
                                    )
                                }
                            }
                        })()}

                        {showPreview && (
                            <div
                                className="timeline-preview"
                                style={{
                                    left: `${(previewTime / duration) * 100}%`,
                                    transform: 'translateX(-50%)'
                                }}
                            >
                                <canvas ref={canvasRef} />
                                <div className="preview-time">{formatTime(previewTime)}</div>
                            </div>
                        )}
                    </div>

                    <div className="volume-control">
                        <span>🔊</span>
                        <input
                            type="range"
                            min="0"
                            max="1"
                            step="0.1"
                            value={volume}
                            onChange={handleVolumeChange}
                        />
                    </div>

                    <button onClick={onPrev} className="nav-btn">⏮</button>
                    <button onClick={onNext} className="nav-btn">⏭</button>
                    <button
                        onClick={() => {
                            fetchPeople()
                            setShowPersonModal(true)
                        }}
                        className="tag-person-btn"
                        title="Tag Person"
                    >
                        👤+
                    </button>
                </div>

                {/* Person Tagging Modal */}
                {showPersonModal && (
                    <div className="person-modal-overlay" onClick={() => setShowPersonModal(false)}>
                        <div className="person-modal" onClick={e => e.stopPropagation()}>
                            <h3>Tag Person</h3>
                            <input
                                type="text"
                                placeholder="Search people..."
                                value={searchQuery}
                                onChange={(e) => setSearchQuery(e.target.value)}
                                className="person-search"
                            />
                            <div className="person-list">
                                {people
                                    .filter(p =>
                                        p.name.toLowerCase().includes(searchQuery.toLowerCase()) &&
                                        !taggedPeople.find(tp => tp.id === p.id)
                                    )
                                    .map(person => (
                                        <div
                                            key={person.id}
                                            className="person-item"
                                            onClick={() => handleTagPerson(person.id)}
                                        >
                                            {person.name}
                                        </div>
                                    ))
                                }
                            </div>
                            <button onClick={() => setShowPersonModal(false)} className="close-modal">Close</button>
                        </div>
                    </div>
                )}
            </div>
        </div>
    )
}

export default VideoPlayer
