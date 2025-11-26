import { useState } from 'react'
import './ImageList.css'
import Lightbox from './Lightbox'

function ImageList({ images, onRefresh, meta, onPageChange }) {
    const [lightboxImage, setLightboxImage] = useState(null)
    const [lightboxIndex, setLightboxIndex] = useState(0)

    const handleDeleteImage = async (imageId, e) => {
        e.stopPropagation()

        if (!confirm('Are you sure you want to delete this image?')) {
            return
        }

        try {
            const response = await fetch(`/api/images/${imageId}`, { method: 'DELETE' })

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

    const openLightbox = (image, index) => {
        setLightboxImage(image)
        setLightboxIndex(index)
    }

    const closeLightbox = () => {
        setLightboxImage(null)
    }

    const nextImage = () => {
        const nextIndex = (lightboxIndex + 1) % images.length
        setLightboxIndex(nextIndex)
        setLightboxImage(images[nextIndex])
    }

    const prevImage = () => {
        const prevIndex = (lightboxIndex - 1 + images.length) % images.length
        setLightboxIndex(prevIndex)
        setLightboxImage(images[prevIndex])
    }

    return (
        <div className="image-list">
            <div className="image-header">
                <h2>All Images</h2>
                <button onClick={onRefresh}>🔄 Refresh</button>
            </div>

            {images.length === 0 ? (
                <div className="no-images">
                    <p>No images found. Add a source to start crawling!</p>
                </div>
            ) : (
                <div className="images-grid">
                    {images.map((image, index) => (
                        <div
                            key={image.id}
                            className="image-card"
                            onClick={() => openLightbox(image, index)}
                        >
                            <div className="image-thumbnail">
                                <img
                                    src={`/api/thumbnails/${image.filename}`}
                                    alt={image.filename}
                                    loading="lazy"
                                />
                            </div>
                            <div className="image-info">
                                {/* <p className="gallery-name">{image.gallery?.name || 'Unknown'}</p> */}
                                <p className="image-filename">{image.filename}</p>
                            </div>
                            <button
                                className="delete-image-btn"
                                onClick={(e) => handleDeleteImage(image.id, e)}
                                title="Delete Image"
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

            {lightboxImage && (
                <Lightbox
                    image={lightboxImage}
                    onClose={closeLightbox}
                    onNext={nextImage}
                    onPrev={prevImage}
                />
            )}
        </div>
    )
}

export default ImageList
