import { useState } from 'react'
import VideoPlayer from './VideoPlayer'
// Use ImageList CSS for now as it's a grid
import './ImageList.css'
import './VideoList.css'

function VideoList({ videos, onRefresh, meta, onPageChange }) {
    const [lightboxVideo, setLightboxVideo] = useState(null)
    const [lightboxIndex, setLightboxIndex] = useState(0)

    const handleDeleteVideo = async (videoId, e) => {
        e.stopPropagation()
        if (!confirm('Are you sure you want to delete this video?')) return

        try {
            const response = await fetch(`/api/images/${videoId}`, { method: 'DELETE' })
            if (response.ok) {
                onRefresh()
            } else {
                alert('Failed to delete video')
            }
        } catch (error) {
            console.error('Error deleting video:', error)
            alert('Error deleting video')
        }
    }

    const openLightbox = (video, index) => {
        setLightboxVideo(video)
        setLightboxIndex(index)
    }

    const closeLightbox = () => {
        setLightboxVideo(null)
    }

    const nextVideo = () => {
        const nextIndex = (lightboxIndex + 1) % videos.length
        setLightboxIndex(nextIndex)
        setLightboxVideo(videos[nextIndex])
    }

    const prevVideo = () => {
        const prevIndex = (lightboxIndex - 1 + videos.length) % videos.length
        setLightboxIndex(prevIndex)
        setLightboxVideo(videos[prevIndex])
    }

    return (
        <div className="image-list">
            <div className="image-header">
                <h2>All Videos</h2>
                <button onClick={onRefresh}>🔄 Refresh</button>
            </div>

            {videos.length === 0 ? (
                <div className="no-images">
                    <p>No videos found. Add a source with video files!</p>
                </div>
            ) : (
                <div className="images-grid">
                    {videos.map((video, index) => (
                        <div
                            key={video.id}
                            className="video-card"
                            onClick={() => openLightbox(video, index)}
                        >
                            <div className="video-thumbnail-container">
                                <img
                                    src={video.thumbnail_path || `/api/thumbnails/${video.filename}`}
                                    alt={video.title || video.filename}
                                    loading="lazy"
                                    onError={(e) => {
                                        e.target.onerror = null;
                                        e.target.src = "https://via.placeholder.com/200x200?text=No+Thumb"
                                    }}
                                />
                                <div className="play-icon-overlay">▶</div>

                                {/* Duration Badge */}
                                {video.duration > 0 && (
                                    <div className="video-duration-badge">
                                        {(() => {
                                            const h = Math.floor(video.duration / 3600);
                                            const m = Math.floor((video.duration % 3600) / 60);
                                            const s = Math.floor(video.duration % 60);
                                            return h > 0
                                                ? `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
                                                : `${m}:${s.toString().padStart(2, '0')}`;
                                        })()}
                                    </div>
                                )}

                                {/* Quality Badge */}
                                {video.height > 0 && (
                                    <div className="video-quality-badge">
                                        {video.width >= 3840 ? '4K' :
                                            video.height >= 1080 ? '1080p' :
                                                video.height >= 720 ? '720p' : `${video.height}p`}
                                    </div>
                                )}
                            </div>

                            <div className="video-info">
                                <h3 className="video-title" title={video.source?.name || video.title}>
                                    {video.source?.name || video.title || "Untitled Video"}
                                </h3>
                            </div>

                            <button
                                className="delete-image-btn"
                                onClick={(e) => handleDeleteVideo(video.id, e)}
                                title="Delete Video"
                            >
                                🗑️
                            </button>
                        </div>
                    ))}
                </div>
            )}

            {meta && meta.total_pages > 1 && (
                <div className="pagination">
                    <button
                        disabled={meta.current_page === 1}
                        onClick={() => onPageChange(meta.current_page - 1)}
                    >
                        Previous
                    </button>
                    <span>Page {meta.current_page} of {meta.total_pages}</span>
                    <button
                        disabled={meta.current_page === meta.total_pages}
                        onClick={() => onPageChange(meta.current_page + 1)}
                    >
                        Next
                    </button>
                </div>
            )}

            {lightboxVideo && (
                <VideoPlayer
                    video={lightboxVideo}
                    onClose={closeLightbox}
                    onNext={nextVideo}
                    onPrev={prevVideo}
                />
            )}
        </div>
    )
}

export default VideoList
