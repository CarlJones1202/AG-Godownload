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

    return (
        <div className="lightbox" onClick={onClose}>
            <div className="lightbox-content" onClick={(e) => e.stopPropagation()}>
                <button className="lightbox-close" onClick={onClose}>✕</button>
                <button
                    className={`lightbox-info-toggle ${showInfo ? 'active' : ''}`}
                    onClick={() => setShowInfo(!showInfo)}
                    title="Toggle Info"
                >
                    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <circle cx="12" cy="12" r="10"></circle>
                        <line x1="12" y1="16" x2="12" y2="12"></line>
                        <line x1="12" y1="8" x2="12.01" y2="8"></line>
                    </svg>
                </button>

                <button className="lightbox-nav lightbox-prev" onClick={onPrev}>
                    ‹
                </button>

                <div className="lightbox-image-container">
                    <img
                        src={image.web_path || `/api/images/${image.filename}`}
                        alt={`Image ${image.id}`}
                    />
                </div>

                <button className="lightbox-nav lightbox-next" onClick={onNext}>
                    ›
                </button>

                <div className="lightbox-footer">
                    <span>{currentIndex} / {totalImages}</span>
                </div>

                {showInfo && (
                    <div className="lightbox-metadata">
                        <h3>Image Info</h3>
                        <div className="metadata-item">
                            <label>Filename:</label>
                            <span>{image.filename}</span>
                        </div>
                        <div className="metadata-item">
                            <label>Galleries:</label>
                            <div className="tags">
                                {image.galleries && image.galleries.length > 0 ? (
                                    image.galleries.map(g => (
                                        <span key={g.id} className="tag">{g.name}</span>
                                    ))
                                ) : (
                                    <span className="tag">{image.gallery?.name || 'Unknown'}</span>
                                )}
                            </div>
                        </div>
                        <div className="metadata-item">
                            <label>Source URL:</label>
                            <a href={image.original_url} target="_blank" rel="noopener noreferrer" className="link">
                                Open Original
                            </a>
                        </div>
                        <div className="metadata-item">
                            <label>Created:</label>
                            <span>{new Date(image.created_at).toLocaleDateString()}</span>
                        </div>
                    </div>
                )}
            </div>
        </div>
    )
}

export default Lightbox
