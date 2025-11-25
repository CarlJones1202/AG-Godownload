import { useEffect } from 'react'
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

    return (
        <div className="lightbox" onClick={onClose}>
            <div className="lightbox-content" onClick={(e) => e.stopPropagation()}>
                <button className="lightbox-close" onClick={onClose}>✕</button>

                <button className="lightbox-nav lightbox-prev" onClick={onPrev}>
                    ‹
                </button>

                <img
                    src={`/api/images/${image.filename}`}
                    alt={`Image ${image.id}`}
                />

                <button className="lightbox-nav lightbox-next" onClick={onNext}>
                    ›
                </button>

                <div className="lightbox-info">
                    <span>{currentIndex} / {totalImages}</span>
                </div>
            </div>
        </div>
    )
}

export default Lightbox
