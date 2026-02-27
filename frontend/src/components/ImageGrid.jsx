import { useState } from 'react'
import './ImageGrid.css'
import Lightbox from './Lightbox'

function ImageGrid({ gallery, onRefresh }) {
    const [selectedImage, setSelectedImage] = useState(null)
    const [selectedIndex, setSelectedIndex] = useState(0)

    const handleImageClick = (image, index) => {
        setSelectedImage(image)
        setSelectedIndex(index)
    }

    const handleClose = () => {
        setSelectedImage(null)
    }

    const handleNext = () => {
        const nextIndex = (selectedIndex + 1) % gallery.images.length
        setSelectedIndex(nextIndex)
        setSelectedImage(gallery.images[nextIndex])
    }

    const handlePrev = () => {
        const prevIndex = (selectedIndex - 1 + gallery.images.length) % gallery.images.length
        setSelectedIndex(prevIndex)
        setSelectedImage(gallery.images[prevIndex])
    }

    const handleDeleteImage = async (imageId, e) => {
        e.stopPropagation()
        if (!confirm('Are you sure you want to delete this image?')) return

        try {
            const response = await fetch(`/api/images/${imageId}`, {
                method: 'DELETE'
            })
            if (response.ok) {
                onRefresh()
            } else {
                alert('Failed to delete image')
            }
        } catch (error) {
            console.error('Error deleting image:', error)
            alert('Error deleting image')
        }
    }

    const handleToggleFavorite = async (e, image) => {
        e.stopPropagation()
        try {
            const response = await fetch(`/api/images/${image.id}/favorite`, {
                method: 'POST'
            })
            if (response.ok) {
                // Optimistic update for UI responsiveness
                // We rely on onRefresh to fetch the real state eventually, 
                // but for immediate feedback we assume success
                onRefresh()
            }
        } catch (error) {
            console.error('Error toggling favorite:', error)
        }
    }

    if (!gallery.images || gallery.images.length === 0) {
        return <div className="empty-state">No images in this gallery yet.</div>
    }

    return (
        <>
            <div className="image-grid">
                {gallery.images.map((image, index) => (
                    <div
                        key={image.id}
                        className="image-card"
                        onClick={() => handleImageClick(image, index)}
                    >
                        <img
                            src={image.thumbnail_path || `/api/thumbnails/${image.filename}`}
                            alt={`Image ${image.id}`}
                            loading="lazy"
                        />
                        <button
                            className={`favorite-btn ${image.is_favorite ? 'active' : ''}`}
                            onClick={(e) => handleToggleFavorite(e, image)}
                            title={image.is_favorite ? "Unfavorite" : "Favorite"}
                        >
                            <svg width="16" height="16" viewBox="0 0 24 24" fill={image.is_favorite ? "currentColor" : "none"} stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"></path>
                            </svg>
                        </button>
                        <button
                            className="delete-image-btn"
                            onClick={(e) => handleDeleteImage(image.id, e)}
                            title="Delete image"
                        >
                            ×
                        </button>
                    </div>
                ))}
            </div>

            {selectedImage && (
                <Lightbox
                    image={selectedImage}
                    images={gallery.images}
                    currentIndex={selectedIndex}
                    totalImages={gallery.images.length}
                    onClose={handleClose}
                    onNext={handleNext}
                    onPrev={handlePrev}
                />
            )}
        </>
    )
}

export default ImageGrid
