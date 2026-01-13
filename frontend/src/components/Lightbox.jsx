import { useState, useEffect } from 'react'
import './Lightbox.css'

function Lightbox({ image, onClose, onNext, onPrev, currentIndex, totalImages }) {
    useEffect(() => {
        const handleKeyDown = (e) => {
            if (e.key === 'Escape') onClose()
            if (e.key === 'ArrowRight') onNext()
            if (e.key === 'ArrowLeft') onPrev()
        }

        window.addEventListener('keydown', handleKeyDown)
        return () => window.removeEventListener('keydown', handleKeyDown)
    }, [onClose, onNext, onPrev])

    const [showInfo, setShowInfo] = useState(false)
    const [isFavorite, setIsFavorite] = useState(image.is_favorite)

    useEffect(() => {
        setIsFavorite(image.is_favorite)
    }, [image])

    const handleToggleFavorite = async (e) => {
        e.stopPropagation()
        try {
            const response = await fetch(`/api/images/${image.id}/favorite`, {
                method: 'POST'
            })
            if (response.ok) {
                const data = await response.json()
                setIsFavorite(data.is_favorite)
                // Note: The parent list won't update until refreshed, 
                // but that's acceptable for now.
            }
        } catch (error) {
            console.error('Error toggling favorite:', error)
        }
    }

    return (
        <div className={`lightbox ${showInfo ? 'info-open' : ''}`} onClick={onClose}>
            {/* Main Content Area */}
            <div className="lightbox-main" onClick={(e) => e.stopPropagation()}>
                <div className="lightbox-content">
                    {(image.type === 'video' || (typeof isVideo !== 'undefined' && isVideo)) ? (
                        <video
                            src={image.web_path || `/api/images/${image.filename}`}
                            controls
                            autoPlay
                            className="lightbox-media"
                        >
                            {image.trickplay_vtt && (
                                <track
                                    kind="metadata"
                                    src={image.trickplay_vtt}
                                    default
                                />
                            )}
                        </video>
                    ) : (
                        <img
                            src={image.web_path || `/api/images/${image.filename}`}
                            alt={`Image ${image.id}`}
                            className="lightbox-media"
                        />
                    )}
                </div>

                {/* Navigation & Controls */}
                <button className="lightbox-nav lightbox-prev" onClick={onPrev} title="Previous">
                    <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                        <polyline points="15 18 9 12 15 6"></polyline>
                    </svg>
                </button>
                <button className="lightbox-nav lightbox-next" onClick={onNext} title="Next">
                    <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                        <polyline points="9 18 15 12 9 6"></polyline>
                    </svg>
                </button>

                <div className="lightbox-footer">
                    <span>{currentIndex} / {totalImages}</span>
                </div>
            </div>

            {/* Top Right Controls */}
            <div className="lightbox-controls" onClick={(e) => e.stopPropagation()}>
                <button
                    className={`control-btn-icon favorite ${isFavorite ? 'active' : ''}`}
                    onClick={handleToggleFavorite}
                    title={isFavorite ? "Unfavorite" : "Favorite"}
                >
                    <svg width="24" height="24" viewBox="0 0 24 24" fill={isFavorite ? "currentColor" : "none"} stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"></path>
                    </svg>
                </button>
                <button
                    className={`control-btn-icon ${showInfo ? 'active' : ''}`}
                    onClick={() => setShowInfo(!showInfo)}
                    title="Toggle Info"
                >
                    <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <circle cx="12" cy="12" r="10"></circle>
                        <line x1="12" y1="16" x2="12" y2="12"></line>
                        <line x1="12" y1="8" x2="12.01" y2="8"></line>
                    </svg>
                </button>
                <button className="control-btn-icon" onClick={onClose} title="Close">
                    <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <line x1="18" y1="6" x2="6" y2="18"></line>
                        <line x1="6" y1="6" x2="18" y2="18"></line>
                    </svg>
                </button>
            </div>

            {/* Metadata Sidebar */}
            <div
                className={`lightbox-metadata ${showInfo ? 'open' : ''}`}
                onClick={(e) => e.stopPropagation()}
            >
                <div className="metadata-header">
                    <h3>Image Info</h3>
                    <button onClick={() => setShowInfo(false)} className="close-panel-btn">
                        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <line x1="18" y1="6" x2="6" y2="18"></line>
                            <line x1="6" y1="6" x2="18" y2="18"></line>
                        </svg>
                    </button>
                </div>

                <div className="metadata-content">
                    <div className="metadata-item">
                        <label>Filename</label>
                        <span className="file-name" title={image.filename}>{image.filename}</span>
                    </div>

                    <div className="metadata-item">
                        <label>Source</label>
                        {image.original_url ? (
                            <a href={image.original_url} target="_blank" rel="noopener noreferrer" className="link">
                                Open Original ↗
                            </a>
                        ) : (
                            <span className="text-muted">No source URL</span>
                        )}
                    </div>

                    <div className="metadata-item">
                        <label>Date Added</label>
                        <span>{new Date(image.created_at).toLocaleDateString()}</span>
                    </div>

                    <div className="metadata-item">
                        <label>Galleries</label>
                        <div className="tags-list">
                            {image.galleries && image.galleries.length > 0 ? (
                                image.galleries.map(g => (
                                    <span key={g.id} className="tag">{g.name}</span>
                                ))
                            ) : (
                                <span className="tag-ghost">{image.gallery?.name || 'Unlinked'}</span>
                            )}
                        </div>
                    </div>

                    {image.dominant_colors && (() => {
                        try {
                            const colors = JSON.parse(image.dominant_colors);
                            if (colors && colors.length > 0) {
                                return (
                                    <div className="metadata-item">
                                        <label>Dominant Colors</label>
                                        <div className="color-palette">
                                            {colors.map((color, index) => (
                                                <div
                                                    key={index}
                                                    className="color-swatch"
                                                    style={{ backgroundColor: color }}
                                                    title={color}
                                                    onClick={() => navigator.clipboard.writeText(color)}
                                                />
                                            ))}
                                        </div>
                                    </div>
                                );
                            }
                        } catch (e) {
                            return null;
                        }
                        return null;
                    })()}
                </div>
            </div>
        </div>
    )
}

export default Lightbox
