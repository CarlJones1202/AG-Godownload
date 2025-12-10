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

    // Close info panel when image changes (optional, but keeps UI clean)
    // useEffect(() => setShowInfo(false), [image]) 
    // Actually, keeping it open is better for browsing with info.

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

                {/* Navigation & Controls (keep these relative to main content) */}
                <button className="lightbox-nav lightbox-prev" onClick={onPrev} title="Previous">
                    ‹
                </button>
                <button className="lightbox-nav lightbox-next" onClick={onNext} title="Next">
                    ›
                </button>

                <div className="lightbox-footer">
                    <span>{currentIndex} / {totalImages}</span>
                </div>
            </div>

            {/* Top Right Controls */}
            <div className="lightbox-controls" onClick={(e) => e.stopPropagation()}>
                <button
                    className={`control-btn-icon ${showInfo ? 'active' : ''}`}
                    onClick={() => setShowInfo(!showInfo)}
                    title="Toggle Info"
                >
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <circle cx="12" cy="12" r="10"></circle>
                        <line x1="12" y1="16" x2="12" y2="12"></line>
                        <line x1="12" y1="8" x2="12.01" y2="8"></line>
                    </svg>
                </button>
                <button className="control-btn-icon" onClick={onClose} title="Close">
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
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
                    <button onClick={() => setShowInfo(false)} className="close-panel-btn">✕</button>
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
